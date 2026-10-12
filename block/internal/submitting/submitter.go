package submitting

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/evstack/ev-node/block/internal/cache"
	"github.com/evstack/ev-node/block/internal/common"
	coreexecutor "github.com/evstack/ev-node/core/execution"
	coresequencer "github.com/evstack/ev-node/core/sequencer"
	"github.com/evstack/ev-node/pkg/blobsize"
	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/signer"
	"github.com/evstack/ev-node/pkg/store"
	"github.com/evstack/ev-node/types"
)

// DASubmitterAPI defines minimal methods needed by Submitter for DA submissions.
type DASubmitterAPI interface {
	SubmitHeaders(ctx context.Context, headers []*types.SignedHeader, marshalledHeaders [][]byte, cache cache.Manager, signer signer.Signer) error
	SubmitData(ctx context.Context, signedDataList []*types.SignedData, marshalledData [][]byte, cache cache.Manager, signer signer.Signer) error
}

// Submitter handles DA submission and inclusion processing for both sync and aggregator nodes
type Submitter struct {
	// Core components
	store     store.Store
	exec      coreexecutor.Executor
	sequencer coresequencer.Sequencer
	config    config.Config
	genesis   genesis.Genesis

	// Shared components
	cache   cache.Manager
	metrics *common.Metrics

	// DA submitter
	daSubmitter DASubmitterAPI

	// Optional signer (only for aggregator nodes)
	signer signer.Signer

	// DA state
	daIncludedHeight *atomic.Uint64

	// Submission state to prevent concurrent submissions
	headerSubmissionMtx sync.Mutex
	dataSubmissionMtx   sync.Mutex

	// Batching strategy state
	lastHeaderSubmit atomic.Int64 // stores Unix nanoseconds
	lastDataSubmit   atomic.Int64 // stores Unix nanoseconds
	batchingStrategy BatchingStrategy

	// Channels for coordination
	errorCh chan<- error // Channel to report critical execution client failures

	// Logging
	logger zerolog.Logger

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSubmitter creates a new DA submitter component
func NewSubmitter(
	store store.Store,
	exec coreexecutor.Executor,
	cache cache.Manager,
	metrics *common.Metrics,
	config config.Config,
	genesis genesis.Genesis,
	daSubmitter DASubmitterAPI,
	sequencer coresequencer.Sequencer, // Can be nil for sync nodes
	signer signer.Signer, // Can be nil for sync nodes
	logger zerolog.Logger,
	errorCh chan<- error,
) *Submitter {
	submitterLogger := logger.With().Str("component", "submitter").Logger()

	strategy, err := NewBatchingStrategy(config.DA)
	if err != nil {
		submitterLogger.Warn().Err(err).Msg("failed to create batching strategy, using time-based default")
		strategy = NewTimeBasedStrategy(config.DA.BlockTime.Duration, 0, 1)
	}

	submitter := &Submitter{
		store:            store,
		exec:             exec,
		cache:            cache,
		metrics:          metrics,
		config:           config,
		genesis:          genesis,
		daSubmitter:      daSubmitter,
		sequencer:        sequencer,
		signer:           signer,
		daIncludedHeight: &atomic.Uint64{},
		batchingStrategy: strategy,
		errorCh:          errorCh,
		logger:           submitterLogger,
	}

	now := time.Now().UnixNano()
	submitter.lastHeaderSubmit.Store(now)
	submitter.lastDataSubmit.Store(now)

	return submitter
}

// Start begins the submitting component
func (s *Submitter) Start(ctx context.Context) (err error) {
	if s.cancel != nil {
		return errors.New("submitter already started")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	defer func() { // if error during init cancel context
		if err != nil {
			s.cancel()
			s.ctx, s.cancel = nil, nil
		}
	}()

	// Initialize DA included height
	if err = s.initializeDAIncludedHeight(ctx); err != nil {
		return err
	}

	// Start DA submission loop if signer is available (aggregator nodes only)
	if s.signer != nil {
		s.logger.Info().Msg("starting DA submission loop")
		s.wg.Go(s.daSubmissionLoop)
	}

	// Start DA inclusion processing loop (both sync and aggregator nodes)
	s.wg.Go(s.processDAInclusionLoop)

	return nil
}

// Stop shuts down the submitting component
func (s *Submitter) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	// Wait for goroutines to finish with a timeout to prevent hanging
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// All goroutines finished cleanly
	case <-time.After(5 * time.Second):
		s.logger.Warn().Msg("submitter shutdown timed out waiting for goroutines, proceeding anyway")
	}
	s.logger.Info().Msg("submitter stopped")
	return nil
}

// daSubmissionLoop handles submission of headers and data to DA layer (aggregator nodes only)
func (s *Submitter) daSubmissionLoop() {
	s.logger.Info().Msg("starting DA submission loop")
	defer s.logger.Info().Msg("DA submission loop stopped")

	// Use a shorter ticker interval to check batching strategy more frequently
	checkInterval := max(s.config.DA.BlockTime.Duration/4, 100*time.Millisecond)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			// Check if we should submit headers based on batching strategy
			headersNb := s.cache.NumPendingHeaders()
			if headersNb > 0 {
				lastSubmitNanos := s.lastHeaderSubmit.Load()
				timeSinceLastSubmit := time.Since(time.Unix(0, lastSubmitNanos))

				// For strategy decision, we need to estimate the size
				// We'll fetch headers to check, but only submit if strategy approves
				if s.headerSubmissionMtx.TryLock() {
					s.logger.Debug().Time("t", time.Now()).Uint64("headers", headersNb).Msg("Header submission in progress")
					s.wg.Add(1)
					go func() {
						defer func() {
							s.headerSubmissionMtx.Unlock()
							s.logger.Debug().Time("t", time.Now()).Uint64("headers", headersNb).Msg("Header submission completed")
							s.wg.Done()
						}()

						// Get headers with marshalled bytes from cache
						headers, marshalledHeaders, err := s.cache.GetPendingHeaders(s.ctx)
						if err != nil {
							s.logger.Error().Err(err).Msg("failed to get pending headers for batching decision")
							return
						}

						// Calculate total size (excluding signature)
						totalSize := uint64(0)
						for _, marshalled := range marshalledHeaders {
							totalSize += uint64(len(marshalled))
						}

						shouldSubmit := s.batchingStrategy.ShouldSubmit(
							uint64(len(headers)),
							totalSize,
							blobsize.DefaultMaxBlobSize,
							timeSinceLastSubmit,
						)

						if shouldSubmit {
							s.logger.Debug().
								Time("t", time.Now()).
								Uint64("headers", headersNb).
								Uint64("total_size_kb", totalSize/1024).
								Dur("time_since_last", timeSinceLastSubmit).
								Msg("batching strategy triggered header submission")

							if err := s.daSubmitter.SubmitHeaders(s.ctx, headers, marshalledHeaders, s.cache, s.signer); err != nil {
								// Check for unrecoverable errors that indicate a critical issue
								if errors.Is(err, common.ErrOversizedItem) {
									s.logger.Error().Err(err).
										Msg("CRITICAL: Header exceeds DA blob size limit - halting to prevent live lock")
									s.sendCriticalError(fmt.Errorf("unrecoverable DA submission error: %w", err))
									return
								}
								s.logger.Error().Err(err).Msg("failed to submit headers")
							} else {
								s.lastHeaderSubmit.Store(time.Now().UnixNano())
							}
						}
					}()
				}
			}

			// Check if we should submit data based on batching strategy
			dataNb := s.cache.NumPendingData()
			if dataNb > 0 {
				lastSubmitNanos := s.lastDataSubmit.Load()
				timeSinceLastSubmit := time.Since(time.Unix(0, lastSubmitNanos))
				if s.dataSubmissionMtx.TryLock() {
					s.logger.Debug().Time("t", time.Now()).Uint64("data", dataNb).Msg("Data submission in progress")
					s.wg.Add(1)
					go func() {
						defer func() {
							s.dataSubmissionMtx.Unlock()
							s.logger.Debug().Time("t", time.Now()).Uint64("data", dataNb).Msg("Data submission completed")
							s.wg.Done()
						}()

						// Get data with marshalled bytes from cache
						signedDataList, marshalledData, err := s.cache.GetPendingData(s.ctx)
						if err != nil {
							s.logger.Error().Err(err).Msg("failed to get pending data for batching decision")
							return
						}

						// Calculate total size (excluding signature)
						totalSize := uint64(0)
						for _, marshalled := range marshalledData {
							totalSize += uint64(len(marshalled))
						}

						shouldSubmit := s.batchingStrategy.ShouldSubmit(
							uint64(len(signedDataList)),
							totalSize,
							blobsize.DefaultMaxBlobSize,
							timeSinceLastSubmit,
						)

						if shouldSubmit {
							s.logger.Debug().
								Time("t", time.Now()).
								Uint64("data", dataNb).
								Uint64("total_size_kb", totalSize/1024).
								Dur("time_since_last", timeSinceLastSubmit).
								Msg("batching strategy triggered data submission")

							if err := s.daSubmitter.SubmitData(s.ctx, signedDataList, marshalledData, s.cache, s.signer); err != nil {
								// Check for unrecoverable errors that indicate a critical issue
								if errors.Is(err, common.ErrOversizedItem) {
									s.logger.Error().Err(err).
										Msg("CRITICAL: Data exceeds DA blob size limit - halting to prevent live lock")
									s.sendCriticalError(fmt.Errorf("unrecoverable DA submission error: %w", err))
									return
								}
								s.logger.Error().Err(err).Msg("failed to submit data")
							} else {
								s.lastDataSubmit.Store(time.Now().UnixNano())
							}
						}
					}()
				}
			}

			// Update metrics with current pending counts
			s.metrics.DASubmitterPendingBlobs.Set(float64(headersNb + dataNb))
		}
	}
}

// processDAInclusionLoop handles DA inclusion processing (both sync and aggregator nodes)
func (s *Submitter) processDAInclusionLoop() {
	s.logger.Info().Msg("starting DA inclusion processing loop")
	defer s.logger.Info().Msg("DA inclusion processing loop stopped")

	ticker := time.NewTicker(s.config.DA.BlockTime.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			currentDAIncluded := s.GetDAIncludedHeight()
			s.metrics.DAInclusionHeight.Set(float64(currentDAIncluded))

			for {
				nextHeight := currentDAIncluded + 1

				// Get block data first
				_, data, err := s.store.GetBlockData(s.ctx, nextHeight)
				if err != nil {
					break
				}

				// Check if this height is DA included
				if included, err := s.IsHeightDAIncluded(nextHeight, data); err != nil || !included {
					break
				}

				s.logger.Debug().Uint64("height", nextHeight).Msg("advancing DA included height")

				// Set node height to DA height mapping using already retrieved data
				if err := s.setNodeHeightToDAHeight(s.ctx, nextHeight, data, currentDAIncluded == 0); err != nil {
					s.logger.Error().Err(err).Uint64("height", nextHeight).Msg("failed to set node height to DA height mapping")
					break
				}

				// Set final height in executor. A failure must not stop the node:
				// block production and P2P sync do not depend on finality, so the
				// DA included height stays put and the next tick retries.
				if err := s.setFinalWithRetry(nextHeight); err != nil {
					s.logger.Error().Err(err).Uint64("height", nextHeight).Msg("failed to set final height after retries, will retry on next tick")
					break
				}

				// Persist DA included height before advancing in-memory state
				if err := putUint64Metadata(s.ctx, s.store, store.DAIncludedHeightKey, nextHeight); err != nil {
					s.logger.Error().Err(err).Uint64("height", nextHeight).Msg("failed to persist DA included height")
					break
				}

				// Update DA included height
				s.SetDAIncludedHeight(nextHeight)
				currentDAIncluded = nextHeight

				// Delete height cache for that height
				// This can only be performed after the height has been persisted to store
				s.cache.DeleteHeight(nextHeight)
			}
		}
	}
}

// setFinalWithRetry sets the final height in executor with retry logic.
// NOTE: the function retries the execution client call regardless of the error. Callers decide how to handle exhaustion.
func (s *Submitter) setFinalWithRetry(nextHeight uint64) error {
	for attempt := 1; attempt <= common.MaxRetriesBeforeHalt; attempt++ {
		if err := s.exec.SetFinal(s.ctx, nextHeight); err != nil {
			if attempt == common.MaxRetriesBeforeHalt {
				return fmt.Errorf("failed to set final height after %d attempts: %w", attempt, err)
			}

			s.logger.Error().Err(err).
				Int("attempt", attempt).
				Int("max_attempts", common.MaxRetriesBeforeHalt).
				Uint64("da_height", nextHeight).
				Msg("failed to set final height, retrying")

			select {
			case <-time.After(common.MaxRetriesTimeout):
				continue
			case <-s.ctx.Done():
				return fmt.Errorf("context cancelled during retry: %w", s.ctx.Err())
			}
		}

		return nil
	}

	return nil
}

// GetDAIncludedHeight returns the DA included height
func (s *Submitter) GetDAIncludedHeight() uint64 {
	return s.daIncludedHeight.Load()
}

// SetDAIncludedHeight updates the DA included height
func (s *Submitter) SetDAIncludedHeight(height uint64) {
	s.daIncludedHeight.Store(height)
}

// initializeDAIncludedHeight loads the DA included height from store
func (s *Submitter) initializeDAIncludedHeight(ctx context.Context) error {
	if height, err := s.store.GetMetadata(ctx, store.DAIncludedHeightKey); err == nil && len(height) == 8 {
		s.SetDAIncludedHeight(binary.LittleEndian.Uint64(height))
	}
	return nil
}

// putUint64Metadata encodes val as 8-byte little-endian and writes it to the store.
func putUint64Metadata(ctx context.Context, st store.Store, key string, val uint64) error {
	bz := make([]byte, 8)
	binary.LittleEndian.PutUint64(bz, val)
	return st.SetMetadata(ctx, key, bz)
}

// sendCriticalError sends a critical error to the error channel without blocking
func (s *Submitter) sendCriticalError(err error) {
	if s.errorCh != nil {
		select {
		case s.errorCh <- err:
		default:
			// Channel full, error already reported
		}
	}
}

// setNodeHeightToDAHeight persists the DA heights for a block's header and data.
// For empty-tx blocks, both use the header DA height since no data blob is posted.
func (s *Submitter) setNodeHeightToDAHeight(ctx context.Context, height uint64, data *types.Data, genesisInclusion bool) error {
	dataHash := data.DACommitment()

	daHeightForHeader, ok := s.cache.GetHeaderDAIncludedByHeight(height)
	if !ok {
		return fmt.Errorf("header for height %d not found in cache", height)
	}

	if err := putUint64Metadata(ctx, s.store, store.GetHeightToDAHeightHeaderKey(height), daHeightForHeader); err != nil {
		return err
	}

	genesisDAIncludedHeight := daHeightForHeader
	// For empty transactions, use the same DA height as the header.
	dataDAHeight := daHeightForHeader
	if !bytes.Equal(dataHash, common.DataHashForEmptyTxs) {
		daHeightForData, ok := s.cache.GetDataDAIncludedByHeight(height)
		if !ok {
			return fmt.Errorf("data for height %d not found in cache", height)
		}
		dataDAHeight = daHeightForData
		// if data posted before header, use data da included height for genesis da height
		genesisDAIncludedHeight = min(daHeightForData, genesisDAIncludedHeight)
	}
	if err := putUint64Metadata(ctx, s.store, store.GetHeightToDAHeightDataKey(height), dataDAHeight); err != nil {
		return err
	}

	if genesisInclusion {
		if err := putUint64Metadata(ctx, s.store, store.GenesisDAHeightKey, genesisDAIncludedHeight); err != nil {
			return err
		}

		if s.sequencer != nil {
			s.sequencer.SetDAHeight(genesisDAIncludedHeight)
			s.logger.Debug().Uint64("genesis_da_height", genesisDAIncludedHeight).Msg("initialized sequencer DA height from persisted genesis DA height")
		}
	}

	return nil
}

// IsHeightDAIncluded reports whether the block at height has been DA-included.
func (s *Submitter) IsHeightDAIncluded(height uint64, data *types.Data) (bool, error) {
	// Already finalized — cache entries were cleared, but we know it's included.
	if height <= s.GetDAIncludedHeight() {
		return true, nil
	}

	currentHeight, err := s.store.Height(s.ctx)
	if err != nil {
		return false, err
	}

	if currentHeight < height {
		return false, nil
	}

	dataCommitment := data.DACommitment()

	_, headerIncluded := s.cache.GetHeaderDAIncludedByHeight(height)
	_, dataIncluded := s.cache.GetDataDAIncludedByHeight(height)

	dataIncluded = bytes.Equal(dataCommitment, common.DataHashForEmptyTxs) || dataIncluded

	return headerIncluded && dataIncluded, nil
}
