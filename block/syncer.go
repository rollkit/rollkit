package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"encoding/binary"

	goheaderstore "github.com/celestiaorg/go-header/store"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
	rollkitproto "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

// Syncer is responsible for syncing blocks from P2P and DA
type Syncer struct {
	eventBus         *events.Bus
	store            store.Store
	dalc             coreda.Client
	headerCache      *HeaderCache
	dataCache        *DataCache
	headerStore      *goheaderstore.Store[*types.SignedHeader]
	dataStore        *goheaderstore.Store[*types.Data]
	stateManager     *StateManager
	logger           Logger
	config           config.Config
	daHeight         atomic.Uint64
	daIncludedHeight atomic.Uint64
	stateMtx         *sync.RWMutex
	exec             coreexecutor.Executor

	// Channels for event coordination
	headerInCh    chan NewHeaderEvent
	dataInCh      chan NewDataEvent
	headerStoreCh chan struct{}
	dataStoreCh   chan struct{}
	retrieveCh    chan struct{}
}

// SyncerOptions contains options for creating a new Syncer
type SyncerOptions struct {
	EventBus         *events.Bus
	Store            store.Store
	DALC             coreda.Client
	HeaderStore      *goheaderstore.Store[*types.SignedHeader]
	DataStore        *goheaderstore.Store[*types.Data]
	Logger           Logger
	Config           config.Config
	DAHeight         uint64
	DAIncludedHeight uint64
	StateMtx         *sync.RWMutex
	Exec             coreexecutor.Executor
}

// NewSyncer creates a new block syncer
func NewSyncer(opts SyncerOptions) *Syncer {
	s := &Syncer{
		eventBus:      opts.EventBus,
		store:         opts.Store,
		dalc:          opts.DALC,
		headerCache:   NewHeaderCache(),
		dataCache:     NewDataCache(),
		headerStore:   opts.HeaderStore,
		dataStore:     opts.DataStore,
		logger:        opts.Logger,
		config:        opts.Config,
		stateMtx:      opts.StateMtx,
		exec:          opts.Exec,
		headerInCh:    make(chan NewHeaderEvent, headerInChLength),
		dataInCh:      make(chan NewDataEvent, headerInChLength),
		headerStoreCh: make(chan struct{}, 1),
		dataStoreCh:   make(chan struct{}, 1),
		retrieveCh:    make(chan struct{}, 1),
	}

	s.daHeight.Store(opts.DAHeight)
	s.daIncludedHeight.Store(opts.DAIncludedHeight)

	return s
}

// Start starts the syncer
func (s *Syncer) Start(ctx context.Context) error {
	// Subscribe to events
	s.eventBus.Subscribe(EventHeaderRetrieved, s.handleHeaderRetrieved)
	s.eventBus.Subscribe(EventDataRetrieved, s.handleDataRetrieved)
	s.eventBus.Subscribe(EventBlockDAIncluded, s.handleBlockDAIncluded)

	return nil
}

// SetDALC sets the DA layer client
func (s *Syncer) SetDALC(dalc coreda.Client) {
	s.dalc = dalc
}

// SetDAHeight sets the DA height
func (s *Syncer) SetDAHeight(height uint64) {
	s.daHeight.Store(height)
}

// GetDAIncludedHeight returns the DA included height
func (s *Syncer) GetDAIncludedHeight() uint64 {
	return s.daIncludedHeight.Load()
}

// GetHeaderInCh returns the header input channel
func (s *Syncer) GetHeaderInCh() chan NewHeaderEvent {
	return s.headerInCh
}

// GetDataInCh returns the data input channel
func (s *Syncer) GetDataInCh() chan NewDataEvent {
	return s.dataInCh
}

// IsBlockHashSeen returns whether a block hash has been seen
func (s *Syncer) IsBlockHashSeen(blockHash string) bool {
	return s.headerCache.IsSeen(blockHash)
}

// IsDAIncluded returns whether a block hash has been included in DA
func (s *Syncer) IsDAIncluded(hash types.Hash) bool {
	return s.headerCache.IsDAIncluded(hash.String())
}

// handleHeaderRetrieved handles HeaderRetrievedEvent
func (s *Syncer) handleHeaderRetrieved(ctx context.Context, evt events.Event) {
	headerEvent, ok := evt.(HeaderRetrievedEvent)
	if !ok {
		s.logger.Error("invalid event type", "expected", "HeaderRetrievedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	// Add header to cache
	header := headerEvent.Header
	headerHeight := header.Height()
	headerHash := header.Hash().String()

	if headerHeight <= s.store.Height() || s.headerCache.IsSeen(headerHash) {
		return
	}

	s.headerCache.SetItem(headerHeight, header)

	// Handle empty data hash
	s.handleEmptyDataHash(ctx, &header.Header)
}

// handleDataRetrieved handles DataRetrievedEvent
func (s *Syncer) handleDataRetrieved(ctx context.Context, evt events.Event) {
	dataEvent, ok := evt.(DataRetrievedEvent)
	if !ok {
		s.logger.Error("invalid event type", "expected", "DataRetrievedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	// Add data to cache
	data := dataEvent.Data
	dataHeight := data.Metadata.Height
	dataHash := data.Hash().String()

	if dataHeight <= s.store.Height() || s.dataCache.IsSeen(dataHash) {
		return
	}

	s.dataCache.SetItem(dataHeight, data)
}

// handleBlockDAIncluded handles BlockDAIncludedEvent
func (s *Syncer) handleBlockDAIncluded(ctx context.Context, evt events.Event) {
	daEvent, ok := evt.(BlockDAIncludedEvent)
	if !ok {
		s.logger.Error("invalid event type", "expected", "BlockDAIncludedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	// Mark block as DA included
	header := daEvent.Header
	headerHash := header.Hash().String()
	s.headerCache.SetDAIncluded(headerHash)

	// Update DA included height if needed
	if daEvent.Height > s.daIncludedHeight.Load() {
		s.setDAIncludedHeight(ctx, daEvent.Height)
	}
}

// setDAIncludedHeight updates the DA included height
func (s *Syncer) setDAIncludedHeight(ctx context.Context, newHeight uint64) error {
	for {
		currentHeight := s.daIncludedHeight.Load()
		if newHeight <= currentHeight {
			break
		}
		if s.daIncludedHeight.CompareAndSwap(currentHeight, newHeight) {
			heightBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBytes, newHeight)
			return s.store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		}
	}
	return nil
}

// RetrieveLoop is responsible for retrieving blocks from DA
func (s *Syncer) RetrieveLoop(ctx context.Context) {
	// Signal to check for new blocks immediately on startup
	s.sendNonBlockingSignalToRetrieveCh()

	// Set up a ticker for regular checks
	ticker := time.NewTicker(s.config.DA.BlockTime.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendNonBlockingSignalToRetrieveCh()
		case <-s.retrieveCh:
			if err := s.processNextDAHeader(ctx); err != nil && ctx.Err() == nil {
				// if the requested da height is not yet available, wait silently, otherwise log the error and wait
				if !strings.Contains(err.Error(), ErrHeightFromFutureStr) {
					s.logger.Error("failed to retrieve block from DALC",
						"daHeight", s.daHeight.Load(),
						"errors", err.Error())
				}
				continue
			}

			// Signal to try and retrieve the next block immediately
			s.sendNonBlockingSignalToRetrieveCh()

			// Update the DA height
			newHeight := s.daHeight.Load() + 1
			s.daHeight.Store(newHeight)

			// Publish DA height changed event
			s.eventBus.Publish(DAHeightChangedEvent{
				Height: newHeight,
			})
		}
	}
}

// HeaderStoreRetrieveLoop retrieves headers from the P2P network
func (s *Syncer) HeaderStoreRetrieveLoop(ctx context.Context) {
	lastHeaderStoreHeight := uint64(0)

	// Set up a ticker for regular checks
	ticker := time.NewTicker(s.config.Node.BlockTime.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendNonBlockingSignalToHeaderStoreCh()
		case <-s.headerStoreCh:
			headerStoreHeight := s.headerStore.Height()
			if headerStoreHeight > lastHeaderStoreHeight {
				headers, err := s.getHeadersFromHeaderStore(ctx, lastHeaderStoreHeight+1, headerStoreHeight)
				if err != nil {
					s.logger.Error("failed to get headers from Header Store",
						"lastHeaderHeight", lastHeaderStoreHeight,
						"headerStoreHeight", headerStoreHeight,
						"errors", err.Error())
					continue
				}

				daHeight := s.daHeight.Load()
				for _, header := range headers {
					// Check for context cancellation
					select {
					case <-ctx.Done():
						return
					default:
					}

					// Validate the header
					if !s.isValidHeader(header) {
						continue
					}

					s.logger.Debug("header retrieved from P2P",
						"headerHeight", header.Height(),
						"daHeight", daHeight)

					// Publish header retrieved event
					s.eventBus.Publish(HeaderRetrievedEvent{
						Header:   header,
						DAHeight: daHeight,
						Source:   "p2p",
					})
				}
			}
			lastHeaderStoreHeight = headerStoreHeight
		}
	}
}

// DataStoreRetrieveLoop retrieves data from the P2P network
func (s *Syncer) DataStoreRetrieveLoop(ctx context.Context) {
	lastDataStoreHeight := uint64(0)

	// Set up a ticker for regular checks
	ticker := time.NewTicker(s.config.Node.BlockTime.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendNonBlockingSignalToDataStoreCh()
		case <-s.dataStoreCh:
			dataStoreHeight := s.dataStore.Height()
			if dataStoreHeight > lastDataStoreHeight {
				data, err := s.getDataFromDataStore(ctx, lastDataStoreHeight+1, dataStoreHeight)
				if err != nil {
					s.logger.Error("failed to get data from Data Store",
						"lastDataStoreHeight", lastDataStoreHeight,
						"dataStoreHeight", dataStoreHeight,
						"errors", err.Error())
					continue
				}

				daHeight := s.daHeight.Load()
				for _, d := range data {
					// Check for context cancellation
					select {
					case <-ctx.Done():
						return
					default:
					}

					s.logger.Debug("data retrieved from P2P",
						"dataHeight", d.Metadata.Height,
						"daHeight", daHeight)

					// Publish data retrieved event
					s.eventBus.Publish(DataRetrievedEvent{
						Data:     d,
						DAHeight: daHeight,
						Source:   "p2p",
					})
				}
			}
			lastDataStoreHeight = dataStoreHeight
		}
	}
}

// trySyncNextBlock tries to execute as many blocks as possible from the cache
func (s *Syncer) trySyncNextBlock(ctx context.Context, daHeight uint64) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentHeight := s.store.Height()
		h := s.headerCache.GetItem(currentHeight + 1)
		if h == nil {
			s.logger.Debug("header not found in cache", "height", currentHeight+1)
			return nil
		}

		d := s.dataCache.GetItem(currentHeight + 1)
		if d == nil {
			s.logger.Debug("data not found in cache", "height", currentHeight+1)
			return nil
		}

		hHeight := h.Height()
		s.logger.Info("Syncing header and data", "height", hHeight)

		// Validate the received block before applying
		if err := s.validateBlock(ctx, h, d); err != nil {
			return fmt.Errorf("failed to validate block: %w", err)
		}

		// Get current state for applying block
		s.stateMtx.RLock()
		lastState := s.stateManager.GetLastState()
		s.stateMtx.RUnlock()

		// Apply block and update state
		rawTxs := make([][]byte, len(d.Txs))
		for i := range d.Txs {
			rawTxs[i] = d.Txs[i]
		}

		// Execute transactions to get new state root
		newStateRoot, _, err := s.exec.ExecuteTxs(ctx, rawTxs, h.Height(), h.Time(), lastState.AppHash)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			// if call to ExecuteTxs fails, we halt the node
			panic(fmt.Errorf("failed to ExecuteTxs: %w", err))
		}

		// Save the block to store
		err = s.store.SaveBlockData(ctx, h, d, &h.Signature)
		if err != nil {
			return SaveBlockError{err}
		}

		// Finalize the block
		err = s.exec.SetFinal(ctx, h.Height())
		if err != nil {
			return fmt.Errorf("failed to finalize block: %w", err)
		}

		// Create new state with updated block info
		newState := types.State{
			Version:         lastState.Version,
			ChainID:         lastState.ChainID,
			InitialHeight:   lastState.InitialHeight,
			LastBlockHeight: h.Height(),
			LastBlockTime:   h.Time(),
			AppHash:         newStateRoot,
			DAHeight:        daHeight,
		}

		// Update the store height
		s.store.SetHeight(ctx, hHeight)

		// Publish state updated event
		s.eventBus.Publish(StateUpdatedEvent{
			State:  newState,
			Height: hHeight,
		})

		s.headerCache.DeleteItem(currentHeight + 1)
		s.dataCache.DeleteItem(currentHeight + 1)
	}
}

// validateBlock validates a block before applying it
func (s *Syncer) validateBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) error {
	// This is a placeholder for validation logic
	// In a real implementation, this would validate the block against consensus rules

	if header.Height() != data.Metadata.Height {
		return fmt.Errorf("header height (%d) and data height (%d) do not match",
			header.Height(), data.Metadata.Height)
	}

	if !bytes.Equal(header.DataHash, data.Hash()) {
		return fmt.Errorf("data hash mismatch: header has %X, calculated %X",
			header.DataHash, data.Hash())
	}

	return nil
}

// isValidHeader checks if the header is valid
func (s *Syncer) isValidHeader(header *types.SignedHeader) bool {
	// Placeholder for validation logic
	// In a real implementation, this would validate the header against consensus rules
	return header.ValidateBasic() == nil
}

// handleEmptyDataHash handles headers with empty data hash
func (s *Syncer) handleEmptyDataHash(ctx context.Context, header *types.Header) {
	headerHeight := header.Height()
	if bytes.Equal(header.DataHash, dataHashForEmptyTxs) {
		var lastDataHash types.Hash
		var err error
		var lastData *types.Data

		if headerHeight > 1 {
			_, lastData, err = s.store.GetBlockData(ctx, headerHeight-1)
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
			s.dataCache.SetItem(headerHeight, d)
		}
	}
}

// decodeHeader decodes a header from bytes
func (s *Syncer) decodeHeader(bz []byte) (*types.SignedHeader, error) {
	header := new(types.SignedHeader)

	// Decode the header
	var headerPb rollkitproto.SignedHeader
	if err := headerPb.Unmarshal(bz); err != nil {
		return nil, fmt.Errorf("failed to unmarshal header: %w", err)
	}

	if err := header.FromProto(&headerPb); err != nil {
		return nil, fmt.Errorf("failed to convert header from proto: %w", err)
	}

	return header, nil
}

// fetchHeaders retrieves headers from the DA layer
func (s *Syncer) fetchHeaders(ctx context.Context, daHeight uint64) (coreda.ResultRetrieve, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
	defer cancel()

	retrieveRes := s.dalc.Retrieve(ctx, daHeight)
	if retrieveRes.Code == coreda.StatusError {
		return retrieveRes, fmt.Errorf("failed to retrieve block: %s", retrieveRes.Message)
	}

	return retrieveRes, nil
}

// getHeadersFromHeaderStore retrieves headers from the header store
func (s *Syncer) getHeadersFromHeaderStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.SignedHeader, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}

	headers := make([]*types.SignedHeader, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		header, err := s.headerStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		headers[i-startHeight] = header
	}

	return headers, nil
}

// getDataFromDataStore retrieves data from the data store
func (s *Syncer) getDataFromDataStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.Data, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}

	data := make([]*types.Data, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		d, err := s.dataStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		data[i-startHeight] = d
	}

	return data, nil
}

// sendNonBlockingSignalToRetrieveCh sends a signal to the retrieve channel without blocking
func (s *Syncer) sendNonBlockingSignalToRetrieveCh() {
	select {
	case s.retrieveCh <- struct{}{}:
	default:
	}
}

// sendNonBlockingSignalToHeaderStoreCh sends a signal to the header store channel without blocking
func (s *Syncer) sendNonBlockingSignalToHeaderStoreCh() {
	select {
	case s.headerStoreCh <- struct{}{}:
	default:
	}
}

// sendNonBlockingSignalToDataStoreCh sends a signal to the data store channel without blocking
func (s *Syncer) sendNonBlockingSignalToDataStoreCh() {
	select {
	case s.dataStoreCh <- struct{}{}:
	default:
	}
}

// SyncLoop processes retrieved headers and data
func (s *Syncer) SyncLoop(ctx context.Context) {
	// Subscribe to events to feed channels
	s.eventBus.Subscribe(EventHeaderRetrieved, func(ctx context.Context, evt events.Event) {
		headerEvent, ok := evt.(HeaderRetrievedEvent)
		if !ok {
			s.logger.Error("invalid event type", "expected", "HeaderRetrievedEvent", "got", fmt.Sprintf("%T", evt))
			return
		}
		s.headerInCh <- NewHeaderEvent{
			Header:   headerEvent.Header,
			DAHeight: headerEvent.DAHeight,
		}
	})

	s.eventBus.Subscribe(EventDataRetrieved, func(ctx context.Context, evt events.Event) {
		dataEvent, ok := evt.(DataRetrievedEvent)
		if !ok {
			s.logger.Error("invalid event type", "expected", "DataRetrievedEvent", "got", fmt.Sprintf("%T", evt))
			return
		}
		s.dataInCh <- NewDataEvent{
			Data:     dataEvent.Data,
			DAHeight: dataEvent.DAHeight,
		}
	})

	for {
		select {
		case <-ctx.Done():
			return
		case headerEvent := <-s.headerInCh:
			// Process header
			header := headerEvent.Header
			daHeight := headerEvent.DAHeight
			headerHash := header.Hash().String()
			headerHeight := header.Height()

			s.logger.Debug("processing header",
				"height", headerHeight,
				"daHeight", daHeight,
				"hash", headerHash,
			)

			if headerHeight <= s.store.Height() || s.headerCache.IsSeen(headerHash) {
				s.logger.Debug("header already seen", "height", headerHeight, "hash", headerHash)
				continue
			}

			s.headerCache.SetItem(headerHeight, header)

			// Check if the data hash is for empty txs
			s.handleEmptyDataHash(ctx, &header.Header)

			if err := s.trySyncNextBlock(ctx, daHeight); err != nil {
				s.logger.Info("failed to sync next block", "error", err)
				continue
			}

			s.headerCache.SetSeen(headerHash)

		case dataEvent := <-s.dataInCh:
			// Process data
			data := dataEvent.Data
			daHeight := dataEvent.DAHeight
			dataHash := data.Hash().String()
			dataHeight := data.Metadata.Height

			s.logger.Debug("processing data",
				"height", dataHeight,
				"daHeight", daHeight,
				"hash", dataHash,
			)

			if dataHeight <= s.store.Height() || s.dataCache.IsSeen(dataHash) {
				s.logger.Debug("data already seen", "height", dataHeight, "hash", dataHash)
				continue
			}

			s.dataCache.SetItem(dataHeight, data)

			if err := s.trySyncNextBlock(ctx, daHeight); err != nil {
				s.logger.Info("failed to sync next block", "error", err)
				continue
			}

			s.dataCache.SetSeen(dataHash)
		}
	}
}

// processNextDAHeader retrieves and processes the next header from DA
func (s *Syncer) processNextDAHeader(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// TODO(tzdybal): extract configuration option
	maxRetries := 10
	daHeight := s.daHeight.Load()

	var err error
	s.logger.Debug("trying to retrieve block from DA", "daHeight", daHeight)

	for r := 0; r < maxRetries; r++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		headerResp, fetchErr := s.fetchHeaders(ctx, daHeight)
		if fetchErr == nil {
			if headerResp.Code == coreda.StatusNotFound {
				s.logger.Debug("no header found", "daHeight", daHeight, "reason", headerResp.Message)
				return nil
			}

			s.logger.Debug("retrieved potential headers", "n", len(headerResp.Data), "daHeight", daHeight)
			for _, bz := range headerResp.Data {
				header, decodeErr := s.decodeHeader(bz)
				if decodeErr != nil {
					s.logger.Error("failed to decode header", "error", decodeErr)
					continue
				}

				// Validate the header
				if !s.isValidHeader(header) {
					continue
				}

				blockHash := header.Hash().String()
				s.headerCache.SetDAIncluded(blockHash)

				// Update DA included height if needed
				if header.Height() > s.daIncludedHeight.Load() {
					s.setDAIncludedHeight(ctx, header.Height())
				}

				s.logger.Info("block marked as DA included", "blockHeight", header.Height(), "blockHash", blockHash)

				if !s.headerCache.IsSeen(blockHash) {
					// Check for context cancellation
					select {
					case <-ctx.Done():
						return fmt.Errorf("unable to process header, context done: %w", ctx.Err())
					default:
					}

					// Publish header retrieved event
					s.eventBus.Publish(HeaderRetrievedEvent{
						Header:   header,
						DAHeight: daHeight,
						Source:   "da",
					})
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
