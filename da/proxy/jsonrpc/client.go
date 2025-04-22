package jsonrpc

import (
	"context"
	"fmt"
	"net/http"

	"cosmossdk.io/log"
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/rollkit/rollkit/core/da"
)

// Define a package-level logger for the client

// Module wraps the DA interface
//
//go:generate mockgen -destination=mocks/api.go -package=mocks . Module
type Module interface {
	da.DA
}

// API defines the jsonrpc service module API
type API struct {
	logger   log.Logger
	Internal struct {
		MaxBlobSize       func(ctx context.Context) (uint64, error)                                      `perm:"read"`
		Get               func(ctx context.Context, ids []da.ID, ns []byte) ([]da.Blob, error)           `perm:"read"`
		GetIDs            func(ctx context.Context, height uint64, ns []byte) (*da.GetIDsResult, error)  `perm:"read"`
		GetProofs         func(ctx context.Context, ids []da.ID, ns []byte) ([]da.Proof, error)          `perm:"read"`
		Commit            func(ctx context.Context, blobs []da.Blob, ns []byte) ([]da.Commitment, error) `perm:"read"`
		Validate          func(context.Context, []da.ID, []da.Proof, []byte) ([]bool, error)             `perm:"read"`
		Submit            func(context.Context, []da.Blob, float64, []byte) ([]da.ID, error)             `perm:"write"`
		SubmitWithOptions func(context.Context, []da.Blob, float64, []byte, []byte) ([]da.ID, error)     `perm:"write"`
		GasMultiplier     func(context.Context) (float64, error)                                         `perm:"read"`
		GasPrice          func(context.Context) (float64, error)                                         `perm:"read"`
	}
}

// MaxBlobSize returns the max blob size
func (api *API) MaxBlobSize(ctx context.Context) (uint64, error) {
	res, err := api.Internal.MaxBlobSize(ctx)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "MaxBlobSize", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "MaxBlobSize", "result", res)
	}
	return res, err
}

// Get returns Blob for each given ID, or an error.
func (api *API) Get(ctx context.Context, ids []da.ID, ns []byte) ([]da.Blob, error) {
	api.logger.Debug("Making RPC call", "method", "Get", "num_ids", len(ids), "namespace", string(ns))
	res, err := api.Internal.Get(ctx, ids, ns)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "Get", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "Get", "num_blobs_returned", len(res))
	}
	return res, err
}

// GetIDs returns IDs of all Blobs located in DA at given height.
func (api *API) GetIDs(ctx context.Context, height uint64, ns []byte) (*da.GetIDsResult, error) {
	api.logger.Debug("Making RPC call", "method", "GetIDs", "height", height, "namespace", string(ns))
	res, err := api.Internal.GetIDs(ctx, height, ns)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "GetIDs", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "GetIDs")
	}
	return res, err
}

// GetProofs returns inclusion Proofs for Blobs specified by their IDs.
func (api *API) GetProofs(ctx context.Context, ids []da.ID, ns []byte) ([]da.Proof, error) {
	api.logger.Debug("Making RPC call", "method", "GetProofs", "num_ids", len(ids), "namespace", string(ns))
	res, err := api.Internal.GetProofs(ctx, ids, ns)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "GetProofs", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "GetProofs", "num_proofs_returned", len(res))
	}
	return res, err
}

// Commit creates a Commitment for each given Blob.
func (api *API) Commit(ctx context.Context, blobs []da.Blob, ns []byte) ([]da.Commitment, error) {
	api.logger.Debug("Making RPC call", "method", "Commit", "num_blobs", len(blobs), "namespace", string(ns))
	res, err := api.Internal.Commit(ctx, blobs, ns)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "Commit", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "Commit", "num_commitments_returned", len(res))
	}
	return res, err
}

// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
func (api *API) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, ns []byte) ([]bool, error) {
	api.logger.Debug("Making RPC call", "method", "Validate", "num_ids", len(ids), "num_proofs", len(proofs), "namespace", string(ns))
	res, err := api.Internal.Validate(ctx, ids, proofs, ns)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "Validate", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "Validate", "num_results_returned", len(res))
	}
	return res, err
}

// Submit submits the Blobs to Data Availability layer.
func (api *API) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, ns []byte) ([]da.ID, error) {
	api.logger.Debug("Making RPC call", "method", "Submit", "num_blobs", len(blobs), "gas_price", gasPrice, "namespace", string(ns))
	res, err := api.Internal.Submit(ctx, blobs, gasPrice, ns)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "Submit", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "Submit", "num_ids_returned", len(res))
	}
	return res, err
}

// SubmitWithOptions submits the Blobs to Data Availability layer with additional options.
func (api *API) SubmitWithOptions(ctx context.Context, blobs []da.Blob, gasPrice float64, ns []byte, options []byte) ([]da.ID, error) {
	api.logger.Debug("Making RPC call", "method", "SubmitWithOptions", "num_blobs", len(blobs), "gas_price", gasPrice, "namespace", string(ns))
	res, err := api.Internal.SubmitWithOptions(ctx, blobs, gasPrice, ns, options)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "SubmitWithOptions", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "SubmitWithOptions", "num_ids_returned", len(res))
	}
	return res, err
}

func (api *API) GasMultiplier(ctx context.Context) (float64, error) {
	api.logger.Debug("Making RPC call", "method", "GasMultiplier")
	res, err := api.Internal.GasMultiplier(ctx)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "GasMultiplier", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "GasMultiplier", "result", res)
	}
	return res, err
}

func (api *API) GasPrice(ctx context.Context) (float64, error) {
	api.logger.Debug("Making RPC call", "method", "GasPrice")
	res, err := api.Internal.GasPrice(ctx)
	if err != nil {
		api.logger.Error("RPC call failed", "method", "GasPrice", "error", err)
	} else {
		api.logger.Debug("RPC call successful", "method", "GasPrice", "result", res)
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
func NewClient(ctx context.Context, logger log.Logger, addr string, token string) (*Client, error) {
	authHeader := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", token)}}
	return newClient(ctx, logger, addr, authHeader)
}

func newClient(ctx context.Context, logger log.Logger, addr string, authHeader http.Header) (*Client, error) {
	var multiCloser multiClientCloser
	var client Client
	client.DA.logger = logger // Assign the logger here
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
