package avail

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	ds "github.com/ipfs/go-datastore"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/avail/datasubmit"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
)

type Config struct {
	BaseURL    string  `json:"base_url"`
	Seed       string  `json:"seed"`
	ApiURL     string  `json:"api_url"`
	AppID      int     `json:"app_id"`
	confidence float64 `json:"confidence"`
}

type DataAvailabilityLayerClient struct {
	namespace types.NamespaceID
	config    Config
	logger    log.Logger
}

type Confidence struct {
	Block                uint32  `json:"block"`
	Confidence           float64 `json:"confidence"`
	SerialisedConfidence *string `json:"serialised_confidence,omitempty"`
}

type AppData struct {
	Block      uint32   `json:"block"`
	Extrinsics []string `json:"extrinsics"`
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

// Init initializes DataAvailabilityLayerClient instance.
func (c *DataAvailabilityLayerClient) Init(namespaceID types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error {
	c.logger = logger

	if len(config) > 0 {
		return json.Unmarshal(config, &c.config)
	}

	return nil
}

// Start prepares DataAvailabilityLayerClient to work.
func (c *DataAvailabilityLayerClient) Start() error {

	c.logger.Info("starting avail Data Availability Layer Client", "baseURL", c.config.ApiURL)

	return nil
}

// Stop stops DataAvailabilityLayerClient.
func (c *DataAvailabilityLayerClient) Stop() error {

	c.logger.Info("stopping Avail Data Availability Layer Client")

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
		err = datasubmit.SubmitData(c.config.ApiURL, c.config.Seed, c.config.AppID, data)

		if err != nil {
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
			Message:  "data submitted succesfully",
			DAHeight: 1,
		},
	}

}

// CheckBlockAvailability queries DA layer to check data availability of block.
func (c *DataAvailabilityLayerClient) CheckBlockAvailability(ctx context.Context, dataLayerHeight uint64) da.ResultCheckBlock {

	blockNumber := dataLayerHeight
	confidenceURL := fmt.Sprintf(c.config.BaseURL+"/confidence/%d", blockNumber)

	response, err := http.Get(confidenceURL)

	if err != nil {
		return da.ResultCheckBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return da.ResultCheckBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	var confidenceObject Confidence
	err = json.Unmarshal(responseData, &confidenceObject)
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
			DAHeight: uint64(confidenceObject.Block),
		},
		DataAvailable: confidenceObject.Confidence > float64(c.config.confidence),
	}
}

//RetrieveBlocks gets the block from DA layer.

func (c *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {

	blocks := []*types.Block{}

	blockNumber := dataLayerHeight

	appDataURL := fmt.Sprintf(c.config.BaseURL+"/appdata/%d?decode=true", blockNumber)

	response, err := http.Get(appDataURL)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	var appDataObject AppData
	err = json.Unmarshal(responseData, &appDataObject)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
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
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: types.Txs{txnsByteArray},
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: nil,
			},
		},
	}

	blocks = append(blocks, block)

	return da.ResultRetrieveBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: uint64(appDataObject.Block),
			Message:  "block data: " + strings.Join(appDataObject.Extrinsics, " "),
		},
		Blocks: blocks,
	}
}
