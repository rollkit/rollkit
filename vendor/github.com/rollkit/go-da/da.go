package da

import "context"

// DA defines very generic interface for interaction with Data Availability layers.
type DA interface {
	// MaxBlobSize returns the max blob size
	MaxBlobSize(ctx context.Context) (uint64, error)

	// Get returns Blob for each given ID, or an error.
	//
	// Error should be returned if ID is not formatted properly, there is no Blob for given ID or any other client-level
	// error occurred (dropped connection, timeout, etc).
	Get(ctx context.Context, ids []ID, namespace Namespace) ([]Blob, error)

	// GetIDs returns IDs of all Blobs located in DA at given height.
	GetIDs(ctx context.Context, height uint64, namespace Namespace) ([]ID, error)

	// GetProofs returns inclusion Proofs for all Blobs located in DA at given height.
	GetProofs(ctx context.Context, ids []ID, namespace Namespace) ([]Proof, error)

	// Commit creates a Commitment for each given Blob.
	Commit(ctx context.Context, blobs []Blob, namespace Namespace) ([]Commitment, error)

	// Submit submits the Blobs to Data Availability layer.
	//
	// This method is synchronous. Upon successful submission to Data Availability layer, it returns the IDs identifying blobs
	// in DA.
	Submit(ctx context.Context, blobs []Blob, gasPrice float64, namespace Namespace) ([]ID, error)

	// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
	Validate(ctx context.Context, ids []ID, proofs []Proof, namespace Namespace) ([]bool, error)
}

// Namespace is an optional parameter used to set the location a blob should be
// posted to, for DA layers supporting the functionality.
type Namespace = []byte

// Blob is the data submitted/received from DA interface.
type Blob = []byte

// ID should contain serialized data required by the implementation to find blob in Data Availability layer.
type ID = []byte

// Commitment should contain serialized cryptographic commitment to Blob value.
type Commitment = []byte

// Proof should contain serialized proof of inclusion (publication) of Blob in Data Availability layer.
type Proof = []byte
