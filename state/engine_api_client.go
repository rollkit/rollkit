package state

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/rollkit/rollkit/execution"
	rollkitTypes "github.com/rollkit/rollkit/types"
)

/*
 ***********************************
 * TO-DO move to go-execution-EVM  *
 ***********************************
 */

// Define necessary types and constants
type PayloadStatus string

const (
	PayloadStatusValid   PayloadStatus = "VALID"
	PayloadStatusInvalid PayloadStatus = "INVALID"
	PayloadStatusSyncing PayloadStatus = "SYNCING"
)

var (
	ErrNilPayloadStatus     = errors.New("nil payload status")
	ErrInvalidPayloadStatus = errors.New("invalid payload status")
)

type EngineAPIExecutionClient struct {
	ethClient    *ethclient.Client
	engineClient *rpc.Client
	genesisHash  common.Hash
	feeRecipient common.Address
}

// NewEngineAPIExecutionClient creates a new instance of EngineAPIExecutionClient.
func NewEngineAPIExecutionClient(ethURL, engineURL string, genesisHash common.Hash, feeRecipient common.Address) (*EngineAPIExecutionClient, error) {
	ethClient, err := ethclient.Dial(ethURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %w", err)
	}

	engineClient, err := rpc.Dial(engineURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Engine API: %w", err)
	}

	return &EngineAPIExecutionClient{
		ethClient:    ethClient,
		engineClient: engineClient,
		genesisHash:  genesisHash,
		feeRecipient: feeRecipient,
	}, nil
}

var _ execution.Execute = (*EngineAPIExecutionClient)(nil)

// InitChain initializes the blockchain with genesis information.
func (c *EngineAPIExecutionClient) InitChain(
	genesisTime time.Time,
	initialHeight uint64,
	chainID string,
) (rollkitTypes.Hash, uint64, error) {
	ctx := context.Background()

	var forkchoiceResult map[string]interface{}
	err := c.engineClient.CallContext(ctx, &forkchoiceResult, "engine_forkchoiceUpdatedV1",
		map[string]interface{}{
			"headBlockHash":      c.genesisHash,
			"safeBlockHash":      c.genesisHash,
			"finalizedBlockHash": c.genesisHash,
		},
		map[string]interface{}{
			"timestamp":             genesisTime.Unix(),
			"prevRandao":            common.Hash{}, // TO-DO
			"suggestedFeeRecipient": c.feeRecipient,
		},
	)
	if err != nil {
		return rollkitTypes.Hash{}, 0, fmt.Errorf("engine_forkchoiceUpdatedV1 failed: %w", err)
	}

	payloadID, ok := forkchoiceResult["payloadId"].(string)
	if !ok {
		return rollkitTypes.Hash{}, 0, ErrNilPayloadStatus
	}

	var payload map[string]interface{}
	err = c.engineClient.CallContext(ctx, &payload, "engine_getPayloadV1", payloadID)
	if err != nil {
		return rollkitTypes.Hash{}, 0, fmt.Errorf("engine_getPayloadV1 failed: %w", err)
	}

	stateRoot := common.HexToHash(payload["stateRoot"].(string))
	gasLimit := uint64(payload["gasLimit"].(float64))

	var rollkitStateRoot rollkitTypes.Hash
	copy(rollkitStateRoot[:], stateRoot[:])

	return rollkitStateRoot, gasLimit, nil
}

// GetTxs retrieves transactions from the transaction pool.
func (c *EngineAPIExecutionClient) GetTxs() ([]rollkitTypes.Tx, error) {
	ctx := context.Background()

	var result struct {
		Pending map[string]map[string]*types.Transaction `json:"pending"`
		Queued  map[string]map[string]*types.Transaction `json:"queued"`
	}

	err := c.ethClient.Client().CallContext(ctx, &result, "txpool_content")
	if err != nil {
		return nil, fmt.Errorf("failed to get tx pool content: %w", err)
	}

	var txs []rollkitTypes.Tx

	for _, accountTxs := range result.Pending {
		for _, tx := range accountTxs {
			txBytes, err := tx.MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("failed to marshal transaction: %w", err)
			}
			txs = append(txs, rollkitTypes.Tx(txBytes))
		}
	}

	for _, accountTxs := range result.Queued {
		for _, tx := range accountTxs {
			txBytes, err := tx.MarshalBinary()
			if err != nil {
				return nil, fmt.Errorf("failed to marshal transaction: %w", err)
			}
			txs = append(txs, rollkitTypes.Tx(txBytes))
		}
	}

	return txs, nil
}

// ExecuteTxs executes the given transactions and returns the new state root and gas used.
func (c *EngineAPIExecutionClient) ExecuteTxs(
	txs []rollkitTypes.Tx,
	blockHeight uint64,
	timestamp time.Time,
	prevStateRoot rollkitTypes.Hash,
) (rollkitTypes.Hash, uint64, error) {
	ctx := context.Background()

	ethTxs := make([][]byte, len(txs))
	for i, tx := range txs {
		ethTxs[i] = tx
	}

	prevRandao := c.derivePrevRandao(blockHeight)

	var forkchoiceResult map[string]interface{}
	err := c.engineClient.CallContext(ctx, &forkchoiceResult, "engine_forkchoiceUpdatedV1",
		map[string]interface{}{
			"headBlockHash":      common.BytesToHash(prevStateRoot[:]),
			"safeBlockHash":      common.BytesToHash(prevStateRoot[:]),
			"finalizedBlockHash": common.BytesToHash(prevStateRoot[:]),
		},
		map[string]interface{}{
			"timestamp":             timestamp.Unix(),
			"prevRandao":            prevRandao,
			"suggestedFeeRecipient": c.feeRecipient,
		},
	)
	if err != nil {
		return rollkitTypes.Hash{}, 0, fmt.Errorf("engine_forkchoiceUpdatedV1 failed: %w", err)
	}

	payloadID, ok := forkchoiceResult["payloadId"].(string)
	if !ok {
		return rollkitTypes.Hash{}, 0, ErrNilPayloadStatus
	}

	var payload map[string]interface{}
	err = c.engineClient.CallContext(ctx, &payload, "engine_getPayloadV1", payloadID)
	if err != nil {
		return rollkitTypes.Hash{}, 0, fmt.Errorf("engine_getPayloadV1 failed: %w", err)
	}

	payload["transactions"] = ethTxs

	var newPayloadResult map[string]interface{}
	err = c.engineClient.CallContext(ctx, &newPayloadResult, "engine_newPayloadV1", payload)
	if err != nil {
		return rollkitTypes.Hash{}, 0, fmt.Errorf("engine_newPayloadV1 failed: %w", err)
	}

	status, ok := newPayloadResult["status"].(string)
	if !ok || PayloadStatus(status) != PayloadStatusValid {
		return rollkitTypes.Hash{}, 0, ErrInvalidPayloadStatus
	}

	newStateRoot := common.HexToHash(payload["stateRoot"].(string))
	gasUsed := uint64(payload["gasUsed"].(float64))

	var rollkitNewStateRoot rollkitTypes.Hash
	copy(rollkitNewStateRoot[:], newStateRoot[:])

	return rollkitNewStateRoot, gasUsed, nil
}

// SetFinal marks a block at the given height as final.
func (c *EngineAPIExecutionClient) SetFinal(blockHeight uint64) error {
	ctx := context.Background()

	block, err := c.ethClient.BlockByNumber(ctx, big.NewInt(int64(blockHeight)))
	if err != nil {
		return fmt.Errorf("failed to get block at height %d: %w", blockHeight, err)
	}

	var result map[string]interface{}
	err = c.engineClient.CallContext(ctx, &result, "engine_forkchoiceUpdatedV1",
		map[string]interface{}{
			"headBlockHash":      block.Hash(),
			"safeBlockHash":      block.Hash(),
			"finalizedBlockHash": block.Hash(),
		},
		nil, // No payload attributes for finalization
	)
	if err != nil {
		return fmt.Errorf("engine_forkchoiceUpdatedV1 failed for finalization: %w", err)
	}

	payloadStatus, ok := result["payloadStatus"].(map[string]interface{})
	if !ok {
		return ErrNilPayloadStatus
	}

	status, ok := payloadStatus["status"].(string)
	if !ok || PayloadStatus(status) != PayloadStatusValid {
		return ErrInvalidPayloadStatus
	}

	return nil
}

// derivePrevRandao generates a deterministic prevRandao value based on block height.
func (c *EngineAPIExecutionClient) derivePrevRandao(blockHeight uint64) common.Hash {
	// TO-DO
	return common.BigToHash(big.NewInt(int64(blockHeight)))
}
