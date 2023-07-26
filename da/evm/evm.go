package evm

import (
	"context"
	"crypto/ecdsa"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	ds "github.com/ipfs/go-datastore"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/evm/contracts"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/types"
)

var localEthJsonRPC = "http://localhost:8545"
var alice = common.HexToAddress("0x20f33CE90A13a4b5E7697E3544c3083B8F8A51D4")

// DataAvailabilityLayerClient use celestia-node public API.
type DataAvailabilityLayerClient struct {
	rpc        *ethclient.Client
	namespace  [32]byte
	inboxAddr  common.Address
	logger     log.Logger
	privateKey *ecdsa.PrivateKey
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
	c.logger = logger
	var err error
	c.privateKey, err = crypto.HexToECDSA("fffdbb37105441e14b0ee6330d855d8504ff39e705c3afa8f859ac9865f99306")
	if err != nil {
		return err
	}
	c.inboxAddr = common.HexToAddress("0x18Df82C7E422A42D47345Ed86B0E935E9718eBda")
	c.namespace[0] = 0x01
	return err
}

// Start prepares DataAvailabilityLayerClient to work.
func (c *DataAvailabilityLayerClient) Start() error {
	c.logger.Info("starting Celestia Data Availability Layer Client", "baseURL", localEthJsonRPC)
	var err error
	c.rpc, err = ethclient.DialContext(context.TODO(), localEthJsonRPC)
	if err != nil {
		c.logger.Error("Failed to Start")
	}

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
		c.logger.Error("Failed to MarshalBlock")
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	transactor, err := contracts.NewRollkitInbox(c.inboxAddr, c.rpc)
	if err != nil {
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	tx, err := transactor.SubmitBlock(BuildTxOpts(c.rpc, alice, c.privateKey), c.namespace, data)
	if err != nil {
		c.logger.Error("Failed to SubmitBlock")
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	receipt, err := WaitForMined(c.rpc, tx)
	if err != nil {
		c.logger.Error("Failed to WaitForMind")
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
		}
	}

	if receipt.Status == 0 {
		c.logger.Error("Tx reverted")
		return da.ResultSubmitBlock{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: "ethereum tx failed",
			},
		}
	}

	c.logger.Debug("successfully submitted L1 transaction", "receipt", receipt)

	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "ethereum l1 tx hash: " + tx.Hash().Hex(),
			DAHeight: receipt.BlockNumber.Uint64(),
		},
	}
}

// CheckBlockAvailability queries DA layer to check data availability of block at given height.
func (c *DataAvailabilityLayerClient) CheckBlockAvailability(ctx context.Context, dataLayerHeight uint64) da.ResultCheckBlock {
	res := c.RetrieveBlocks(ctx, dataLayerHeight)
	return da.ResultCheckBlock{
		BaseResult:    res.BaseResult,
		DataAvailable: res.Blocks != nil,
	}

}

// RetrieveBlocks gets a batch of blocks from DA layer.
func (c *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {
	transactor, err := contracts.NewRollkitInbox(c.inboxAddr, c.rpc)
	if err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: err.Error(),
			},
			Blocks: nil,
		}
	}

	bz, err := transactor.RetrieveBlock(&bind.CallOpts{}, c.namespace, dataLayerHeight)
	if bz == nil || err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:     da.StatusError,
				DAHeight: dataLayerHeight,
			},
			Blocks: nil,
		}
	}

	blocks := make([]*types.Block, 1)
	blocks[0] = new(types.Block)

	if err = (blocks[0]).UnmarshalBinary(bz); err != nil {
		return da.ResultRetrieveBlocks{
			BaseResult: da.BaseResult{
				Code:     da.StatusNotFound,
				DAHeight: dataLayerHeight,
			},
			Blocks: nil,
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
