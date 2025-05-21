package block

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

const (
	dAefetcherTimeout = 30 * time.Second
	dAFetcherRetries  = 10
	dAFetcherWorkers  = 4
)

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:

			var wg sync.WaitGroup
			workCh := make(chan uint64, dAFetcherWorkers*2)
			stopCh := make(chan struct{}, 1)
			validHeightCh := make(chan uint64, dAFetcherWorkers)
			defer close(stopCh)
			daHeight := m.daHeight.Load()

			//start the multiple go routines that will fetch the blocks
			for i := 0; i < dAFetcherWorkers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for height := range workCh {
						err := m.processNextDAHeaderAndData(ctx, height)
						if err != nil && ctx.Err() == nil {
							// if the requested da height is not yet available, wait silently, otherwise log the error and wait
							if !m.areAllErrorsHeightFromFuture(err) {
								m.logger.Error("failed to retrieve data from DALC", "daHeight", height, "errors", err.Error())
							} else { //if the request height is not yet available stop fetching it
								select {
								case stopCh <- struct{}{}:
								default:
								}
							}
						} else if err == nil {
							select {
							case validHeightCh <- height:
							case <-ctx.Done():
								return
							}
						}
					}
				}()
			}

			//start the goroutine that will update the manager.daHeight
			go func() {
				daHeight := m.daHeight.Load()
				for height := range validHeightCh {
					if height > daHeight {
						m.daHeight.Store(height)
						daHeight = height
					}
				}
			}()

			//start the retrieve loop that will send block height that the workers will retrieve
		retrieveLoop:
			for {
				select {
				case <-ctx.Done():
					close(workCh)
					wg.Wait()
					close(validHeightCh)
					return
				case <-stopCh: //if we are going to far we stop adding future heights
					close(workCh)
					wg.Wait()
					close(validHeightCh)
					break retrieveLoop
				case workCh <- daHeight:
					daHeight++
				}
			}
		}
	}
}

// processNextDAHeaderAndData is responsible for retrieving a header and data from the DA layer.
// It returns an error if the context is done or if the DA layer returns an error.
func (m *Manager) processNextDAHeaderAndData(ctx context.Context, daHeight uint64) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

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
			m.logger.Debug("retrieved potential data", "n", len(blobsResp.Data), "daHeight", daHeight)
			for _, bz := range blobsResp.Data {
				if len(bz) == 0 {
					m.logger.Debug("ignoring nil or empty blob", "daHeight", daHeight)
					continue
				}
				if m.handlePotentialHeader(ctx, bz, daHeight) {
					continue
				}
				m.handlePotentialBatch(ctx, bz, daHeight)
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

// handlePotentialBatch tries to decode and process a batch. No return value.
func (m *Manager) handlePotentialBatch(ctx context.Context, bz []byte, daHeight uint64) {
	var batchPb pb.Batch
	err := proto.Unmarshal(bz, &batchPb)
	if err != nil {
		m.logger.Debug("failed to unmarshal batch", "error", err)
		return
	}
	if len(batchPb.Txs) == 0 {
		m.logger.Debug("ignoring empty batch", "daHeight", daHeight)
		return
	}
	data := &types.Data{
		Txs: make(types.Txs, len(batchPb.Txs)),
	}
	for i, tx := range batchPb.Txs {
		data.Txs[i] = types.Tx(tx)
	}
	dataHashStr := data.DACommitment().String()
	m.dataCache.SetDAIncluded(dataHashStr)
	m.sendNonBlockingSignalToDAIncluderCh()
	m.logger.Info("batch marked as DA included", "batchHash", dataHashStr, "daHeight", daHeight)
	if !m.dataCache.IsSeen(dataHashStr) {
		select {
		case <-ctx.Done():
			return
		default:
			m.logger.Warn("dataInCh backlog full, dropping batch", "daHeight", daHeight)
		}
		m.dataInCh <- NewDataEvent{data, daHeight}
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

// featchHeaders retrieves blobs from the DA layer
func (m *Manager) fetchBlobs(ctx context.Context, daHeight uint64) (coreda.ResultRetrieve, error) {
	var err error
	ctx, cancel := context.WithTimeout(ctx, dAefetcherTimeout)
	defer cancel()
	//TODO: we should maintain the original error instead of creating a new one as we lose context by creating a new error.
	blobsRes := types.RetrieveWithHelpers(ctx, m.da, m.logger, daHeight)
	if blobsRes.Code == coreda.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", blobsRes.Message)
	}
	return blobsRes, err
}
