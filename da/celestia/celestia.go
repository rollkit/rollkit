package celestia

import (
	"context"
	"encoding/json"
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/libs/cnrc"
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
	pb "github.com/celestiaorg/optimint/types/pb/optimint"
	"github.com/gogo/protobuf/proto"
	"time"
)

// DataAvailabilityLayerClient use celestia-node public API.
type DataAvailabilityLayerClient struct {
	client *cnrc.Client

	config Config
	logger log.Logger
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

type Config struct {
	BaseURL     string        `json:"base_url"`
	Timeout     time.Duration `json:"timeout"`
	GasLimit    uint64        `json:"gas_limit"`
	NamespaceID [8]byte       `json:"namespace_id"`
}

func (c *DataAvailabilityLayerClient) Init(config []byte, kvStore store.KVStore, logger log.Logger) error {
	c.logger = logger

	if len(config) > 0 {
		return json.Unmarshal(config, &c.config)
	}

	return nil
}

func (c *DataAvailabilityLayerClient) Start() error {
	c.logger.Info("starting Celestia Data Availability Layer Client", "baseURL", c.config.BaseURL)
	var err error
	c.client, err = cnrc.NewClient(c.config.BaseURL, cnrc.WithTimeout(c.config.Timeout))
	return err
}

func (c *DataAvailabilityLayerClient) Stop() error {
	c.logger.Info("stopping Celestia Data Availability Layer Client")
	return nil
}

func (c *DataAvailabilityLayerClient) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	blob, err := block.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{
			DAResult: da.DAResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	txResponse, err := c.client.SubmitPFD(context.TODO(), c.config.NamespaceID, blob, c.config.GasLimit)

	if err != nil {
		return da.ResultSubmitBlock{
			DAResult: da.DAResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	return da.ResultSubmitBlock{
		DAResult: da.DAResult{
			Code:     da.StatusSuccess,
			Message:  "tx hash: " + txResponse.TxHash,
			DAHeight: uint64(txResponse.Height),
		},
	}
}

func (c *DataAvailabilityLayerClient) CheckBlockAvailability(dataLayerHeight uint64) da.ResultCheckBlock {
	shares, err := c.client.NamespacedShares(context.TODO(), c.config.NamespaceID, dataLayerHeight)
	if err != nil {
		return da.ResultCheckBlock{
			DAResult: da.DAResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	msgs, err := parseMsgs(shares)
	if err != nil {
		return da.ResultCheckBlock{
			DAResult: da.DAResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	return da.ResultCheckBlock{
		DAResult: da.DAResult{
			Code:     da.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		DataAvailable: len(msgs.MessagesList) != 0,
	}
}

func (c *DataAvailabilityLayerClient) RetrieveBlocks(dataLayerHeight uint64) da.ResultRetrieveBlocks {
	shares, err := c.client.NamespacedShares(context.TODO(), c.config.NamespaceID, dataLayerHeight)
	if err != nil {
		return da.ResultRetrieveBlocks{
			DAResult: da.DAResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	msgs, err := parseMsgs(shares)
	if err != nil {
		return da.ResultRetrieveBlocks{
			DAResult: da.DAResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	var blocks []*types.Block
	for i, msg := range msgs.MessagesList {
		var block pb.Block
		err = proto.Unmarshal(msg.Data, &block)
		if err != nil {
			return da.ResultRetrieveBlocks{
				DAResult: da.DAResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
		err := blocks[i].FromProto(&block)
		if err != nil {
			return da.ResultRetrieveBlocks{
				DAResult: da.DAResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return da.ResultRetrieveBlocks{
		DAResult: da.DAResult{
			Code:     0,
			Message:  "",
			DAHeight: 0,
		},
		Blocks: blocks,
	}
}
