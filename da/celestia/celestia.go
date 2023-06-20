package celestia

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/math"
	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	rpcproto "github.com/rollkit/celestia-openrpc/proto/blob/rollkit"

	"github.com/celestiaorg/nmt/namespace"

	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/blob"

	appns "github.com/rollkit/celestia-openrpc/types/namespace"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// DataAvailabilityLayerClient use celestia-node public API.
type DataAvailabilityLayerClient struct {
	rpc *openrpc.Client

	namespace appns.Namespace
	config    Config
	logger    log.Logger
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
	// FIXME: prefix 2 zero bytes for compat
	c.namespace = appns.MustNewV0(append([]byte{0x00, 0x00}, namespaceID[:]...))
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
			Blob: rpcproto.Blob{
				NamespaceId:      c.namespace.ID,
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

	c.logger.Debug("suucessfully submitted PayForBlob transaction", "height", txResponse.Height, "hash", txResponse.TxHash)

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
		if strings.Contains(err.Error(), ErrNotAvailable.Error()) {
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
	c.logger.Debug("querying GetAll blobs with params", "height", dataLayerHeight, "namespace", hex.EncodeToString(c.namespace.Bytes()))
	blobs, err := c.rpc.Blob.GetAll(ctx, dataLayerHeight, []namespace.ID{c.namespace.Bytes()})
	status := dataRequestErrorToStatus(err)
	if status != da.StatusSuccess {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    status,
				Message: err.Error(),
			},
		}
	}

	blocks := make([]*types.Block, len(blobs))
	for i, blob := range blobs {
		var block pb.Block
		err = proto.Unmarshal(blob.Data, &block)
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
