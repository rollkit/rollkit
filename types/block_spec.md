# Block and Header Validity

## Abstract

Like all blockchains, rollups are composed of a chain of blocks from the rollup's genesis to its head, each consisting of metadata in the header, and transaction data in the body.

## Verification

Both full and light nodes perform validation of the block headers, full nodes additionally verify the block's body.

## SignedHeader validation

1. Valid serialization according to the Protobuf specification
2. `BaseHeader`:

- `Height`: corresponds to the correct, expected height of the block
- `Time`: unused in Rollkit
- `ChainID`: corresponds to the correct chainID for the rollup.

3. `Header`:

- `Version`: Matches the App and Block versions from the previous state
- `LastHeaderHash`: links to the previous block in the chain
- `LastCommitHash`: valid hash of the previous commit
- `DataHash`: correct hash of the block's transaction data
- `ConsensusHash`: unused in Rollkit
- `AppHash`: commitment to the state of the rollup after applying txs from the current block
- `LastResultsHash`: root hash of all the results from the txs from the previous block
- `ProposerAddress`: Validation rules depend on the rollup's configured aggregation scheme
- `AggregatorsHash`: valid hash of the `SignedHeader`'s `Validators` field

4. Commit:

- `Signatures`: array of signatures, to be validated according to the rollup's aggregation scheme.

## Block specification

Blocks contain the `SignedHeader` and `Data` as follows:

```go
type Block struct {
	SignedHeader SignedHeader
	Data         Data
}
```

## Signed Header Specification

```go
type SignedHeader struct {
	Header
	Commit     Commit
	Validators *cmtypes.ValidatorSet
}

type Commit struct {
	Signatures []Signature // most of the time this is a single signature
}

type ValidatorSet struct {
	// NOTE: persisted via reflect, must be exported.
	Validators []*Validator `json:"validators"`
	Proposer   *Validator   `json:"proposer"`

	// cached (unexported)
	totalVotingPower int64
}

type Validator struct {
	Address     Address       `json:"address"`
	PubKey      crypto.PubKey `json:"pub_key"`
	VotingPower int64         `json:"voting_power"`

	ProposerPriority int64 `json:"proposer_priority"`
}
```

## Header Specification

```go
type Header struct {
	BaseHeader
	// Block and App version
	Version Version

	// prev block info
	LastHeaderHash Hash

	// hashes of block data
	LastCommitHash Hash // commit from aggregator(s) from the last block
	DataHash       Hash // Block.Data root aka Transactions
	ConsensusHash  Hash // consensus params for current block
	AppHash        Hash // state after applying txs from the current block

	// Root hash of all results from the txs from the previous block.
	// This is ABCI specific but smart-contract chains require some way of committing
	// to transaction receipts/results.
	LastResultsHash Hash

	// Note that the address can be derived from the pubkey which can be derived
	// from the signature when using secp256k.
	// We keep this in case users choose another signature format where the
	// pubkey can't be recovered by the signature (e.g. ed25519).
	ProposerAddress []byte // original proposer of the block

	// Hash of block aggregator set, at a time of block creation
	AggregatorsHash Hash
}

type BaseHeader struct {
	// Height represents the block height (aka block number) of a given header
	Height uint64
	// Time contains Unix nanotime of a block
	Time uint64
	// The Chain ID
	ChainID string
}
```
