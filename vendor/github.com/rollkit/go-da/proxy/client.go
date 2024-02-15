package proxy

import (
	"context"

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
	c.conn, err = grpc.Dial(target, opts...)
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
		return 0, err
	}
	return resp.MaxBlobSize, nil
}

// Get returns Blob for each given ID, or an error.
func (c *Client) Get(ctx context.Context, ids []da.ID) ([]da.Blob, error) {
	req := &pbda.GetRequest{
		Ids: make([]*pbda.ID, len(ids)),
	}
	for i := range ids {
		req.Ids[i] = &pbda.ID{Value: ids[i]}
	}
	resp, err := c.client.Get(ctx, req)
	if err != nil {
		return nil, err
	}

	return blobsPB2DA(resp.Blobs), nil
}

// GetIDs returns IDs of all Blobs located in DA at given height.
func (c *Client) GetIDs(ctx context.Context, height uint64) ([]da.ID, error) {
	req := &pbda.GetIDsRequest{Height: height}
	resp, err := c.client.GetIDs(ctx, req)
	if err != nil {
		return nil, err
	}

	return idsPB2DA(resp.Ids), nil
}

// Commit creates a Commitment for each given Blob.
func (c *Client) Commit(ctx context.Context, blobs []da.Blob) ([]da.Commitment, error) {
	req := &pbda.CommitRequest{
		Blobs: blobsDA2PB(blobs),
	}

	resp, err := c.client.Commit(ctx, req)
	if err != nil {
		return nil, err
	}

	return commitsPB2DA(resp.Commitments), nil
}

// Submit submits the Blobs to Data Availability layer.
func (c *Client) Submit(ctx context.Context, blobs []da.Blob, gasPrice float64) ([]da.ID, []da.Proof, error) {
	req := &pbda.SubmitRequest{
		Blobs:    blobsDA2PB(blobs),
		GasPrice: gasPrice,
	}

	resp, err := c.client.Submit(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	ids := make([]da.ID, len(resp.Ids))
	proofs := make([]da.Proof, len(resp.Proofs))
	for i := range resp.Ids {
		ids[i] = resp.Ids[i].Value
		proofs[i] = resp.Proofs[i].Value
	}

	return ids, proofs, nil
}

// Validate validates Commitments against the corresponding Proofs. This should be possible without retrieving the Blobs.
func (c *Client) Validate(ctx context.Context, ids []da.ID, proofs []da.Proof) ([]bool, error) {
	req := &pbda.ValidateRequest{
		Ids:    idsDA2PB(ids),
		Proofs: proofsDA2PB(proofs),
	}
	resp, err := c.client.Validate(ctx, req)
	return resp.Results, err
}
