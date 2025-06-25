package da

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

// DA defines very generic interface for interaction with Data Availability layers.
type DA interface {
	// Get returns Blob for each given ID, or an error.
	//
	// Error should be returned if ID is not formatted properly, there is no Blob for given ID or any other client-level
	// error occurred (dropped connection, timeout, etc).
	Get(ctx context.Context, ids []ID, namespace []byte) ([]Blob, error)

	// GetIDs returns IDs of all Blobs located in DA at given height.
	GetIDs(ctx context.Context, height uint64, namespace []byte) (*GetIDsResult, error)

	// GetProofs returns inclusion Proofs for Blobs specified by their IDs.
	GetProofs(ctx context.Context, ids []ID, namespace []byte) ([]Proof, error)

	// Commit creates a Commitment for each given Blob.
	Commit(ctx context.Context, blobs []Blob, namespace []byte) ([]Commitment, error)

	// Submit submits the Blobs to Data Availability layer.
	//
	// This method is synchronous. Upon successful submission to Data Availability layer, it returns the IDs identifying blobs
	// in DA.
	Submit(ctx context.Context, blobs []Blob, gasPrice float64, namespace []byte) ([]ID, error)

	// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
	Validate(ctx context.Context, ids []ID, proofs []Proof, namespace []byte) ([]bool, error)

	// GasPrice returns the gas price for the DA layer.
	GasPrice(ctx context.Context) (float64, error)

	// GasMultiplier returns the gas multiplier for the DA layer.
	GasMultiplier(ctx context.Context) (float64, error)
}

// Blob is the data submitted/received from DA interface.
type Blob = []byte

// ID should contain serialized data required by the implementation to find blob in Data Availability layer.
type ID = []byte

// Commitment should contain serialized cryptographic commitment to Blob value.
type Commitment = []byte

// Proof should contain serialized proof of inclusion (publication) of Blob in Data Availability layer.
type Proof = []byte

// GetIDsResult holds the result of GetIDs call: IDs and timestamp of corresponding block.
type GetIDsResult struct {
	IDs       []ID
	Timestamp time.Time
}

// ResultSubmit contains information returned from DA layer after block headers/data submission.
type ResultSubmit struct {
	BaseResult
}

// ResultRetrieveHeaders contains batch of block headers returned from DA layer client.
type ResultRetrieve struct {
	BaseResult
	// Data is the block data retrieved from Data Availability Layer.
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
	StatusIncorrectAccountSequence
	StatusContextCanceled
	StatusHeightFromFuture
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
	// IDs is the list of IDs of the blobs submitted.
	IDs [][]byte
	// Timestamp is the timestamp of the posted data on Data Availability Layer.
	Timestamp time.Time
}

// makeID creates an ID from a height and a commitment.
func makeID(height uint64, commitment []byte) []byte {
	id := make([]byte, len(commitment)+8)
	binary.LittleEndian.PutUint64(id, height)
	copy(id[8:], commitment)
	return id
}

// SplitID splits an ID into a height and a commitment.
// if len(id) <= 8, it returns 0 and nil.
func SplitID(id []byte) (uint64, []byte, error) {
	if len(id) <= 8 {
		return 0, nil, fmt.Errorf("invalid ID length: %d", len(id))
	}
	commitment := id[8:]
	return binary.LittleEndian.Uint64(id[:8]), commitment, nil
}
