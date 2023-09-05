package avail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/avail/datasubmit"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
)

const BLOCK_NOT_FOUND = "\"Not found\""
const PROCESSING_BLOCK = "\"Processing block\""

// Config stores Avail DALC configuration parameters.
type Config struct {
	BaseURL    string  `json:"base_url"`
	Seed       string  `json:"seed"`
	ApiURL     string  `json:"api_url"`
	AppDataURL string  `json:"app_data_url"`
	AppID      int     `json:"app_id"`
	Confidence float64 `json:"confidence"`
}

// DataAvailabilityLayerClient uses Avail-Node configuration parameters
type DataAvailabilityLayerClient struct {
	_      types.NamespaceID
	config Config
	logger log.Logger
}

// Confidence stores block params retireved from Avail Light Node Endpoint
type Confidence struct {
	Block                uint32  `json:"block"`
	Confidence           float64 `json:"confidence"`
	SerialisedConfidence *string `json:"serialised_confidence,omitempty"`
}

// AppData stores Extrinsics retrieved from Avail Light Node Endpoint
type AppData struct {
	Block      uint32   `json:"block"`
	Extrinsics []string `json:"extrinsics"`
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

// Init initializes DataAvailabilityLayerClient instance.
func (c *DataAvailabilityLayerClient) Init(_ types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error {
	c.logger = logger

	if len(config) > 0 {
		return json.Unmarshal(config, &c.config)
	}

	return nil
}

// Start prepares DataAvailabilityLayerClient to work.
func (c *DataAvailabilityLayerClient) Start() error {

	c.logger.Info("starting avail data availability layer client", "baseURL", c.config.ApiURL)

	return nil
}

// Stop stops DataAvailabilityLayerClient.
func (c *DataAvailabilityLayerClient) Stop() error {

	c.logger.Info("stopping avail data availability layer client")

	return nil
}

// SubmitBlock submits a block to DA layer.
func (c *DataAvailabilityLayerClient) SubmitBlocks(ctx context.Context, blocks []*types.Block) da.ResultSubmitBlocks {

	for _, block := range blocks {
		data, err := block.MarshalBinary()
		if err != nil {
			return da.ResultSubmitBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
		if err := datasubmit.SubmitData(c.config.ApiURL, c.config.Seed, c.config.AppID, data); err != nil {
			return da.ResultSubmitBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}

	}

	return da.ResultSubmitBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "success",
			DAHeight: 1,
		},
	}

}

// RetrieveBlocks gets the block from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {

	blocks := []*types.Block{}

Loop:
	blockNumber := dataLayerHeight

	appDataURL := fmt.Sprintf(c.config.BaseURL+c.config.AppDataURL, blockNumber)

	// Sanitize and validate the URL
	parsedURL, err := url.Parse(appDataURL)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	// Create an HTTP request with the sanitized URL
	req, err := http.NewRequest("GET", parsedURL.String(), nil)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	// Perform the HTTP request
	client := http.DefaultClient
	response, err := client.Do(req)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}
	defer func() {
		_ = response.Body.Close()
	}()

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	var appDataObject AppData
	if string(responseData) == BLOCK_NOT_FOUND {

		appDataObject = AppData{Block: uint32(blockNumber), Extrinsics: []string{}}

	} else if string(responseData) == PROCESSING_BLOCK {

		goto Loop

	} else {
		if err = json.Unmarshal(responseData, &appDataObject); err != nil {
			return da.ResultRetrieveBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	txnsByteArray := []byte{}
	for _, extrinsic := range appDataObject.Extrinsics {
		txnsByteArray = append(txnsByteArray, []byte(extrinsic)...)
	}

	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: blockNumber,
				},
			}},
		Data: types.Data{
			Txs: types.Txs{txnsByteArray},
		},
	}

	blocks = append(blocks, block)

	return da.ResultRetrieveBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: uint64(appDataObject.Block),
		},
		Blocks: blocks,
	}
}
