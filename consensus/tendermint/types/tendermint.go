package types

import (
	"fmt"
	"time"

	ligoTypes "github.com/ligo-ai/ligo-chain/core/types"
	"github.com/ligo-ai/ligo-chain/common/hexutil"
	"github.com/ligo-libs/merkle-go"
	"github.com/ligo-libs/wire-go"
)

type TendermintExtra struct {
	ChainID         string    `json:"chain_id"`
	Height          uint64    `json:"height"`
	Time            time.Time `json:"time"`
	NeedToSave      bool      `json:"need_to_save"`
	NeedToBroadcast bool      `json:"need_to_broadcast"`
	EpochNumber     uint64    `json:"epoch_number"`
	SeenCommitHash  []byte    `json:"last_commit_hash"`
	ValidatorsHash  []byte    `json:"validators_hash"`
	SeenCommit      *Commit   `json:"seen_commit"`
	EpochBytes      []byte    `json:"epoch_bytes"`
}

func (te *TendermintExtra) Copy() *TendermintExtra {

	return &TendermintExtra{
		ChainID:         te.ChainID,
		Height:          te.Height,
		Time:            te.Time,
		NeedToSave:      te.NeedToSave,
		NeedToBroadcast: te.NeedToBroadcast,
		EpochNumber:     te.EpochNumber,
		SeenCommitHash:  te.SeenCommitHash,
		ValidatorsHash:  te.ValidatorsHash,
		SeenCommit:      te.SeenCommit,
		EpochBytes:      te.EpochBytes,
	}
}

func (te *TendermintExtra) Hash() []byte {
	if len(te.ValidatorsHash) == 0 {
		return nil
	}
	return merkle.SimpleHashFromMap(map[string]interface{}{
		"ChainID":         te.ChainID,
		"Height":          te.Height,
		"Time":            te.Time,
		"NeedToSave":      te.NeedToSave,
		"NeedToBroadcast": te.NeedToBroadcast,
		"EpochNumber":     te.EpochNumber,
		"Validators":      te.ValidatorsHash,
		"EpochBytes":      te.EpochBytes,
	})
}

func ExtractTendermintExtra(h *ligoTypes.Header) (*TendermintExtra, error) {

	if len(h.Extra) == 0 {
		return &TendermintExtra{}, nil
	}

	var ncExtra = TendermintExtra{}
	err := wire.ReadBinaryBytes(h.Extra[:], &ncExtra)

	if err != nil {
		return nil, err
	}
	return &ncExtra, nil
}

func (te *TendermintExtra) String() string {
	str := fmt.Sprintf(`Network info: {
ChainID:   %s
EpochNo.:  %v
Height:    %v
Timestamp: %v

}
`, te.ChainID, te.EpochNumber, te.Height, te.Time)
	return str
}

func DecodeExtraData(extra string) (ncExtra *TendermintExtra, err error) {
	ncExtra = &TendermintExtra{}
	extraByte, err := hexutil.Decode(extra)
	if err != nil {
		return nil, err
	}

	err = wire.ReadBinaryBytes(extraByte, ncExtra)
	if err != nil {
		return nil, err
	}
	return ncExtra, nil
}
