package celestia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/math"
	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/blob"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// DataAvailabilityLayerClient use celestia-node public API.
type DataAvailabilityLayerClient struct {
	rpc *openrpc.Client

	namespaceID types.NamespaceID
	config      Config
	logger      log.Logger
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

var ErrNotAvailable = errors.New("share: data not available")

// Config stores Celestia DALC configuration parameters.
type Config struct {
	AuthToken string        `json:"auth_token"`
	BaseURL   string        `json:"base_url"`
	Timeout   time.Duration `json:"timeout"`
	Fee       int64         `json:"fee"`
	GasLimit  uint64        `json:"gas_limit"`
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
	ctx := context.Background()
	c.rpc, err = openrpc.NewClient(ctx, c.config.BaseURL, c.config.AuthToken)
	return err
}

// Stop stops DataAvailabilityLayerClient.
func (c *DataAvailabilityLayerClient) Stop() error {
	c.logger.Info("stopping Celestia Data Availability Layer Client")
	return nil
}

// SubmitBlock submits a block to DA layer.
func (c *DataAvailabilityLayerClient) SubmitBlock(ctx context.Context, block *types.Block) da.ResultSubmitBlock {
	data, err := block.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	blobs := []*blob.Blob{
		{
			Blob: tmproto.Blob{
				NamespaceId:      c.namespaceID[:],
				Data:             data,
				ShareVersion:     0,
				NamespaceVersion: 0,
			},
			Commitment: []byte{},
		},
	}

	txResponse, err := c.rpc.State.SubmitPayForBlob(ctx, math.NewInt(c.config.Fee), c.config.GasLimit, blobs)

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
	header, err := c.rpc.Header.GetByHeight(ctx, dataLayerHeight)
	if err != nil {
		return da.ResultCheckBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}
	if header.DAH == nil {
		return da.ResultCheckBlock{
			BaseResult: da.BaseResult{
				Code:     da.StatusSuccess,
				DAHeight: dataLayerHeight,
			},
			DataAvailable: false,
		}
	}
	err = c.rpc.Share.SharesAvailable(ctx, header.DAH)
	if err != nil {
		if err.Error() == ErrNotAvailable.Error() {
			return da.ResultCheckBlock{
				BaseResult: da.BaseResult{
					Code:     da.StatusSuccess,
					DAHeight: dataLayerHeight,
				},
				DataAvailable: false,
			}
		}
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
		DataAvailable: true,
	}
}

// RetrieveBlocks gets a batch of blocks from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {
	header, err := c.rpc.Header.GetByHeight(ctx, dataLayerHeight)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}
	rows, err := c.rpc.Share.GetSharesByNamespace(ctx, header.DAH, c.namespaceID[:])
	if err != nil {
		var code da.StatusCode
		if strings.Contains(err.Error(), da.ErrDataNotFound.Error()) || strings.Contains(err.Error(), da.ErrEDSNotFound.Error()) {
			code = da.StatusNotFound
		} else {
			code = da.StatusError
		}
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    code,
				Message: err.Error(),
			},
		}
	}

	size := 0
	for _, row := range rows {
		size += len(row.Shares)
	}
	blocks := make([]*types.Block, size)
	for _, row := range rows {
		for i, share := range row.Shares {
			var block pb.Block
			err = proto.Unmarshal(share, &block)
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
	}

	return da.ResultRetrieveBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Blocks: blocks,
	}
}

func dataRequestErrorToStatus(err error) da.StatusCode {
	switch {
	case err == nil,
		// ErrNamespaceNotFound is a success because it means no retries are necessary, the
		// namespace doesn't exist in the block.
		// TODO: Once node implements non-inclusion proofs, ErrNamespaceNotFound needs to be verified
		strings.Contains(err.Error(), da.ErrNamespaceNotFound.Error()):
		return da.StatusSuccess
	case strings.Contains(err.Error(), da.ErrDataNotFound.Error()),
		strings.Contains(err.Error(), da.ErrEDSNotFound.Error()):
		return da.StatusNotFound
	default:
		return da.StatusError
	}
}
