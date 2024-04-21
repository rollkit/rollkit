package celestia

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"strings"

	"github.com/celestiaorg/nmt"

	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/appconsts"
	"github.com/rollkit/celestia-openrpc/types/blob"
	"github.com/rollkit/celestia-openrpc/types/share"
	"github.com/rollkit/go-da"
)

// RandomNamespace generates a random namespace.
func RandomNamespace() share.Namespace {
	bytes := make([]byte, 4) // 4 bytes to produce 8 hex characters
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	ns, err := share.NewBlobNamespaceV0(bytes)
	if err != nil {
		panic(err)
	}
	return ns
}

// CelestiaDA is a DA implementation that uses the Celestia network for data availability.
type CelestiaDA struct {
	logger    *slog.Logger
	context   context.Context
	client    *openrpc.Client
	namespace share.Namespace
}

// NewCelestiaDA creates a new CelestiaDA.
func NewCelestiaDA(ctx context.Context, client *openrpc.Client) *CelestiaDA {
	return &CelestiaDA{
		context:   ctx,
		client:    client,
		namespace: RandomNamespace(),
		logger:    slog.Default(),
	}
}

var _ da.DA = &CelestiaDA{}

// Commit implements da.DA.
func (c *CelestiaDA) Commit(blobs [][]byte) ([][]byte, error) {
	c.logger.Info("CelestiaDA Commit", "blobs len", len(blobs))
	_, commitments, err := c.blobsAndCommitments(blobs)
	return commitments, err
}

// Get implements da.DA.
func (c *CelestiaDA) Get(ids [][]byte) ([][]byte, error) {
	c.logger.Info("CelestiaDA Get", "ids len", len(ids))
	var blobs []da.Blob
	for _, id := range ids {
		height, commitment := splitID(id)
		blob, err := c.client.Blob.Get(c.context, height, c.namespace, commitment)
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, blob.Data)
	}
	return blobs, nil
}

// GetIDs implements da.DA.
func (c *CelestiaDA) GetIDs(height uint64) ([][]byte, error) {
	c.logger.Info("CelestiaDA GetIDs", "height", height)
	var ids []da.ID
	blobs, err := c.client.Blob.GetAll(c.context, height, []share.Namespace{c.namespace})
	if err != nil {
		if strings.Contains(err.Error(), blob.ErrBlobNotFound.Error()) {
			return nil, nil
		}
		return nil, err
	}
	for _, b := range blobs {
		ids = append(ids, makeID(height, b.Commitment))
	}
	return ids, nil
}

// MaxBlobSize implements da.DA.
func (c *CelestiaDA) MaxBlobSize() (uint64, error) {
	c.logger.Info("CelestiaDA MaxBlobSize", "result", appconsts.DefaultMaxBytes)
	return appconsts.DefaultMaxBytes, nil
}

// Submit implements da.DA.
func (c *CelestiaDA) Submit(daBlobs [][]byte) ([][]byte, [][]byte, error) {
	c.logger.Info("CelestiaDA Submit", "daBlobs len", len(daBlobs))
	blobs, commitments, err := c.blobsAndCommitments(daBlobs)
	if err != nil {
		return nil, nil, err
	}
	height, err := c.client.Blob.Submit(c.context, blobs, openrpc.DefaultSubmitOptions())
	if err != nil {
		return nil, nil, err
	}
	ids := make([]da.ID, len(daBlobs))
	proofs := make([]da.Proof, len(daBlobs))
	for i, commitment := range commitments {
		ids[i] = makeID(height, commitment)
		proof, err := c.client.Blob.GetProof(c.context, height, c.namespace, commitment)
		if err != nil {
			return nil, nil, err
		}
		proofs[i], err = (*proof)[0].MarshalJSON()
		if err != nil {
			return nil, nil, err
		}
	}
	return ids, proofs, nil
}

// Validate implements da.DA.
func (c *CelestiaDA) Validate(ids [][]byte, proofs [][]byte) ([]bool, error) {
	c.logger.Info("CelestiaDA Validate", "ids len", len(ids))
	var included []bool
	var bProofs []*blob.Proof
	for _, p := range proofs {
		nmtProof := &nmt.Proof{}
		if err := nmtProof.UnmarshalJSON(p); err != nil {
			return nil, err
		}
		proof := &blob.Proof{nmtProof}
		bProofs = append(bProofs, proof)
	}
	for i, id := range ids {
		height, commitment := splitID(id)
		isIncluded, _ := c.client.Blob.Included(c.context, height, c.namespace, bProofs[i], commitment)
		included = append(included, isIncluded)
	}
	return included, nil
}

func (c *CelestiaDA) blobsAndCommitments(daBlobs []da.Blob) ([]*blob.Blob, []da.Commitment, error) {
	var blobs []*blob.Blob
	var commitments []da.Commitment
	for _, daBlob := range daBlobs {
		b, err := blob.NewBlobV0(c.namespace, daBlob)
		if err != nil {
			return nil, nil, err
		}
		blobs = append(blobs, b)

		commitment, err := blob.CreateCommitment(b)
		if err != nil {
			return nil, nil, err
		}
		commitments = append(commitments, commitment)
	}
	return blobs, commitments, nil
}

// heightLen is a length (in bytes) of serialized height.
//
// This is 8 as uint64 consist of 8 bytes.
const heightLen = 8

func makeID(height uint64, commitment da.Commitment) da.ID {
	id := make([]byte, heightLen+len(commitment))
	binary.LittleEndian.PutUint64(id, height)
	copy(id[heightLen:], commitment)
	return id
}

func splitID(id da.ID) (uint64, da.Commitment) {
	if len(id) <= heightLen {
		return 0, nil
	}
	return binary.LittleEndian.Uint64(id[:heightLen]), id[heightLen:]
}
