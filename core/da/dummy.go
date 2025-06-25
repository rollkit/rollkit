package da

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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
	gasPrice           float64
	gasMultiplier      float64
	height             uint64
	blockTime          time.Duration
	stopCh             chan struct{}
}

var ErrHeightFromFutureStr = fmt.Errorf("given height is from the future")

// NewDummyDA creates a new instance of DummyDA with the specified maximum blob size and block time.
func NewDummyDA(maxBlobSize uint64, gasPrice float64, gasMultiplier float64, blockTime time.Duration) *DummyDA {
	return &DummyDA{
		blobs:              make(map[string]Blob),
		commitments:        make(map[string]Commitment),
		proofs:             make(map[string]Proof),
		blobsByHeight:      make(map[uint64][]ID),
		timestampsByHeight: make(map[uint64]time.Time),
		maxBlobSize:        maxBlobSize,
		gasPrice:           gasPrice,
		gasMultiplier:      gasMultiplier,
		height:             1,
		blockTime:          blockTime,
		stopCh:             make(chan struct{}),
	}
}

// StartHeightTicker starts a goroutine that increments currentHeight every blockTime.
func (d *DummyDA) StartHeightTicker() {
	go func() {
		ticker := time.NewTicker(d.blockTime)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.mu.Lock()
				d.height++
				d.mu.Unlock()
			case <-d.stopCh:
				return
			}
		}
	}()
}

// StopHeightTicker stops the height ticker goroutine.
func (d *DummyDA) StopHeightTicker() {
	close(d.stopCh)
}

// MaxBlobSize returns the maximum blob size.
func (d *DummyDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	return d.maxBlobSize, nil
}

// Get returns blobs for the given IDs.
func (d *DummyDA) Get(ctx context.Context, ids []ID) ([]Blob, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	blobs := make([]Blob, 0, len(ids))
	for _, id := range ids {
		blob, exists := d.blobs[string(id)]
		if !exists {
			return nil, ErrBlobNotFound // Use the specific error type
		}
		blobs = append(blobs, blob)
	}
	return blobs, nil
}

// GetIDs returns IDs of all blobs at the given height.
func (d *DummyDA) GetIDs(ctx context.Context, height uint64) (*GetIDsResult, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if height > d.height {
		return nil, fmt.Errorf("%w: requested %d, current %d", ErrHeightFromFutureStr, height, d.height)
	}

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
func (d *DummyDA) GetProofs(ctx context.Context, ids []ID) ([]Proof, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	proofs := make([]Proof, 0, len(ids))
	for _, id := range ids {
		proof, exists := d.proofs[string(id)]
		if !exists {
			return nil, errors.New("proof not found")
		}
		proofs = append(proofs, proof)
	}
	return proofs, nil
}

// Commit creates commitments for the given blobs.
func (d *DummyDA) Commit(ctx context.Context, blobs []Blob) ([]Commitment, error) {
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

// SubmitWithOptions submits blobs to the DA layer with additional options.
func (d *DummyDA) Submit(ctx context.Context, blobs []Blob, options []byte) ([]ID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	batchHeight := d.height
	d.height++

	ids := make([]ID, 0, len(blobs))
	var currentSize uint64

	for _, blob := range blobs { // Use _ instead of i
		blobLen := uint64(len(blob))
		// Check individual blob size first
		if blobLen > d.maxBlobSize {
			// Mimic DAClient behavior: if the first blob is too large, return error.
			// Otherwise, we would have submitted the previous fitting blobs.
			// Since DummyDA processes all at once, we return error if any *individual* blob is too large.
			// A more complex dummy could simulate partial submission based on cumulative size.
			// For now, error out if any single blob is too big.
			return nil, ErrBlobSizeOverLimit // Use specific error type
		}

		// Check cumulative batch size
		if currentSize+blobLen > d.maxBlobSize {
			// Stop processing blobs for this batch, return IDs collected so far
			// d.logger.Info("DummyDA: Blob size limit reached for batch", "maxBlobSize", d.maxBlobSize, "index", i, "currentSize", currentSize, "nextBlobSize", blobLen) // Removed logger call
			break
		}
		currentSize += blobLen

		// Create a commitment using SHA-256 hash
		bz := sha256.Sum256(blob)
		commitment := bz[:]

		// Create ID from height and commitment
		id := makeID(batchHeight, commitment)
		idStr := string(id)

		d.blobs[idStr] = blob
		d.commitments[idStr] = commitment
		d.proofs[idStr] = commitment // Simple proof

		ids = append(ids, id)
	}

	d.blobsByHeight[batchHeight] = ids
	d.timestampsByHeight[batchHeight] = time.Now()

	return ids, nil
}

// Validate validates commitments against proofs.
func (d *DummyDA) Validate(ctx context.Context, ids []ID, proofs []Proof) ([]bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(ids) != len(proofs) {
		return nil, errors.New("number of IDs and proofs must match")
	}

	results := make([]bool, len(ids))
	for i, id := range ids {
		_, exists := d.blobs[string(id)]
		results[i] = exists
	}

	return results, nil
}
