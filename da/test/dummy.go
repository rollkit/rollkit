package test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/rollkit/go-da"
)

// DefaultMaxBlobSize is the default max blob size
const DefaultMaxBlobSize = 64 * 64 * 482

// DummyDA is a simple implementation of in-memory DA. Not production ready! Intended only for testing!
//
// Data is stored in a map, where key is a serialized sequence number. This key is returned as ID.
// Commitments are simply hashes, and proofs are ED25519 signatures.
type DummyDA struct {
	mu          *sync.Mutex // protects data and height
	data        map[uint64][]kvp
	timestamps  map[uint64]time.Time
	maxBlobSize uint64
	height      uint64
	privKey     ed25519.PrivateKey
	pubKey      ed25519.PublicKey
}

type kvp struct {
	key, value []byte
}

// NewDummyDA create new instance of DummyDA
func NewDummyDA(opts ...func(*DummyDA) *DummyDA) *DummyDA {
	da := &DummyDA{
		mu:          new(sync.Mutex),
		data:        make(map[uint64][]kvp),
		timestamps:  make(map[uint64]time.Time),
		maxBlobSize: DefaultMaxBlobSize,
	}
	for _, f := range opts {
		da = f(da)
	}
	da.pubKey, da.privKey, _ = ed25519.GenerateKey(rand.Reader)
	return da
}

var _ da.DA = &DummyDA{}

// MaxBlobSize returns the max blob size in bytes.
func (d *DummyDA) MaxBlobSize(ctx context.Context) (uint64, error) {
	return d.maxBlobSize, nil
}

// Get returns Blobs for given IDs.
func (d *DummyDA) Get(ctx context.Context, ids []da.ID, _ da.Namespace) ([]da.Blob, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	blobs := make([]da.Blob, len(ids))
	for i, id := range ids {
		if len(id) < 8 {
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
			return nil, &da.ErrBlobNotFound{}
		}
	}
	return blobs, nil
}

// GetIDs returns IDs of Blobs at given DA height.
func (d *DummyDA) GetIDs(ctx context.Context, height uint64, _ da.Namespace) (*da.GetIDsResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if height > d.height {
		return nil, &da.ErrFutureHeight{}
	}

	kvps, ok := d.data[height]
	if !ok {
		return nil, nil
	}

	ids := make([]da.ID, len(kvps))
	for i, kv := range kvps {
		ids[i] = kv.key
	}
	return &da.GetIDsResult{IDs: ids, Timestamp: d.timestamps[height]}, nil
}

// GetProofs returns inclusion Proofs for all Blobs located in DA at given height.
func (d *DummyDA) GetProofs(ctx context.Context, ids []da.ID, _ da.Namespace) ([]da.Proof, error) {
	blobs, err := d.Get(ctx, ids, nil)

	d.mu.Lock()
	defer d.mu.Unlock()
	if err != nil {
		return nil, err
	}
	proofs := make([]da.Proof, len(blobs))
	for i, blob := range blobs {
		proofs[i] = d.getProof(ids[i], blob)
	}
	return proofs, nil
}

// Commit returns cryptographic Commitments for given blobs.
func (d *DummyDA) Commit(ctx context.Context, blobs []da.Blob, _ da.Namespace) ([]da.Commitment, error) {
	commits := make([]da.Commitment, len(blobs))
	for i, blob := range blobs {
		commits[i] = d.getHash(blob)
	}
	return commits, nil
}

// Submit stores blobs in DA layer.
func (d *DummyDA) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, ns da.Namespace) ([]da.ID, error) {
	return d.SubmitWithOptions(ctx, blobs, gasPrice, ns, nil)
}

// SubmitWithOptions stores blobs in DA layer (options are ignored).
func (d *DummyDA) SubmitWithOptions(ctx context.Context, blobs []da.Blob, gasPrice float64, _ da.Namespace, _ []byte) ([]da.ID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]da.ID, len(blobs))
	d.height += 1
	d.timestamps[d.height] = time.Now()
	for i, blob := range blobs {
		ids[i] = append(d.nextID(), d.getHash(blob)...)

		d.data[d.height] = append(d.data[d.height], kvp{ids[i], blob})
	}

	return ids, nil
}

// Validate checks the Proofs for given IDs.
func (d *DummyDA) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, _ da.Namespace) ([]bool, error) {
	if len(ids) != len(proofs) {
		return nil, errors.New("number of IDs doesn't equal to number of proofs")
	}
	results := make([]bool, len(ids))
	for i := 0; i < len(ids); i++ {
		results[i] = ed25519.Verify(d.pubKey, ids[i][8:], proofs[i])
	}
	return results, nil
}

func (d *DummyDA) nextID() []byte {
	return d.getID(d.height)
}

func (d *DummyDA) getID(cnt uint64) []byte {
	id := make([]byte, 8)
	binary.LittleEndian.PutUint64(id, cnt)
	return id
}

func (d *DummyDA) getHash(blob []byte) []byte {
	sha := sha256.Sum256(blob)
	return sha[:]
}

func (d *DummyDA) getProof(id, blob []byte) []byte {
	sign, _ := d.privKey.Sign(rand.Reader, d.getHash(blob), &ed25519.Options{})
	return sign
}
