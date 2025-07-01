package block

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

const (
	dAefetcherTimeout = 2 * time.Minute // Increased from 30s to 2 minutes for large blob retrieval
	dAFetcherRetries  = 10
	// Default values for retry behavior
	defaultMaxConsecutiveFailures = 5
	defaultRetryBackoffBase       = 100 * time.Millisecond
	defaultMaxRetryBackoff        = 10 * time.Second
)

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {
	// blobsFoundCh is used to track when we successfully found a header so
	// that we can continue to try and find headers that are in the next DA height.
	// This enables syncing faster than the DA block time.
	blobsFoundCh := make(chan struct{}, 1)
	defer close(blobsFoundCh)

	// Get configuration values with defaults
	maxConsecutiveFailures := m.config.DA.MaxConsecutiveFailures
	if maxConsecutiveFailures <= 0 {
		maxConsecutiveFailures = defaultMaxConsecutiveFailures
	}

	retryBackoffBase := m.config.DA.RetryBackoffBase
	if retryBackoffBase <= 0 {
		retryBackoffBase = defaultRetryBackoffBase
	}

	maxRetryBackoff := m.config.DA.MaxRetryBackoff
	if maxRetryBackoff <= 0 {
		maxRetryBackoff = defaultMaxRetryBackoff
	}

	// Track consecutive failures for the current DA height
	var consecutiveFailures int
	var lastFailedHeight uint64
	var lastProcessedHeight uint64

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:
		case <-blobsFoundCh:
		}
		daHeight := m.daHeight.Load()

		// Skip if we've already processed this DA height
		if daHeight <= lastProcessedHeight {
			m.logger.Debug("skipping already processed DA height", "daHeight", daHeight, "lastProcessed", lastProcessedHeight)
			// Add a small delay to prevent busy waiting
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		err := m.processNextDAHeaderAndData(ctx)

		if err != nil && ctx.Err() == nil {
			// Check if this is the same height that failed before
			if daHeight == lastFailedHeight {
				consecutiveFailures++
			} else {
				// Reset counter for new height
				consecutiveFailures = 1
				lastFailedHeight = daHeight
			}

			// if the requested da height is not yet available, wait silently, otherwise log the error and wait
			if !m.areAllErrorsHeightFromFuture(err) {
				m.logger.Error("failed to retrieve data from DALC",
					"daHeight", daHeight,
					"errors", err.Error(),
					"consecutiveFailures", consecutiveFailures,
					"maxConsecutiveFailures", maxConsecutiveFailures)
			}

			// If we've failed too many times on the same height, skip it to prevent infinite loops
			if consecutiveFailures >= maxConsecutiveFailures {
				m.logger.Warn("skipping DA height after consecutive failures",
					"daHeight", daHeight,
					"consecutiveFailures", consecutiveFailures,
					"maxConsecutiveFailures", maxConsecutiveFailures,
					"error", err.Error())

				// Reset failure tracking and move to next height
				consecutiveFailures = 0
				lastFailedHeight = 0
				m.daHeight.Store(daHeight + 1)

				// Add a brief delay before continuing to avoid rapid cycling
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryBackoffBase):
				}
			} else {
				// Apply exponential backoff for retries
				backoffDuration := time.Duration(consecutiveFailures) * retryBackoffBase
				if backoffDuration > maxRetryBackoff {
					backoffDuration = maxRetryBackoff
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(backoffDuration):
				}
			}
			continue
		}

		// Success - reset failure tracking
		consecutiveFailures = 0
		lastFailedHeight = 0
		lastProcessedHeight = daHeight

		// Only signal to continue if we're not at the latest DA height
		// This prevents unnecessary polling when we've caught up
		select {
		case blobsFoundCh <- struct{}{}:
		default:
		}
	}
}

// processNextDAHeaderAndData is responsible for retrieving a header and data from the DA layer.
// It returns an error if the context is done or if the DA layer returns an error.
func (m *Manager) processNextDAHeaderAndData(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	daHeight := m.daHeight.Load()

	var err error
	m.logger.Debug("trying to retrieve data from DA", "daHeight", daHeight)

	// Add a maximum time limit for the entire process to prevent infinite hanging
	// Use a longer timeout since we can see data is being retrieved successfully
	processCtx, processCancel := context.WithTimeout(ctx, 15*time.Minute)
	defer processCancel()

	for r := 0; r < dAFetcherRetries; r++ {
		select {
		case <-processCtx.Done():
			m.logger.Warn("DA fetch process timed out", "daHeight", daHeight, "totalRetries", r)
			return fmt.Errorf("DA fetch process timed out after %d retries", r)
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		m.logger.Debug("attempting DA fetch", "daHeight", daHeight, "retry", r+1, "maxRetries", dAFetcherRetries)
		blobsResp, fetchErr := m.fetchBlobs(processCtx, daHeight)

		// Check for successful retrieval first
		if fetchErr == nil && blobsResp.Code == coreda.StatusSuccess {
			m.logger.Info("successfully retrieved blob data", "n", len(blobsResp.Data), "daHeight", daHeight, "retry", r+1)
			blocksProcessed := 0
			for _, bz := range blobsResp.Data {
				if len(bz) == 0 {
					m.logger.Debug("ignoring nil or empty blob", "daHeight", daHeight)
					continue
				}
				if m.handlePotentialHeader(ctx, bz, daHeight) {
					blocksProcessed++
					continue
				}
				if m.handlePotentialData(ctx, bz, daHeight) {
					blocksProcessed++
				}
			}
			m.logger.Info("completed processing blob data", "daHeight", daHeight, "blocksProcessed", blocksProcessed)

			// Always move to next DA height after processing all blobs at current height
			// This prevents requesting the same DA height multiple times
			m.daHeight.Store(daHeight + 1)
			return nil
		}

		// Check for not found (this is also success, just no data)
		if blobsResp.Code == coreda.StatusNotFound {
			m.logger.Debug("no blob data found", "daHeight", daHeight, "reason", blobsResp.Message)
			// No data found, move to next DA height
			m.daHeight.Store(daHeight + 1)
			return nil
		}

		// Handle height from future
		if fetchErr != nil && strings.Contains(fetchErr.Error(), coreda.ErrHeightFromFuture.Error()) {
			m.logger.Debug("height from future", "daHeight", daHeight, "reason", fetchErr.Error())
			return fetchErr
		}

		// Handle context cancellation
		if blobsResp.Code == coreda.StatusContextCanceled {
			m.logger.Debug("DA fetch cancelled", "daHeight", daHeight, "reason", blobsResp.Message)
			return ctx.Err()
		}

		// Handle timeout/deadline specifically - this should be treated as an error for retry logic
		if blobsResp.Code == coreda.StatusContextDeadline ||
		   (fetchErr != nil && (strings.Contains(fetchErr.Error(), "context deadline exceeded") ||
		                       strings.Contains(fetchErr.Error(), "timeout"))) {
			m.logger.Debug("DA fetch timeout", "daHeight", daHeight, "reason", blobsResp.Message, "error", fetchErr)
			// Ensure we have an error to return for the retry logic
			if fetchErr == nil {
				fetchErr = fmt.Errorf("context deadline exceeded: %s", blobsResp.Message)
			}
		}

		// If we have any error (including timeout), track it
		if fetchErr != nil {
			// Check if this is a timeout/deadline error that might be recoverable
			isTimeoutError := strings.Contains(fetchErr.Error(), "context deadline exceeded") ||
				strings.Contains(fetchErr.Error(), "timeout") ||
				strings.Contains(fetchErr.Error(), coreda.ErrContextDeadline.Error())

			if isTimeoutError {
				m.logger.Debug("timeout error during DA fetch",
					"daHeight", daHeight,
					"retry", r+1,
					"maxRetries", dAFetcherRetries,
					"error", fetchErr.Error())
			} else {
				m.logger.Debug("error during DA fetch",
					"daHeight", daHeight,
					"retry", r+1,
					"maxRetries", dAFetcherRetries,
					"error", fetchErr.Error())
			}

			// Track the error
			err = errors.Join(err, fetchErr)
		} else {
			// If no error but also not success, something is wrong
			m.logger.Debug("unexpected DA fetch result",
				"daHeight", daHeight,
				"retry", r+1,
				"maxRetries", dAFetcherRetries,
				"code", blobsResp.Code,
				"message", blobsResp.Message)

			// Create an error for unexpected states
			fetchErr = fmt.Errorf("unexpected DA fetch result: code=%d, message=%s", blobsResp.Code, blobsResp.Message)
			err = errors.Join(err, fetchErr)
		}

		// Apply progressive backoff for retries within the same height
		retryBackoffBase := m.config.DA.RetryBackoffBase
		if retryBackoffBase <= 0 {
			retryBackoffBase = defaultRetryBackoffBase
		}

		maxRetryBackoff := m.config.DA.MaxRetryBackoff
		if maxRetryBackoff <= 0 {
			maxRetryBackoff = defaultMaxRetryBackoff
		}

		retryBackoff := time.Duration(r+1) * retryBackoffBase
		if retryBackoff > maxRetryBackoff {
			retryBackoff = maxRetryBackoff
		}

		// Delay before retrying
		select {
		case <-ctx.Done():
			return err
		case <-time.After(retryBackoff):
		}
	}
	return err
}

// handlePotentialHeader tries to decode and process a header. Returns true if successful or skipped, false if not a header.
func (m *Manager) handlePotentialHeader(ctx context.Context, bz []byte, daHeight uint64) bool {
	header := new(types.SignedHeader)
	var headerPb pb.SignedHeader
	err := proto.Unmarshal(bz, &headerPb)
	if err != nil {
		m.logger.Debug("failed to unmarshal header", "error", err)
		return false
	}
	err = header.FromProto(&headerPb)
	if err != nil {
		// treat as handled, but not valid
		m.logger.Debug("failed to decode unmarshalled header", "error", err)
		return true
	}
	// Stronger validation: check for obviously invalid headers using ValidateBasic
	if err := header.ValidateBasic(); err != nil {
		m.logger.Debug("blob does not look like a valid header", "daHeight", daHeight, "error", err)
		return false
	}
	// early validation to reject junk headers
	if !m.isUsingExpectedSingleSequencer(header) {
		m.logger.Debug("skipping header from unexpected sequencer",
			"headerHeight", header.Height(),
			"headerHash", header.Hash().String())
		return true
	}
	headerHash := header.Hash().String()
	m.headerCache.SetDAIncluded(headerHash, daHeight)
	m.sendNonBlockingSignalToDAIncluderCh()
	m.logger.Info("header marked as DA included", "headerHeight", header.Height(), "headerHash", headerHash)
	if !m.headerCache.IsSeen(headerHash) {
		select {
		case <-ctx.Done():
			return true
		default:
			m.logger.Warn("headerInCh backlog full, dropping header", "daHeight", daHeight)
		}
		m.headerInCh <- NewHeaderEvent{header, daHeight}
	}
	return true
}

// handlePotentialData tries to decode and process a data. Returns true if data was processed.
func (m *Manager) handlePotentialData(ctx context.Context, bz []byte, daHeight uint64) bool {
	var signedData types.SignedData
	err := signedData.UnmarshalBinary(bz)
	if err != nil {
		m.logger.Debug("failed to unmarshal signed data", "error", err)
		return false
	}
	if len(signedData.Txs) == 0 {
		m.logger.Debug("ignoring empty signed data", "daHeight", daHeight)
		return false
	}

	// Early validation to reject junk data
	if !m.isValidSignedData(&signedData) {
		m.logger.Debug("invalid data signature", "daHeight", daHeight)
		return false
	}

	dataHashStr := signedData.Data.DACommitment().String()
	m.dataCache.SetDAIncluded(dataHashStr, daHeight)
	m.sendNonBlockingSignalToDAIncluderCh()
	m.logger.Info("signed data marked as DA included", "dataHash", dataHashStr, "daHeight", daHeight, "height", signedData.Height())
	if !m.dataCache.IsSeen(dataHashStr) {
		select {
		case <-ctx.Done():
			return true
		default:
			m.logger.Warn("dataInCh backlog full, dropping signed data", "daHeight", daHeight)
		}
		m.dataInCh <- NewDataEvent{&signedData.Data, daHeight}
	}
	return true
}

// areAllErrorsHeightFromFuture checks if all errors in a joined error are ErrHeightFromFutureStr
func (m *Manager) areAllErrorsHeightFromFuture(err error) bool {
	if err == nil {
		return false
	}

	// Check if the error itself is ErrHeightFromFutureStr
	if strings.Contains(err.Error(), ErrHeightFromFutureStr.Error()) {
		return true
	}

	// If it's a joined error, check each error recursively
	if joinedErr, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joinedErr.Unwrap() {
			if !m.areAllErrorsHeightFromFuture(e) {
				return false
			}
		}
		return true
	}

	return false
}

// fetchBlobs retrieves blobs from the DA layer
func (m *Manager) fetchBlobs(ctx context.Context, daHeight uint64) (coreda.ResultRetrieve, error) {
	var err error
	ctx, cancel := context.WithTimeout(ctx, dAefetcherTimeout)
	defer cancel()

	// Check if this DA height is already being fetched
	fetchKey := fmt.Sprintf("fetch_%d", daHeight)
	if _, loaded := m.ongoingFetches.LoadOrStore(fetchKey, true); loaded {
		m.logger.Debug("DA height already being fetched by another goroutine, skipping", "daHeight", daHeight)
		// Return a "not found" result to indicate this request should be skipped
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusNotFound,
				Message: "fetch already in progress",
				Height:  daHeight,
			},
		}, nil
	}

	// Ensure we clean up the fetch tracking when done
	defer func() {
		m.ongoingFetches.Delete(fetchKey)
	}()

	m.logger.Debug("fetching blobs from DA",
		"daHeight", daHeight,
		"namespace", string([]byte(m.genesis.ChainID)),
		"timeout", dAefetcherTimeout)

	// TODO: we should maintain the original error instead of creating a new one as we lose context by creating a new error.
	blobsRes := types.RetrieveWithHelpers(ctx, m.da, m.logger, daHeight, []byte(m.genesis.ChainID))
	switch blobsRes.Code {
	case coreda.StatusError:
		err = fmt.Errorf("failed to retrieve block: %s", blobsRes.Message)
		m.logger.Debug("DA fetch failed with error status",
			"daHeight", daHeight,
			"message", blobsRes.Message)
	case coreda.StatusHeightFromFuture:
		// Keep the root cause intact for callers that may rely on errors.Is/As.
		err = fmt.Errorf("%w: %s", coreda.ErrHeightFromFuture, blobsRes.Message)
		m.logger.Debug("DA fetch failed - height from future",
			"daHeight", daHeight,
			"message", blobsRes.Message)
	case coreda.StatusContextDeadline:
		// Create an error for timeout/deadline cases
		err = fmt.Errorf("context deadline exceeded: %s", blobsRes.Message)
		m.logger.Debug("DA fetch failed - timeout",
			"daHeight", daHeight,
			"message", blobsRes.Message)
	case coreda.StatusContextCanceled:
		// Create an error for cancellation cases
		err = fmt.Errorf("context canceled: %s", blobsRes.Message)
		m.logger.Debug("DA fetch failed - canceled",
			"daHeight", daHeight,
			"message", blobsRes.Message)
	case coreda.StatusSuccess:
		m.logger.Debug("DA fetch successful",
			"daHeight", daHeight,
			"numBlobs", len(blobsRes.Data))
	case coreda.StatusNotFound:
		m.logger.Debug("DA fetch - no blobs found",
			"daHeight", daHeight)
	}
	return blobsRes, err
}