package grpc

import (
	"context"
	"encoding/json"
	"strconv"

	"google.golang.org/grpc"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
	"github.com/rollkit/rollkit/types/pb/dalc"
)

// DataAvailabilityLayerClient is a generic client that proxies all DA requests via gRPC.
type DataAvailabilityLayerClient struct {
	config Config

	conn   *grpc.ClientConn
	client dalc.DALCServiceClient

	logger log.Logger
}

// Config contains configuration options for DataAvailabilityLayerClient.
type Config struct {
	// TODO(tzdybal): add more options!
	Host string `json:"host"`
	Port int    `json:"port"`
}

// DefaultConfig defines default values for DataAvailabilityLayerClient configuration.
var DefaultConfig = Config{
	Host: "127.0.0.1",
	Port: 7980,
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

// Init sets the configuration options.
func (d *DataAvailabilityLayerClient) Init(_, _ types.NamespaceID, config []byte, _ ds.Datastore, logger log.Logger) error {
	d.logger = logger
	if len(config) == 0 {
		d.config = DefaultConfig
		return nil
	}
	return json.Unmarshal(config, &d.config)
}

// Start creates connection to gRPC server and instantiates gRPC client.
func (d *DataAvailabilityLayerClient) Start() error {
	d.logger.Info("starting GRPC DALC", "host", d.config.Host, "port", d.config.Port)
	var err error
	var opts []grpc.DialOption
	// TODO(tzdybal): add more options
	opts = append(opts, grpc.WithInsecure())
	d.conn, err = grpc.Dial(d.config.Host+":"+strconv.Itoa(d.config.Port), opts...)
	if err != nil {
		return err
	}

	d.client = dalc.NewDALCServiceClient(d.conn)

	return nil
}

// Stop closes connection to gRPC server.
func (d *DataAvailabilityLayerClient) Stop() error {
	d.logger.Info("stopoing GRPC DALC")
	return d.conn.Close()
}

// SubmitBlockHeader proxies SubmitBlock request to gRPC server.
func (d *DataAvailabilityLayerClient) SubmitBlockHeader(ctx context.Context, header *types.SignedHeader) da.ResultSubmitBlock {
	hp, err := header.ToProto()
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()},
		}
	}
	resp, err := d.client.SubmitBlockHeader(ctx, &dalc.SubmitBlockHeaderRequest{Header: hp})
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()},
		}
	}
	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusCode(resp.Result.Code),
			Message:  resp.Result.Message,
			DAHeight: resp.Result.DAHeight,
		},
	}
}

// SubmitBlockData proxies SubmitBlockHeader request to gRPC server.
func (d *DataAvailabilityLayerClient) SubmitBlockData(ctx context.Context, data *types.Data) da.ResultSubmitBlock {
	dp := data.ToProto()
	resp, err := d.client.SubmitBlockData(ctx, &dalc.SubmitBlockDataRequest{Data: dp})
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()},
		}
	}
	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusCode(resp.Result.Code),
			Message:  resp.Result.Message,
			DAHeight: resp.Result.DAHeight,
		},
	}
}

// CheckBlockHeaderAvailability proxies CheckBlockHeaderAvailability request to gRPC server.
func (d *DataAvailabilityLayerClient) CheckBlockHeaderAvailability(ctx context.Context, daHeight uint64) da.ResultCheckBlock {
	resp, err := d.client.CheckBlockHeaderAvailability(ctx, &dalc.CheckBlockHeaderAvailabilityRequest{DAHeight: daHeight})
	if err != nil {
		return da.ResultCheckBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}
	return da.ResultCheckBlock{
		BaseResult:    da.BaseResult{Code: da.StatusCode(resp.Result.Code), Message: resp.Result.Message},
		DataAvailable: resp.DataAvailable,
	}
}

// CheckBlockDataAvailability proxies CheckBlockDataAvailability request to gRPC server.
func (d *DataAvailabilityLayerClient) CheckBlockDataAvailability(ctx context.Context, daHeight uint64) da.ResultCheckBlock {
	resp, err := d.client.CheckBlockDataAvailability(ctx, &dalc.CheckBlockDataAvailabilityRequest{DAHeight: daHeight})
	if err != nil {
		return da.ResultCheckBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}
	return da.ResultCheckBlock{
		BaseResult:    da.BaseResult{Code: da.StatusCode(resp.Result.Code), Message: resp.Result.Message},
		DataAvailable: resp.DataAvailable,
	}
}

// RetrieveBlockHeaders proxies RetrieveBlocks request to gRPC server.
func (d *DataAvailabilityLayerClient) RetrieveBlockHeaders(ctx context.Context, daHeight uint64) da.ResultRetrieveBlockHeaders {
	resp, err := d.client.RetrieveBlockHeaders(ctx, &dalc.RetrieveBlockHeadersRequest{DAHeight: daHeight})
	if err != nil {
		return da.ResultRetrieveBlockHeaders{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	headers := make([]*types.SignedHeader, len(resp.Headers))
	for i, header := range resp.Headers {
		var h types.SignedHeader
		err = h.FromProto(header)
		if err != nil {
			return da.ResultRetrieveBlockHeaders{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}
		headers[i] = &h
	}
	return da.ResultRetrieveBlockHeaders{
		BaseResult: da.BaseResult{
			Code:     da.StatusCode(resp.Result.Code),
			Message:  resp.Result.Message,
			DAHeight: daHeight,
		},
		Headers: headers,
	}
}

// RetrieveBlockDatas proxies RetrieveBlocks request to gRPC server.
func (d *DataAvailabilityLayerClient) RetrieveBlockDatas(ctx context.Context, daHeight uint64) da.ResultRetrieveBlockDatas {
	resp, err := d.client.RetrieveBlockDatas(ctx, &dalc.RetrieveBlockDatasRequest{DAHeight: daHeight})
	if err != nil {
		return da.ResultRetrieveBlockDatas{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	datas := make([]*types.Data, len(resp.Datas))
	for i, data := range resp.Datas {
		var d types.Data
		err = d.FromProto(data)
		if err != nil {
			return da.ResultRetrieveBlockDatas{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}
		datas[i] = &d
	}
	return da.ResultRetrieveBlockDatas{
		BaseResult: da.BaseResult{
			Code:     da.StatusCode(resp.Result.Code),
			Message:  resp.Result.Message,
			DAHeight: daHeight,
		},
		Datas: datas,
	}
}
