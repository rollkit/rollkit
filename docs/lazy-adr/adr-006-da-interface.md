# ADR 005: Data Availability Client Interface

## Changelog

- 2021.04.30: Initial draft
- 2021.06.03: Init method added
- 2021.07.09: Added CheckBlockAvailability method, added KVStore to Init method, added missing result types

## Context

Optimint requires data availability layer. Different implementations are expected.

## Alternative Approaches

> This section contains information around alternative options that are considered before making a decision. It should contain a explanation on why the alternative approach(es) were not chosen.

## Decision

Defined interface should be very generic.
Interface should consist of 5 methods: `Init`, `Start`, `Stop`, `SubmitBlock`, `CheckBlockAvailability`.
There is also optional interface `BlockRetriever` for data availability layer clients that are also able to get block data.
All the details are implementation-specific.

## Detailed Design

Definition of interface:
```go
type DataAvailabilityLayerClient interface {
	// Init is called once to allow DA client to read configuration and initialize resources.
	Init(config []byte, kvStore store.KVStore, logger log.Logger) error

	Start() error
	Stop() error

	// SubmitBlock submits the passed in block to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlock(block *types.Block) ResultSubmitBlock

	// CheckBlockAvailability queries DA layer to check block's data availability.
	CheckBlockAvailability(block *types.Block) ResultCheckBlock
}

// BlockRetriever is additional interface that can be implemented by Data Availability Layer Client that is able to retrieve
// block data from DA layer. This gives the ability to use it for block synchronization.
type BlockRetriever interface {
	RetrieveBlock(height uint64) ResultRetrieveBlock
}

// TODO define an enum of different non-happy-path cases
// that might need to be handled by Optimint independent of
// the underlying DA chain.
type StatusCode uint64

// Data Availability return codes.
const (
	StatusUnknown StatusCode = iota
	StatusSuccess
	StatusTimeout
	StatusError
)

type DAResult struct {
	// Code is to determine if the action succeeded.
	Code StatusCode
	// Message may contain DA layer specific information (like DA block height/hash, detailed error message, etc)
	Message string
}

// ResultSubmitBlock contains information returned from DA layer after block submission.
type ResultSubmitBlock struct {
	DAResult
	// Not sure if this needs to be bubbled up to other
	// parts of Optimint.
	// Hash hash.Hash
}

// ResultCheckBlock contains information about block availability, returned from DA layer client.
type ResultCheckBlock struct {
	DAResult
	// DataAvailable is the actual answer whether the block is available or not.
	// It can be true if and only if Code is equal to StatusSuccess.
	DataAvailable bool
}

type ResultRetrieveBlock struct {
	DAResult
	// Block is the full block retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Block *types.Block
}
```
>

## Status

Implemented

## Consequences

> This section describes the consequences, after applying the decision. All consequences should be summarized here, not just the "positive" ones.

### Positive

### Negative

### Neutral

## References

> Are there any relevant PR comments, issues that led up to this, or articles referenced for why we made the given design choice? If so link them here!

