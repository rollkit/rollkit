package celestia

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

	headerNamespaceID types.NamespaceID
	dataNamespaceID   types.NamespaceID
	config            Config
	logger            log.Logger
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
func (c *DataAvailabilityLayerClient) Init(headerNamespaceID, dataNamespaceID types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error {
	c.headerNamespaceID = headerNamespaceID
	c.dataNamespaceID = dataNamespaceID
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

// SubmitBlockHeader submits a block header to DA layer.
func (c *DataAvailabilityLayerClient) SubmitBlockHeader(ctx context.Context, header *types.SignedHeader) da.ResultSubmitBlock {
	headerBlob, err := header.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	txResponse, err := c.client.SubmitPFB(ctx, c.headerNamespaceID, headerBlob, c.config.Fee, c.config.GasLimit)
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

// SubmitBlockData submits a block data to DA layer.
func (c *DataAvailabilityLayerClient) SubmitBlockData(ctx context.Context, data *types.Data) da.ResultSubmitBlock {
	dataBlob, err := data.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	txResponse, err := c.client.SubmitPFB(ctx, c.dataNamespaceID, dataBlob, c.config.Fee, c.config.GasLimit)
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

	dataHash, err := data.Hash()
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "tx hash: " + txResponse.TxHash,
			DAHeight: uint64(txResponse.Height),
		},
		Hash: dataHash,
	}
}

// CheckBlockHeaderAvailability queries DA layer to check data availability of block header at given height.
func (c *DataAvailabilityLayerClient) CheckBlockHeaderAvailability(ctx context.Context, dataLayerHeight uint64) da.ResultCheckBlock {
	headerShares, err := c.client.NamespacedShares(ctx, c.headerNamespaceID, dataLayerHeight)
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
		DataAvailable: len(headerShares) > 0,
	}
}

// CheckBlockDataAvailability queries DA layer to check data availability of block data at given height.
func (c *DataAvailabilityLayerClient) CheckBlockDataAvailability(ctx context.Context, dataLayerHeight uint64) da.ResultCheckBlock {
	dataShares, err := c.client.NamespacedShares(ctx, c.dataNamespaceID, dataLayerHeight)
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
		DataAvailable: len(dataShares) > 0,
	}
}

// RetrieveBlockHeaders gets a batch of block headers from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlockHeaders(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlockHeaders {
	headerBlobs, err := c.client.NamespacedData(ctx, c.headerNamespaceID, dataLayerHeight)
	if err != nil {
		var code da.StatusCode
		if strings.Contains(err.Error(), da.ErrDataNotFound.Error()) || strings.Contains(err.Error(), da.ErrEDSNotFound.Error()) {
			code = da.StatusNotFound
		} else {
			code = da.StatusError
		}
		return da.ResultRetrieveBlockHeaders{
			BaseResult: da.BaseResult{
				Code:    code,
				Message: err.Error(),
			},
		}
	}

	headers := make([]*types.SignedHeader, len(headerBlobs))
	for i, headerBlob := range headerBlobs {
		var h pb.SignedHeader
		err = proto.Unmarshal(headerBlob, &h)
		if err != nil {
			c.logger.Error("failed to unmarshal header", "daHeight", dataLayerHeight, "position", i, "error", err)
			continue
		}

		headers[i] = new(types.SignedHeader)
		err := headers[i].FromProto(&h)
		if err != nil {
			return da.ResultRetrieveBlockHeaders{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return da.ResultRetrieveBlockHeaders{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Headers: headers,
	}
}

// RetrieveBlockData gets a batch of block datas from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlockData(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlockData {
	dataBlobs, err := c.client.NamespacedData(ctx, c.dataNamespaceID, dataLayerHeight)
	if err != nil {
		var code da.StatusCode
		if strings.Contains(err.Error(), da.ErrDataNotFound.Error()) || strings.Contains(err.Error(), da.ErrEDSNotFound.Error()) {
			code = da.StatusNotFound
		} else {
			code = da.StatusError
		}
		return da.ResultRetrieveBlockData{
			BaseResult: da.BaseResult{
				Code:    code,
				Message: err.Error(),
			},
		}
	}

	data := make([]*types.Data, len(dataBlobs))
	for i, dataBlob := range dataBlobs {
		var d pb.Data
		err = proto.Unmarshal(dataBlob, &d)
		if err != nil {
			c.logger.Error("failed to unmarshal data", "daHeight", dataLayerHeight, "position", i, "error", err)
			continue
		}

		data[i] = new(types.Data)
		err := data[i].FromProto(&d)
		if err != nil {
			return da.ResultRetrieveBlockData{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return da.ResultRetrieveBlockData{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Data: data,
	}
}
