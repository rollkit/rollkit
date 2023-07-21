package celestia

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/math"
	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	openrpc "github.com/rollkit/celestia-openrpc"
	"github.com/rollkit/celestia-openrpc/types/blob"
	"github.com/rollkit/celestia-openrpc/types/share"

	openrpcns "github.com/rollkit/celestia-openrpc/types/namespace"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// DataAvailabilityLayerClient use celestia-node public API.
type DataAvailabilityLayerClient struct {
	rpc *openrpc.Client

	namespace openrpcns.Namespace
	config    Config
	logger    log.Logger
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

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
	namespace, err := share.NewBlobNamespaceV0(namespaceID[:])
	if err != nil {
		return err
	}
	c.namespace = namespace.ToAppNamespace()
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
	c.rpc, err = openrpc.NewClient(context.Background(), c.config.BaseURL, c.config.AuthToken)
	return err
}

// Stop stops DataAvailabilityLayerClient.
func (c *DataAvailabilityLayerClient) Stop() error {
	c.logger.Info("stopping Celestia Data Availability Layer Client")
	return nil
}

// SubmitBlocks submits blocks to DA layer.
func (c *DataAvailabilityLayerClient) SubmitBlocks(ctx context.Context, blocks []*types.Block) da.ResultSubmitBlocks {
	blobs := make([]*blob.Blob, len(blocks))
	for blockIndex, block := range blocks {
		data, err := block.MarshalBinary()
		if err != nil {
			return da.ResultSubmitBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
		blockBlob, err := blob.NewBlobV0(c.namespace.Bytes(), data)
		if err != nil {
			return da.ResultSubmitBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: err.Error(),
				},
			}
		}
		blobs[blockIndex] = blockBlob
	}

	txResponse, err := c.rpc.State.SubmitPayForBlob(ctx, math.NewInt(c.config.Fee), c.config.GasLimit, blobs)
	if err != nil {
		return da.ResultSubmitBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	c.logger.Debug("successfully submitted PayForBlob transaction",
		"fee", c.config.Fee, "gasLimit", c.config.GasLimit,
		"daHeight", txResponse.Height, "daTxHash", txResponse.TxHash)

	if txResponse.Code != 0 {
		return da.ResultSubmitBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: fmt.Sprintf("Codespace: '%s', Code: %d, Message: %s", txResponse.Codespace, txResponse.Code, txResponse.RawLog),
			},
		}
	}

	return da.ResultSubmitBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "tx hash: " + txResponse.TxHash,
			DAHeight: uint64(txResponse.Height),
		},
	}
}

// RetrieveBlocks gets a batch of blocks from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {
	c.logger.Debug("trying to retrieve blob using Blob.GetAll", "daHeight", dataLayerHeight, "namespace", hex.EncodeToString(c.namespace.Bytes()))
	blobs, err := c.rpc.Blob.GetAll(ctx, dataLayerHeight, []share.Namespace{c.namespace.Bytes()})
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
		strings.Contains(err.Error(), da.ErrEDSNotFound.Error()),
		strings.Contains(err.Error(), da.ErrBlobNotFound.Error()):
		return da.StatusNotFound
	default:
		return da.StatusError
	}
}
