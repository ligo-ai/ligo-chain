package main

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ligo-ai/ligo-chain/consensus"
	"github.com/ligo-ai/ligo-chain/consensus/tendermint/epoch"
	tdmTypes "github.com/ligo-ai/ligo-chain/consensus/tendermint/types"
	"github.com/ligo-ai/ligo-chain/core"
	"github.com/ligo-ai/ligo-chain/core/rawdb"
	"github.com/ligo-ai/ligo-chain/core/state"
	"github.com/ligo-ai/ligo-chain/core/types"
	"github.com/ligo-ai/ligo-chain/log"
	"github.com/ligo-ai/ligo-chain/trie"
	ligoAbi "github.com/ligo-ai/ligo-chain/ligoabi/abi"
	"github.com/ligo-ai/ligo-chain/ligoclient"
	"github.com/ligo-ai/ligo-chain/ligodb"
	"github.com/ligo-ai/ligo-chain/ligoprotocol"
	"github.com/ligo-ai/ligo-chain/node"
	"github.com/ligo-ai/ligo-chain/params"
	"github.com/ligo-ai/ligo-chain/common"
	"github.com/ligo-ai/ligo-chain/common/math"
	"github.com/ligo-ai/ligo-chain/rlp"
	"github.com/ligo-libs/crypto-go"
	dbm "github.com/ligo-libs/db-go"
)

type CrossChainHelper struct {
	mtx             sync.Mutex
	chainInfoDB     dbm.DB
	localTX3CacheDB ligodb.Database

	client      *ligoclient.Client
	mainChainId string
}

func (cch *CrossChainHelper) GetMutex() *sync.Mutex {
	return &cch.mtx
}

func (cch *CrossChainHelper) GetChainInfoDB() dbm.DB {
	return cch.chainInfoDB
}

func (cch *CrossChainHelper) GetClient() *ligoclient.Client {
	return cch.client
}

func (cch *CrossChainHelper) GetMainChainId() string {
	return cch.mainChainId
}

func (cch *CrossChainHelper) CanCreateSideChain(from common.Address, chainId string, minValidators uint16, minDepositAmount, startupCost *big.Int, startBlock, endBlock *big.Int) error {

	if chainId == "" || strings.Contains(chainId, ";") {
		return errors.New("chainId is nil or empty, or contains ';', should be meaningful")
	}

	pass, _ := regexp.MatchString("^[a-z]+[a-z0-9_]*$", chainId)
	if !pass {
		return errors.New("chainId must be start with letter (a-z) and contains alphanumeric(lower case) or underscore, try use other name instead")
	}

	if utf8.RuneCountInString(chainId) > 30 {
		return errors.New("max characters of chain id is 30, try use other name instead")
	}

	if chainId == MainChain || chainId == TestnetChain {
		return errors.New("you can't create LigoAI as a side chain, try use other name instead")
	}

	ci := core.GetChainInfo(cch.chainInfoDB, chainId)
	if ci != nil {
		return fmt.Errorf("Chain %s has already exist, try use other name instead", chainId)
	}

	cci := core.GetPendingSideChainData(cch.chainInfoDB, chainId)
	if cci != nil {
		return fmt.Errorf("Chain %s has already applied, try use other name instead", chainId)
	}

	if minValidators < core.OFFICIAL_MINIMUM_VALIDATORS {
		return fmt.Errorf("Validators count is not meet the minimum official validator count (%v)", core.OFFICIAL_MINIMUM_VALIDATORS)
	}

	officialMinimumDeposit := math.MustParseBig256(core.OFFICIAL_MINIMUM_DEPOSIT)
	if minDepositAmount.Cmp(officialMinimumDeposit) == -1 {
		return fmt.Errorf("Deposit amount is not meet the minimum official deposit amount (%v $LAI)", new(big.Int).Div(officialMinimumDeposit, big.NewInt(params.Lai)))
	}

	if startupCost.Cmp(officialMinimumDeposit) != 0 {
		return fmt.Errorf("Startup cost is not meet the required amount (%v $LAI)", new(big.Int).Div(officialMinimumDeposit, big.NewInt(params.Lai)))
	}

	if startBlock.Cmp(endBlock) >= 0 {
		return errors.New("start block number must be less than end block number")
	}

	ligochain := MustGetLigoChainFromNode(chainMgr.mainChain.LigoNode)
	currentBlock := ligochain.BlockChain().CurrentBlock()
	if endBlock.Cmp(currentBlock.Number()) <= 0 {
		return errors.New("end block number has already passed")
	}

	return nil
}

func (cch *CrossChainHelper) CreateSideChain(from common.Address, chainId string, minValidators uint16, minDepositAmount *big.Int, startBlock, endBlock *big.Int) error {
	log.Debug("CreateSideChain - start")

	cci := &core.CoreChainInfo{
		Owner:            from,
		ChainId:          chainId,
		MinValidators:    minValidators,
		MinDepositAmount: minDepositAmount,
		StartBlock:       startBlock,
		EndBlock:         endBlock,
		JoinedValidators: make([]core.JoinedValidator, 0),
	}
	core.CreatePendingSideChainData(cch.chainInfoDB, cci)

	log.Debug("CreateSideChain - end")
	return nil
}

func (cch *CrossChainHelper) ValidateJoinSideChain(from common.Address, consensusPubkey []byte, chainId string, depositAmount *big.Int, signature []byte) error {
	log.Debug("ValidateJoinSideChain - start")

	if chainId == MainChain || chainId == TestnetChain {
		return errors.New("you can't join LigoAI as a side chain, try use other name instead")
	}

	if err := crypto.CheckConsensusPubKey(from, consensusPubkey, signature); err != nil {
		return err
	}

	ci := core.GetPendingSideChainData(cch.chainInfoDB, chainId)
	if ci == nil {
		if core.GetChainInfo(cch.chainInfoDB, chainId) != nil {
			return fmt.Errorf("chain %s has already created/started, try use other name instead", chainId)
		} else {
			return fmt.Errorf("side chain %s not exist, try use other name instead", chainId)
		}
	}

	find := false
	for _, joined := range ci.JoinedValidators {
		if from == joined.Address {
			find = true
			break
		}
	}

	if find {
		return errors.New(fmt.Sprintf("You have already joined the Side Chain %s", chainId))
	}

	if !(depositAmount != nil && depositAmount.Sign() == 1) {
		return errors.New("deposit amount must be greater than 0")
	}

	log.Debug("ValidateJoinSideChain - end")
	return nil
}

func (cch *CrossChainHelper) JoinSideChain(from common.Address, pubkey crypto.PubKey, chainId string, depositAmount *big.Int) error {
	log.Debug("JoinSideChain - start")

	ci := core.GetPendingSideChainData(cch.chainInfoDB, chainId)
	if ci == nil {
		log.Errorf("JoinSideChain - Side Chain %s not exist, you can't join the chain", chainId)
		return fmt.Errorf("Side Chain %s not exist, you can't join the chain", chainId)
	}

	for _, joined := range ci.JoinedValidators {
		if from == joined.Address {
			return nil
		}
	}

	jv := core.JoinedValidator{
		PubKey:        pubkey,
		Address:       from,
		DepositAmount: depositAmount,
	}

	ci.JoinedValidators = append(ci.JoinedValidators, jv)

	core.UpdatePendingSideChainData(cch.chainInfoDB, ci)

	log.Debug("JoinSideChain - end")
	return nil
}

func (cch *CrossChainHelper) ReadyForLaunchSideChain(height *big.Int, stateDB *state.StateDB) ([]string, []byte, []string) {

	readyId, updateBytes, removedId := core.GetSideChainForLaunch(cch.chainInfoDB, height, stateDB)
	if len(readyId) == 0 {

	} else {

	}

	return readyId, updateBytes, removedId
}

func (cch *CrossChainHelper) ProcessPostPendingData(newPendingIdxBytes []byte, deleteSideChainIds []string) {
	core.ProcessPostPendingData(cch.chainInfoDB, newPendingIdxBytes, deleteSideChainIds)
}

func (cch *CrossChainHelper) VoteNextEpoch(ep *epoch.Epoch, from common.Address, voteHash common.Hash, txHash common.Hash) error {

	voteSet := ep.GetNextEpoch().GetEpochValidatorVoteSet()
	if voteSet == nil {
		voteSet = epoch.NewEpochValidatorVoteSet()
	}

	vote, exist := voteSet.GetVoteByAddress(from)

	if exist {

		vote.VoteHash = voteHash
		vote.TxHash = txHash
	} else {

		vote = &epoch.EpochValidatorVote{
			Address:  from,
			VoteHash: voteHash,
			TxHash:   txHash,
		}
		voteSet.StoreVote(vote)
	}

	epoch.SaveEpochVoteSet(ep.GetDB(), ep.GetNextEpoch().Number, voteSet)
	return nil
}

func (cch *CrossChainHelper) RevealVote(ep *epoch.Epoch, from common.Address, pubkey crypto.PubKey, depositAmount *big.Int, salt string, txHash common.Hash) error {

	voteSet := ep.GetNextEpoch().GetEpochValidatorVoteSet()
	vote, exist := voteSet.GetVoteByAddress(from)

	if exist {

		vote.PubKey = pubkey
		vote.Amount = depositAmount
		vote.Salt = salt
		vote.TxHash = txHash
	}

	epoch.SaveEpochVoteSet(ep.GetDB(), ep.GetNextEpoch().Number, voteSet)
	return nil
}

func (cch *CrossChainHelper) UpdateNextEpoch(ep *epoch.Epoch, from common.Address, pubkey crypto.PubKey, depositAmount *big.Int, salt string, txHash common.Hash) error {
	voteSet := ep.GetNextEpoch().GetEpochValidatorVoteSet()
	if voteSet == nil {
		voteSet = epoch.NewEpochValidatorVoteSet()
	}

	vote, exist := voteSet.GetVoteByAddress(from)

	if exist {
		vote.Amount = depositAmount
		vote.TxHash = txHash
	} else {
		vote = &epoch.EpochValidatorVote{
			Address: from,
			PubKey:  pubkey,
			Amount:  depositAmount,
			Salt:    "ligochain",
			TxHash:  txHash,
		}

		voteSet.StoreVote(vote)
	}

	epoch.SaveEpochVoteSet(ep.GetDB(), ep.GetNextEpoch().Number, voteSet)
	return nil
}

func (cch *CrossChainHelper) GetHeightFromMainChain() *big.Int {
	ligochain := MustGetLigoChainFromNode(chainMgr.mainChain.LigoNode)
	return ligochain.BlockChain().CurrentBlock().Number()
}

func (cch *CrossChainHelper) GetTxFromMainChain(txHash common.Hash) *types.Transaction {
	ligochain := MustGetLigoChainFromNode(chainMgr.mainChain.LigoNode)
	chainDb := ligochain.ChainDb()

	tx, _, _, _ := rawdb.ReadTransaction(chainDb, txHash)
	return tx
}

func (cch *CrossChainHelper) GetEpochFromMainChain() (string, *epoch.Epoch) {
	ligochain := MustGetLigoChainFromNode(chainMgr.mainChain.LigoNode)
	var ep *epoch.Epoch
	if tendermint, ok := ligochain.Engine().(consensus.Tendermint); ok {
		ep = tendermint.GetEpoch()
	}
	return ligochain.ChainConfig().LigoChainId, ep
}

func (cch *CrossChainHelper) ChangeValidators(chainId string) {

	if chainMgr == nil {
		return
	}

	var chain *Chain = nil
	if chainId == MainChain || chainId == TestnetChain {
		chain = chainMgr.mainChain
	} else if chn, ok := chainMgr.sideChains[chainId]; ok {
		chain = chn
	}

	if chain == nil || chain.LigoNode == nil {
		return
	}

	if address, ok := chainMgr.getNodeValidator(chain.LigoNode); ok {
		chainMgr.server.AddLocalValidator(chainId, address)
	}
}

func (cch *CrossChainHelper) VerifySideChainProofData(bs []byte) error {

	log.Debug("VerifySideChainProofData - start")

	var proofData types.SideChainProofData
	err := rlp.DecodeBytes(bs, &proofData)
	if err != nil {
		return err
	}

	header := proofData.Header

	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {

	}

	ncExtra, err := tdmTypes.ExtractTendermintExtra(header)
	if err != nil {
		return err
	}

	chainId := ncExtra.ChainID
	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
		return fmt.Errorf("invalid side chain id: %s", chainId)
	}

	if header.Nonce != (types.TendermintEmptyNonce) && !bytes.Equal(header.Nonce[:], types.TendermintNonce) {
		return errors.New("invalid nonce")
	}

	if header.MixDigest != types.TendermintDigest {
		return errors.New("invalid mix digest")
	}

	if header.UncleHash != types.TendermintNilUncleHash {
		return errors.New("invalid uncle Hash")
	}

	if header.Difficulty == nil || header.Difficulty.Cmp(types.TendermintDefaultDifficulty) != 0 {
		return errors.New("invalid difficulty")
	}

	if ncExtra.EpochBytes != nil && len(ncExtra.EpochBytes) != 0 {
		ep := epoch.FromBytes(ncExtra.EpochBytes)
		if ep != nil && ep.Number == 0 {
			return nil
		}
	}

	if chainId != "side_0" {
		ci := core.GetChainInfo(cch.chainInfoDB, chainId)
		if ci == nil {
			return fmt.Errorf("chain info %s not found", chainId)
		}
		epoch := ci.GetEpochByBlockNumber(ncExtra.Height)
		if epoch == nil {
			return fmt.Errorf("could not get epoch for block height %v", ncExtra.Height)
		}
		valSet := epoch.Validators
		if !bytes.Equal(valSet.Hash(), ncExtra.ValidatorsHash) {
			return errors.New("inconsistent validator set")
		}

		seenCommit := ncExtra.SeenCommit
		if !bytes.Equal(ncExtra.SeenCommitHash, seenCommit.Hash()) {
			return errors.New("invalid committed seals")
		}

		if err = valSet.VerifyCommit(ncExtra.ChainID, ncExtra.Height, seenCommit); err != nil {
			return err
		}
	}

	log.Debug("VerifySideChainProofData - end")
	return nil
}

func (cch *CrossChainHelper) SaveSideChainProofDataToMainChain(bs []byte) error {
	log.Debug("SaveSideChainProofDataToMainChain - start")

	var proofData types.SideChainProofData
	err := rlp.DecodeBytes(bs, &proofData)
	if err != nil {
		return err
	}

	header := proofData.Header
	ncExtra, err := tdmTypes.ExtractTendermintExtra(header)
	if err != nil {
		return err
	}

	chainId := ncExtra.ChainID
	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
		return fmt.Errorf("invalid side chain id: %s", chainId)
	}

	if len(ncExtra.EpochBytes) != 0 {
		ep := epoch.FromBytes(ncExtra.EpochBytes)
		if ep != nil {
			ci := core.GetChainInfo(cch.chainInfoDB, ncExtra.ChainID)

			if ci == nil {
				for {

					time.Sleep(3 * time.Second)
					ci = core.GetChainInfo(cch.chainInfoDB, ncExtra.ChainID)
					if ci != nil {
						break
					}
				}
			}

			futureEpoch := ep.Number > ci.EpochNumber && ncExtra.Height < ep.StartBlock
			if futureEpoch {

				core.SaveFutureEpoch(cch.chainInfoDB, ep, chainId)
				log.Infof("Future epoch saved from chain: %s, epoch: %v", chainId, ep)
			} else if ep.Number == 0 || ep.Number >= ci.EpochNumber {

				ci.EpochNumber = ep.Number
				ci.Epoch = ep
				core.SaveChainInfo(cch.chainInfoDB, ci)
				log.Infof("Epoch saved from chain: %s, epoch: %v", chainId, ep)
			}
		}
	}

	log.Debug("SaveSideChainProofDataToMainChain - end")
	return nil
}

func (cch *CrossChainHelper) ValidateTX3ProofData(proofData *types.TX3ProofData) error {
	log.Debug("ValidateTX3ProofData - start")

	header := proofData.Header

	if header.Time.Cmp(big.NewInt(time.Now().Unix())) > 0 {

	}

	ncExtra, err := tdmTypes.ExtractTendermintExtra(header)
	if err != nil {
		return err
	}

	chainId := ncExtra.ChainID
	if chainId == "" || chainId == MainChain || chainId == TestnetChain {
		return fmt.Errorf("invalid side chain id: %s", chainId)
	}

	if header.Nonce != (types.TendermintEmptyNonce) && !bytes.Equal(header.Nonce[:], types.TendermintNonce) {
		return errors.New("invalid nonce")
	}

	if header.MixDigest != types.TendermintDigest {
		return errors.New("invalid mix digest")
	}

	if header.UncleHash != types.TendermintNilUncleHash {
		return errors.New("invalid uncle Hash")
	}

	if header.Difficulty == nil || header.Difficulty.Cmp(types.TendermintDefaultDifficulty) != 0 {
		return errors.New("invalid difficulty")
	}

	if ncExtra.EpochBytes != nil && len(ncExtra.EpochBytes) != 0 {
		ep := epoch.FromBytes(ncExtra.EpochBytes)
		if ep != nil && ep.Number == 0 {
			return nil
		}
	}

	ci := core.GetChainInfo(cch.chainInfoDB, chainId)
	if ci == nil {
		return fmt.Errorf("chain info %s not found", chainId)
	}
	epoch := ci.GetEpochByBlockNumber(ncExtra.Height)
	if epoch == nil {
		return fmt.Errorf("could not get epoch for block height %v", ncExtra.Height)
	}
	valSet := epoch.Validators
	if !bytes.Equal(valSet.Hash(), ncExtra.ValidatorsHash) {
		return errors.New("inconsistent validator set")
	}

	seenCommit := ncExtra.SeenCommit
	if !bytes.Equal(ncExtra.SeenCommitHash, seenCommit.Hash()) {
		return errors.New("invalid committed seals")
	}

	if err = valSet.VerifyCommit(ncExtra.ChainID, ncExtra.Height, seenCommit); err != nil {
		return err
	}

	keybuf := new(bytes.Buffer)
	for i, txIndex := range proofData.TxIndexs {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(txIndex))
		_, _, err := trie.VerifyProof(header.TxHash, keybuf.Bytes(), proofData.TxProofs[i])
		if err != nil {
			return err
		}
	}

	log.Debug("ValidateTX3ProofData - end")
	return nil
}

func (cch *CrossChainHelper) ValidateTX4WithInMemTX3ProofData(tx4 *types.Transaction, tx3ProofData *types.TX3ProofData) error {

	signer := types.NewEIP155Signer(tx4.ChainId())
	from, err := types.Sender(signer, tx4)
	if err != nil {
		return core.ErrInvalidSender
	}

	var args ligoAbi.WithdrawFromMainChainArgs

	if !ligoAbi.IsLigoChainContractAddr(tx4.To()) {
		return errors.New("invalid TX4: wrong To()")
	}

	data := tx4.Data()
	function, err := ligoAbi.FunctionTypeFromId(data[:4])
	if err != nil {
		return err
	}

	if function != ligoAbi.WithdrawFromMainChain {
		return errors.New("invalid TX4: wrong function")
	}

	if err := ligoAbi.ChainABI.UnpackMethodInputs(&args, ligoAbi.WithdrawFromMainChain.String(), data[4:]); err != nil {
		return err
	}

	header := tx3ProofData.Header
	if err != nil {
		return err
	}
	keybuf := new(bytes.Buffer)
	rlp.Encode(keybuf, tx3ProofData.TxIndexs[0])
	val, _, err := trie.VerifyProof(header.TxHash, keybuf.Bytes(), tx3ProofData.TxProofs[0])
	if err != nil {
		return err
	}

	var tx3 types.Transaction
	err = rlp.DecodeBytes(val, &tx3)
	if err != nil {
		return err
	}

	signer2 := types.NewEIP155Signer(tx3.ChainId())
	tx3From, err := types.Sender(signer2, &tx3)
	if err != nil {
		return core.ErrInvalidSender
	}

	var tx3Args ligoAbi.WithdrawFromSideChainArgs
	tx3Data := tx3.Data()
	if err := ligoAbi.ChainABI.UnpackMethodInputs(&tx3Args, ligoAbi.WithdrawFromSideChain.String(), tx3Data[4:]); err != nil {
		return err
	}

	if from != tx3From || args.ChainId != tx3Args.ChainId || args.Amount.Cmp(tx3.Value()) != 0 {
		return errors.New("params are not consistent with tx in side chain")
	}

	return nil
}

func (cch *CrossChainHelper) GetTX3(chainId string, txHash common.Hash) *types.Transaction {
	return rawdb.GetTX3(cch.localTX3CacheDB, chainId, txHash)
}

func (cch *CrossChainHelper) DeleteTX3(chainId string, txHash common.Hash) {
	rawdb.DeleteTX3(cch.localTX3CacheDB, chainId, txHash)
}

func (cch *CrossChainHelper) WriteTX3ProofData(proofData *types.TX3ProofData) error {
	return rawdb.WriteTX3ProofData(cch.localTX3CacheDB, proofData)
}

func (cch *CrossChainHelper) GetTX3ProofData(chainId string, txHash common.Hash) *types.TX3ProofData {
	return rawdb.GetTX3ProofData(cch.localTX3CacheDB, chainId, txHash)
}

func (cch *CrossChainHelper) GetAllTX3ProofData() []*types.TX3ProofData {
	return rawdb.GetAllTX3ProofData(cch.localTX3CacheDB)
}

func MustGetLigoChainFromNode(node *node.Node) *ligoprotocol.LigoAI {
	ligoChain, err := getLigoChainFromNode(node)
	if err != nil {
		panic("getLigoChainFromNode error: " + err.Error())
	}
	return ligoChain
}

func getLigoChainFromNode(node *node.Node) (*ligoprotocol.LigoAI, error) {
	var ligoChain *ligoprotocol.LigoAI
	if err := node.Service(&ligoChain); err != nil {
		return nil, err
	}

	return ligoChain, nil
}
