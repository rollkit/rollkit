package grpc

import (
	"context"
	"encoding/json"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		DAResult: da.DAResult{Code: da.StatusCode(resp.Result.Code), Message: resp.Result.Message},
	}
}

func (d *DataAvailabilityLayerClient) CheckBlockAvailability(header *types.Header) da.ResultCheckBlock {
	resp, err := d.client.CheckBlockAvailability(context.TODO(), &dalc.CheckBlockAvailabilityRequest{Header: header.ToProto()})
	if err != nil {
		return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}
	return da.ResultCheckBlock{
		DAResult:      da.DAResult{Code: da.StatusCode(resp.Result.Code), Message: resp.Result.Message},
		DataAvailable: resp.DataAvailable,
	}
}

func (d *DataAvailabilityLayerClient) RetrieveBlock(height uint64) da.ResultRetrieveBlock {
	resp, err := d.client.RetrieveBlock(context.TODO(), &dalc.RetrieveBlockRequest{Height: height})
	if err != nil {
		return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	var b types.Block
	err = b.FromProto(resp.Block)
	if err != nil {
		return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}
	return da.ResultRetrieveBlock{
		DAResult: da.DAResult{Code: da.StatusCode(resp.Result.Code), Message: resp.Result.Message},
		Block:    &b,
	}
}
