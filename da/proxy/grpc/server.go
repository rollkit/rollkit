package grpc

import (
	"context"

	"github.com/cosmos/gogoproto/types"
	"google.golang.org/grpc"

	"github.com/rollkit/rollkit/da"
	pbda "github.com/rollkit/rollkit/types/pb/rollkit"
)

// NewServer creates new gRPC Server configured to serve DA proxy.
func NewServer(d da.DA, opts ...grpc.ServerOption) *grpc.Server {
	srv := grpc.NewServer(opts...)

	proxy := &proxySrv{target: d}

	pbda.RegisterDAServiceServer(srv, proxy)

	return srv
}

type proxySrv struct {
	target da.DA
}

func (p *proxySrv) MaxBlobSize(
	ctx context.Context, request *pbda.MaxBlobSizeRequest,
) (*pbda.MaxBlobSizeResponse, error) {
	maxBlobSize, err := p.target.MaxBlobSize(ctx)
	return &pbda.MaxBlobSizeResponse{MaxBlobSize: maxBlobSize}, err
}

func (p *proxySrv) Get(ctx context.Context, request *pbda.GetRequest) (*pbda.GetResponse, error) {
	ids := idsPB2DA(request.Ids)
	blobs, err := p.target.Get(ctx, ids, request.Namespace.GetValue())
	return &pbda.GetResponse{Blobs: blobsDA2PB(blobs)}, err
}

func (p *proxySrv) GetIds(ctx context.Context, request *pbda.GetIdsRequest) (*pbda.GetIdsResponse, error) {
	ret, err := p.target.GetIDs(ctx, request.Height, request.Namespace.GetValue())
	if err != nil {
		return nil, err
	}

	// for ErrBlobNotFound cases
	if ret == nil {
		return &pbda.GetIdsResponse{Ids: []*pbda.ID{}, Timestamp: nil}, nil
	}

	timestamp, err := types.TimestampProto(ret.Timestamp)
	if err != nil {
		return nil, err
	}
	return &pbda.GetIdsResponse{Ids: idsDA2PB(ret.IDs), Timestamp: timestamp}, nil
}

func (p *proxySrv) Commit(ctx context.Context, request *pbda.CommitRequest) (*pbda.CommitResponse, error) {
	blobs := blobsPB2DA(request.Blobs)
	commits, err := p.target.Commit(ctx, blobs, request.Namespace.GetValue())
	if err != nil {
		return nil, err
	}

	return &pbda.CommitResponse{Commitments: commitsDA2PB(commits)}, nil
}

func (p *proxySrv) GetProofs(ctx context.Context, request *pbda.GetProofsRequest) (*pbda.GetProofsResponse, error) {
	ids := idsPB2DA(request.Ids)
	proofs, err := p.target.GetProofs(ctx, ids, request.Namespace.GetValue())
	if err != nil {
		return nil, err
	}

	return &pbda.GetProofsResponse{Proofs: proofsDA2PB(proofs)}, nil
}

func (p *proxySrv) Submit(ctx context.Context, request *pbda.SubmitRequest) (*pbda.SubmitResponse, error) {
	blobs := blobsPB2DA(request.Blobs)

	ids, err := p.target.SubmitWithOptions(ctx, blobs, request.GasPrice, request.Namespace.GetValue(), request.Options)
	if err != nil {
		return nil, err
	}

	resp := &pbda.SubmitResponse{
		Ids: make([]*pbda.ID, len(ids)),
	}

	for i := range ids {
		resp.Ids[i] = &pbda.ID{Value: ids[i]}
	}

	return resp, nil
}

func (p *proxySrv) Validate(ctx context.Context, request *pbda.ValidateRequest) (*pbda.ValidateResponse, error) {
	ids := idsPB2DA(request.Ids)
	proofs := proofsPB2DA(request.Proofs)
	//TODO implement me
	validity, err := p.target.Validate(ctx, ids, proofs, request.Namespace.GetValue())
	if err != nil {
		return nil, err
	}
	return &pbda.ValidateResponse{Results: validity}, nil
}
