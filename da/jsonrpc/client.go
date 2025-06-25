package jsonrpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"cosmossdk.io/log"
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/rollkit/rollkit/core/da"
	internal "github.com/rollkit/rollkit/da/jsonrpc/internal"
)

//go:generate mockgen -destination=mocks/api.go -package=mocks . Module
type Module interface {
	da.DA
}

// API defines the jsonrpc service module API
type API struct {
	Logger      log.Logger
	Namespace   []byte
	MaxBlobSize uint64
	Internal    struct {
		Get           func(ctx context.Context, ids []da.ID, ns []byte) ([]da.Blob, error)           `perm:"read"`
		GetIDs        func(ctx context.Context, height uint64, ns []byte) (*da.GetIDsResult, error)  `perm:"read"`
		GetProofs     func(ctx context.Context, ids []da.ID, ns []byte) ([]da.Proof, error)          `perm:"read"`
		Commit        func(ctx context.Context, blobs []da.Blob, ns []byte) ([]da.Commitment, error) `perm:"read"`
		Validate      func(context.Context, []da.ID, []da.Proof, []byte) ([]bool, error)             `perm:"read"`
		Submit        func(context.Context, []da.Blob, []byte, []byte) ([]da.ID, error)              `perm:"write"`
		GasMultiplier func(context.Context) (float64, error)                                         `perm:"read"`
		GasPrice      func(context.Context) (float64, error)                                         `perm:"read"`
	}
}

// Get returns Blob for each given ID, or an error.
func (api *API) Get(ctx context.Context, ids []da.ID) ([]da.Blob, error) {
	api.Logger.Debug("Making RPC call", "method", "Get", "num_ids", len(ids), "namespace", string(api.Namespace))
	res, err := api.Internal.Get(ctx, ids, api.Namespace)
	if err != nil {
		if strings.Contains(err.Error(), context.Canceled.Error()) {
			api.Logger.Debug("RPC call canceled due to context cancellation", "method", "Get")
			return res, context.Canceled
		}
		api.Logger.Error("RPC call failed", "method", "Get", "error", err)
		// Wrap error for context, potentially using the translated error from the RPC library
		return nil, fmt.Errorf("failed to get blobs: %w", err)
	}
	api.Logger.Debug("RPC call successful", "method", "Get", "num_blobs_returned", len(res))
	return res, nil
}

// GetIDs returns IDs of all Blobs located in DA at given height.
func (api *API) GetIDs(ctx context.Context, height uint64) (*da.GetIDsResult, error) {
	api.Logger.Debug("Making RPC call", "method", "GetIDs", "height", height, "namespace", string(api.Namespace))
	res, err := api.Internal.GetIDs(ctx, height, api.Namespace)
	if err != nil {
		// Using strings.contains since JSON RPC serialization doesn't preserve error wrapping
		// Check if the error is specifically BlobNotFound, otherwise log and return
		if strings.Contains(err.Error(), da.ErrBlobNotFound.Error()) { // Use the error variable directly
			api.Logger.Debug("RPC call indicates blobs not found", "method", "GetIDs", "height", height)
			return nil, err // Return the specific ErrBlobNotFound
		}
		if strings.Contains(err.Error(), da.ErrHeightFromFuture.Error()) {
			api.Logger.Debug("RPC call indicates height from future", "method", "GetIDs", "height", height)
			return nil, err // Return the specific ErrHeightFromFuture
		}
		if strings.Contains(err.Error(), context.Canceled.Error()) {
			api.Logger.Debug("RPC call canceled due to context cancellation", "method", "GetIDs")
			return res, context.Canceled
		}
		api.Logger.Error("RPC call failed", "method", "GetIDs", "error", err)
		return nil, err
	}

	// Handle cases where the RPC call succeeds but returns no IDs
	if res == nil || len(res.IDs) == 0 {
		api.Logger.Debug("RPC call successful but no IDs found", "method", "GetIDs", "height", height)
		return nil, da.ErrBlobNotFound // Return specific error for not found (use variable directly)
	}

	api.Logger.Debug("RPC call successful", "method", "GetIDs")
	return res, nil
}

// GetProofs returns inclusion Proofs for Blobs specified by their IDs.
func (api *API) GetProofs(ctx context.Context, ids []da.ID) ([]da.Proof, error) {
	api.Logger.Debug("Making RPC call", "method", "GetProofs", "num_ids", len(ids), "namespace", string(api.Namespace))
	res, err := api.Internal.GetProofs(ctx, ids, api.Namespace)
	if err != nil {
		api.Logger.Error("RPC call failed", "method", "GetProofs", "error", err)
	} else {
		api.Logger.Debug("RPC call successful", "method", "GetProofs", "num_proofs_returned", len(res))
	}
	return res, err
}

// Commit creates a Commitment for each given Blob.
func (api *API) Commit(ctx context.Context, blobs []da.Blob) ([]da.Commitment, error) {
	api.Logger.Debug("Making RPC call", "method", "Commit", "num_blobs", len(blobs), "namespace", string(api.Namespace))
	res, err := api.Internal.Commit(ctx, blobs, api.Namespace)
	if err != nil {
		api.Logger.Error("RPC call failed", "method", "Commit", "error", err)
	} else {
		api.Logger.Debug("RPC call successful", "method", "Commit", "num_commitments_returned", len(res))
	}
	return res, err
}

// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
func (api *API) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof) ([]bool, error) {
	api.Logger.Debug("Making RPC call", "method", "Validate", "num_ids", len(ids), "num_proofs", len(proofs), "namespace", string(api.Namespace))
	res, err := api.Internal.Validate(ctx, ids, proofs, api.Namespace)
	if err != nil {
		api.Logger.Error("RPC call failed", "method", "Validate", "error", err)
	} else {
		api.Logger.Debug("RPC call successful", "method", "Validate", "num_results_returned", len(res))
	}
	return res, err
}

// Submit submits the Blobs to Data Availability layer with additional options.
// It checks blobs against MaxBlobSize and submits only those that fit.
func (api *API) Submit(ctx context.Context, inputBlobs []da.Blob, options []byte) ([]da.ID, error) {
	maxBlobSize := api.MaxBlobSize

	var (
		blobsToSubmit []da.Blob = make([]da.Blob, 0, len(inputBlobs))
		currentSize   uint64
		oversizeBlobs int
	)

	for i, blob := range inputBlobs {
		blobLen := uint64(len(blob))
		if blobLen > maxBlobSize {
			api.Logger.Warn("Individual blob exceeds MaxBlobSize, cannot submit", "index", i, "blobSize", blobLen, "maxBlobSize", maxBlobSize)
			oversizeBlobs++
			continue
		}
		if currentSize+blobLen > maxBlobSize {
			api.Logger.Info("Blob size limit reached for batch", "maxBlobSize", maxBlobSize, "index", i, "currentSize", currentSize, "nextBlobSize", blobLen)
			break
		}
		currentSize += blobLen
		blobsToSubmit = append(blobsToSubmit, blob)
	}

	if oversizeBlobs > 0 {
		api.Logger.Error("Blobs exceeded size limit", "oversize_count", oversizeBlobs, "total_blobs", len(inputBlobs))
		return nil, da.ErrBlobSizeOverLimit
	}

	if len(blobsToSubmit) == 0 {
		api.Logger.Info("No blobs to submit after filtering by size")
		if len(inputBlobs) > 0 {
			return nil, da.ErrBlobSizeOverLimit
		}
		return []da.ID{}, nil
	}

	api.Logger.Debug("Making RPC call", "method", "Submit", "num_blobs_original", len(inputBlobs), "num_blobs_to_submit", len(blobsToSubmit), "namespace", string(api.Namespace))
	res, err := api.Internal.Submit(ctx, blobsToSubmit, api.Namespace, options)
	if err != nil {
		if strings.Contains(err.Error(), context.Canceled.Error()) {
			api.Logger.Debug("RPC call canceled due to context cancellation", "method", "Submit")
			return res, context.Canceled
		}
		api.Logger.Error("RPC call failed", "method", "Submit", "error", err)
	} else {
		api.Logger.Debug("RPC call successful", "method", "Submit", "num_ids_returned", len(res))
	}

	return res, err
}

// Client is the jsonrpc client
type Client struct {
	DA     API
	closer multiClientCloser
}

// multiClientCloser is a wrapper struct to close clients across multiple namespaces.
type multiClientCloser struct {
	closers []jsonrpc.ClientCloser
}

// register adds a new closer to the multiClientCloser
func (m *multiClientCloser) register(closer jsonrpc.ClientCloser) {
	m.closers = append(m.closers, closer)
}

// closeAll closes all saved clients.
func (m *multiClientCloser) closeAll() {
	for _, closer := range m.closers {
		closer()
	}
}

// Close closes the connections to all namespaces registered on the staticClient.
func (c *Client) Close() {
	c.closer.closeAll()
}

// NewClient creates a new Client with one connection per namespace with the
// given token as the authorization token.
func NewClient(ctx context.Context, logger log.Logger, addr string, token, ns string) (*Client, error) {
	authHeader := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
	return newClient(ctx, logger, addr, authHeader, ns)
}

func newClient(ctx context.Context, logger log.Logger, addr string, authHeader http.Header, namespace string) (*Client, error) {
	var multiCloser multiClientCloser
	var client Client
	client.DA.Logger = logger
	client.DA.MaxBlobSize = internal.DefaultMaxBytes
	namespaceBytes, err := hex.DecodeString(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to decode namespace: %w", err)
	}
	client.DA.Namespace = namespaceBytes
	logger.Info("creating new client", "namespace", namespace)
	errs := getKnownErrorsMapping()
	for name, module := range moduleMap(&client) {
		closer, err := jsonrpc.NewMergeClient(ctx, addr, name, []interface{}{module}, authHeader, jsonrpc.WithErrors(errs))
		if err != nil {
			// If an error occurs, close any previously opened connections
			multiCloser.closeAll()
			return nil, err
		}
		multiCloser.register(closer)
	}

	client.closer = multiCloser // Assign the multiCloser to the client

	return &client, nil
}

func moduleMap(client *Client) map[string]interface{} {
	// TODO: this duplication of strings many times across the codebase can be avoided with issue #1176
	return map[string]interface{}{
		"da": &client.DA.Internal,
	}
}
