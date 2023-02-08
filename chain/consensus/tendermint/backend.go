package tendermint

import (
	"crypto/ecdsa"
	"sync"

	"github.com/ligo-ai/ligo-chain/chain/consensus"
	"github.com/ligo-ai/ligo-chain/chain/consensus/tendermint/types"
	"github.com/ligo-ai/ligo-chain/chain/core"
	ligoTypes "github.com/ligo-ai/ligo-chain/chain/core/types"
	"github.com/ligo-ai/ligo-chain/chain/log"
	"github.com/ligo-ai/ligo-chain/params"
	"github.com/ligo-ai/ligo-chain/utilities/common"
	"github.com/ligo-ai/ligo-chain/utilities/event"
	"gopkg.in/urfave/cli.v1"
)

// New creates an Ethereum backend for Tendermint core engine.
func New(chainConfig *params.ChainConfig, cliCtx *cli.Context,
	privateKey *ecdsa.PrivateKey, cch core.CrossChainHelper) consensus.Tendermint {
	// Allocate the snapshot caches and create the engine
	//recents, _ := lru.NewARC(inmemorySnapshots)
	//recentMessages, _ := lru.NewARC(inmemoryPeers)
	//knownMessages, _ := lru.NewARC(inmemoryMessages)

	config := GetTendermintConfig(chainConfig.LigoChainId, cliCtx)

	backend := &backend{
		//config:             config,
		chainConfig:     chainConfig,
		tendermintEventMux: new(event.TypeMux),
		privateKey:      privateKey,
		//address:          crypto.PubkeyToAddress(privateKey.PublicKey),
		//core:             node,
		//chain:     chain,
		logger:    chainConfig.ChainLogger,
		commitCh:  make(chan *ligoTypes.Block, 1),
		vcommitCh: make(chan *types.IntermediateBlockResult, 1),
		//recents:          recents,
		//candidates:  make(map[common.Address]bool),
		coreStarted: false,
		//recentMessages:   recentMessages,
		//knownMessages:    knownMessages,
	}
	backend.core = MakeTendermintNode(backend, config, chainConfig, cch)
	return backend
}

type backend struct {
	//config             cfg.Config
	chainConfig     *params.ChainConfig
	tendermintEventMux *event.TypeMux
	privateKey      *ecdsa.PrivateKey
	address         common.Address
	core            *Node
	logger          log.Logger
	chain           consensus.ChainReader
	currentBlock    func() *ligoTypes.Block
	hasBadBlock     func(hash common.Hash) bool

	// the channels for istanbul engine notifications
	commitCh          chan *ligoTypes.Block
	vcommitCh         chan *types.IntermediateBlockResult
	proposedBlockHash common.Hash
	sealMu            sync.Mutex
	shouldStart       bool
	coreStarted       bool
	coreMu            sync.RWMutex
	broadcaster consensus.Broadcaster

}



func GetBackend() backend {
	return backend{}
}
