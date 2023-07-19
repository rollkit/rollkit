package da

import (
	"context"
	"errors"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
)

var (
	// ErrDataNotFound is used to indicated that requested data failed to be retrieved.
	ErrDataNotFound = errors.New("data not found")
	// ErrNamespaceNotFound is used to indicate that the block contains data, but not for the requested namespace.
	ErrNamespaceNotFound = errors.New("namespace not found in data")
	ErrBlobNotFound      = errors.New("blob: not found")
	ErrEDSNotFound       = errors.New("eds not found")
)

// StatusCode is a type for DA layer return status.
// TODO: define an enum of different non-happy-path cases
// that might need to be handled by Rollkit independent of
// the underlying DA chain.
type StatusCode uint64

// Data Availability return codes.
const (
	StatusUnknown StatusCode = iota
	StatusSuccess
	StatusNotFound
	StatusError
)

// BaseResult contains basic information returned by DA layer.
type BaseResult struct {
	// Code is to determine if the action succeeded.
	Code StatusCode
	// Message may contain DA layer specific information (like DA block height/hash, detailed error message, etc)
	Message string
	// DAHeight informs about a height on Data Availability Layer for given result.
	DAHeight uint64
}

// ResultSubmitBlocks contains information returned from DA layer after blocks submission.
type ResultSubmitBlocks struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}

// ResultCheckBlock contains information about block availability, returned from DA layer client.
type ResultCheckBlock struct {
	BaseResult
	// DataAvailable is the actual answer whether the block is available or not.
	// It can be true if and only if Code is equal to StatusSuccess.
	DataAvailable bool
}

// ResultRetrieveBlocks contains batch of blocks returned from DA layer client.
type ResultRetrieveBlocks struct {
	BaseResult
	// Block is the full block retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Blocks []*types.Block
}

// DataAvailabilityLayerClient defines generic interface for DA layer block submission.
// It also contains life-cycle methods.
type DataAvailabilityLayerClient interface {
	// Init is called once to allow DA client to read configuration and initialize resources.
	Init(namespaceID types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error

	// Start is called once, after Init. It's implementation should start operation of DataAvailabilityLayerClient.
	Start() error

	// Stop is called once, when DataAvailabilityLayerClient is no longer needed.
	Stop() error

	// SubmitBlocks submits the passed in blocks to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlocks(ctx context.Context, blocks []*types.Block) ResultSubmitBlocks
}

// BlockRetriever is additional interface that can be implemented by Data Availability Layer Client that is able to retrieve
// block data from DA layer. This gives the ability to use it for block synchronization.
type BlockRetriever interface {
	// RetrieveBlocks returns blocks at given data layer height from data availability layer.
	RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) ResultRetrieveBlocks
}
