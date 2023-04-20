package types

import (
	"encoding"

	tmtypes "github.com/tendermint/tendermint/types"
)

type NamespaceID [8]byte

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
// This is equivalent to the tmversion.Consensus type in Tendermint.
type Version struct {
	Block uint64
	App   uint64
}

// Block defines the structure of Rollkit block.
type Block struct {
	SignedHeader SignedHeader
	Data         Data
}

var _ encoding.BinaryMarshaler = &Block{}
var _ encoding.BinaryUnmarshaler = &Block{}

// Data defines Rollkit block data.
type Data struct {
	Txs                    Txs
	IntermediateStateRoots IntermediateStateRoots
	Evidence               EvidenceData
}

// EvidenceData defines how evidence is stored in block.
type EvidenceData struct {
	Evidence []Evidence
}

// Commit contains evidence of block creation.
type Commit struct {
	Signatures []Signature // most of the time this is a single signature
}

// SignedHeader combines Header and its Commit.
//
// Used mostly for gossiping.
type SignedHeader struct {
	Header
	Commit     Commit
	Validators *tmtypes.ValidatorSet
}

// Signature represents signature of block creator.
type Signature []byte

// IntermediateStateRoots describes the state between transactions.
// They are required for fraud proofs.
type IntermediateStateRoots struct {
	RawRootsList [][]byte
}

type TxWithISRs struct {
	preISR  []byte
	Tx      Tx
	postISR []byte
}
