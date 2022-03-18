package grpc

import (
	"context"
	"encoding/json"
	"strconv"

	"google.golang.org/grpc"

	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
	"github.com/celestiaorg/optimint/types/pb/dalc"
)

type DataAvailabilityLayerClient struct {
	config Config

	conn   *grpc.ClientConn
	client dalc.DALCServiceClient

	logger log.Logger
}

type Config struct {
	// TODO(tzdybal): add more options!
	Host string `json:"host"`
	Port int    `json:"port"`
}

var DefaultConfig = Config{
	Host: "127.0.0.1",
	Port: 7980,
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

func (d *DataAvailabilityLayerClient) Init(config []byte, _ store.KVStore, logger log.Logger) error {
	d.logger = logger
	if len(config) == 0 {
		d.config = DefaultConfig
		return nil
	}
	return json.Unmarshal(config, &d.config)
}

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

func (d *DataAvailabilityLayerClient) Stop() error {
	d.logger.Info("stopoing GRPC DALC")
	return d.conn.Close()
}

func (d *DataAvailabilityLayerClient) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	resp, err := d.client.SubmitBlock(context.TODO(), &dalc.SubmitBlockRequest{Block: block.ToProto()})
	if err != nil {
		return da.ResultSubmitBlock{
			DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()},
		}
	}
	return da.ResultSubmitBlock{
		DAResult: da.DAResult{
			Code:            da.StatusCode(resp.Result.Code),
			Message:         resp.Result.Message,
			DataLayerHeight: resp.Result.DataLayerHeight,
		},
	}
}

func (d *DataAvailabilityLayerClient) CheckBlockAvailability(dataLayerHeight uint64) da.ResultCheckBlock {
	resp, err := d.client.CheckBlockAvailability(context.TODO(), &dalc.CheckBlockAvailabilityRequest{DataLayerHeight: dataLayerHeight})
	if err != nil {
		return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}
	return da.ResultCheckBlock{
		DAResult:      da.DAResult{Code: da.StatusCode(resp.Result.Code), Message: resp.Result.Message},
		DataAvailable: resp.DataAvailable,
	}
}

func (d *DataAvailabilityLayerClient) RetrieveBlocks(dataLayerHeight uint64) da.ResultRetrieveBlock {
	resp, err := d.client.RetrieveBlocks(context.TODO(), &dalc.RetrieveBlocksRequest{DataLayerHeight: dataLayerHeight})
	if err != nil {
		return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	blocks := make([]*types.Block, len(resp.Blocks))
	for i, block := range resp.Blocks {
		var b types.Block
		err = b.FromProto(block)
		if err != nil {
			return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
		}
		blocks[i] = &b
	}
	return da.ResultRetrieveBlock{
		DAResult: da.DAResult{
			Code:            da.StatusCode(resp.Result.Code),
			Message:         resp.Result.Message,
			DataLayerHeight: dataLayerHeight,
		},
		Blocks: blocks,
	}
}
