package core

import (
	"errors"
	"math/big"
	"sync"

	"github.com/ligo-ai/ligo-chain/chain/consensus/tendermint/epoch"
	"github.com/ligo-ai/ligo-chain/chain/core/state"
	"github.com/ligo-ai/ligo-chain/chain/core/types"
	ligoAbi "github.com/ligo-ai/ligo-chain/ligoabi/abi"
	"github.com/ligo-ai/ligo-chain/ligoclient"
	"github.com/ligo-ai/ligo-chain/utilities/common"
	"github.com/ligo-libs/crypto-go"
	dbm "github.com/ligo-libs/db-go"
)

type TX3LocalCache interface {
	GetTX3(chainId string, txHash common.Hash) *types.Transaction
	DeleteTX3(chainId string, txHash common.Hash)

	WriteTX3ProofData(proofData *types.TX3ProofData) error

	GetTX3ProofData(chainId string, txHash common.Hash) *types.TX3ProofData
	GetAllTX3ProofData() []*types.TX3ProofData
}

type CrossChainHelper interface {
	GetMutex() *sync.Mutex
	GetClient() *ligoclient.Client
	GetMainChainId() string
	GetChainInfoDB() dbm.DB

	CanCreateSideChain(from common.Address, chainId string, minValidators uint16, minDepositAmount, startupCost *big.Int, startBlock, endBlock *big.Int) error
	CreateSideChain(from common.Address, chainId string, minValidators uint16, minDepositAmount *big.Int, startBlock, endBlock *big.Int) error
	ValidateJoinSideChain(from common.Address, pubkey []byte, chainId string, depositAmount *big.Int, signature []byte) error
	JoinSideChain(from common.Address, pubkey crypto.PubKey, chainId string, depositAmount *big.Int) error
	ReadyForLaunchSideChain(height *big.Int, stateDB *state.StateDB) ([]string, []byte, []string)
	ProcessPostPendingData(newPendingIdxBytes []byte, deleteSideChainIds []string)

	VoteNextEpoch(ep *epoch.Epoch, from common.Address, voteHash common.Hash, txHash common.Hash) error
	RevealVote(ep *epoch.Epoch, from common.Address, pubkey crypto.PubKey, depositAmount *big.Int, salt string, txHash common.Hash) error
	UpdateNextEpoch(ep *epoch.Epoch, from common.Address, pubkey crypto.PubKey, depositAmount *big.Int, salt string, txHash common.Hash) error

	GetHeightFromMainChain() *big.Int
	GetEpochFromMainChain() (string, *epoch.Epoch)
	GetTxFromMainChain(txHash common.Hash) *types.Transaction

	ChangeValidators(chainId string)

	// for epoch only
	VerifySideChainProofData(bs []byte) error
	SaveSideChainProofDataToMainChain(bs []byte) error

	TX3LocalCache
	ValidateTX3ProofData(proofData *types.TX3ProofData) error
	ValidateTX4WithInMemTX3ProofData(tx4 *types.Transaction, tx3ProofData *types.TX3ProofData) error

	////SaveDataToMainV1 acceps both epoch and tx3
	//VerifySideChainProofDataV1(proofData *types.SideChainProofDataV1) error
	//SaveSideChainProofDataToMainChainV1(proofData *types.SideChainProofDataV1) error
}

// CrossChain Callback
type CrossChainValidateCb = func(tx *types.Transaction, state *state.StateDB, cch CrossChainHelper) error
type CrossChainApplyCb = func(tx *types.Transaction, state *state.StateDB, ops *types.PendingOps, cch CrossChainHelper, mining bool) error

// Non-CrossChain Callback
type NonCrossChainValidateCb = func(tx *types.Transaction, state *state.StateDB, bc *BlockChain) error
type NonCrossChainApplyCb = func(tx *types.Transaction, state *state.StateDB, bc *BlockChain, ops *types.PendingOps) error

type EtdInsertBlockCb func(bc *BlockChain, block *types.Block)

var validateCbMap = make(map[ligoAbi.FunctionType]interface{})
var applyCbMap = make(map[ligoAbi.FunctionType]interface{})
var insertBlockCbMap = make(map[string]EtdInsertBlockCb)

func RegisterValidateCb(function ligoAbi.FunctionType, validateCb interface{}) error {

	_, ok := validateCbMap[function]
	if ok {
		return errors.New("the name has registered in validateCbMap")
	}

	validateCbMap[function] = validateCb
	return nil
}

func GetValidateCb(function ligoAbi.FunctionType) interface{} {

	cb, ok := validateCbMap[function]
	if ok {
		return cb
	}

	return nil
}

func RegisterApplyCb(function ligoAbi.FunctionType, applyCb interface{}) error {

	_, ok := applyCbMap[function]
	if ok {
		return errors.New("the name has registered in applyCbMap")
	}

	applyCbMap[function] = applyCb

	return nil
}

func GetApplyCb(function ligoAbi.FunctionType) interface{} {

	cb, ok := applyCbMap[function]
	if ok {
		return cb
	}

	return nil
}

func RegisterInsertBlockCb(name string, insertBlockCb EtdInsertBlockCb) error {

	_, ok := insertBlockCbMap[name]
	if ok {
		return errors.New("the name has registered in insertBlockCbMap")
	}

	insertBlockCbMap[name] = insertBlockCb

	return nil
}

func GetInsertBlockCb(name string) EtdInsertBlockCb {

	cb, ok := insertBlockCbMap[name]
	if ok {
		return cb
	}

	return nil
}

func GetInsertBlockCbMap() map[string]EtdInsertBlockCb {

	return insertBlockCbMap
}
