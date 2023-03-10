package tendermint

import (
	"bytes"
	"errors"
	"math/big"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ligo-ai/ligo-chain/chain/consensus"
	"github.com/ligo-ai/ligo-chain/chain/consensus/tendermint/epoch"
	tdmTypes "github.com/ligo-ai/ligo-chain/chain/consensus/tendermint/types"
	"github.com/ligo-ai/ligo-chain/core/state"
	"github.com/ligo-ai/ligo-chain/core/types"
	"github.com/ligo-ai/ligo-chain/network/rpc"
	"github.com/ligo-ai/ligo-chain/params"
	"github.com/ligo-ai/ligo-chain/common"
	"github.com/ligo-libs/wire-go"
)

const (
	fetcherID = "tendermint"
)

var (
	errInvalidProposal = errors.New("invalid proposal")

	errInvalidSignature = errors.New("invalid signature")

	errUnknownBlock = errors.New("unknown block")

	errUnauthorized = errors.New("unauthorized")

	errInvalidDifficulty = errors.New("invalid difficulty")

	errInvalidExtraDataFormat = errors.New("invalid extra data format")

	errInvalidMixDigest = errors.New("invalid Tendermint mix digest")

	errInvalidNonce = errors.New("invalid nonce")

	errInvalidUncleHash = errors.New("non empty uncle hash")

	errInconsistentValidatorSet = errors.New("inconsistent validator set")

	errInvalidTimestamp = errors.New("invalid timestamp")

	errInvalidVotingChain = errors.New("invalid voting chain")

	errInvalidVote = errors.New("vote nonce not 0x00..0 or 0xff..f")

	errInvalidCommittedSeals = errors.New("invalid committed seals")

	errEmptyCommittedSeals = errors.New("zero committed seals")

	errMismatchTxhashes = errors.New("mismatch transactions hashes")

	errInvalidMainChainNumber = errors.New("invalid Main Chain Height")

	errMainChainNotCatchup = errors.New("unable proceed the block due to main chain not catch up by waiting for more than 300 seconds, please catch up the main chain first")
)

var (
	now = time.Now

	inmemoryAddresses  = 20
	recentAddresses, _ = lru.NewARC(inmemoryAddresses)

	_ consensus.Engine = (*backend)(nil)

	sideChainRewardAddress = common.StringToAddress("LigooND9QsiAFbMan5tqVg899qAv2EsQ")
)

func (sb *backend) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "lai",
		Version:   "1.0",
		Service:   &API{chain: chain, tendermint: sb},
		Public:    true,
	}}
}

func (sb *backend) Start(chain consensus.ChainReader, currentBlock func() *types.Block, hasBadBlock func(hash common.Hash) bool) error {

	sb.logger.Info("Tendermint backend Start")

	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if sb.coreStarted {
		return ErrStartedEngine
	}

	sb.proposedBlockHash = common.Hash{}
	if sb.commitCh != nil {
		close(sb.commitCh)
	}
	sb.commitCh = make(chan *types.Block, 1)
	if sb.vcommitCh != nil {
		close(sb.vcommitCh)
	}
	sb.vcommitCh = make(chan *tdmTypes.IntermediateBlockResult, 1)

	sb.chain = chain

	sb.currentBlock = currentBlock
	sb.hasBadBlock = hasBadBlock

	if _, err := sb.core.Start(); err != nil {
		return err
	}

	sb.coreStarted = true

	return nil
}

func (sb *backend) Stop() error {

	sb.logger.Info("Tendermint backend stop")

	sb.coreMu.Lock()
	defer sb.coreMu.Unlock()
	if !sb.coreStarted {
		return ErrStoppedEngine
	}
	if !sb.core.Stop() {
		return errors.New("tendermint stop error")
	}
	sb.coreStarted = false

	return nil
}

func (sb *backend) Close() error {
	sb.core.epochDB.Close()
	return nil
}

func (sb *backend) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

func (sb *backend) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {

	sb.logger.Info("Your node is synchronized. Congratulations")

	return sb.verifyHeader(chain, header, nil)
}

func (sb *backend) verifyHeader(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {

	if header.Number == nil {
		return errUnknownBlock
	}

	if header.Time.Cmp(big.NewInt(now().Unix())) > 0 {
		sb.logger.Warnf("date/time different between different nodes. block from future with time:%v, bigger than now:%v", header.Time.Uint64(), now().Unix())

	}

	if _, err := tdmTypes.ExtractTendermintExtra(header); err != nil {
		return errInvalidExtraDataFormat
	}

	if header.Nonce != (types.TendermintEmptyNonce) && !bytes.Equal(header.Nonce[:], types.TendermintNonce) {
		return errInvalidNonce
	}

	if header.MixDigest != types.TendermintDigest {
		return errInvalidMixDigest
	}

	if header.UncleHash != types.TendermintNilUncleHash {
		return errInvalidUncleHash
	}

	if header.Difficulty == nil || header.Difficulty.Cmp(types.TendermintDefaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	if header.Number.Uint64() > sb.GetEpoch().EndBlock {
		for {
			duration := 2 * time.Second
			sb.logger.Infof("Tendermint VerifyHeader, Epoch Switch, wait for %v then try again", duration)
			time.Sleep(duration)

			if header.Number.Uint64() <= sb.GetEpoch().EndBlock {
				break
			}
		}
	}

	if fieldError := sb.verifyCascadingFields(chain, header, parents); fieldError != nil {
		return fieldError
	}

	if !sb.chainConfig.IsMainChain() {
		if header.MainChainNumber == nil {
			return errInvalidMainChainNumber
		}

		tried := 0
		for {

			ourMainChainHeight := sb.core.cch.GetHeightFromMainChain()
			if ourMainChainHeight.Cmp(header.MainChainNumber) >= 0 {
				break
			}

			if tried == 10 {
				sb.logger.Warnf("Tendermint VerifyHeader, Main Chain Number mismatch, after retried %d times", tried)
				return errMainChainNotCatchup
			}

			duration := 30 * time.Second
			tried++
			sb.logger.Infof("Tendermint VerifyHeader, Main Chain Number mismatch, wait for %v then try again (count %d)", duration, tried)
			time.Sleep(duration)
		}
	}

	return nil
}

func (sb *backend) verifyCascadingFields(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {

	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}

	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}
	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}

	err := sb.verifyCommittedSeals(chain, header, parents)
	return err
}

func (sb *backend) VerifyHeaderBeforeConsensus(chain consensus.ChainReader, header *types.Header, seal bool) error {
	sb.logger.Info("Tendermint backend verify header before consensus")

	if header.Number == nil {
		return errUnknownBlock
	}

	if header.Time.Cmp(big.NewInt(now().Unix())) > 0 {
		sb.logger.Warnf("date/time different between different nodes. block from future with time:%v, bigger than now:%v", header.Time.Uint64(), now().Unix())

	}

	if header.Nonce != (types.TendermintEmptyNonce) && !bytes.Equal(header.Nonce[:], types.TendermintNonce) {
		return errInvalidNonce
	}

	if header.MixDigest != types.TendermintDigest {
		return errInvalidMixDigest
	}

	if header.UncleHash != types.TendermintNilUncleHash {
		return errInvalidUncleHash
	}

	if header.Difficulty == nil || header.Difficulty.Cmp(types.TendermintDefaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	return nil
}

func (sb *backend) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for i, header := range headers {
			err := sb.verifyHeader(chain, header, headers[:i])
			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()

	return abort, results
}

func (sb *backend) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {

	if len(block.Uncles()) > 0 {
		return errInvalidUncleHash
	}
	return nil
}

func (sb *backend) verifyCommittedSeals(chain consensus.ChainReader, header *types.Header, parents []*types.Header) error {

	ncExtra, err := tdmTypes.ExtractTendermintExtra(header)
	if err != nil {
		return errInvalidExtraDataFormat
	}

	epoch := sb.core.consensusState.Epoch
	if epoch == nil || epoch.Validators == nil {
		sb.logger.Errorf("verifyCommittedSeals error. Epoch %v", epoch)
		return errInconsistentValidatorSet
	}

	epoch = epoch.GetEpochByBlockNumber(header.Number.Uint64())
	if epoch == nil || epoch.Validators == nil {
		sb.logger.Errorf("verifyCommittedSeals error. Epoch %v", epoch)
		return errInconsistentValidatorSet
	}

	valSet := epoch.Validators
	if !bytes.Equal(valSet.Hash(), ncExtra.ValidatorsHash) {
		sb.logger.Errorf("verifyCommittedSeals error. Our Validator Set %x, ncExtra Valdiator %x", valSet.Hash(), ncExtra.ValidatorsHash)
		sb.logger.Errorf("verifyCommittedSeals error. epoch validator set %v, extra data %v", valSet.String(), ncExtra.String())
		return errInconsistentValidatorSet
	}

	seenCommit := ncExtra.SeenCommit
	if !bytes.Equal(ncExtra.SeenCommitHash, seenCommit.Hash()) {
		sb.logger.Errorf("verifyCommittedSeals SeenCommit is %#+v", seenCommit)
		sb.logger.Errorf("verifyCommittedSeals error. Our SeenCommitHash %x, ncExtra SeenCommitHash %x", seenCommit.Hash(), ncExtra.SeenCommitHash)
		return errInvalidCommittedSeals
	}

	if err = valSet.VerifyCommit(ncExtra.ChainID, ncExtra.Height, seenCommit); err != nil {
		sb.logger.Errorf("verifyCommittedSeals verify commit err %v", err)
		return errInvalidSignature
	}

	return nil
}

func (sb *backend) VerifySeal(chain consensus.ChainReader, header *types.Header) error {

	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}

	if header.Difficulty.Cmp(types.TendermintDefaultDifficulty) != 0 {
		return errInvalidDifficulty
	}

	return nil
}

func (sb *backend) Prepare(chain consensus.ChainReader, header *types.Header) error {

	header.Nonce = types.TendermintEmptyNonce
	header.MixDigest = types.TendermintDigest

	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}

	header.Difficulty = types.TendermintDefaultDifficulty

	extra, err := prepareExtra(header, nil)
	if err != nil {
		return err
	}
	header.Extra = extra

	header.Time = big.NewInt(time.Now().Unix())

	if sb.chainConfig.LigoChainId != params.MainnetChainConfig.LigoChainId && sb.chainConfig.LigoChainId != params.TestnetChainConfig.LigoChainId {
		header.MainChainNumber = sb.core.cch.GetHeightFromMainChain()
	}

	return nil
}

func (sb *backend) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	totalGasFee *big.Int, uncles []*types.Header, receipts []*types.Receipt, ops *types.PendingOps) (*types.Block, error) {

	sb.logger.Debugf("Tendermint Finalize, receipts are: %v", receipts)

	if sb.chainConfig.LigoChainId == params.MainnetChainConfig.LigoChainId || sb.chainConfig.LigoChainId == params.TestnetChainConfig.LigoChainId {

		readyId, updateBytes, removedId := sb.core.cch.ReadyForLaunchSideChain(header.Number, state)
		if len(readyId) > 0 || updateBytes != nil || len(removedId) > 0 {
			if ok := ops.Append(&types.LaunchSideChainsOp{
				SideChainIds:       readyId,
				NewPendingIdx:      updateBytes,
				DeleteSideChainIds: removedId,
			}); !ok {

				sb.logger.Error("Tendermint Finalize, Fail to append LaunchSideChainsOp, only one LaunchSideChainsOp is allowed in each block")
			}
		}
	}

	curBlockNumber := header.Number.Uint64()
	epoch := sb.GetEpoch().GetEpochByBlockNumber(curBlockNumber)

	accumulateRewards(sb.chainConfig, state, header, epoch, totalGasFee)

	if ok, newValidators, _ := epoch.ShouldEnterNewEpoch(header.Number.Uint64(), state); ok {
		ops.Append(&tdmTypes.SwitchEpochOp{
			ChainId:       sb.chainConfig.LigoChainId,
			NewValidators: newValidators,
		})

	}

	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.TendermintNilUncleHash

	return types.NewBlock(header, txs, nil, receipts), nil
}

func (sb *backend) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (interface{}, error) {

	header := block.Header()
	number := header.Number.Uint64()
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return nil, consensus.ErrUnknownAncestor
	}
	block, err := sb.updateBlock(parent, block)
	if err != nil {
		return nil, err
	}

	delay := time.Unix(block.Header().Time.Int64(), 0).Sub(now())
	select {
	case <-time.After(delay):
	case <-stop:
		return nil, nil
	}

	sb.sealMu.Lock()
	sb.proposedBlockHash = block.Hash()
	clear := func() {
		sb.proposedBlockHash = common.Hash{}
		sb.sealMu.Unlock()
	}
	defer clear()

	go tdmTypes.FireEventRequest(sb.core.EventSwitch(), tdmTypes.EventDataRequest{Proposal: block})

	for {
		select {
		case result, ok := <-sb.commitCh:

			if ok {
				sb.logger.Debugf("Tendermint Seal, got result with block.Hash: %x, result.Hash: %x", block.Hash(), result.Hash())

				if block.Hash() == result.Hash() {
					return result, nil
				}
				sb.logger.Debug("Tendermint Seal, hash are different")
			} else {
				sb.logger.Debug("Tendermint Seal, has been restart, just return")
				return nil, nil
			}

		case iresult, ok := <-sb.vcommitCh:

			if ok {
				sb.logger.Debugf("Tendermint Seal, v got result with block.Hash: %x, result.Hash: %x", block.Hash(), iresult.Block.Hash())
				if block.Hash() != iresult.Block.Hash() {
					return iresult, nil
				}
				sb.logger.Debug("Tendermint Seal, v hash are the same")
			} else {
				sb.logger.Debug("Tendermint Seal, v has been restart, just return")
				return nil, nil
			}

		case <-stop:
			sb.logger.Debug("Tendermint Seal, stop")
			return nil, nil
		}
	}

	return nil, nil
}

func (sb *backend) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {

	return types.TendermintDefaultDifficulty
}

func (sb *backend) Commit(proposal *tdmTypes.NCBlock, seals [][]byte, isProposer func() bool) error {

	block := proposal.Block

	h := block.Header()

	err := writeCommittedSeals(h, proposal.NTCExtra)
	if err != nil {
		return err
	}

	block = block.WithSeal(h)

	sb.logger.Debugf("Tendermint Commit, hash: %x, number: %v", block.Hash(), block.Number().Int64())
	sb.logger.Debugf("Tendermint Commit, block: %s", block.String())

	if isProposer() && (sb.proposedBlockHash == block.Hash()) {

		sb.logger.Debugf("Tendermint Commit, proposer | feed to Seal: %x", block.Hash())
		sb.commitCh <- block
		return nil
	} else {
		if proposal.IntermediateResult != nil {
			sb.logger.Debugf("Tendermint Commit, validator | feed to Seal: %x", block.Hash())
			proposal.IntermediateResult.Block = block
			sb.vcommitCh <- proposal.IntermediateResult
		} else {
			sb.logger.Debugf("Tendermint Commit, validator | fetcher enqueue: %x", block.Hash())
			if sb.broadcaster != nil {
				sb.broadcaster.Enqueue(fetcherID, block)
			}
		}
		return nil
	}
}

func (sb *backend) ChainReader() consensus.ChainReader {

	return sb.chain
}

func (sb *backend) ShouldStart() bool {
	return sb.shouldStart
}

func (sb *backend) IsStarted() bool {
	sb.coreMu.RLock()
	start := sb.coreStarted
	sb.coreMu.RUnlock()

	return start
}

func (sb *backend) ForceStart() {
	sb.shouldStart = true
}

func (sb *backend) GetEpoch() *epoch.Epoch {
	return sb.core.consensusState.Epoch
}

func (sb *backend) SetEpoch(ep *epoch.Epoch) {
	sb.core.consensusState.Epoch = ep
}

func (sb *backend) PrivateValidator() common.Address {
	if sb.core.privValidator != nil {
		return sb.core.privValidator.Address
	}
	return common.Address{}
}

func (sb *backend) updateBlock(parent *types.Header, block *types.Block) (*types.Block, error) {

	sb.logger.Debug("Tendermint backend update block")

	header := block.Header()

	err := writeSeal(header, []byte{})
	if err != nil {
		return nil, err
	}

	return block.WithSeal(header), nil
}

func prepareExtra(header *types.Header, vals []common.Address) ([]byte, error) {

	header.Extra = types.MagicExtra
	return nil, nil
}

func writeSeal(h *types.Header, seal []byte) error {

	payload := types.MagicExtra
	h.Extra = payload
	return nil
}

func writeCommittedSeals(h *types.Header, ncExtra *tdmTypes.TendermintExtra) error {

	h.Extra = wire.BinaryBytes(*ncExtra)
	return nil
}

func accumulateRewards(config *params.ChainConfig, state *state.StateDB, header *types.Header, ep *epoch.Epoch, totalGasFee *big.Int) {

	var coinbaseReward *big.Int
	if config.LigoChainId == params.MainnetChainConfig.LigoChainId || config.LigoChainId == params.TestnetChainConfig.LigoChainId {

		rewardPerBlock := ep.RewardPerBlock
		if rewardPerBlock != nil && rewardPerBlock.Sign() == 1 {
			coinbaseReward = big.NewInt(0)
			coinbaseReward.Add(rewardPerBlock, totalGasFee)
		} else {
			coinbaseReward = totalGasFee
		}
	} else {

		rewardPerBlock := state.GetSideChainRewardPerBlock()
		if rewardPerBlock != nil && rewardPerBlock.Sign() == 1 {
			sideChainRewardBalance := state.GetBalance(sideChainRewardAddress)
			if sideChainRewardBalance.Cmp(rewardPerBlock) == -1 {
				rewardPerBlock = sideChainRewardBalance
			}

			state.SubBalance(sideChainRewardAddress, rewardPerBlock)

			coinbaseReward = new(big.Int).Add(rewardPerBlock, totalGasFee)
		} else {
			coinbaseReward = totalGasFee
		}
	}

	selfDeposit := state.GetDepositBalance(header.Coinbase)
	totalProxiedDeposit := state.GetTotalDepositProxiedBalance(header.Coinbase)
	totalDeposit := new(big.Int).Add(selfDeposit, totalProxiedDeposit)

	var selfReward, delegateReward *big.Int
	if totalProxiedDeposit.Sign() == 0 {
		selfReward = coinbaseReward
	} else {
		selfReward = new(big.Int)

		selfPercent := new(big.Float).Quo(new(big.Float).SetInt(selfDeposit), new(big.Float).SetInt(totalDeposit))

		new(big.Float).Mul(new(big.Float).SetInt(coinbaseReward), selfPercent).Int(selfReward)

		delegateReward = new(big.Int).Sub(coinbaseReward, selfReward)
		commission := state.GetCommission(header.Coinbase)
		if commission > 0 {

			commissionReward := new(big.Int).Mul(delegateReward, big.NewInt(int64(commission)))
			commissionReward.Quo(commissionReward, big.NewInt(100))

			selfReward.Add(selfReward, commissionReward)

			delegateReward.Sub(delegateReward, commissionReward)
		}
	}

	state.AddRewardBalanceByDelegateAddress(header.Coinbase, header.Coinbase, selfReward)

	if delegateReward != nil && delegateReward.Sign() > 0 {
		totalIndividualReward := big.NewInt(0)

		state.ForEachProxied(header.Coinbase, func(key common.Address, proxiedBalance, depositProxiedBalance, pendingRefundBalance *big.Int) bool {
			if depositProxiedBalance.Sign() == 1 {

				individualReward := new(big.Int).Quo(new(big.Int).Mul(depositProxiedBalance, delegateReward), totalProxiedDeposit)

				state.AddRewardBalanceByDelegateAddress(key, header.Coinbase, individualReward)

				totalIndividualReward.Add(totalIndividualReward, individualReward)
			}
			return true
		})

		cmp := delegateReward.Cmp(totalIndividualReward)
		if cmp == 1 {

			diff := new(big.Int).Sub(delegateReward, totalIndividualReward)

			state.AddRewardBalanceByDelegateAddress(header.Coinbase, header.Coinbase, diff)
		} else if cmp == -1 {

			diff := new(big.Int).Sub(totalIndividualReward, delegateReward)

			state.SubRewardBalanceByDelegateAddress(header.Coinbase, header.Coinbase, diff)
		}
	}
}
