package da

import "context"

type Client interface {
	SubmitHeaders(ctx context.Context, headers [][]byte, maxBlobSize uint64, gasPrice float64) ResultSubmit

	// RetrieveHeaders retrieves block headers from DA layer.
	RetrieveHeaders(ctx context.Context, dataLayerHeight uint64) ResultRetrieveHeaders
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

// ResultSubmit contains information returned from DA layer after block headers/data submission.
type ResultSubmit struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}
