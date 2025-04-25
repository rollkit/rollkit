package block

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
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
			if !m.areAllErrorsHeightFromFuture(err) {
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
			m.logger.Debug("retrieved potential headers", "n", len(headerResp.Data), "daHeight", daHeight)
			for _, bz := range headerResp.Data {
				header := new(types.SignedHeader)
				// decode the header
				var headerPb pb.SignedHeader
				err := proto.Unmarshal(bz, &headerPb)
				if err != nil {
					m.logger.Debug("failed to unmarshal header", "error", err, "DAHeight", daHeight)
					continue
				}
				err = header.FromProto(&headerPb)
				if err != nil {
					m.logger.Error("failed to create local header", "error", err, "DAHeight", daHeight)
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

func (m *Manager) fetchHeaders(ctx context.Context, daHeight uint64) (coreda.ResultRetrieve, error) {
	var err error
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second) //TODO: make this configurable
	defer cancel()
	headerRes := m.dalc.Retrieve(ctx, daHeight)
	if headerRes.Code == coreda.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", headerRes.Message)
	}
	return headerRes, err
}

func (m *Manager) getSignature(header types.Header) (types.Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return m.signer.Sign(b)
}

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
