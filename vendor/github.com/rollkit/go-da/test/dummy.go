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
func (d *DummyDA) Get(ctx context.Context, ids []da.ID) ([]da.Blob, error) {
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
			return nil, errors.New("no blob for given ID")
		}
	}
	return blobs, nil
}

// GetIDs returns IDs of Blobs at given DA height.
func (d *DummyDA) GetIDs(ctx context.Context, height uint64) ([]da.ID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	kvps := d.data[height]
	ids := make([]da.ID, len(kvps))
	for i, kv := range kvps {
		ids[i] = kv.key
	}
	return ids, nil
}

// Commit returns cryptographic Commitments for given blobs.
func (d *DummyDA) Commit(ctx context.Context, blobs []da.Blob) ([]da.Commitment, error) {
	commits := make([]da.Commitment, len(blobs))
	for i, blob := range blobs {
		commits[i] = d.getHash(blob)
	}
	return commits, nil
}

// Submit stores blobs in DA layer.
func (d *DummyDA) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64) ([]da.ID, []da.Proof, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]da.ID, len(blobs))
	proofs := make([]da.Proof, len(blobs))
	d.height += 1
	for i, blob := range blobs {
		ids[i] = append(d.nextID(), d.getHash(blob)...)
		proofs[i] = d.getProof(ids[i], blob)

		d.data[d.height] = append(d.data[d.height], kvp{ids[i], blob})
	}

	return ids, proofs, nil
}

// Validate checks the Proofs for given IDs.
func (d *DummyDA) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof) ([]bool, error) {
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
