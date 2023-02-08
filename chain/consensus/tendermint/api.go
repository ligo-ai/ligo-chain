package tendermint

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ligo-ai/ligo-chain/chain/consensus"
	"github.com/ligo-ai/ligo-chain/chain/consensus/tendermint/epoch"
	tdmTypes "github.com/ligo-ai/ligo-chain/chain/consensus/tendermint/types"
	"github.com/ligo-ai/ligo-chain/utilities/common"
	"github.com/ligo-ai/ligo-chain/utilities/common/hexutil"
	ligoCrypto "github.com/ligo-ai/ligo-chain/utilities/crypto"
	"github.com/ligo-libs/crypto-go"
)

type API struct {
	chain   consensus.ChainReader
	tendermint *backend
}

func (api *API) GetCurrentEpochNumber() (hexutil.Uint64, error) {
	return hexutil.Uint64(api.tendermint.core.consensusState.Epoch.Number), nil
}

func (api *API) GetEpoch(num hexutil.Uint64) (*tdmTypes.EpochApiForConsole, error) {

	number := uint64(num)
	var resultEpoch *epoch.Epoch
	curEpoch := api.tendermint.core.consensusState.Epoch
	if number < 0 || number > curEpoch.Number {
		return nil, errors.New("epoch number out of range")
	}

	if number == curEpoch.Number {
		resultEpoch = curEpoch
	} else {
		resultEpoch = epoch.LoadOneEpoch(curEpoch.GetDB(), number, nil)
	}

	validators := make([]*tdmTypes.EpochValidatorForConsole, len(resultEpoch.Validators.Validators))
	for i, val := range resultEpoch.Validators.Validators {
		validators[i] = &tdmTypes.EpochValidatorForConsole{
			Address:        common.BytesToAddress(val.Address).String(),
			PubKey:         val.PubKey.KeyString(),
			Amount:         (*hexutil.Big)(val.VotingPower),
			RemainingEpoch: hexutil.Uint64(val.RemainingEpoch),
		}
	}

	return &tdmTypes.EpochApiForConsole{
		Number:         hexutil.Uint64(resultEpoch.Number),
		RewardPerBlock: (*hexutil.Big)(resultEpoch.RewardPerBlock),
		StartBlock:     hexutil.Uint64(resultEpoch.StartBlock),
		EndBlock:       hexutil.Uint64(resultEpoch.EndBlock),
		StartTime:      resultEpoch.StartTime,
		EndTime:        resultEpoch.EndTime,
		Validators:     validators,
	}, nil
}

func (api *API) GetNextEpochVote() (*tdmTypes.EpochVotesApiForConsole, error) {

	ep := api.tendermint.core.consensusState.Epoch
	if ep.GetNextEpoch() != nil {

		var votes []*epoch.EpochValidatorVote
		if ep.GetNextEpoch().GetEpochValidatorVoteSet() != nil {
			votes = ep.GetNextEpoch().GetEpochValidatorVoteSet().Votes
		}
		votesApi := make([]*tdmTypes.EpochValidatorVoteApiForConsole, 0, len(votes))
		for _, v := range votes {
			var pkstring string
			if v.PubKey != nil {
				pkstring = v.PubKey.KeyString()
			}

			votesApi = append(votesApi, &tdmTypes.EpochValidatorVoteApiForConsole{
				EpochValidatorForConsole: tdmTypes.EpochValidatorForConsole{
					Address: v.Address.String(),
					PubKey:  pkstring,
					Amount:  (*hexutil.Big)(v.Amount),
				},
				Salt:     v.Salt,
				VoteHash: v.VoteHash,
				TxHash:   v.TxHash,
			})
		}

		return &tdmTypes.EpochVotesApiForConsole{
			EpochNumber: hexutil.Uint64(ep.GetNextEpoch().Number),
			StartBlock:  hexutil.Uint64(ep.GetNextEpoch().StartBlock),
			EndBlock:    hexutil.Uint64(ep.GetNextEpoch().EndBlock),
			Votes:       votesApi,
		}, nil
	}
	return nil, errors.New("next epoch has not been proposed")
}

func (api *API) GetNextEpochValidators() ([]*tdmTypes.EpochValidatorForConsole, error) {

	//height := api.chain.CurrentBlock().NumberU64()

	ep := api.tendermint.core.consensusState.Epoch
	nextEp := ep.GetNextEpoch()
	if nextEp == nil {
		return nil, errors.New("voting for next epoch has not started yet")
	} else {
		state, err := api.chain.State()
		if err != nil {
			return nil, err
		}

		nextValidators := ep.Validators.Copy()
		err = epoch.DryRunUpdateEpochValidatorSet(state, nextValidators, nextEp.GetEpochValidatorVoteSet())
		if err != nil {
			return nil, err
		}

		validators := make([]*tdmTypes.EpochValidatorForConsole, 0, len(nextValidators.Validators))
		for _, val := range nextValidators.Validators {
			var pkstring string
			if val.PubKey != nil {
				pkstring = val.PubKey.KeyString()
			}
			validators = append(validators, &tdmTypes.EpochValidatorForConsole{
				Address:        common.BytesToAddress(val.Address).String(),
				PubKey:         pkstring,
				Amount:         (*hexutil.Big)(val.VotingPower),
				RemainingEpoch: hexutil.Uint64(val.RemainingEpoch),
			})
		}

		return validators, nil
	}
}

// CreateValidator
func (api *API) CreateValidator(from common.Address) (*tdmTypes.PrivV, error) {
	validator := tdmTypes.GenPrivValidatorKey(from)
	privV := &tdmTypes.PrivV{
		Address: validator.Address.String(),
		PubKey:  validator.PubKey,
		PrivKey: validator.PrivKey,
	}
	return privV, nil
}

// decode extra data
func (api *API) DecodeExtraData(extra string) (extraApi *tdmTypes.TendermintExtraApi, err error) {
	ncExtra, err := tdmTypes.DecodeExtraData(extra)
	if err != nil {
		return nil, err
	}
	extraApi = &tdmTypes.TendermintExtraApi{
		ChainID:         ncExtra.ChainID,
		Height:          hexutil.Uint64(ncExtra.Height),
		Time:            ncExtra.Time,
		NeedToSave:      ncExtra.NeedToSave,
		NeedToBroadcast: ncExtra.NeedToBroadcast,
		EpochNumber:     hexutil.Uint64(ncExtra.EpochNumber),
		SeenCommitHash:  hexutil.Encode(ncExtra.SeenCommitHash),
		ValidatorsHash:  hexutil.Encode(ncExtra.ValidatorsHash),
		SeenCommit: &tdmTypes.CommitApi{
			BlockID: tdmTypes.BlockIDApi{
				Hash: hexutil.Encode(ncExtra.SeenCommit.BlockID.Hash),
				PartsHeader: tdmTypes.PartSetHeaderApi{
					Total: hexutil.Uint64(ncExtra.SeenCommit.BlockID.PartsHeader.Total),
					Hash:  hexutil.Encode(ncExtra.SeenCommit.BlockID.PartsHeader.Hash),
				},
			},
			Height:   hexutil.Uint64(ncExtra.SeenCommit.Height),
			Round:    ncExtra.SeenCommit.Round,
			SignAggr: ncExtra.SeenCommit.SignAggr,
			BitArray: ncExtra.SeenCommit.BitArray,
		},
		EpochBytes: ncExtra.EpochBytes,
	}
	return extraApi, nil
}

// get consensus publickey of the block
func (api *API) GetConsensusPublicKey(extra string) ([]string, error) {
	ncExtra, err := tdmTypes.DecodeExtraData(extra)
	if err != nil {
		return nil, err
	}

	//fmt.Printf("GetConsensusPublicKey ncExtra %v\n", ncExtra)
	number := uint64(ncExtra.EpochNumber)
	var resultEpoch *epoch.Epoch
	curEpoch := api.tendermint.core.consensusState.Epoch
	if number < 0 || number > curEpoch.Number {
		return nil, errors.New("epoch number out of range")
	}

	if number == curEpoch.Number {
		resultEpoch = curEpoch
	} else {
		resultEpoch = epoch.LoadOneEpoch(curEpoch.GetDB(), number, nil)
	}

	//fmt.Printf("GetConsensusPublicKey result epoch %v\n", resultEpoch)
	validatorSet := resultEpoch.Validators
	//fmt.Printf("GetConsensusPublicKey validatorset %v\n", validatorSet)

	aggr, err := validatorSet.GetAggrPubKeyAndAddress(ncExtra.SeenCommit.BitArray)
	if err != nil {
		return nil, err
	}

	var pubkeys []string
	if len(aggr.PublicKeys) > 0 {
		for _, v := range aggr.PublicKeys {
			if v != "" {
				pubkeys = append(pubkeys, v)
			}
		}
	}

	return pubkeys, nil
}

func (api *API) GetVoteHash(from common.Address, pubkey crypto.BLSPubKey, amount *hexutil.Big, salt string) common.Hash {
	byteData := [][]byte{
		from.Bytes(),
		pubkey.Bytes(),
		(*big.Int)(amount).Bytes(),
		[]byte(salt),
	}
	return ligoCrypto.Keccak256Hash(ConcatCopyPreAllocate(byteData))
}

func (api *API) GetValidatorStatus(from common.Address) (*tdmTypes.ValidatorStatus, error) {
	state, err := api.chain.State()
	if state == nil || err != nil {
		return nil, err
	}
	status := &tdmTypes.ValidatorStatus{
		IsBanned: state.GetOrNewStateObject(from).IsBanned(),
	}

	return status, nil
}

func (api *API) GetCandidateList() (*tdmTypes.CandidateApi, error) {
	state, err := api.chain.State()

	if state == nil || err != nil {
		return nil, err
	}

	candidateList := make([]string, 0)
	candidateSet := state.GetCandidateSet()
	fmt.Printf("candidate set %v", candidateSet)
	for addr := range candidateSet {
		candidateList = append(candidateList, addr.String())
	}

	candidates := &tdmTypes.CandidateApi{
		CandidateList: candidateList,
	}

	return candidates, nil
}

func (api *API) GetBannedList() (*tdmTypes.BannedApi, error) {
	state, err := api.chain.State()

	if state == nil || err != nil {
		return nil, err
	}

	bannedList := make([]string, 0)
	bannedSet := state.GetBannedSet()
	fmt.Printf("banned set %v", bannedSet)
	for addr := range bannedSet {
		bannedList = append(bannedList, addr.String())
	}

	bannedAddresses := &tdmTypes.BannedApi{
		BannedList: bannedList,
	}

	return bannedAddresses, nil
}
