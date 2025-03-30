package grpc

import (
	"context"

	"github.com/cosmos/gogoproto/types"
	"google.golang.org/grpc"

	"github.com/rollkit/go-da"
	pbda "github.com/rollkit/go-da/types/pb/da"
)

// Client is a gRPC proxy client for DA interface.
type Client struct {
	conn *grpc.ClientConn

	client pbda.DAServiceClient
}

// NewClient returns new Client instance.
func NewClient() *Client {
	return &Client{}
}

// Start connects Client to target, with given options.
func (c *Client) Start(target string, opts ...grpc.DialOption) (err error) {
	c.conn, err = grpc.NewClient(target, opts...)
	if err != nil {
		return err
	}
	c.client = pbda.NewDAServiceClient(c.conn)

	return nil
}

// Stop gently closes Client connection.
func (c *Client) Stop() error {
	return c.conn.Close()
}

// MaxBlobSize returns the DA MaxBlobSize
func (c *Client) MaxBlobSize(ctx context.Context) (uint64, error) {
	req := &pbda.MaxBlobSizeRequest{}
	resp, err := c.client.MaxBlobSize(ctx, req)
	if err != nil {
		return 0, tryToMapError(err)
	}
	return resp.MaxBlobSize, nil
}

// Get returns Blob for each given ID, or an error.
func (c *Client) Get(ctx context.Context, ids []da.ID, namespace da.Namespace) ([]da.Blob, error) {
	req := &pbda.GetRequest{
		Ids:       make([]*pbda.ID, len(ids)),
		Namespace: &pbda.Namespace{Value: namespace},
	}
	for i := range ids {
		req.Ids[i] = &pbda.ID{Value: ids[i]}
	}
	resp, err := c.client.Get(ctx, req)
	if err != nil {
		return nil, tryToMapError(err)
	}

	return blobsPB2DA(resp.Blobs), nil
}

// GetIDs returns IDs of all Blobs located in DA at given height.
func (c *Client) GetIDs(ctx context.Context, height uint64, namespace da.Namespace) (*da.GetIDsResult, error) {
	req := &pbda.GetIdsRequest{Height: height, Namespace: &pbda.Namespace{Value: namespace}}
	resp, err := c.client.GetIds(ctx, req)
	if err != nil {
		return nil, tryToMapError(err)
	}

	timestamp, err := types.TimestampFromProto(resp.Timestamp)
	if err != nil {
		return nil, err
	}
	return &da.GetIDsResult{IDs: idsPB2DA(resp.Ids), Timestamp: timestamp}, nil
}

// GetProofs returns inclusion Proofs for all Blobs located in DA at given height.
func (c *Client) GetProofs(ctx context.Context, ids []da.ID, namespace da.Namespace) ([]da.Proof, error) {
	req := &pbda.GetProofsRequest{Ids: make([]*pbda.ID, len(ids)), Namespace: &pbda.Namespace{Value: namespace}}
	for i := range ids {
		req.Ids[i] = &pbda.ID{Value: ids[i]}
	}
	resp, err := c.client.GetProofs(ctx, req)
	if err != nil {
		return nil, err
	}

	return proofsPB2DA(resp.Proofs), nil
}

// Commit creates a Commitment for each given Blob.
func (c *Client) Commit(ctx context.Context, blobs []da.Blob, namespace da.Namespace) ([]da.Commitment, error) {
	req := &pbda.CommitRequest{
		Blobs:     blobsDA2PB(blobs),
		Namespace: &pbda.Namespace{Value: namespace},
	}

	resp, err := c.client.Commit(ctx, req)
	if err != nil {
		return nil, tryToMapError(err)
	}

	return commitsPB2DA(resp.Commitments), nil
}

// Submit submits the Blobs to Data Availability layer.
func (c *Client) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64, namespace da.Namespace) ([]da.ID, error) {
	req := &pbda.SubmitRequest{
		Blobs:     blobsDA2PB(blobs),
		GasPrice:  gasPrice,
		Namespace: &pbda.Namespace{Value: namespace},
	}

	resp, err := c.client.Submit(ctx, req)
	if err != nil {
		return nil, tryToMapError(err)
	}

	ids := make([]da.ID, len(resp.Ids))
	for i := range resp.Ids {
		ids[i] = resp.Ids[i].Value
	}

	return ids, nil
}

// SubmitWithOptions submits the Blobs to Data Availability layer.
func (c *Client) SubmitWithOptions(ctx context.Context, blobs []da.Blob, gasPrice float64, namespace da.Namespace, options []byte) ([]da.ID, error) {
	req := &pbda.SubmitRequest{
		Blobs:     blobsDA2PB(blobs),
		GasPrice:  gasPrice,
		Namespace: &pbda.Namespace{Value: namespace},
		Options:   options,
	}

	resp, err := c.client.Submit(ctx, req)
	if err != nil {
		return nil, tryToMapError(err)
	}

	ids := make([]da.ID, len(resp.Ids))
	for i := range resp.Ids {
		ids[i] = resp.Ids[i].Value
	}

	return ids, nil
}

// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
func (c *Client) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof, namespace da.Namespace) ([]bool, error) {
	req := &pbda.ValidateRequest{
		Ids:       idsDA2PB(ids),
		Proofs:    proofsDA2PB(proofs),
		Namespace: &pbda.Namespace{Value: namespace},
	}
	resp, err := c.client.Validate(ctx, req)
	return resp.Results, tryToMapError(err)
}
