package mockserv

import (
	"context"

	ds "github.com/ipfs/go-datastore"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"

	grpcda "github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/types"
	"github.com/rollkit/rollkit/types/pb/dalc"
	"github.com/rollkit/rollkit/types/pb/rollkit"
)

// GetServer creates and returns gRPC server instance.
func GetServer(kv ds.Datastore, conf grpcda.Config, mockConfig []byte, logger tmlog.Logger) *grpc.Server {
	srv := grpc.NewServer()
	mockImpl := &mockImpl{}
	err := mockImpl.mock.Init([8]byte{}, [8]byte{}, mockConfig, kv, logger)
	if err != nil {
		logger.Error("failed to initialize mock DALC", "error", err)
		panic(err)
	}
	err = mockImpl.mock.Start()
	if err != nil {
		logger.Error("failed to start mock DALC", "error", err)
		panic(err)
	}
	dalc.RegisterDALCServiceServer(srv, mockImpl)
	return srv
}

type mockImpl struct {
	mock mock.DataAvailabilityLayerClient
}

func (m *mockImpl) SubmitBlockHeader(ctx context.Context, request *dalc.SubmitBlockHeaderRequest) (*dalc.SubmitBlockResponse, error) {
	var h types.SignedHeader
	err := h.FromProto(request.Header)
	if err != nil {
		return nil, err
	}
	resp := m.mock.SubmitBlockHeader(ctx, &h)
	return &dalc.SubmitBlockResponse{
		Result: &dalc.DAResponse{
			Code:     dalc.StatusCode(resp.Code),
			Message:  resp.Message,
			DAHeight: resp.DAHeight,
		},
	}, nil
}

func (m *mockImpl) SubmitBlockData(ctx context.Context, request *dalc.SubmitBlockDataRequest) (*dalc.SubmitBlockResponse, error) {
	var d types.Data
	err := d.FromProto(request.Data)
	if err != nil {
		return nil, err
	}
	resp := m.mock.SubmitBlockData(ctx, &d)
	return &dalc.SubmitBlockResponse{
		Result: &dalc.DAResponse{
			Code:     dalc.StatusCode(resp.Code),
			Message:  resp.Message,
			DAHeight: resp.DAHeight,
		},
	}, nil
}

func (m *mockImpl) CheckBlockHeaderAvailability(ctx context.Context, request *dalc.CheckBlockHeaderAvailabilityRequest) (*dalc.CheckBlockAvailabilityResponse, error) {
	resp := m.mock.CheckBlockHeaderAvailability(ctx, request.DAHeight)
	return &dalc.CheckBlockAvailabilityResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		DataAvailable: resp.DataAvailable,
	}, nil
}

func (m *mockImpl) CheckBlockDataAvailability(ctx context.Context, request *dalc.CheckBlockDataAvailabilityRequest) (*dalc.CheckBlockAvailabilityResponse, error) {
	resp := m.mock.CheckBlockDataAvailability(ctx, request.DAHeight)
	return &dalc.CheckBlockAvailabilityResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		DataAvailable: resp.DataAvailable,
	}, nil
}

func (m *mockImpl) RetrieveBlockHeaders(ctx context.Context, request *dalc.RetrieveBlockHeadersRequest) (*dalc.RetrieveBlockHeadersResponse, error) {
	resp := m.mock.RetrieveBlockHeaders(ctx, request.DAHeight)
	headers := make([]*rollkit.SignedHeader, len(resp.Headers))
	for i := range resp.Headers {
		hp, err := resp.Headers[i].ToProto()
		if err != nil {
			return nil, err
		}
		headers[i] = hp
	}
	return &dalc.RetrieveBlockHeadersResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		Headers: headers,
	}, nil
}

func (m *mockImpl) RetrieveBlockData(ctx context.Context, request *dalc.RetrieveBlockDataRequest) (*dalc.RetrieveBlockDataResponse, error) {
	resp := m.mock.RetrieveBlockData(ctx, request.DAHeight)
	dataArr := make([]*rollkit.Data, len(resp.Data))
	for i := range resp.Data {
		dataArr[i] = resp.Data[i].ToProto()
	}
	return &dalc.RetrieveBlockDataResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		Data: dataArr,
	}, nil
}
