package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	"google.golang.org/protobuf/proto"
)

// SyncLoop is responsible for syncing blocks.
//
// SyncLoop processes headers gossiped in P2P network to know what's the latest block height,
// block data is retrieved from DA layer.
func (m *Manager) SyncLoop(ctx context.Context) {
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
				"height", headerHash,
				"daHeight", daHeight,
				"hash", headerHash,
			)
			if headerHeight <= m.store.Height() || m.headerCache.IsSeen(headerHash) {
				m.logger.Debug("header already seen", "height", headerHeight, "block hash", headerHash)
				continue
			}
			m.headerCache.SetItem(headerHeight, header)

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
			m.headerCache.SetSeen(headerHash)
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
			if dataHeight <= m.store.Height() || m.dataCache.IsSeen(dataHash) {
				m.logger.Debug("data already seen", "height", dataHeight, "data hash", dataHash)
				continue
			}
			m.dataCache.SetItem(dataHeight, data)

			m.sendNonBlockingSignalToDataStoreCh()

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
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
		currentHeight := m.store.Height()
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
		m.logger.Info("Syncing header and data", "height", hHeight)
		// Validate the received block before applying
		if err := m.execValidate(m.lastState, h, d); err != nil {
			return fmt.Errorf("failed to validate block: %w", err)
		}
		newState, err := m.applyBlock(ctx, h, d)
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
		_, err = m.execCommit(ctx, newState, h, d)
		if err != nil {
			return fmt.Errorf("failed to Commit: %w", err)
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
		m.headerCache.DeleteItem(currentHeight + 1)
		m.dataCache.DeleteItem(currentHeight + 1)
	}
}

func (m *Manager) handleEmptyDataHash(ctx context.Context, header *types.Header) {
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
	}
}

func (m *Manager) processNextDAHeader(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// TODO(tzdybal): extract configuration option
	maxRetries := 10
	daHeight := atomic.LoadUint64(&m.daHeight)

	var err error
	m.logger.Debug("trying to retrieve block from DA", "daHeight", daHeight)
	for r := 0; r < maxRetries; r++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		headerResp, fetchErr := m.fetchHeaders(ctx, daHeight)
		if fetchErr == nil {
			if headerResp.Code == coreda.StatusNotFound {
				m.logger.Debug("no header found", "daHeight", daHeight, "reason", headerResp.Message)
				return nil
			}
			m.logger.Debug("retrieved potential headers", "n", len(headerResp.Data), "daHeight", daHeight)
			for _, bz := range headerResp.Data {
				header := new(types.SignedHeader)
				// decode the header
				var headerPb pb.SignedHeader
				err := proto.Unmarshal(bz, &headerPb)
				if err != nil {
					m.logger.Error("failed to unmarshal header", "error", err)
					continue
				}
				err = header.FromProto(&headerPb)
				if err != nil {
					m.logger.Error("failed to unmarshal header", "error", err)
					continue
				}
				// early validation to reject junk headers
				if !m.isUsingExpectedCentralizedSequencer(header) {
					m.logger.Debug("skipping header from unexpected sequencer",
						"headerHeight", header.Height(),
						"headerHash", header.Hash().String())
					continue
				}
				blockHash := header.Hash().String()
				m.headerCache.SetDAIncluded(blockHash)
				err = m.setDAIncludedHeight(ctx, header.Height())
				if err != nil {
					return err
				}
				m.logger.Info("block marked as DA included", "blockHeight", header.Height(), "blockHash", blockHash)
				if !m.headerCache.IsSeen(blockHash) {
					// Check for shut down event prior to logging
					// and sending block to blockInCh. The reason
					// for checking for the shutdown event
					// separately is due to the inconsistent nature
					// of the select statement when multiple cases
					// are satisfied.
					select {
					case <-ctx.Done():
						return fmt.Errorf("unable to send block to blockInCh, context done: %w", ctx.Err())
					default:
					}
					m.headerInCh <- NewHeaderEvent{header, daHeight}
				}
			}
			return nil
		}

		// Track the error
		err = errors.Join(err, fetchErr)
		// Delay before retrying
		select {
		case <-ctx.Done():
			return err
		case <-time.After(100 * time.Millisecond):
		}
	}
	return err
}

// HeaderStoreRetrieveLoop is responsible for retrieving headers from the Header Store.
func (m *Manager) HeaderStoreRetrieveLoop(ctx context.Context) {
	lastHeaderStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.headerStoreCh:
		}
		headerStoreHeight := m.headerStore.Height()
		if headerStoreHeight > lastHeaderStoreHeight {
			headers, err := m.getHeadersFromHeaderStore(ctx, lastHeaderStoreHeight+1, headerStoreHeight)
			if err != nil {
				m.logger.Error("failed to get headers from Header Store", "lastHeaderHeight", lastHeaderStoreHeight, "headerStoreHeight", headerStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.daHeight)
			for _, header := range headers {
				// Check for shut down event prior to logging
				// and sending header to headerInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				// early validation to reject junk headers
				if !m.isUsingExpectedCentralizedSequencer(header) {
					continue
				}
				m.logger.Debug("header retrieved from p2p header sync", "headerHeight", header.Height(), "daHeight", daHeight)
				m.headerInCh <- NewHeaderEvent{header, daHeight}
			}
		}
		lastHeaderStoreHeight = headerStoreHeight
	}
}

// DataStoreRetrieveLoop is responsible for retrieving data from the Data Store.
func (m *Manager) DataStoreRetrieveLoop(ctx context.Context) {
	lastDataStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.dataStoreCh:
		}
		dataStoreHeight := m.dataStore.Height()
		if dataStoreHeight > lastDataStoreHeight {
			data, err := m.getDataFromDataStore(ctx, lastDataStoreHeight+1, dataStoreHeight)
			if err != nil {
				m.logger.Error("failed to get data from Data Store", "lastDataStoreHeight", lastDataStoreHeight, "dataStoreHeight", dataStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.daHeight)
			for _, d := range data {
				// Check for shut down event prior to logging
				// and sending header to dataInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				//TODO: remove junk if possible
				m.logger.Debug("data retrieved from p2p data sync", "dataHeight", d.Metadata.Height, "daHeight", daHeight)
				m.dataInCh <- NewDataEvent{d, daHeight}
			}
		}
		lastDataStoreHeight = dataStoreHeight
	}
}

func (m *Manager) getHeadersFromHeaderStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.SignedHeader, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	headers := make([]*types.SignedHeader, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		header, err := m.headerStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		headers[i-startHeight] = header
	}
	return headers, nil
}

func (m *Manager) getDataFromDataStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.Data, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	data := make([]*types.Data, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		d, err := m.dataStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		data[i-startHeight] = d
	}
	return data, nil
}

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {
	// blockFoundCh is used to track when we successfully found a block so
	// that we can continue to try and find blocks that are in the next DA height.
	// This enables syncing faster than the DA block time.
	headerFoundCh := make(chan struct{}, 1)
	defer close(headerFoundCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:
		case <-headerFoundCh:
		}
		daHeight := atomic.LoadUint64(&m.daHeight)
		err := m.processNextDAHeader(ctx)
		if err != nil && ctx.Err() == nil {
			// if the requested da height is not yet available, wait silently, otherwise log the error and wait
			if !errors.Is(err, ErrHeightFromFutureStr) {
				m.logger.Error("failed to retrieve block from DALC", "daHeight", daHeight, "errors", err.Error())
			}
			continue
		}
		// Signal the headerFoundCh to try and retrieve the next block
		select {
		case headerFoundCh <- struct{}{}:
		default:
		}
		atomic.AddUint64(&m.daHeight, 1)
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
