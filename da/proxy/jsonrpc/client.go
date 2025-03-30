package jsonrpc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/rollkit/go-da"
)

// Module wraps the DA interface
//
//go:generate mockgen -destination=mocks/api.go -package=mocks . Module
type Module interface {
	da.DA
}

// API defines the jsonrpc service module API
type API struct {
	Internal struct {
		MaxBlobSize       func(ctx context.Context) (uint64, error)                                            `perm:"read"`
		Get               func(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Blob, error)           `perm:"read"`
		GetIDs            func(ctx context.Context, height uint64, ns da.Namespace) (*da.GetIDsResult, error)  `perm:"read"`
		GetProofs         func(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Proof, error)          `perm:"read"`
		Commit            func(ctx context.Context, blobs []da.Blob, ns da.Namespace) ([]da.Commitment, error) `perm:"read"`
		Validate          func(context.Context, []da.ID, []da.Proof, da.Namespace) ([]bool, error)             `perm:"read"`
		Submit            func(context.Context, []da.Blob, float64, da.Namespace) ([]da.ID, error)             `perm:"write"`
		SubmitWithOptions func(context.Context, []da.Blob, float64, da.Namespace, []byte) ([]da.ID, error)     `perm:"write"`
	}
}

// MaxBlobSize returns the max blob size
func (api *API) MaxBlobSize(ctx context.Context) (uint64, error) {
	return api.Internal.MaxBlobSize(ctx)
}

// Get returns Blob for each given ID, or an error.
func (api *API) Get(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Blob, error) {
	return api.Internal.Get(ctx, ids, ns)
}

// GetIDs returns IDs of all Blobs located in DA at given height.
func (api *API) GetIDs(ctx context.Context, height uint64, ns da.Namespace) (*da.GetIDsResult, error) {
	return api.Internal.GetIDs(ctx, height, ns)
}

// GetProofs returns inclusion Proofs for Blobs specified by their IDs.
func (api *API) GetProofs(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Proof, error) {
	return api.Internal.GetProofs(ctx, ids, ns)
}

// Commit creates a Commitment for each given Blob.
func (api *API) Commit(ctx context.Context, blobs []da.Blob, ns da.Namespace) ([]da.Commitment, error) {
	return api.Internal.Commit(ctx, blobs, ns)
}

// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
func (api *API) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, ns da.Namespace) ([]bool, error) {
	return api.Internal.Validate(ctx, ids, proofs, ns)
}

// Submit submits the Blobs to Data Availability layer.
func (api *API) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, ns da.Namespace) ([]da.ID, error) {
	return api.Internal.Submit(ctx, blobs, gasPrice, ns)
}

// SubmitWithOptions submits the Blobs to Data Availability layer.
func (api *API) SubmitWithOptions(ctx context.Context, blobs []da.Blob, gasPrice float64, ns da.Namespace, options []byte) ([]da.ID, error) {
	return api.Internal.SubmitWithOptions(ctx, blobs, gasPrice, ns, options)
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
func NewClient(ctx context.Context, addr string, token string) (*Client, error) {
	authHeader := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
	return newClient(ctx, addr, authHeader)
}

func newClient(ctx context.Context, addr string, authHeader http.Header) (*Client, error) {
	var multiCloser multiClientCloser
	var client Client
	errs := getKnownErrorsMapping()
	for name, module := range moduleMap(&client) {
		closer, err := jsonrpc.NewMergeClient(ctx, addr, name, []interface{}{module}, authHeader, jsonrpc.WithErrors(errs))
		if err != nil {
			return nil, err
		}
		multiCloser.register(closer)
	}

	return &client, nil
}

func moduleMap(client *Client) map[string]interface{} {
	// TODO: this duplication of strings many times across the codebase can be avoided with issue #1176
	return map[string]interface{}{
		"da": &client.DA.Internal,
	}
}
