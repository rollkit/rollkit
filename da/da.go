package da

import (
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

// StatusCode is a type for DA layer return status.
// TODO: define an enum of different non-happy-path cases
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

// DataAvailabilityLayerClient defines generic interface for DA layer block submission.
// It also contains life-cycle methods.
type DataAvailabilityLayerClient interface {
	// Init is called once to allow DA client to read configuration and initialize resources.
	Init(config []byte, kvStore store.KVStore, logger log.Logger) error

	Start() error
	Stop() error

	// SubmitBlock submits the passed in block to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlock(block *types.Block) ResultSubmitBlock

	// CheckBlockAvailability queries DA layer to check data availability of block corresponding to given header.
	CheckBlockAvailability(header *types.Header) ResultCheckBlock
}

// BlockRetriever is additional interface that can be implemented by Data Availability Layer Client that is able to retrieve
// block data from DA layer. This gives the ability to use it for block synchronization.
type BlockRetriever interface {
	// RetrieveBlock returns block at given height from data availability layer.
	RetrieveBlock(height uint64) ResultRetrieveBlock
}
