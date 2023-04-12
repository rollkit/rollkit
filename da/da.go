package da

import (
	"context"
	"errors"

	"github.com/celestiaorg/go-header"
	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
)

// ErrNotFound is used to indicated that requested data could not be found.
var ErrDataNotFound = errors.New("data not found")
var ErrEDSNotFound = errors.New("eds not found")

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

// ResultSubmitBlock contains information returned from DA layer after block header/data submission.
type ResultSubmitBlock struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	Hash header.Hash
}

// ResultCheckBlock contains information about block availability, returned from DA layer client.
type ResultCheckBlock struct {
	BaseResult
	// DataAvailable is the actual answer whether the block is available or not.
	// It can be true if and only if Code is equal to StatusSuccess.
	DataAvailable bool
}

// ResultRetrieveBlockHeaders contains batch of block headers returned from DA layer client.
type ResultRetrieveBlockHeaders struct {
	BaseResult
	// Header is the block header retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Headers []*types.SignedHeader
}

// ResultRetrieveBlockDatas contains batch of block datas returned from DA layer client.
type ResultRetrieveBlockDatas struct {
	BaseResult
	// Data is the block data retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Datas []*types.Data
}

// DataAvailabilityLayerClient defines generic interface for DA layer block submission.
// It also contains life-cycle methods.
type DataAvailabilityLayerClient interface {
	// Init is called once to allow DA client to read configuration and initialize resources.
	Init(headerNamespaceID, dataNamespaceID types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error

	// Start is called once, after Init. It's implementation should start operation of DataAvailabilityLayerClient.
	Start() error

	// Stop is called once, when DataAvailabilityLayerClient is no longer needed.
	Stop() error

	// SubmitBlockHeader submits the passed in block header to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlockHeader(ctx context.Context, header *types.SignedHeader) ResultSubmitBlock

	// SubmitBlockData submits the passed in block data to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlockData(ctx context.Context, data *types.Data) ResultSubmitBlock

	// CheckBlockHeaderAvailability queries DA layer to check data availability of block header corresponding at given height.
	CheckBlockHeaderAvailability(ctx context.Context, dataLayerHeight uint64) ResultCheckBlock

	// CheckBlockDataAvailability queries DA layer to check data availability of block data corresponding at given height.
	CheckBlockDataAvailability(ctx context.Context, dataLayerHeight uint64) ResultCheckBlock
}

// BlockRetriever is additional interface that can be implemented by Data Availability Layer Client that is able to retrieve
// block data from DA layer. This gives the ability to use it for block synchronization.
type BlockRetriever interface {
	// RetrieveBlockHeaders returns block headers at given data layer height from data availability layer.
	RetrieveBlockHeaders(ctx context.Context, dataLayerHeight uint64) ResultRetrieveBlockHeaders

	// RetrieveBlockDatas returns block datas at given data layer height from data availability layer.
	RetrieveBlockDatas(ctx context.Context, dataLayerHeight uint64) ResultRetrieveBlockDatas
}
