package da

import "context"

// Client is the interface for the DA layer client.
type Client interface {
	// SubmitHeaders submits block headers to DA layer.
	// The caller is responsible for setting a timeout, if needed.
	SubmitHeaders(ctx context.Context, headers [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmit

	// RetrieveHeaders retrieves block headers from DA layer.
	// The caller is responsible for decoding headers and setting a timeout, if needed.
	RetrieveHeaders(ctx context.Context, dataLayerHeight uint64) ResultRetrieveHeaders

	// MaxBlobSize returns the maximum blob size for the DA layer.
	MaxBlobSize(ctx context.Context) (uint64, error)

	// SubmitBatch submits a batch of blobs to the DA layer.
	SubmitBatch(ctx context.Context, data [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmitBatch
}

// ResultRetrieveHeaders contains batch of block headers returned from DA layer client.
type ResultRetrieveHeaders struct {
	BaseResult
	// Header is the block header retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Headers [][]byte
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
	// DAHeight informs about a height on Data Availability Layer for given result.
	DAHeight uint64
	// SubmittedCount is the number of successfully submitted blocks.
	SubmittedCount uint64
}

//--------------------------------
// batches
//--------------------------------

// ResultSubmit contains information returned from DA layer after block headers/data submission.
type ResultSubmit struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}

// ResultSubmitBatch contains information returned from DA layer after block headers/data submission.
type ResultSubmitBatch struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}

// ResultRetrieveBatch contains batch of block data returned from DA layer client.
type ResultRetrieveBatch struct {
	BaseResult
	// Data is the block data retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Data [][]byte
}
