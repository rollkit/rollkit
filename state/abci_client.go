package state

import (
	"context"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/rollkit/rollkit/execution"
	"github.com/rollkit/rollkit/types"
)

/*
 ***********************************
 * TO-DO move to go-execution-abci *
 ***********************************
 */

type ABCIExecutionClient struct {
	blockExecutor *BlockExecutor
}

func NewABCIExecutionClient(blockExecutor *BlockExecutor) *ABCIExecutionClient {
	return &ABCIExecutionClient{
		blockExecutor: blockExecutor,
	}
}

var _ execution.Execute = (*ABCIExecutionClient)(nil)

// InitChain initializes the blockchain with genesis information.
func (c *ABCIExecutionClient) InitChain(
	genesisTime time.Time,
	initialHeight uint,
	chainID string,
) (types.Hash, uint, error) {
	genesis := &cmtypes.GenesisDoc{
		GenesisTime:     genesisTime,
		ChainID:         chainID,
		ConsensusParams: c.blockExecutor.genesis.ConsensusParams,
		Validators:      c.blockExecutor.genesis.Validators,
		AppState:        c.blockExecutor.genesis.AppState,
		InitialHeight:   int64(initialHeight),
	}

	response, err := c.blockExecutor.InitChain(genesis)
	if err != nil {
		return types.Hash{}, 0, err
	}

	stateRoot := types.Hash(response.AppHash)
	maxBytes := response.ConsensusParams.Block.MaxBytes
	return stateRoot, uint(maxBytes), nil
}

// GetTxs retrieves all available transactions from the mempool.
func (c *ABCIExecutionClient) GetTxs() ([]types.Tx, error) {
	state, err := c.blockExecutor.GetState(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get current state: %w", err)
	}

	maxBytes := state.ConsensusParams.Block.MaxBytes
	if maxBytes == -1 {
		maxBytes = int64(cmtypes.MaxBlockSizeBytes)
	}
	if maxBytes > int64(c.blockExecutor.maxBytes) {
		c.blockExecutor.logger.Debug("limiting maxBytes to", "maxBytes", c.blockExecutor.maxBytes)
		maxBytes = int64(c.blockExecutor.maxBytes)
	}

	cmTxs := c.blockExecutor.mempool.ReapMaxTxs(int(maxBytes))

	rollkitTxs := make([]types.Tx, len(cmTxs))
	for i, tx := range cmTxs {
		rollkitTxs[i] = types.Tx(tx)
	}

	return rollkitTxs, nil
}

// ExecuteTxs executes a set of transactions to produce a new block.
func (c *ABCIExecutionClient) ExecuteTxs(
	txs []types.Tx,
	blockHeight uint64,
	timestamp time.Time,
	prevStateRoot types.Hash,
) (types.Hash, uint64, error) {
	ctx := context.Background()

	state, err := c.blockExecutor.GetState(ctx)
	if err != nil {
		return types.Hash{}, 0, fmt.Errorf("failed to get current state: %w", err)
	}

	cmTxs := fromRollkitTxs(txs)

	var lastSignature *types.Signature
	var lastHeaderHash types.Hash
	var lastExtendedCommit abci.ExtendedCommitInfo

	if blockHeight == uint64(c.blockExecutor.genesis.InitialHeight) {
		lastSignature = &types.Signature{}
		lastHeaderHash = types.Hash{}
		lastExtendedCommit = abci.ExtendedCommitInfo{}
	} else {
		lastSignature, err = c.blockExecutor.store.GetSignature(ctx, blockHeight-1)
		if err != nil {
			return types.Hash{}, 0, fmt.Errorf("error while loading last commit: %w", err)
		}

		lastHeader, _, err := c.blockExecutor.store.GetBlockData(ctx, blockHeight-1)
		if err != nil {
			return types.Hash{}, 0, fmt.Errorf("error while loading last block: %w", err)
		}
		lastHeaderHash = lastHeader.Hash()

		extCommit, err := c.blockExecutor.store.GetExtendedCommit(ctx, blockHeight-1)
		if err != nil {
			return types.Hash{}, 0, fmt.Errorf("failed to load extended commit for height %d: %w", blockHeight-1, err)
		}
		if extCommit != nil {
			lastExtendedCommit = *extCommit
		}
	}

	header, data, err := c.blockExecutor.CreateBlock(
		blockHeight,
		lastSignature,
		lastExtendedCommit,
		lastHeaderHash,
		state,
		cmTxs,
		timestamp,
	)
	if err != nil {
		return types.Hash{}, 0, fmt.Errorf("failed to create block: %w", err)
	}

	isValid, err := c.blockExecutor.ProcessProposal(header, data, state)
	if err != nil {
		return types.Hash{}, 0, fmt.Errorf("failed to process proposal: %w", err)
	}
	if !isValid {
		return types.Hash{}, 0, fmt.Errorf("proposal was not valid")
	}

	newState, resp, err := c.blockExecutor.ApplyBlock(ctx, state, header, data)
	if err != nil {
		return types.Hash{}, 0, fmt.Errorf("failed to apply block: %w", err)
	}

	appHash, _, err := c.blockExecutor.Commit(ctx, newState, header, data, resp)
	if err != nil {
		return types.Hash{}, 0, fmt.Errorf("failed to commit: %w", err)
	}

	return types.Hash(appHash), uint64(newState.ConsensusParams.Block.MaxBytes), nil
}

// SetFinal marks a block at the given height as final.
func (c *ABCIExecutionClient) SetFinal(blockHeight uint64) error {
	ctx := context.Background()

	header, data, err := c.blockExecutor.store.GetBlockData(ctx, blockHeight)
	if err != nil {
		return fmt.Errorf("failed to get block data for height %d: %w", blockHeight, err)
	}

	state, err := c.blockExecutor.GetState(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current state: %w", err)
	}

	resp, err := c.blockExecutor.proxyApp.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{
		Hash:               header.Hash(),
		Height:             int64(blockHeight),
		Time:               header.Time(),
		Txs:                data.Txs.ToSliceOfBytes(),
		ProposerAddress:    header.ProposerAddress,
		NextValidatorsHash: state.Validators.Hash(),
	})
	if err != nil {
		return fmt.Errorf("failed to finalize block at height %d: %w", blockHeight, err)
	}

	state.AppHash = resp.AppHash
	if err := c.blockExecutor.store.UpdateState(ctx, state); err != nil {
		return fmt.Errorf("failed to update state after finalizing block %d: %w", blockHeight, err)
	}

	c.blockExecutor.logger.Info("Block finalized", "height", blockHeight, "hash", header.Hash())

	return nil
}
