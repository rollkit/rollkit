package state

import (
	"time"

	"github.com/rollkit/rollkit/execution"
	"github.com/rollkit/rollkit/types"
)

/*
 ***********************************
 * TO-DO move to go-execution-EVM  *
 ***********************************
 */

type EngineAPIExecutionClient struct {
}

// NewEngineAPIExecutionClient creates a new instance of EngineAPIExecutionClient.
func NewEngineAPIExecutionClient() *EngineAPIExecutionClient {
	return &EngineAPIExecutionClient{}
}

var _ execution.Execute = (*EngineAPIExecutionClient)(nil)

// InitChain initializes the blockchain with genesis information.
func (c *EngineAPIExecutionClient) InitChain(
	genesisTime time.Time,
	initialHeight uint,
	chainID string,
) (types.Hash, uint, error) {
	return types.Hash{}, 0, nil
}

// GetTxs retrieves all available transactions from the mempool.
func (c *EngineAPIExecutionClient) GetTxs() ([]types.Tx, error) {
	return nil, nil
}

// ExecuteTxs executes a set of transactions to produce a new block.
func (c *EngineAPIExecutionClient) ExecuteTxs(
	txs []types.Tx,
	blockHeight uint64,
	timestamp time.Time,
	prevStateRoot types.Hash,
) (types.Hash, uint64, error) {
	return types.Hash{}, 0, nil
}

// SetFinal marks a block at the given height as final.
func (c *EngineAPIExecutionClient) SetFinal(blockHeight uint64) error {
	return nil
}
