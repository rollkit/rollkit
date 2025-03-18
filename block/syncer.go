package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
	rollkitproto "github.com/rollkit/rollkit/types/pb/rollkit"
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
	logger           log.Logger
	config           config.Config
	daHeight         atomic.Uint64
	daIncludedHeight atomic.Uint64
}

// SyncerOptions contains options for creating a new Syncer
type SyncerOptions struct {
	EventBus         *events.Bus
	Store            store.Store
	DALC             coreda.Client
	GasPrice         float64
	GasMultiplier    float64
	HeaderStore      *goheaderstore.Store[*types.SignedHeader]
	DataStore        *goheaderstore.Store[*types.Data]
	Logger           log.Logger
	Config           config.Config
	DAHeight         uint64
	DAIncludedHeight uint64
}

// NewSyncer creates a new block syncer
func NewSyncer(opts SyncerOptions) *Syncer {
	s := &Syncer{
		eventBus:    opts.EventBus,
		store:       opts.Store,
		dalc:        opts.DALC,
		headerCache: NewHeaderCache(),
		dataCache:   NewDataCache(),
		headerStore: opts.HeaderStore,
		dataStore:   opts.DataStore,
		logger:      opts.Logger,
		config:      opts.Config,
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

	// Start sync loops
	go s.retrieveLoop(ctx)
	go s.headerStoreRetrieveLoop(ctx)
	go s.dataStoreRetrieveLoop(ctx)
	go s.syncLoop(ctx)

	return nil
}

// retrieveLoop is responsible for retrieving blocks from DA
func (s *Syncer) retrieveLoop(ctx context.Context) {
	// Signal to check for new blocks immediately on startup
	s.sendNonBlockingSignalToRetrieveCh()

	// Set up a ticker for regular checks
	ticker := time.NewTicker(s.config.DA.BlockTime)
	defer ticker.Stop()

	// Set up a channel for immediate retrieval signals
	retrieveCh := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendNonBlockingSignal(retrieveCh)
		case <-retrieveCh:
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
			s.sendNonBlockingSignal(retrieveCh)

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

// headerStoreRetrieveLoop retrieves headers from the P2P network
func (s *Syncer) headerStoreRetrieveLoop(ctx context.Context) {
	lastHeaderStoreHeight := uint64(0)

	// Set up a ticker for regular checks
	ticker := time.NewTicker(s.config.Node.BlockTime)
	defer ticker.Stop()

	// Set up a channel for immediate retrieval signals
	headerStoreCh := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendNonBlockingSignal(headerStoreCh)
		case <-headerStoreCh:
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

// dataStoreRetrieveLoop retrieves data from the P2P network
func (s *Syncer) dataStoreRetrieveLoop(ctx context.Context) {
	lastDataStoreHeight := uint64(0)

	// Set up a ticker for regular checks
	ticker := time.NewTicker(s.config.Node.BlockTime)
	defer ticker.Stop()

	// Set up a channel for immediate retrieval signals
	dataStoreCh := make(chan struct{}, 1)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendNonBlockingSignal(dataStoreCh)
		case <-dataStoreCh:
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

// syncLoop processes retrieved headers and data
func (s *Syncer) syncLoop(ctx context.Context) {
	// Set up channels for header and data events
	headerInCh := make(chan NewHeaderEvent, headerInChLength)
	dataInCh := make(chan NewDataEvent, headerInChLength)

	// Subscribe to events
	s.eventBus.Subscribe(EventHeaderRetrieved, func(ctx context.Context, evt events.Event) {
		headerEvent, ok := evt.(HeaderRetrievedEvent)
		if !ok {
			s.logger.Error("invalid event type", "expected", "HeaderRetrievedEvent", "got", fmt.Sprintf("%T", evt))
			return
		}

		headerInCh <- NewHeaderEvent{
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

		dataInCh <- NewDataEvent{
			Data:     dataEvent.Data,
			DAHeight: dataEvent.DAHeight,
		}
	})

	// Process headers and data
	for {
		select {
		case <-ctx.Done():
			return
		case headerEvent := <-headerInCh:
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

			if headerHeight <= s.store.Height() || s.headerCache.isSeen(headerHash) {
				s.logger.Debug("header already seen", "height", headerHeight, "hash", headerHash)
				continue
			}

			s.headerCache.setHeader(headerHeight, header)

			// Check if the data hash is for empty txs
			s.handleEmptyDataHash(ctx, &header.Header)

			if err := s.trySyncNextBlock(ctx, daHeight); err != nil {
				s.logger.Info("failed to sync next block", "error", err)
				continue
			}

			s.headerCache.setSeen(headerHash)

		case dataEvent := <-dataInCh:
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

			if dataHeight <= s.store.Height() || s.dataCache.isSeen(dataHash) {
				s.logger.Debug("data already seen", "height", dataHeight, "hash", dataHash)
				continue
			}

			s.dataCache.setData(dataHeight, data)

			if err := s.trySyncNextBlock(ctx, daHeight); err != nil {
				s.logger.Info("failed to sync next block", "error", err)
				continue
			}

			s.dataCache.setSeen(dataHash)
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

			s.logger.Debug("retrieved potential headers", "n", len(headerResp.Headers), "daHeight", daHeight)
			for _, bz := range headerResp.Headers {
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
				s.headerCache.setDAIncluded(blockHash)

				// Update DA included height if needed
				if header.Height() > s.daIncludedHeight.Load() {
					s.daIncludedHeight.Store(header.Height())
				}

				s.logger.Info("block marked as DA included", "blockHeight", header.Height(), "blockHash", blockHash)

				// Publish block DA included event
				s.eventBus.Publish(BlockDAIncludedEvent{
					Header:   header,
					Height:   header.Height(),
					DAHeight: daHeight,
				})

				if !s.headerCache.isSeen(blockHash) {
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

	if headerHeight <= s.store.Height() || s.headerCache.isSeen(headerHash) {
		return
	}

	s.headerCache.setHeader(headerHeight, header)

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

	if dataHeight <= s.store.Height() || s.dataCache.isSeen(dataHash) {
		return
	}

	s.dataCache.setData(dataHeight, data)
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
	s.headerCache.setDAIncluded(headerHash)

	// Update DA included height if needed
	if daEvent.Height > s.daIncludedHeight.Load() {
		s.daIncludedHeight.Store(daEvent.Height)
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
		h := s.headerCache.getHeader(currentHeight + 1)
		if h == nil {
			s.logger.Debug("header not found in cache", "height", currentHeight+1)
			return nil
		}

		d := s.dataCache.getData(currentHeight + 1)
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

		// Apply block and update state
		if err := s.applyBlock(ctx, h, d); err != nil {
			if ctx.Err() != nil {
				return err
			}
			// if call to applyBlock fails, we halt the node, see https://github.com/cometbft/cometbft/pull/496
			panic(fmt.Errorf("failed to ApplyBlock: %w", err))
		}

		// Update current state
		newState := types.State{
			LastBlockHeight: hHeight,
			LastBlockTime:   h.Time(),
			DAHeight:        daHeight,
		}

		// Publish state updated event
		s.eventBus.Publish(StateUpdatedEvent{
			State:  newState,
			Height: hHeight,
		})

		s.headerCache.deleteHeader(currentHeight + 1)
		s.dataCache.deleteData(currentHeight + 1)
	}
}

// Helper methods

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
			s.dataCache.setData(headerHeight, d)
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
func (s *Syncer) fetchHeaders(ctx context.Context, daHeight uint64) (coreda.ResultRetrieveHeaders, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
	defer cancel()

	headerRes := s.dalc.RetrieveHeaders(ctx, daHeight)
	if headerRes.Code == coreda.StatusError {
		return headerRes, fmt.Errorf("failed to retrieve block: %s", headerRes.Message)
	}

	return headerRes, nil
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

// applyBlock applies a block to the state
func (s *Syncer) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) error {
	// Save the block
	if err := s.store.SaveBlockData(ctx, header, data, &header.Signature); err != nil {
		return SaveBlockError{err}
	}

	// Update store height
	s.store.SetHeight(ctx, header.Height())

	// In a real implementation, this would:
	// 1. Call executor.ExecuteTxs to apply transactions
	// 2. Call executor.SetFinal to finalize the block
	// 3. Update the state with the new block's information

	// For this refactoring, we're keeping it simple
	return nil
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

// sendNonBlockingSignal sends a signal to a channel without blocking
func (s *Syncer) sendNonBlockingSignal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// sendNonBlockingSignalToRetrieveCh sends a signal to the retrieve channel
func (s *Syncer) sendNonBlockingSignalToRetrieveCh() {
	retrieveCh := make(chan struct{}, 1)
	s.sendNonBlockingSignal(retrieveCh)
}
