package block

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rollkit/go-sequencing"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
)

func (m *Manager) publishBlock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if !m.isProposer {
		return ErrNotProposer
	}

	if m.config.Node.MaxPendingBlocks != 0 && m.pendingHeaders.numPendingHeaders() >= m.config.Node.MaxPendingBlocks {
		return fmt.Errorf("refusing to create block: pending blocks [%d] reached limit [%d]",
			m.pendingHeaders.numPendingHeaders(), m.config.Node.MaxPendingBlocks)
	}

	var (
		lastSignature  *types.Signature
		lastHeaderHash types.Hash
		lastDataHash   types.Hash
		lastHeaderTime time.Time
		err            error
	)
	height := m.store.Height()
	newHeight := height + 1
	// this is a special case, when first block is produced - there is no previous commit
	if newHeight <= m.genesis.InitialHeight {
		// Special handling for genesis block
		lastSignature = &types.Signature{}
	} else {
		lastSignature, err = m.store.GetSignature(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last commit: %w", err)
		}
		lastHeader, lastData, err := m.store.GetBlockData(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last block: %w", err)
		}
		lastHeaderHash = lastHeader.Hash()
		lastDataHash = lastData.Hash()
		lastHeaderTime = lastHeader.Time()
	}

	var (
		header    *types.SignedHeader
		data      *types.Data
		signature types.Signature
	)

	// Check if there's an already stored block at a newer height
	// If there is use that instead of creating a new block
	pendingHeader, pendingData, err := m.store.GetBlockData(ctx, newHeight)
	if err == nil {
		m.logger.Info("Using pending block", "height", newHeight)
		header = pendingHeader
		data = pendingData
	} else {
		execTxs, err := m.exec.GetTxs(ctx)
		if err != nil {
			m.logger.Error("failed to get txs from executor", "err", err)
			// Continue but log the state
			m.logger.Info("Current state",
				"height", height,
				"isProposer", m.isProposer,
				"pendingHeaders", m.pendingHeaders.numPendingHeaders())
		}

		m.logger.Debug("Submitting transaction to sequencer",
			"txCount", len(execTxs))
		_, err = m.sequencer.SubmitRollupBatchTxs(ctx, coresequencer.SubmitRollupBatchTxsRequest{
			RollupId: sequencing.RollupId(m.genesis.ChainID),
			Batch:    &coresequencer.Batch{Transactions: execTxs},
		})
		if err != nil {
			m.logger.Error("failed to submit rollup transaction to sequencer",
				"err", err,
				"chainID", m.genesis.ChainID)
			// Add retry logic or proper error handling

			m.logger.Debug("Successfully submitted transaction to sequencer")
		}

		batchData, err := m.getTxsFromBatch()
		if errors.Is(err, ErrNoBatch) {
			m.logger.Debug("No batch available, creating empty block")
			// Create an empty block instead of returning
			batchData = &BatchData{
				Batch: &coresequencer.Batch{Transactions: [][]byte{}},
				Time:  time.Now().Round(0).UTC(),
				Data:  [][]byte{},
			}
		} else if err != nil {
			return fmt.Errorf("failed to get transactions from batch: %w", err)
		}
		// sanity check timestamp for monotonically increasing
		if batchData.Time.Before(lastHeaderTime) {
			return fmt.Errorf("timestamp is not monotonically increasing: %s < %s", batchData.Time, m.getLastBlockTime())
		}
		m.logger.Info("Creating and publishing block", "height", newHeight)
		header, data, err = m.createBlock(ctx, newHeight, lastSignature, lastHeaderHash, batchData)
		if err != nil {
			return err
		}
		m.logger.Debug("block info", "num_tx", len(data.Txs))

		/*
		   here we set the SignedHeader.DataHash, and SignedHeader.Signature as a hack
		   to make the block pass ValidateBasic() when it gets called by applyBlock on line 681
		   these values get overridden on lines 687-698 after we obtain the IntermediateStateRoots.
		*/
		header.DataHash = data.Hash()
		//header.Validators = m.getLastStateValidators()
		//header.ValidatorHash = header.Validators.Hash()

		signature, err = m.getSignature(header.Header)
		if err != nil {
			return err
		}

		// set the signature to current block's signed header
		header.Signature = signature
		err = m.store.SaveBlockData(ctx, header, data, &signature)
		if err != nil {
			return SaveBlockError{err}
		}
	}

	newState, err := m.applyBlock(ctx, header, data)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		// if call to applyBlock fails, we halt the node, see https://github.com/cometbft/cometbft/pull/496
		panic(err)
	}
	// Before taking the hash, we need updated ISRs, hence after ApplyBlock
	header.Header.DataHash = data.Hash()

	signature, err = m.getSignature(header.Header)
	if err != nil {
		return err
	}

	// set the signature to current block's signed header
	header.Signature = signature

	// append metadata to Data before validating and saving
	data.Metadata = &types.Metadata{
		ChainID:      header.ChainID(),
		Height:       header.Height(),
		Time:         header.BaseHeader.Time,
		LastDataHash: lastDataHash,
	}
	// Validate the created block before storing
	if err := m.execValidate(m.lastState, header, data); err != nil {
		return fmt.Errorf("failed to validate block: %w", err)
	}

	headerHeight := header.Height()

	headerHash := header.Hash().String()
	m.headerCache.SetSeen(headerHash)

	// SaveBlock commits the DB tx
	err = m.store.SaveBlockData(ctx, header, data, &signature)
	if err != nil {
		return SaveBlockError{err}
	}

	// Commit the new state and block which writes to disk on the proxy app
	appHash, err := m.execCommit(ctx, newState, header, data)
	if err != nil {
		return err
	}
	// Update app hash in state
	newState.AppHash = appHash

	// Update the store height before submitting to the DA layer but after committing to the DB
	m.store.SetHeight(ctx, headerHeight)

	newState.DAHeight = atomic.LoadUint64(&m.daHeight)
	// After this call m.lastState is the NEW state returned from ApplyBlock
	// updateState also commits the DB tx
	m.logger.Debug("updating state", "newState", newState)
	err = m.updateState(ctx, newState)
	if err != nil {
		return err
	}
	m.recordMetrics(data)
	// Check for shut down event prior to sending the header and block to
	// their respective channels. The reason for checking for the shutdown
	// event separately is due to the inconsistent nature of the select
	// statement when multiple cases are satisfied.
	select {
	case <-ctx.Done():
		return fmt.Errorf("unable to send header and block, context done: %w", ctx.Err())
	default:
	}

	// Publish header to channel so that header exchange service can broadcast
	m.HeaderCh <- header

	// Publish block to channel so that block exchange service can broadcast
	m.DataCh <- data

	m.logger.Debug("successfully proposed header", "proposer", hex.EncodeToString(header.ProposerAddress), "height", headerHeight)

	return nil
}

func (m *Manager) createBlock(ctx context.Context, height uint64, lastSignature *types.Signature, lastHeaderHash types.Hash, batchData *BatchData) (*types.SignedHeader, *types.Data, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.execCreateBlock(ctx, height, lastSignature, lastHeaderHash, m.lastState, batchData)
}

func (m *Manager) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) (types.State, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.execApplyBlock(ctx, m.lastState, header, data)
}

func (m *Manager) execValidate(_ types.State, _ *types.SignedHeader, _ *types.Data) error {
	// TODO(tzdybal): implement
	return nil
}

func (m *Manager) execCommit(ctx context.Context, newState types.State, h *types.SignedHeader, _ *types.Data) ([]byte, error) {
	err := m.exec.SetFinal(ctx, h.Height())
	return newState.AppHash, err
}

func (m *Manager) execCreateBlock(_ context.Context, height uint64, lastSignature *types.Signature, lastHeaderHash types.Hash, lastState types.State, batchData *BatchData) (*types.SignedHeader, *types.Data, error) {
	data := batchData.Data
	batchdata := convertBatchDataToBytes(data)
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: lastState.Version.Block,
				App:   lastState.Version.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: lastState.ChainID,
				Height:  height,
				Time:    uint64(batchData.Time.UnixNano()), //nolint:gosec // why is time unix? (tac0turtle)
			},
			LastHeaderHash:  lastHeaderHash,
			DataHash:        batchdata,
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         lastState.AppHash,
			ProposerAddress: m.genesis.ProposerAddress(),
		},
		Signature: *lastSignature,
	}

	blockData := &types.Data{
		Txs: make(types.Txs, len(batchData.Batch.Transactions)),
	}
	for i := range batchData.Batch.Transactions {
		blockData.Txs[i] = types.Tx(batchData.Batch.Transactions[i])
	}

	return header, blockData, nil
}

func (m *Manager) execApplyBlock(ctx context.Context, lastState types.State, header *types.SignedHeader, data *types.Data) (types.State, error) {
	rawTxs := make([][]byte, len(data.Txs))
	for i := range data.Txs {
		rawTxs[i] = data.Txs[i]
	}
	newStateRoot, _, err := m.exec.ExecuteTxs(ctx, rawTxs, header.Height(), header.Time(), lastState.AppHash)
	if err != nil {
		return types.State{}, err
	}

	s, err := m.nextState(lastState, header, newStateRoot)
	if err != nil {
		return types.State{}, err
	}

	return s, nil
}

func (m *Manager) recordMetrics(data *types.Data) {
	m.metrics.NumTxs.Set(float64(len(data.Txs)))
	m.metrics.TotalTxs.Add(float64(len(data.Txs)))
	m.metrics.BlockSizeBytes.Set(float64(data.Size()))
	m.metrics.CommittedHeight.Set(float64(data.Metadata.Height))
}
