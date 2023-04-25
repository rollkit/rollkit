package types

import (
	"encoding/json"

	"github.com/celestiaorg/go-header"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/celestiaorg/go-fraud"
)

const StateFraudProof fraud.ProofType = "state-fraud"

type FraudProof abci.FraudProof

func init() {
	fraud.Register(&FraudProof{})
}

func (fp *FraudProof) Type() fraud.ProofType {
	return StateFraudProof
}

func (fp *FraudProof) HeaderHash() []byte {
	return fp.FraudulentBeginBlock.Hash
}

func (fp *FraudProof) Height() uint64 {
	return uint64(fp.BlockHeight)
}

func (fp *FraudProof) Validate(header.Header) error {
	// TODO (ganesh): fill this later
	return nil
}

func (fp *FraudProof) MarshalBinary() (data []byte, err error) {
	return json.Marshal(fp)
}

func (fp *FraudProof) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, fp)
}

var _ fraud.Proof = &FraudProof{}
