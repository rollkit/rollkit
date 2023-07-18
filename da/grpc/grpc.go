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
	"github.com/rollkit/rollkit/types/pb/rollkit"
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
func (d *DataAvailabilityLayerClient) Init(_ types.NamespaceID, config []byte, _ ds.Datastore, logger log.Logger) error {
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

// SubmitBlocks proxies SubmitBlocks request to gRPC server.
func (d *DataAvailabilityLayerClient) SubmitBlocks(ctx context.Context, blocks []*types.Block) da.ResultSubmitBlocks {
	bps := make([]*rollkit.Block, len(blocks))
	// convert blocks to protobuf
	for i, block := range blocks {
		bp, err := block.ToProto()
		if err != nil {
			return da.ResultSubmitBlocks{
				BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()},
			}
		}
		bps[i] = bp
	}

	resp, err := d.client.SubmitBlocks(ctx, &dalc.SubmitBlocksRequest{Blocks: bps})
	if err != nil {
		return da.ResultSubmitBlocks{
			BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()},
		}
	}
	return da.ResultSubmitBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusCode(resp.Result.Code),
			Message:  resp.Result.Message,
			DAHeight: resp.Result.DAHeight,
		},
	}
}

// RetrieveBlocks proxies RetrieveBlocks request to gRPC server.
func (d *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, daHeight uint64) da.ResultRetrieveBlocks {
	resp, err := d.client.RetrieveBlocks(ctx, &dalc.RetrieveBlocksRequest{DAHeight: daHeight})
	if err != nil {
		return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	blocks := make([]*types.Block, len(resp.Blocks))
	for i, block := range resp.Blocks {
		var b types.Block
		err = b.FromProto(block)
		if err != nil {
			return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}
		blocks[i] = &b
	}
	return da.ResultRetrieveBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusCode(resp.Result.Code),
			Message:  resp.Result.Message,
			DAHeight: daHeight,
		},
		Blocks: blocks,
	}
}
