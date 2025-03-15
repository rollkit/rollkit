package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/rollkit/rollkit/types"
)

// ErrInvalidDataCommitment is returned when the data commitment in the header does not match the data.
var ErrInvalidDataCommitment = errors.New("invalid data commitment")

// ErrHeaderNotFound is returned when a header is not found.
var ErrHeaderNotFound = errors.New("header not found")

// ErrDataNotFound is returned when data is not found.
var ErrDataNotFound = errors.New("data not found")

// ProcessBlock processes a block by validating the header and data separately,
// applying the block, updating the state, and recording metrics.
func (m *Manager) ProcessBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) error {
	// Validate the header
	if err := header.Header.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	// Validate the data
	if err := data.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid data: %w", err)
	}

	// Verify that the header and data are consistent
	if err := m.VerifyHeaderData(header, data); err != nil {
		return fmt.Errorf("header and data mismatch: %w", err)
	}

	// Apply the block
	newState, _, err := m.applyBlock(ctx, header, data)
	if err != nil {
		return fmt.Errorf("failed to apply block: %w", err)
	}

	// Update the state
	m.lastStateMtx.Lock()
	m.lastState = newState
	m.lastStateMtx.Unlock()

	// Record metrics
	m.recordMetrics(header.Height())

	return nil
}

// ProcessHeader validates and stores the header, notifying the need to retrieve corresponding data.
func (m *Manager) ProcessHeader(ctx context.Context, header *types.SignedHeader) error {
	// Validate the header
	if err := header.Header.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	// Store the header in the cache
	headerHeight := header.Height()
	m.headerCache.setHeader(headerHeight, header)

	// Mark the header as seen
	headerHash := header.Hash().String()
	m.headerCache.setSeen(headerHash)

	// Try to match with existing data
	m.MatchHeaderAndData(ctx)

	return nil
}

// ProcessData validates and stores the data, notifying the need to match this data with a header.
func (m *Manager) ProcessData(ctx context.Context, data *types.Data) error {
	// Validate the data
	if err := data.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid data: %w", err)
	}

	// Store the data in the cache
	dataHeight := data.Metadata.Height
	m.dataCache.setData(dataHeight, data)

	// Mark the data as seen
	dataHash := data.Hash().String()
	m.dataCache.setSeen(dataHash)

	// Try to match with existing header
	m.MatchHeaderAndData(ctx)

	return nil
}

// MatchHeaderAndData attempts to match headers and data received separately,
// processing the block if both are available.
func (m *Manager) MatchHeaderAndData(ctx context.Context) {
	// Get the current height of the chain
	currentHeight := m.store.Height()

	// Check for unprocessed headers and data
	for height := currentHeight + 1; ; height++ {
		// Get the header from the cache
		header := m.headerCache.getHeader(height)
		if header == nil {
			// No more headers to process
			break
		}

		// Get the data from the cache
		data := m.dataCache.getData(height)
		if data == nil {
			// Data not available for this height
			continue
		}

		// Verify that the header and data are consistent
		if err := m.VerifyHeaderData(header, data); err != nil {
			m.logger.Error("header and data mismatch", "height", height, "err", err)
			continue
		}

		// Process the block
		err := m.ProcessBlock(ctx, header, data)
		if err != nil {
			m.logger.Error("failed to process block", "height", height, "err", err)
			continue
		}

		// Remove the header and data from the cache
		m.headerCache.deleteHeader(height)
		m.dataCache.deleteData(height)

		m.logger.Info("processed block", "height", height)
	}
}

// VerifyHeaderData verifies that the header and data are consistent.
func (m *Manager) VerifyHeaderData(header *types.SignedHeader, data *types.Data) error {
	// Verify that the header's data commitment matches the data
	if !header.Header.ValidateDataCommitment(data) {
		return ErrInvalidDataCommitment
	}

	// Verify that the header and data heights match
	if header.Height() != data.Metadata.Height {
		return fmt.Errorf("header height %d does not match data height %d", header.Height(), data.Metadata.Height)
	}

	// Verify that the header and data chain IDs match
	if header.ChainID() != data.Metadata.ChainID {
		return fmt.Errorf("header chain ID %s does not match data chain ID %s", header.ChainID(), data.Metadata.ChainID)
	}

	return nil
}
