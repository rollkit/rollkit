package block

import (
	"context"
	"encoding/binary"
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
	// headerFoundCh is used to track when we successfully found a header so
	// that we can continue to try and find headers that are in the next DA height.
	// This enables syncing faster than the DA block time.
	headerFoundCh := make(chan struct{}, 1)
	defer close(headerFoundCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:
		case <-m.dataStoreCh:
		case <-headerFoundCh:
		}
		daHeight := m.daHeight.Load()
		err := m.processNextDAHeaderAndBlock(ctx)
		if err != nil && ctx.Err() == nil {
			// if the requested da height is not yet available, wait silently, otherwise log the error and wait
			if !m.areAllErrorsHeightFromFuture(err) {
				m.logger.Error("failed to retrieve data from DALC", "daHeight", daHeight, "errors", err.Error())
			}
			continue
		}
		// Signal the headerFoundCh to try and retrieve the next header
		select {
		case headerFoundCh <- struct{}{}:
		default:
		}
		m.daHeight.Store(daHeight + 1)
	}
}

func (m *Manager) processNextDAHeaderAndBlock(ctx context.Context) error {
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
				m.logger.Debug("no data found", "daHeight", daHeight, "reason", blobsResp.Message)
				return nil
			}
			m.logger.Debug("retrieved potential data", "n", len(blobsResp.Data), "daHeight", daHeight)
			for _, bz := range blobsResp.Data {
				// Try to decode as header first
				header := new(types.SignedHeader)
				var headerPb pb.SignedHeader
				err := proto.Unmarshal(bz, &headerPb)
				if err == nil {
					err = header.FromProto(&headerPb)
					if err == nil {
						// early validation to reject junk headers
						if !m.isUsingExpectedSingleSequencer(header) {
							m.logger.Debug("skipping header from unexpected sequencer",
								"headerHeight", header.Height(),
								"headerHash", header.Hash().String())
							continue
						}
						headerHash := header.Hash().String()
						m.headerCache.SetDAIncluded(headerHash)
						m.logger.Info("header marked as DA included", "headerHeight", header.Height(), "headerHash", headerHash)
						if !m.headerCache.IsSeen(headerHash) {
							select {
							case <-ctx.Done():
								return fmt.Errorf("unable to send header to headerInCh, context done: %w", ctx.Err())
							default:
								m.logger.Warn("headerInCh backlog full, dropping header", "daHeight", daHeight)
							}
							m.headerInCh <- NewHeaderEvent{header, daHeight}
						}
						continue
					}
				}

				// If not a header, try to decode as batch
				var batchPb pb.Batch
				err = proto.Unmarshal(bz, &batchPb)
				if err == nil {
					data := &types.Data{
						Txs: make(types.Txs, len(batchPb.Txs)),
					}
					for i, tx := range batchPb.Txs {
						data.Txs[i] = types.Tx(tx)
					}
					dataHashStr := data.Hash().String()
					m.dataCache.SetDAIncluded(dataHashStr)
					m.logger.Info("batch marked as DA included", "batchHash", dataHashStr, "daHeight", daHeight)
					if !m.dataCache.IsSeen(dataHashStr) {
						select {
						case <-ctx.Done():
							return fmt.Errorf("unable to send batch to dataInCh, context done: %w", ctx.Err())
						default:
							m.logger.Warn("dataInCh backlog full, dropping batch", "daHeight", daHeight)
						}
						m.dataInCh <- NewDataEvent{data, daHeight}
					}
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

// featchHeaders retrieves blobs from the DA layer
func (m *Manager) fetchBlobs(ctx context.Context, daHeight uint64) (coreda.ResultRetrieve, error) {
	var err error
	ctx, cancel := context.WithTimeout(ctx, dAefetcherTimeout)
	defer cancel()
	//TODO: we should maintain the original error instead of creating a new one as we lose context by creating a new error.
	headerRes := types.RetrieveWithHelpers(ctx, m.da, m.logger, daHeight)
	if headerRes.Code == coreda.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", headerRes.Message)
	}
	return blobsRes, err
}

// setDAIncludedHeight sets the DA included height in the store
func (m *Manager) setDAIncludedHeight(ctx context.Context, newHeight uint64) error {
	for {
		currentHeight := m.daIncludedHeight.Load()
		if newHeight <= currentHeight {
			break
		}
		if m.daIncludedHeight.CompareAndSwap(currentHeight, newHeight) {
			heightBytes := make([]byte, 8)
			binary.LittleEndian.PutUint64(heightBytes, newHeight)
			return m.store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		}
	}
	return nil
}
