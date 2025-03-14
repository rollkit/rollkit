package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/types"
	rollkitproto "github.com/rollkit/rollkit/types/pb/rollkit"
)

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
			if !strings.Contains(err.Error(), ErrHeightFromFutureStr) {
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
			m.logger.Debug("retrieved potential headers", "n", len(headerResp.Headers), "daHeight", daHeight)
			for _, bz := range headerResp.Headers {
				header := new(types.SignedHeader)
				// decode the header
				var headerPb rollkitproto.SignedHeader
				err := headerPb.Unmarshal(bz)
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
				m.headerCache.setDAIncluded(blockHash)
				err = m.setDAIncludedHeight(ctx, header.Height())
				if err != nil {
					return err
				}
				m.logger.Info("block marked as DA included", "blockHeight", header.Height(), "blockHash", blockHash)
				if !m.headerCache.isSeen(blockHash) {
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

func (m *Manager) isUsingExpectedCentralizedSequencer(header *types.SignedHeader) bool {
	return bytes.Equal(header.ProposerAddress, m.genesis.ProposerAddress) && header.ValidateBasic() == nil
}

func (m *Manager) fetchHeaders(ctx context.Context, daHeight uint64) (coreda.ResultRetrieveHeaders, error) {
	var err error
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
	defer cancel()
	headerRes := m.dalc.RetrieveHeaders(ctx, daHeight)
	if headerRes.Code == coreda.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", headerRes.Message)
	}
	return headerRes, err
}

func (m *Manager) submitHeadersToDA(ctx context.Context) error {
	submittedAllHeaders := false
	var backoff time.Duration
	headersToSubmit, err := m.pendingHeaders.getPendingHeaders(ctx)
	if len(headersToSubmit) == 0 {
		// There are no pending headers; return because there's nothing to do, but:
		// - it might be caused by error, then err != nil
		// - all pending headers are processed, then err == nil
		// whatever the reason, error information is propagated correctly to the caller
		return err
	}
	if err != nil {
		// There are some pending blocks but also an error. It's very unlikely case - probably some error while reading
		// headers from the store.
		// The error is logged and normal processing of pending blocks continues.
		m.logger.Error("error while fetching blocks pending DA", "err", err)
	}
	numSubmittedHeaders := 0
	attempt := 0
	maxBlobSize, err := m.dalc.MaxBlobSize(ctx)
	if err != nil {
		return err
	}
	initialMaxBlobSize := maxBlobSize
	gasPrice := m.gasPrice
	initialGasPrice := gasPrice

daSubmitRetryLoop:
	for !submittedAllHeaders && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		headersBz := make([][]byte, len(headersToSubmit))
		for i, header := range headersToSubmit {
			headerPb, err := header.ToProto()
			if err != nil {
				// do we drop the header from attempting to be submitted?
				return fmt.Errorf("failed to transform header to proto: %w", err)
			}
			headersBz[i], err = headerPb.Marshal()
			if err != nil {
				// do we drop the header from attempting to be submitted?
				return fmt.Errorf("failed to marshal header: %w", err)
			}
		}

		ctx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
		defer cancel()
		res := m.dalc.SubmitHeaders(ctx, headersBz, maxBlobSize, gasPrice)
		switch res.Code {
		case coreda.StatusSuccess:
			m.logger.Info("successfully submitted Rollkit headers to DA layer", "gasPrice", gasPrice, "daHeight", res.DAHeight, "headerCount", res.SubmittedCount)
			if res.SubmittedCount == uint64(len(headersToSubmit)) {
				submittedAllHeaders = true
			}
			submittedBlocks, notSubmittedBlocks := headersToSubmit[:res.SubmittedCount], headersToSubmit[res.SubmittedCount:]
			numSubmittedHeaders += len(submittedBlocks)
			for _, block := range submittedBlocks {
				m.headerCache.setDAIncluded(block.Hash().String())
				err = m.setDAIncludedHeight(ctx, block.Height())
				if err != nil {
					return err
				}
			}
			lastSubmittedHeight := uint64(0)
			if l := len(submittedBlocks); l > 0 {
				lastSubmittedHeight = submittedBlocks[l-1].Height()
			}
			m.pendingHeaders.setLastSubmittedHeight(ctx, lastSubmittedHeight)
			headersToSubmit = notSubmittedBlocks
			// reset submission options when successful
			// scale back gasPrice gradually
			backoff = 0
			maxBlobSize = initialMaxBlobSize
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.gasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)
		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.config.DA.BlockTime * time.Duration(m.config.DA.MempoolTTL) //nolint:gosec
			if m.gasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * m.gasMultiplier
			}
			m.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)

		case coreda.StatusTooBig:
			maxBlobSize = maxBlobSize / 4
			fallthrough
		default:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.exponentialBackoff(backoff)
		}

		attempt += 1
	}

	if !submittedAllHeaders {
		return fmt.Errorf(
			"failed to submit all blocks to DA layer, submitted %d blocks (%d left) after %d attempts",
			numSubmittedHeaders,
			len(headersToSubmit),
			attempt,
		)
	}
	return nil
}

// HeaderSubmissionLoop is responsible for submitting blocks to the DA layer.
func (m *Manager) HeaderSubmissionLoop(ctx context.Context) {
	timer := time.NewTicker(m.config.DA.BlockTime)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if m.pendingHeaders.isEmpty() {
			continue
		}
		err := m.submitHeadersToDA(ctx)
		if err != nil {
			m.logger.Error("error while submitting block to DA", "error", err)
		}
	}
}
