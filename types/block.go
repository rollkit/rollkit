package types

import (
	"encoding"
)

// Header defines the structure of Optimint block header.
type Header struct {
	// Block and App version
	Version Version
	// NamespaceID identifies this chain e.g. when connected to other rollups via IBC.
	// TODO(ismail): figure out if we want to use namespace.ID here instead (downside is that it isn't fixed size)
	// at least extract the used constants (32, 8) as package variables though.
	NamespaceID [8]byte

	Height uint64
	Time   uint64 // time in tai64 format

	// prev block info
	LastHeaderHash [32]byte

	// hashes of block data
	LastCommitHash [32]byte // commit from aggregator(s) from the last block
	DataHash       [32]byte // Block.Data root aka Transactions
	ConsensusHash  [32]byte // consensus params for current block
	AppHash        [32]byte // state after applying txs from the current block

	// Root hash of all results from the txs from the previous block.
	// This is ABCI specific but smart-contract chains require some way of committing
	// to transaction receipts/results.
	LastResultsHash [32]byte

	// Note that the address can be derived from the pubkey which can be derived
	// from the signature when using secp256k.
	// We keep this in case users choose another signature format where the
	// pubkey can't be recovered by the signature (e.g. ed25519).
	ProposerAddress []byte // original proposer of the block
}

var _ encoding.BinaryMarshaler = &Header{}
var _ encoding.BinaryUnmarshaler = &Header{}

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
// This is equivalent to the tmversion.Consensus type in Tendermint.
type Version struct {
	Block uint64
	App   uint64
}

// Block defines the structure of Optimint block.
type Block struct {
	Header     Header
	Data       Data
	LastCommit Commit
}

var _ encoding.BinaryMarshaler = &Block{}
var _ encoding.BinaryUnmarshaler = &Block{}

// Data defines Optimint block data.
type Data struct {
	Txs                    Txs
	IntermediateStateRoots IntermediateStateRoots
	Evidence               EvidenceData
}

// EvidenceData defines how evidence is stored in block.
type EvidenceData struct {
	Evidence []Evidence
}

// Commit cointains evidence of block creation.
type Commit struct {
	Height     uint64
	HeaderHash [32]byte
	Signatures []Signature // most of the time this is a single signature
}

// Signature represents signature of block creator.
type Signature []byte

// IntermediateStateRoots describes the state between transactions.
// They are required for fraud proofs.
type IntermediateStateRoots struct {
	RawRootsList [][]byte
}
