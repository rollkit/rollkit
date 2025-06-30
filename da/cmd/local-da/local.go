package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	coreda "github.com/rollkit/rollkit/core/da"
)

// DefaultMaxBlobSize is the default max blob size
const DefaultMaxBlobSize = 64 * 64 * 482

// LocalDA is a simple implementation of in-memory DA. Not production ready! Intended only for testing!
//
// Data is stored in a map, where key is a serialized sequence number. This key is returned as ID.
// Commitments are simply hashes, and proofs are ED25519 signatures.
type LocalDA struct {
	mu          *sync.Mutex // protects data and height
	data        map[uint64][]kvp
	timestamps  map[uint64]time.Time
	maxBlobSize uint64
	height      uint64
	privKey     ed25519.PrivateKey
	pubKey      ed25519.PublicKey

	logger logging.EventLogger
}

type kvp struct {
	key, value []byte
}

// NewLocalDA create new instance of DummyDA
func NewLocalDA(logger logging.EventLogger, opts ...func(*LocalDA) *LocalDA) *LocalDA {
	da := &LocalDA{
		mu:          new(sync.Mutex),
		data:        make(map[uint64][]kvp),
		timestamps:  make(map[uint64]time.Time),
		maxBlobSize: DefaultMaxBlobSize,
		logger:      logger,
	}
	for _, f := range opts {
		da = f(da)
	}
	da.pubKey, da.privKey, _ = ed25519.GenerateKey(rand.Reader)
	da.logger.Info("NewLocalDA: initialized LocalDA")
	return da
}

var _ coreda.DA = &LocalDA{}

// MaxBlobSize returns the max blob size in bytes.
func (d *LocalDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	d.logger.Debug("MaxBlobSize called", "maxBlobSize", d.maxBlobSize)
	return d.maxBlobSize, nil
}

// GasMultiplier returns the gas multiplier.
func (d *LocalDA) GasMultiplier(ctx context.Context) (float64, error) {
	d.logger.Debug("GasMultiplier called")
	return 1.0, nil
}

// GasPrice returns the gas price.
func (d *LocalDA) GasPrice(ctx context.Context) (float64, error) {
	d.logger.Debug("GasPrice called")
	return 0.0, nil
}

// Get returns Blobs for given IDs.
func (d *LocalDA) Get(ctx context.Context, ids []coreda.ID, _ []byte) ([]coreda.Blob, error) {
	d.logger.Debug("Get called", "ids", ids)
	d.mu.Lock()
	defer d.mu.Unlock()
	blobs := make([]coreda.Blob, len(ids))
	for i, id := range ids {
		if len(id) < 8 {
			d.logger.Error("Get: invalid ID length", "id", id)
			return nil, errors.New("invalid ID")
		}
		height := binary.LittleEndian.Uint64(id)
		found := false
		for j := 0; !found && j < len(d.data[height]); j++ {
			if bytes.Equal(d.data[height][j].key, id) {
				blobs[i] = d.data[height][j].value
				found = true
			}
		}
		if !found {
			d.logger.Warn("Get: blob not found", "id", id, "height", height)
			return nil, coreda.ErrBlobNotFound
		}
	}
	d.logger.Debug("Get successful", "count", len(blobs))
	return blobs, nil
}

// GetIDs returns IDs of Blobs at given DA height.
func (d *LocalDA) GetIDs(ctx context.Context, height uint64, _ []byte) (*coreda.GetIDsResult, error) {
	d.logger.Debug("GetIDs called", "height", height)
	d.mu.Lock()
	defer d.mu.Unlock()

	if height > d.height {
		d.logger.Error("GetIDs: height in future", "requested", height, "current", d.height)
		return nil, fmt.Errorf("height %d is in the future: %w", height, coreda.ErrHeightFromFuture)
	}

	kvps, ok := d.data[height]
	if !ok {
		d.logger.Debug("GetIDs: no data for height", "height", height)
		return nil, nil
	}

	ids := make([]coreda.ID, len(kvps))
	for i, kv := range kvps {
		ids[i] = kv.key
	}
	d.logger.Debug("GetIDs successful", "count", len(ids))
	return &coreda.GetIDsResult{IDs: ids, Timestamp: d.timestamps[height]}, nil
}

// GetProofs returns inclusion Proofs for all Blobs located in DA at given height.
func (d *LocalDA) GetProofs(ctx context.Context, ids []coreda.ID, _ []byte) ([]coreda.Proof, error) {
	d.logger.Debug("GetProofs called", "ids", ids)
	blobs, err := d.Get(ctx, ids, nil)
	if err != nil {
		d.logger.Error("GetProofs: failed to get blobs", "error", err)
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	proofs := make([]coreda.Proof, len(blobs))
	for i, blob := range blobs {
		proofs[i] = d.getProof(ids[i], blob)
	}
	d.logger.Debug("GetProofs successful", "count", len(proofs))
	return proofs, nil
}

// Commit returns cryptographic Commitments for given blobs.
func (d *LocalDA) Commit(ctx context.Context, blobs []coreda.Blob, _ []byte) ([]coreda.Commitment, error) {
	d.logger.Debug("Commit called", "numBlobs", len(blobs))
	commits := make([]coreda.Commitment, len(blobs))
	for i, blob := range blobs {
		commits[i] = d.getHash(blob)
	}
	d.logger.Debug("Commit successful", "count", len(commits))
	return commits, nil
}

// SubmitWithOptions stores blobs in DA layer (options are ignored).
func (d *LocalDA) SubmitWithOptions(ctx context.Context, blobs []coreda.Blob, gasPrice float64, _ []byte, _ []byte) ([]coreda.ID, error) {
	d.logger.Info("SubmitWithOptions called", "numBlobs", len(blobs), "gasPrice", gasPrice)
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]coreda.ID, len(blobs))
	d.height += 1
	d.timestamps[d.height] = time.Now()
	for i, blob := range blobs {
		ids[i] = append(d.nextID(), d.getHash(blob)...)

		d.data[d.height] = append(d.data[d.height], kvp{ids[i], blob})
	}
	d.logger.Info("SubmitWithOptions successful", "newHeight", d.height, "count", len(ids))
	return ids, nil
}

// Submit stores blobs in DA layer (options are ignored).
func (d *LocalDA) Submit(ctx context.Context, blobs []coreda.Blob, gasPrice float64, _ []byte) ([]coreda.ID, error) {
	d.logger.Info("Submit called", "numBlobs", len(blobs), "gasPrice", gasPrice)
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]coreda.ID, len(blobs))
	d.height += 1
	d.timestamps[d.height] = time.Now()
	for i, blob := range blobs {
		ids[i] = append(d.nextID(), d.getHash(blob)...)

		d.data[d.height] = append(d.data[d.height], kvp{ids[i], blob})
	}
	d.logger.Info("Submit successful", "newHeight", d.height, "count", len(ids))
	return ids, nil
}

// Validate checks the Proofs for given IDs.
func (d *LocalDA) Validate(ctx context.Context, ids []coreda.ID, proofs []coreda.Proof, _ []byte) ([]bool, error) {
	d.logger.Debug("Validate called", "numIDs", len(ids), "numProofs", len(proofs))
	if len(ids) != len(proofs) {
		d.logger.Error("Validate: id/proof count mismatch", "ids", len(ids), "proofs", len(proofs))
		return nil, errors.New("number of IDs doesn't equal to number of proofs")
	}
	results := make([]bool, len(ids))
	for i := 0; i < len(ids); i++ {
		results[i] = ed25519.Verify(d.pubKey, ids[i][8:], proofs[i])
		d.logger.Debug("Validate result", "id", ids[i], "result", results[i])
	}
	d.logger.Debug("Validate finished", "results", results)
	return results, nil
}

func (d *LocalDA) nextID() []byte {
	return d.getID(d.height)
}

func (d *LocalDA) getID(cnt uint64) []byte {
	id := make([]byte, 8)
	binary.LittleEndian.PutUint64(id, cnt)
	return id
}

func (d *LocalDA) getHash(blob []byte) []byte {
	sha := sha256.Sum256(blob)
	return sha[:]
}

func (d *LocalDA) getProof(id, blob []byte) []byte {
	sign, _ := d.privKey.Sign(rand.Reader, d.getHash(blob), &ed25519.Options{})
	return sign
}

// WithMaxBlobSize returns a function that sets the max blob size of LocalDA
func WithMaxBlobSize(maxBlobSize uint64) func(*LocalDA) *LocalDA {
	return func(da *LocalDA) *LocalDA {
		da.maxBlobSize = maxBlobSize
		return da
	}
}
