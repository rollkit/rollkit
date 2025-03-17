package block

import (
	"context"
	"fmt"
	"time"
)

// SyncLoop is responsible for syncing blocks.
//
// SyncLoop processes headers gossiped in P2P network to know what's the latest block height,
// block data is retrieved from DA layer.
func (m *Manager) SyncLoop(ctx context.Context) {
	daTicker := time.NewTicker(m.config.DA.BlockTime)
	defer daTicker.Stop()
	blockTicker := time.NewTicker(m.config.Node.BlockTime)
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
				"height", headerHash,
				"daHeight", daHeight,
				"hash", headerHash,
			)
			if headerHeight <= m.store.Height() || m.headerCache.isSeen(headerHash) {
				m.logger.Debug("header already seen", "height", headerHeight, "block hash", headerHash)
				continue
			}
			m.headerCache.setHeader(headerHeight, header)

			m.sendNonBlockingSignalToHeaderStoreCh()
			m.sendNonBlockingSignalToRetrieveCh()

			// check if the dataHash is dataHashForEmptyTxs
			// no need to wait for syncing Data, instead prepare now and set
			// so that trySyncNextBlock can progress
			m.handleEmptyDataHash(ctx, &header.Header)

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.headerCache.setSeen(headerHash)
		case dataEvent := <-m.dataInCh:
			data := dataEvent.Data
			daHeight := dataEvent.DAHeight
			dataHash := data.Hash().String()
			dataHeight := data.Metadata.Height
			m.logger.Debug("data retrieved",
				"height", dataHash,
				"daHeight", daHeight,
				"hash", dataHash,
			)
			if dataHeight <= m.store.Height() || m.dataCache.isSeen(dataHash) {
				m.logger.Debug("data already seen", "height", dataHeight, "data hash", dataHash)
				continue
			}
			m.dataCache.setData(dataHeight, data)

			m.sendNonBlockingSignalToDataStoreCh()

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.dataCache.setSeen(dataHash)
		case <-ctx.Done():
			return
		}
	}
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
		currentHeight := m.store.Height()
		h := m.headerCache.getHeader(currentHeight + 1)
		if h == nil {
			m.logger.Debug("header not found in cache", "height", currentHeight+1)
			return nil
		}
		d := m.dataCache.getData(currentHeight + 1)
		if d == nil {
			m.logger.Debug("data not found in cache", "height", currentHeight+1)
			return nil
		}

		hHeight := h.Height()
		m.logger.Info("Syncing header and data", "height", hHeight)
		// Validate the received block before applying
		if err := m.execValidate(m.lastState, h, d); err != nil {
			return fmt.Errorf("failed to validate block: %w", err)
		}
		newState, responses, err := m.applyBlock(ctx, h, d)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			// if call to applyBlock fails, we halt the node, see https://github.com/cometbft/cometbft/pull/496
			panic(fmt.Errorf("failed to ApplyBlock: %w", err))
		}
		err = m.store.SaveBlockData(ctx, h, d, &h.Signature)
		if err != nil {
			return SaveBlockError{err}
		}
		_, err = m.execCommit(ctx, newState, h, d, responses)
		if err != nil {
			return fmt.Errorf("failed to Commit: %w", err)
		}

		err = m.store.SaveBlockResponses(ctx, hHeight, responses)
		if err != nil {
			return SaveBlockResponsesError{err}
		}

		// Height gets updated
		m.store.SetHeight(ctx, hHeight)

		if daHeight > newState.DAHeight {
			newState.DAHeight = daHeight
		}
		err = m.updateState(ctx, newState)
		if err != nil {
			m.logger.Error("failed to save updated state", "error", err)
		}
		m.headerCache.deleteHeader(currentHeight + 1)
		m.dataCache.deleteData(currentHeight + 1)
	}
}
