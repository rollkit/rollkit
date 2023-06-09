package types

import (
	"errors"

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

func (fp *StateFraudProof) Validate(header header.Header, verifier fraud.StateMachineVerifier) error {
	status, err := verifier(fp)
	if err != nil {
		return err
	}
	if !status {
		return errors.New("failed to verify fraud proof")
	}
	return nil
}

func (fp *StateFraudProof) MarshalBinary() (data []byte, err error) {
	return fp.Marshal()
}

func (fp *StateFraudProof) UnmarshalBinary(data []byte) error {
	return fp.Unmarshal(data)
}

var _ fraud.Proof = &StateFraudProof{}
