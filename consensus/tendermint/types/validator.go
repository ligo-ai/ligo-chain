package types

import (
	"bytes"
	"fmt"
	"io"

	"math/big"

	ligoTypes "github.com/ligo-ai/ligo-chain/core/types"
	"github.com/ligo-ai/ligo-chain/common"
	. "github.com/ligo-libs/common-go"
	"github.com/ligo-libs/crypto-go"
	"github.com/ligo-libs/wire-go"
)

type Validator struct {
	Address        []byte        `json:"address"`
	PubKey         crypto.PubKey `json:"pub_key"`
	VotingPower    *big.Int      `json:"voting_power"`
	RemainingEpoch uint64        `json:"remain_epoch"`
}

func NewValidator(address []byte, pubKey crypto.PubKey, votingPower *big.Int) *Validator {
	return &Validator{
		Address:     address,
		PubKey:      pubKey,
		VotingPower: votingPower,
	}
}

func (v *Validator) Copy() *Validator {
	vCopy := *v
	vCopy.VotingPower = new(big.Int).Set(v.VotingPower)
	return &vCopy
}

func (v *Validator) Equals(other *Validator) bool {

	return bytes.Equal(v.Address, other.Address) &&
		v.PubKey.Equals(other.PubKey) &&
		v.VotingPower.Cmp(other.VotingPower) == 0
}

func (v *Validator) String() string {
	if v == nil {
		return "nil-Validator"
	}
	return fmt.Sprintf("Validator{ADD:%s PK:%X VP:%v EP:%d}",
		string(v.Address),
		v.PubKey,
		v.VotingPower,
		v.RemainingEpoch)
}

func (v *Validator) Hash() []byte {
	return wire.BinaryRipemd160(v)
}

var ValidatorCodec = validatorCodec{}

type validatorCodec struct{}

func (vc validatorCodec) Encode(o interface{}, w io.Writer, n *int, err *error) {
	wire.WriteBinary(o.(*Validator), w, n, err)
}

func (vc validatorCodec) Decode(r io.Reader, n *int, err *error) interface{} {
	return wire.ReadBinary(&Validator{}, r, 0, n, err)
}

func (vc validatorCodec) Compare(o1 interface{}, o2 interface{}) int {
	PanicSanity("ValidatorCodec.Compare not implemented")
	return 0
}

type RefundValidatorAmount struct {
	Address common.Address
	Amount  *big.Int
	Voteout bool
}

type SwitchEpochOp struct {
	ChainId       string
	NewValidators *ValidatorSet
}

func (op *SwitchEpochOp) Conflict(op1 ligoTypes.PendingOp) bool {
	if _, ok := op1.(*SwitchEpochOp); ok {

		return true
	}
	return false
}

func (op *SwitchEpochOp) String() string {
	return fmt.Sprintf("SwitchEpochOp - ChainId:%v, New Validators: %v", op.ChainId, op.NewValidators)
}
