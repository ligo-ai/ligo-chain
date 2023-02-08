package ligoprotocol

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ligo-ai/ligo-chain//accounts"
	"github.com/ligo-ai/ligo-chain//consensus"
	"github.com/ligo-ai/ligo-chain//consensus/tendermint"
	ntcBackend "github.com/ligo-ai/ligo-chain//consensus/tendermint"
	"github.com/ligo-ai/ligo-chain//core"
	"github.com/ligo-ai/ligo-chain//core/bloombits"
	"github.com/ligo-ai/ligo-chain//core/datareduction"
	"github.com/ligo-ai/ligo-chain//core/rawdb"
	"github.com/ligo-ai/ligo-chain//core/types"
	"github.com/ligo-ai/ligo-chain//core/vm"
	"github.com/ligo-ai/ligo-chain//log"
	"github.com/ligo-ai/ligo-chain/internal/ligoapi"
	"github.com/ligo-ai/ligo-chain/ligodb"
	"github.com/ligo-ai/ligo-chain/ligoprotocol/downloader"
	"github.com/ligo-ai/ligo-chain/ligoprotocol/filters"
	"github.com/ligo-ai/ligo-chain/ligoprotocol/gasprice"
	"github.com/ligo-ai/ligo-chain//node"
	"github.com/ligo-ai/ligo-chain//p2p"
	"github.com/ligo-ai/ligo-chain/network/rpc"
	"github.com/ligo-ai/ligo-chain/params"
	"github.com/ligo-ai/ligo-chain//common"
	"github.com/ligo-ai/ligo-chain//common/hexutil"
	"github.com/ligo-ai/ligo-chain//event"
	"github.com/ligo-ai/ligo-chain//miner"
	"github.com/ligo-ai/ligo-chain//rlp"
	"gopkg.in/urfave/cli.v1"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *core.ChainIndexer)
}

type LigoAI struct {
	config      *Config
	chainConfig *params.ChainConfig

	shutdownChan chan bool

	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager

	chainDb ligodb.Database
	pruneDb ligodb.Database

	eventMux       *event.TypeMux
	engine         consensus.Tendermint
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval
	bloomIndexer  *core.ChainIndexer

	ApiBackend *EthApiBackend

	miner    *miner.Miner
	gasPrice *big.Int
	coinbase common.Address
	solcPath string

	networkId     uint64
	netRPCService *ligoapi.PublicNetAPI

	lock sync.RWMutex
}

func New(ctx *node.ServiceContext, config *Config, cliCtx *cli.Context,
	cch core.CrossChainHelper, logger log.Logger, isTestnet bool) (*LigoAI, error) {

	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := ctx.OpenDatabase("chaindata", config.DatabaseCache, config.DatabaseHandles, "ligochain/db/chaindata/")
	if err != nil {
		return nil, err
	}
	pruneDb, err := ctx.OpenDatabase("prunedata", config.DatabaseCache, config.DatabaseHandles, "ligochain/db/prune/")
	if err != nil {
		return nil, err
	}

	isMainChain := params.IsMainChain(ctx.ChainId())

	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlockWithDefault(chainDb, config.Genesis, isMainChain, isTestnet)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	chainConfig.ChainLogger = logger

	ligoChain := &LigoAI{
		config:         config,
		chainDb:        chainDb,
		pruneDb:        pruneDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         CreateConsensusEngine(ctx, config, chainConfig, chainDb, cliCtx, cch),
		shutdownChan:   make(chan bool),
		networkId:      config.NetworkId,
		gasPrice:       config.MinerGasPrice,
		coinbase:       config.Coinbase,
		solcPath:       config.SolcPath,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	var dbVer = "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	logger.Info("Initialising protocol", "Network", chainConfig.LigoChainId)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, Ligo Chain %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			logger.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}
	var (
		vmConfig    = vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit: config.TrieCleanCache,

			TrieDirtyLimit:    config.TrieDirtyCache,
			TrieDirtyDisabled: config.NoPruning,
			TrieTimeLimit:     config.TrieTimeout,
		}
	)

	ligoChain.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, ligoChain.chainConfig, ligoChain.engine, vmConfig, cch)
	if err != nil {
		return nil, err
	}

	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		logger.Warn("Rewinding chain to upgrade configuration", "err", compat)
		ligoChain.blockchain.SetHead(compat.RewindTo)
		rawdb.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	ligoChain.bloomIndexer.Start(ligoChain.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	ligoChain.txPool = core.NewTxPool(config.TxPool, ligoChain.chainConfig, ligoChain.blockchain, cch)

	if ligoChain.protocolManager, err = NewProtocolManager(ligoChain.chainConfig, config.SyncMode, config.NetworkId, ligoChain.eventMux, ligoChain.txPool, ligoChain.engine, ligoChain.blockchain, chainDb, cch); err != nil {
		return nil, err
	}
	ligoChain.miner = miner.New(ligoChain, ligoChain.chainConfig, ligoChain.EventMux(), ligoChain.engine, config.MinerGasFloor, config.MinerGasCeil, cch)
	ligoChain.miner.SetExtra(makeExtraData(config.ExtraData))

	ligoChain.ApiBackend = &EthApiBackend{ligoChain, nil, cch}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.MinerGasPrice
	}
	ligoChain.ApiBackend.gpo = gasprice.NewOracle(ligoChain.ApiBackend, gpoParams)

	return ligoChain, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {

		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"ligochain",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

func CreateConsensusEngine(ctx *node.ServiceContext, config *Config, chainConfig *params.ChainConfig, db ligodb.Database,
	cliCtx *cli.Context, cch core.CrossChainHelper) consensus.Tendermint {

	if chainConfig.Tendermint.Epoch != 0 {
		config.Tendermint.Epoch = chainConfig.Tendermint.Epoch
	}
	config.Tendermint.ProposerPolicy = tendermint.ProposerPolicy(chainConfig.Tendermint.ProposerPolicy)
	return ntcBackend.New(chainConfig, cliCtx, ctx.NodeKey(), cch)
}

func (s *LigoAI) APIs() []rpc.API {

	apis := ligoapi.GetAPIs(s.ApiBackend, s.solcPath)

	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	apis = append(apis, []rpc.API{
		{
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "lai",
			Version:   "1.0",
			Service:   NewPublicEthereumAPI(s),
			Public:    true,
		}, {
			Namespace: "lai",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "lai",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "eth",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "lai",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
	return apis
}

func (s *LigoAI) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *LigoAI) Coinbase() (eb common.Address, err error) {
	if tendermint, ok := s.engine.(consensus.Tendermint); ok {
		eb = tendermint.PrivateValidator()
		if eb != (common.Address{}) {
			return eb, nil
		} else {
			return eb, errors.New("private validator missing")
		}
	} else {
		s.lock.RLock()
		coinbase := s.coinbase
		s.lock.RUnlock()

		if coinbase != (common.Address{}) {
			return coinbase, nil
		}
		if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
			if accounts := wallets[0].Accounts(); len(accounts) > 0 {
				coinbase := accounts[0].Address

				s.lock.Lock()
				s.coinbase = coinbase
				s.lock.Unlock()

				log.Info("Coinbase automatically configured", "address", coinbase)
				return coinbase, nil
			}
		}
	}
	return common.Address{}, fmt.Errorf("Base address must be explicitly specified")
}

func (self *LigoAI) SetCoinbase(coinbase common.Address) {

	self.lock.Lock()
	self.coinbase = coinbase
	self.lock.Unlock()

	self.miner.SetCoinbase(coinbase)
}

func (s *LigoAI) StartMining(local bool) error {
	var eb common.Address
	if tendermint, ok := s.engine.(consensus.Tendermint); ok {
		eb = tendermint.PrivateValidator()
		if (eb == common.Address{}) {
			log.Error("Cannot start minting without private validator")
			return errors.New("private validator file missing")
		}
	} else {
		_, err := s.Coinbase()
		if err != nil {
			log.Error("Cannot start mining without base address", "err", err)
			return fmt.Errorf("base address missing: %v", err)
		}
	}

	if local {

		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
	}
	go s.miner.Start(eb)
	return nil
}

func (s *LigoAI) StopMining()         { s.miner.Stop() }
func (s *LigoAI) IsMining() bool      { return s.miner.Mining() }
func (s *LigoAI) Miner() *miner.Miner { return s.miner }

func (s *LigoAI) ChainConfig() *params.ChainConfig   { return s.chainConfig }
func (s *LigoAI) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *LigoAI) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *LigoAI) TxPool() *core.TxPool               { return s.txPool }
func (s *LigoAI) EventMux() *event.TypeMux           { return s.eventMux }
func (s *LigoAI) Engine() consensus.Tendermint          { return s.engine }
func (s *LigoAI) ChainDb() ligodb.Database           { return s.chainDb }
func (s *LigoAI) IsListening() bool                  { return true }
func (s *LigoAI) EthVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *LigoAI) NetVersion() uint64                 { return s.networkId }
func (s *LigoAI) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

func (s *LigoAI) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

func (s *LigoAI) Start(srvr *p2p.Server) error {

	s.startBloomHandlers()

	s.netRPCService = ligoapi.NewPublicNetAPI(srvr, s.NetVersion())

	maxPeers := srvr.MaxPeers

	s.protocolManager.Start(maxPeers)

	go s.loopForMiningEvent()

	if s.config.PruneStateData && s.chainConfig.LigoChainId == "side_0" {
		go s.StartScanAndPrune(0)
	}

	return nil
}

func (s *LigoAI) Stop() error {
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.protocolManager.Stop()
	s.txPool.Stop()
	s.miner.Stop()
	s.engine.Close()
	s.miner.Close()
	s.eventMux.Stop()

	s.chainDb.Close()
	s.pruneDb.Close()
	close(s.shutdownChan)

	return nil
}

func (s *LigoAI) loopForMiningEvent() {

	startMiningCh := make(chan core.StartMiningEvent, 1)
	startMiningSub := s.blockchain.SubscribeStartMiningEvent(startMiningCh)

	stopMiningCh := make(chan core.StopMiningEvent, 1)
	stopMiningSub := s.blockchain.SubscribeStopMiningEvent(stopMiningCh)

	defer startMiningSub.Unsubscribe()
	defer stopMiningSub.Unsubscribe()

	for {
		select {
		case <-startMiningCh:
			if !s.IsMining() {
				s.lock.RLock()
				price := s.gasPrice
				s.lock.RUnlock()
				s.txPool.SetGasPrice(price)
				s.chainConfig.ChainLogger.Info("Tendermint Consensus Engine will start shortly")
				s.engine.(consensus.Tendermint).ForceStart()
				s.StartMining(true)
			} else {
				s.chainConfig.ChainLogger.Info("Tendermint Consensus Engine already started")
			}
		case <-stopMiningCh:
			if s.IsMining() {
				s.chainConfig.ChainLogger.Info("Tendermint Consensus Engine will stop shortly")
				s.StopMining()
			} else {
				s.chainConfig.ChainLogger.Info("Tendermint Consensus Engine already stopped")
			}
		case <-startMiningSub.Err():
			return
		case <-stopMiningSub.Err():
			return
		}
	}
}

func (s *LigoAI) StartScanAndPrune(blockNumber uint64) {

	if datareduction.StartPruning() {
		log.Info("Data Reduction - Start")
	} else {
		log.Info("Data Reduction - Pruning is already running")
		return
	}

	latestBlockNumber := s.blockchain.CurrentHeader().Number.Uint64()
	if blockNumber == 0 || blockNumber >= latestBlockNumber {
		blockNumber = latestBlockNumber
		log.Infof("Data Reduction - Last block number %v", blockNumber)
	} else {
		log.Infof("Data Reduction - User defined Last block number %v", blockNumber)
	}

	ps := rawdb.ReadHeadScanNumber(s.pruneDb)
	var scanNumber uint64
	if ps != nil {
		scanNumber = *ps
	}

	pp := rawdb.ReadHeadPruneNumber(s.pruneDb)
	var pruneNumber uint64
	if pp != nil {
		pruneNumber = *pp
	}
	log.Infof("Data Reduction - Last scan number %v, prune number %v", scanNumber, pruneNumber)

	pruneProcessor := datareduction.NewPruneProcessor(s.chainDb, s.pruneDb, s.blockchain, s.config.PruneBlockData)

	lastScanNumber, lastPruneNumber := pruneProcessor.Process(blockNumber, scanNumber, pruneNumber)
	log.Infof("Data Reduction - After prune, last number scan %v, prune number %v", lastScanNumber, lastPruneNumber)
	if s.config.PruneBlockData {
		for i := uint64(1); i < lastPruneNumber; i++ {
			rawdb.DeleteBody(s.chainDb, rawdb.ReadCanonicalHash(s.chainDb, i), i)
		}
		log.Infof("deleted block from 1 to %v", lastPruneNumber)
	}
	log.Info("Data Reduction - Completed")

	datareduction.StopPruning()
}
