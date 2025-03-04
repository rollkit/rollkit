package da

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// DummyDA is a simple in-memory implementation of the DA interface for testing purposes.
type DummyDA struct {
	mu                 sync.RWMutex
	blobs              map[string]Blob
	commitments        map[string]Commitment
	proofs             map[string]Proof
	blobsByHeight      map[uint64][]ID
	timestampsByHeight map[uint64]time.Time
	maxBlobSize        uint64
}

// NewDummyDA creates a new instance of DummyDA with the specified maximum blob size.
func NewDummyDA(maxBlobSize uint64) *DummyDA {
	return &DummyDA{
		blobs:              make(map[string]Blob),
		commitments:        make(map[string]Commitment),
		proofs:             make(map[string]Proof),
		blobsByHeight:      make(map[uint64][]ID),
		timestampsByHeight: make(map[uint64]time.Time),
		maxBlobSize:        maxBlobSize,
	}
}

// MaxBlobSize returns the maximum blob size.
func (d *DummyDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	return d.maxBlobSize, nil
}

// Get returns blobs for the given IDs.
func (d *DummyDA) Get(ctx context.Context, ids []ID, namespace Namespace) ([]Blob, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	blobs := make([]Blob, 0, len(ids))
	for _, id := range ids {
		idStr := string(id)
		blob, exists := d.blobs[idStr]
		if !exists {
			return nil, errors.New("blob not found")
		}
		blobs = append(blobs, blob)
	}
	return blobs, nil
}

// GetIDs returns IDs of all blobs at the given height.
func (d *DummyDA) GetIDs(ctx context.Context, height uint64, namespace Namespace) (*GetIDsResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	ids, exists := d.blobsByHeight[height]
	if !exists {
		return &GetIDsResult{
			IDs:       []ID{},
			Timestamp: time.Now(),
		}, nil
	}

	return &GetIDsResult{
		IDs:       ids,
		Timestamp: d.timestampsByHeight[height],
	}, nil
}

// GetProofs returns proofs for the given IDs.
func (d *DummyDA) GetProofs(ctx context.Context, ids []ID, namespace Namespace) ([]Proof, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	proofs := make([]Proof, 0, len(ids))
	for _, id := range ids {
		idStr := string(id)
		proof, exists := d.proofs[idStr]
		if !exists {
			return nil, errors.New("proof not found")
		}
		proofs = append(proofs, proof)
	}
	return proofs, nil
}

// Commit creates commitments for the given blobs.
func (d *DummyDA) Commit(ctx context.Context, blobs []Blob, namespace Namespace) ([]Commitment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	commitments := make([]Commitment, 0, len(blobs))
	for _, blob := range blobs {
		// For simplicity, we use the blob itself as the commitment
		commitment := blob
		commitments = append(commitments, commitment)
	}
	return commitments, nil
}

// Submit submits blobs to the DA layer.
func (d *DummyDA) Submit(ctx context.Context, blobs []Blob, gasPrice uint64, namespace Namespace) ([]ID, error) {
	return d.SubmitWithOptions(ctx, blobs, gasPrice, namespace, nil)
}

// SubmitWithOptions submits blobs to the DA layer with additional options.
func (d *DummyDA) SubmitWithOptions(ctx context.Context, blobs []Blob, gasPrice uint64, namespace Namespace, options []byte) ([]ID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	height := uint64(len(d.blobsByHeight) + 1)
	ids := make([]ID, 0, len(blobs))

	for _, blob := range blobs {
		if uint64(len(blob)) > d.maxBlobSize {
			return nil, errors.New("blob size exceeds maximum")
		}

		// For simplicity, we use the blob itself as the ID
		id := blob
		idStr := string(id)

		d.blobs[idStr] = blob
		bz := sha256.Sum256(blob)
		d.commitments[idStr] = bz[:] // Simple commitment
		d.proofs[idStr] = bz[:]      // Simple proof

		ids = append(ids, id)
	}

	d.blobsByHeight[height] = ids
	d.timestampsByHeight[height] = time.Now()
	return ids, nil
}

// Validate validates commitments against proofs.
func (d *DummyDA) Validate(ctx context.Context, ids []ID, proofs []Proof, namespace Namespace) ([]bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(ids) != len(proofs) {
		return nil, errors.New("number of IDs and proofs must match")
	}

	results := make([]bool, len(ids))
	for i, id := range ids {
		idStr := string(id)
		_, exists := d.blobs[idStr]
		results[i] = exists
	}

	return results, nil
}
