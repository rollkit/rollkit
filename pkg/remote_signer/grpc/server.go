package grpc

import (
	"context"

	"github.com/rollkit/rollkit/pkg/remote_signer"
	v1 "github.com/rollkit/rollkit/pkg/remote_signer/v1"
)

// RemoteSigner implements the SignerService generated interface from the .proto
type RemoteSignerServer struct {
	signer                              remote_signer.Signer
	v1.UnimplementedSignerServiceServer // Embedded for forward compatibility
}

func NewRemoteSignerServer(s remote_signer.Signer) *RemoteSignerServer {
	return &RemoteSignerServer{signer: s}
}

func (r *RemoteSignerServer) Sign(ctx context.Context, req *v1.SignRequest) (*v1.SignResponse, error) {
	sig, err := r.signer.Sign(req.GetMessage())
	if err != nil {
		return nil, err
	}
	return &v1.SignResponse{
		Signature: sig,
	}, nil
}

func (r *RemoteSignerServer) GetPublic(ctx context.Context, _ v1.GetPublicRequest) (*v1.GetPublicResponse, error) {
	pub, err := r.signer.GetPublic()
	if err != nil {
		return nil, err
	}
	return &v1.GetPublicResponse{
		PublicKey: pub,
	}, nil
}
