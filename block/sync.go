package block

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/rollkit/rollkit/types"
)

// SyncLoop is responsible for syncing blocks.
//
// SyncLoop processes headers gossiped in P2P network to know what's the latest block height, block data is retrieved from DA layer.
func (m *Manager) SyncLoop(ctx context.Context, errCh chan<- error) {
	daTicker := time.NewTicker(m.config.DA.BlockTime.Duration)
	defer daTicker.Stop()
	blockTicker := time.NewTicker(m.config.Node.BlockTime.Duration)
	defer blockTicker.Stop()
	for {
		select {
		case <-daTicker.C:
			m.sendNonBlockingSignalToRetrieveCh()
		case <-blockTicker.C:
			m.sendNonBlockingSignalToHeaderStoreCh()
			m.sendNonBlockingSignalToDataStoreCh()
		case headerEvent := <-m.headerInCh:
			// Only validated headers are sent to headerInCh, so we can safely assume that headerEvent.header is valid
			header := headerEvent.Header
			daHeight := headerEvent.DAHeight
			headerHash := header.Hash().String()
			headerHeight := header.Height()
			m.logger.Debug("header retrieved",
				"height", headerHeight,
				"daHeight", daHeight,
				"hash", headerHash,
			)
			height, err := m.store.Height(ctx)
			if err != nil {
				m.logger.Error("error while getting store height", "error", err)
				continue
			}
			if headerHeight <= height || m.headerCache.IsSeen(headerHash) {
				m.logger.Debug("header already seen", "height", headerHeight, "block hash", headerHash)
				continue
			}
			m.headerCache.SetItem(headerHeight, header)

			m.sendNonBlockingSignalToHeaderStoreCh()
			m.sendNonBlockingSignalToRetrieveCh()

			// check if the dataHash is dataHashForEmptyTxs
			// no need to wait for syncing Data, instead prepare now and set
			// so that trySyncNextBlock can progress
			if !m.handleEmptyDataHash(ctx, &header.Header) {
				dataHashStr := header.DataHash.String()
				data := m.dataCache.GetItemByHash(dataHashStr)
				if data != nil {
					m.dataCache.SetItem(headerHeight, data)
				} else {
					m.dataCommitmentToHeight.LoadOrStore(dataHashStr, headerHeight)
				}
			}

			if err = m.trySyncNextBlock(ctx, daHeight); err != nil {
				errCh <- fmt.Errorf("failed to sync next block: %w", err)
				return
			}

			m.headerCache.SetSeen(headerHash)
		case dataEvent := <-m.dataInCh:
			data := dataEvent.Data
			daHeight := dataEvent.DAHeight
			dataHash := data.DACommitment().String()
			m.logger.Debug("data retrieved",
				"daHeight", daHeight,
				"hash", dataHash,
			)
			if m.dataCache.IsSeen(dataHash) {
				m.logger.Debug("data already seen", "data hash", dataHash)
				continue
			}
			height, err := m.store.Height(ctx)
			if err != nil {
				m.logger.Error("error while getting store height", "error", err)
				continue
			}
			if data.Metadata != nil {
				// Data was sent via the P2P network
				dataHeight := data.Metadata.Height
				if dataHeight <= height {
					m.logger.Debug("data already seen", "height", dataHeight, "data hash", dataHash)
					continue
				}
				m.dataCache.SetItem(dataHeight, data)
				m.dataCache.SetItemByHash(dataHash, data)
			}
			// If the header is synced already, the data commitment should be associated with a height
			if val, ok := m.dataCommitmentToHeight.Load(dataHash); ok {
				dataHeight := val.(uint64)
				m.dataCommitmentToHeight.Delete(dataHash)
				if dataHeight <= height {
					m.logger.Debug("data already seen", "height", dataHeight, "data hash",
						dataHash)
					continue
				}
				m.dataCache.SetItem(dataHeight, data)
			}

			m.dataCache.SetItemByHash(dataHash, data)

			m.sendNonBlockingSignalToDataStoreCh()
			m.sendNonBlockingSignalToRetrieveCh()

			err = m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				errCh <- fmt.Errorf("failed to sync next block: %w", err)
				return
			}
			m.dataCache.SetSeen(dataHash)
		case <-ctx.Done():
			return
		}
	}
}

// trySyncNextBlock tries to execute as many blocks as possible from the blockCache.
//
//	Note: the blockCache contains only valid blocks that are not yet synced
//
// For every block, to be able to apply block at height h, we need to have its Commit. It is contained in block at height h+1.
// If commit for block h+1 is available, we proceed with sync process, and remove synced block from sync cache.
func (m *Manager) trySyncNextBlock(ctx context.Context, daHeight uint64) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		currentHeight, err := m.store.Height(ctx)
		if err != nil {
			return err
		}
		h := m.headerCache.GetItem(currentHeight + 1)
		if h == nil {
			m.logger.Debug("header not found in cache", "height", currentHeight+1)
			return nil
		}
		d := m.dataCache.GetItem(currentHeight + 1)
		if d == nil {
			m.logger.Debug("data not found in cache", "height", currentHeight+1)
			return nil
		}

		hHeight := h.Height()
		m.logger.Info("syncing header and data", "height", hHeight)
		// Validate the received block before applying
		if err := m.Validate(ctx, h, d); err != nil {
			return fmt.Errorf("failed to validate block: %w", err)
		}

		newState, err := m.applyBlock(ctx, h, d)
		if err != nil {
			return fmt.Errorf("failed to apply block: %w", err)
		}

		if err = m.store.SaveBlockData(ctx, h, d, &h.Signature); err != nil {
			return fmt.Errorf("failed to save block: %w", err)
		}

		// Height gets updated
		if err = m.store.SetHeight(ctx, hHeight); err != nil {
			return err
		}

		if daHeight > newState.DAHeight {
			newState.DAHeight = daHeight
		}

		if err = m.updateState(ctx, newState); err != nil {
			return fmt.Errorf("failed to save updated state: %w", err)
		}

		m.headerCache.DeleteItem(currentHeight + 1)
		m.dataCache.DeleteItem(currentHeight + 1)
		m.dataCache.DeleteItemByHash(h.DataHash.String())
		if !bytes.Equal(h.DataHash, dataHashForEmptyTxs) {
			m.dataCache.SetSeen(h.DataHash.String())
		}
		m.headerCache.SetSeen(h.Hash().String())
	}
}

func (m *Manager) handleEmptyDataHash(ctx context.Context, header *types.Header) bool {
	headerHeight := header.Height()
	if bytes.Equal(header.DataHash, dataHashForEmptyTxs) {
		var lastDataHash types.Hash
		var err error
		var lastData *types.Data
		if headerHeight > 1 {
			_, lastData, err = m.store.GetBlockData(ctx, headerHeight-1)
			if lastData != nil {
				lastDataHash = lastData.Hash()
			}
		}
		// if no error then populate data, otherwise just skip and wait for Data to be synced
		if err == nil {
			metadata := &types.Metadata{
				ChainID:      header.ChainID(),
				Height:       headerHeight,
				Time:         header.BaseHeader.Time,
				LastDataHash: lastDataHash,
			}
			d := &types.Data{
				Metadata: metadata,
			}
			m.dataCache.SetItem(headerHeight, d)
		}
		return true
	}
	return false
}

func (m *Manager) sendNonBlockingSignalToHeaderStoreCh() {
	select {
	case m.headerStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToDataStoreCh() {
	select {
	case m.dataStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToRetrieveCh() {
	select {
	case m.retrieveCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToDAIncluderCh() {
	select {
	case m.daIncluderCh <- struct{}{}:
	default:
	}
}

// Updates the state stored in manager's store along the manager's lastState
func (m *Manager) updateState(ctx context.Context, s types.State) error {
	m.logger.Debug("updating state", "newState", s)
	m.lastStateMtx.Lock()
	defer m.lastStateMtx.Unlock()
	err := m.store.UpdateState(ctx, s)
	if err != nil {
		return err
	}
	m.lastState = s
	m.metrics.Height.Set(float64(s.LastBlockHeight))
	return nil
}
