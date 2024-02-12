package da

import (
	"context"
	"fmt"
	"net/http"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/rollkit/go-da"
)

type API struct {
	Internal struct {
		MaxBlobSize func(ctx context.Context) (uint64, error)                                            `perm:"read"`
		Get         func(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Blob, error)           `perm:"read"`
		GetIDs      func(ctx context.Context, height uint64, ns da.Namespace) ([]da.ID, error)           `perm:"read"`
		GetProofs   func(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Proof, error)          `perm:"read"`
		Commit      func(ctx context.Context, blobs []da.Blob, ns da.Namespace) ([]da.Commitment, error) `perm:"read"`
		Validate    func(context.Context, []da.ID, []da.Proof, da.Namespace) ([]bool, error)             `perm:"read"`
		Submit      func(context.Context, []da.Blob, float64, da.Namespace) ([]da.ID, error)             `perm:"write"`
	}
}

func (api *API) MaxBlobSize(ctx context.Context) (uint64, error) {
	return api.Internal.MaxBlobSize(ctx)
}

func (api *API) Get(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Blob, error) {
	return api.Internal.Get(ctx, ids, ns)
}

func (api *API) GetIDs(ctx context.Context, height uint64, ns da.Namespace) ([]da.ID, error) {
	return api.Internal.GetIDs(ctx, height, ns)
}

func (api *API) GetProofs(ctx context.Context, ids []da.ID, ns da.Namespace) ([]da.Proof, error) {
	return api.Internal.GetProofs(ctx, ids, ns)
}

func (api *API) Commit(ctx context.Context, blobs []da.Blob, ns da.Namespace) ([]da.Commitment, error) {
	return api.Internal.Commit(ctx, blobs, ns)
}

func (api *API) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, ns da.Namespace) ([]bool, error) {
	return api.Internal.Validate(ctx, ids, proofs, ns)
}

func (api *API) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, ns da.Namespace) ([]da.ID, error) {
	return api.Internal.Submit(ctx, blobs, gasPrice, ns)
}

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
	for name, module := range moduleMap(&client) {
		closer, err := jsonrpc.NewClient(ctx, addr, name, module, authHeader)
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
