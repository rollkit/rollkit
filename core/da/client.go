package da

import "context"

// Client is the interface for the DA layer client.
type Client interface {
	// SubmitData submits block data to DA layer.
	// The caller is responsible for setting a timeout, if needed.
	Submit(ctx context.Context, data [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmit

	// RetrieveData retrieves block data from DA layer.
	// The caller is responsible for decoding data and setting a timeout, if needed.
	Retrieve(ctx context.Context, dataLayerHeight uint64) ResultRetrieve

	// MaxBlobSize returns the maximum blob size for the DA layer.
	MaxBlobSize(ctx context.Context) (uint64, error)

	// GasPrice returns the gas price for the DA layer.
	GasPrice(ctx context.Context) (float64, error)

	// GasMultiplier returns the gas multiplier for the DA layer.
	GasMultiplier(ctx context.Context) (float64, error)
}

// ResultSubmit contains information returned from DA layer after block headers/data submission.
type ResultSubmit struct {
	BaseResult
}

// ResultRetrieveHeaders contains batch of block headers returned from DA layer client.
type ResultRetrieve struct {
	BaseResult
	// Header is the block header retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Data [][]byte
}

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
	StatusNotIncludedInBlock
	StatusAlreadyInMempool
	StatusTooBig
	StatusContextDeadline
	StatusError
)

// BaseResult contains basic information returned by DA layer.
type BaseResult struct {
	// Code is to determine if the action succeeded.
	Code StatusCode
	// Message may contain DA layer specific information (like DA block height/hash, detailed error message, etc)
	Message string
	// Height is the height of the block on Data Availability Layer for given result.
	Height uint64
	// SubmittedCount is the number of successfully submitted blocks.
	SubmittedCount uint64
	// BlobSize is the size of the blob submitted.
	BlobSize uint64
}
