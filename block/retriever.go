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
	dAefetcherTimeout = 30 * time.Second
	dAFetcherRetries  = 10
)

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {
	// blobsFoundCh is used to track when we successfully found a header so
	// that we can continue to try and find headers that are in the next DA height.
	// This enables syncing faster than the DA block time.
	blobsFoundCh := make(chan struct{}, 1)
	defer close(blobsFoundCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:
		case <-blobsFoundCh:
		}
		daHeight := m.daHeight.Load()
		err := m.processNextDAHeaderAndData(ctx)
		if err != nil && ctx.Err() == nil {
			// if the requested da height is not yet available, wait silently, otherwise log the error and wait
			if !m.areAllErrorsHeightFromFuture(err) {
				m.logger.Error("failed to retrieve data from DALC", "daHeight", daHeight, "errors", err.Error())
			}
			continue
		}
		// Signal the blobsFoundCh to try and retrieve the next set of blobs
		select {
		case blobsFoundCh <- struct{}{}:
		default:
		}
		m.daHeight.Store(daHeight + 1)
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
	for r := 0; r < dAFetcherRetries; r++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		blobsResp, fetchErr := m.fetchBlobs(ctx, daHeight)
		if fetchErr == nil {
			if blobsResp.Code == coreda.StatusNotFound {
				m.logger.Debug("no blob data found", "daHeight", daHeight, "reason", blobsResp.Message)
				return nil
			}
			m.logger.Debug("retrieved potential blob data", "n", len(blobsResp.Data), "daHeight", daHeight)
			for _, bz := range blobsResp.Data {
				if len(bz) == 0 {
					m.logger.Debug("ignoring nil or empty blob", "daHeight", daHeight)
					continue
				}
				if m.handlePotentialHeader(ctx, bz, daHeight) {
					continue
				}
				m.handlePotentialData(ctx, bz, daHeight)
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
	m.headerCache.SetDAIncluded(headerHash)
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

// handlePotentialData tries to decode and process a data. No return value.
func (m *Manager) handlePotentialData(ctx context.Context, bz []byte, daHeight uint64) {
	var signedData types.SignedData
	err := signedData.UnmarshalBinary(bz)
	if err != nil {
		m.logger.Debug("failed to unmarshal signed data", "error", err)
		return
	}
	if len(signedData.Txs) == 0 {
		m.logger.Debug("ignoring empty signed data", "daHeight", daHeight)
		return
	}

	// Early validation to reject junk batches
	if !m.isValidSignedData(&signedData) {
		m.logger.Debug("invalid data signature", "daHeight", daHeight)
		return
	}

	dataHashStr := signedData.Data.DACommitment().String()
	m.dataCache.SetDAIncluded(dataHashStr)
	m.sendNonBlockingSignalToDAIncluderCh()
	m.logger.Info("signed data marked as DA included", "dataHash", dataHashStr, "daHeight", daHeight)
	if !m.dataCache.IsSeen(dataHashStr) {
		select {
		case <-ctx.Done():
			return
		default:
			m.logger.Warn("dataInCh backlog full, dropping signed batch", "daHeight", daHeight)
		}
		m.dataInCh <- NewDataEvent{&signedData.Data, daHeight}
	}
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
	// TODO: we should maintain the original error instead of creating a new one as we lose context by creating a new error.
	blobsRes := types.RetrieveWithHelpers(ctx, m.da, m.logger, daHeight)
	if blobsRes.Code == coreda.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", blobsRes.Message)
	}
	return blobsRes, err
}
