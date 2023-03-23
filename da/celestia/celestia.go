package celestia

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	"github.com/celestiaorg/go-cnc"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// DataAvailabilityLayerClient use celestia-node public API.
type DataAvailabilityLayerClient struct {
	client *cnc.Client

	namespaceID types.NamespaceID
	config      Config
	logger      log.Logger
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

// Config stores Celestia DALC configuration parameters.
type Config struct {
	BaseURL  string        `json:"base_url"`
	Timeout  time.Duration `json:"timeout"`
	Fee      int64         `json:"fee"`
	GasLimit uint64        `json:"gas_limit"`
}

// Init initializes DataAvailabilityLayerClient instance.
func (c *DataAvailabilityLayerClient) Init(namespaceID types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error {
	c.namespaceID = namespaceID
	c.logger = logger

	if len(config) > 0 {
		return json.Unmarshal(config, &c.config)
	}

	return nil
}

// Start prepares DataAvailabilityLayerClient to work.
func (c *DataAvailabilityLayerClient) Start() error {
	c.logger.Info("starting Celestia Data Availability Layer Client", "baseURL", c.config.BaseURL)
	var err error
	c.client, err = cnc.NewClient(c.config.BaseURL, cnc.WithTimeout(c.config.Timeout))
	return err
}

// Stop stops DataAvailabilityLayerClient.
func (c *DataAvailabilityLayerClient) Stop() error {
	c.logger.Info("stopping Celestia Data Availability Layer Client")
	return nil
}

// SubmitBlock submits a block to DA layer.
func (c *DataAvailabilityLayerClient) SubmitBlock(ctx context.Context, block *types.Block) da.ResultSubmitBlock {
	blob, err := block.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	txResponse, err := c.client.SubmitPFB(ctx, c.namespaceID, blob, c.config.Fee, c.config.GasLimit)

	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	if txResponse.Code != 0 {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: fmt.Sprintf("Codespace: '%s', Code: %d, Message: %s", txResponse.Codespace, txResponse.Code, txResponse.RawLog),
			},
		}
	}

	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "tx hash: " + txResponse.TxHash,
			DAHeight: uint64(txResponse.Height),
		},
	}
}

// CheckBlockAvailability queries DA layer to check data availability of block at given height.
func (c *DataAvailabilityLayerClient) CheckBlockAvailability(ctx context.Context, dataLayerHeight uint64) da.ResultCheckBlock {
	shares, err := c.client.NamespacedShares(ctx, c.namespaceID, dataLayerHeight)
	if err != nil {
		return da.ResultCheckBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	return da.ResultCheckBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		DataAvailable: len(shares) > 0,
	}
}

// RetrieveBlocks gets a batch of blocks from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {
	data, err := c.client.NamespacedData(ctx, c.namespaceID, dataLayerHeight)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	blocks := make([]*types.Block, len(data))
	for i, msg := range data {
		var block pb.Block
		err = proto.Unmarshal(msg, &block)
		if err != nil {
			c.logger.Error("failed to unmarshal block", "daHeight", dataLayerHeight, "position", i, "error", err)
			continue
		}
		blocks[i] = new(types.Block)
		err := blocks[i].FromProto(&block)
		if err != nil {
			return da.ResultRetrieveBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return da.ResultRetrieveBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Blocks: blocks,
	}
}
