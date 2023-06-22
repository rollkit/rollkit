package types

import (
	"github.com/celestiaorg/go-header"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/celestiaorg/go-fraud"
)

// Implements Proof interface from https://github.com/celestiaorg/go-fraud/

const StateFraudProofType fraud.ProofType = "state-fraud"

type StateFraudProof struct {
	abci.FraudProof
}

func init() {
	fraud.Register(&StateFraudProof{})
}

func (fp *StateFraudProof) Type() fraud.ProofType {
	return StateFraudProofType
}

func (fp *StateFraudProof) HeaderHash() []byte {
	return fp.FraudulentBeginBlock.Hash
}

func (fp *StateFraudProof) Height() uint64 {
	return uint64(fp.BlockHeight)
}

func (fp *StateFraudProof) Validate(header header.Header) error {
	// TODO (ganesh): fill this later
	return nil
}

func (fp *StateFraudProof) MarshalBinary() (data []byte, err error) {
	return fp.Marshal()
}

func (fp *StateFraudProof) UnmarshalBinary(data []byte) error {
	return fp.Unmarshal(data)
}

var _ fraud.Proof = &StateFraudProof{}
