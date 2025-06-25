package block

import (
	"context"
	"encoding/binary"
	"fmt"

	coreda "github.com/rollkit/rollkit/core/da"
)

// DAIncluderLoop is responsible for advancing the DAIncludedHeight by checking if blocks after the current height
// have both their header and data marked as DA-included in the caches. If so, it calls setDAIncludedHeight.
func (m *Manager) DAIncluderLoop(ctx context.Context, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.daIncluderCh:
			// proceed to check for DA inclusion
		}
		currentDAIncluded := m.GetDAIncludedHeight()
		for {
			nextHeight := currentDAIncluded + 1
			daIncluded, err := m.IsDAIncluded(ctx, nextHeight)
			if err != nil {
				// No more blocks to check at this time
				m.logger.Debug("no more blocks to check at this time", "height", nextHeight, "error", err)
				break
			}
			if daIncluded {
				m.logger.Debug("both header and data are DA-included, advancing height", "height", nextHeight)
				if err := m.SetRollkitHeightToDAHeight(ctx, nextHeight); err != nil {
					errCh <- fmt.Errorf("failed to set rollkit height to DA height: %w", err)
					return
				}
				// Both header and data are DA-included, so we can advance the height
				if err := m.incrementDAIncludedHeight(ctx); err != nil {
					errCh <- fmt.Errorf("error while incrementing DA included height: %w", err)
					return
				}

				currentDAIncluded = nextHeight
			} else {
				// Stop at the first block that is not DA-included
				break
			}
		}
	}
}

// incrementDAIncludedHeight sets the DA included height in the store
// It returns an error if the DA included height is not set.
func (m *Manager) incrementDAIncludedHeight(ctx context.Context) error {
	currentHeight := m.GetDAIncludedHeight()
	newHeight := currentHeight + 1
	m.logger.Debug("setting final", "height", newHeight)
	err := m.exec.SetFinal(ctx, newHeight)
	if err != nil {
		m.logger.Error("failed to set final", "height", newHeight, "error", err)
		return err
	}
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, newHeight)
	m.logger.Debug("setting DA included height", "height", newHeight)
	err = m.store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
	if err != nil {
		m.logger.Error("failed to set DA included height", "height", newHeight, "error", err)
		return err
	}
	if !m.daIncludedHeight.CompareAndSwap(currentHeight, newHeight) {
		return fmt.Errorf("failed to set DA included height: %d", newHeight)
	}

	// Update sequencer metrics if the sequencer supports it
	if seq, ok := m.sequencer.(MetricsRecorder); ok {
		seq.RecordMetrics(m.gasPrice, 0, coreda.StatusSuccess, m.pendingHeaders.numPendingHeaders(), newHeight)
	}

	return nil
}
