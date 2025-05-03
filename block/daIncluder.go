package block

import (
	"context"
	"encoding/binary"
)

// DAIncluderLoop is responsible for advancing the DAIncludedHeight by checking if blocks after the current height
// have both their header and data marked as DA-included in the caches. If so, it calls setDAIncludedHeight.
func (m *Manager) DAIncluderLoop(ctx context.Context) {
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
			header, data, err := m.store.GetBlockData(ctx, nextHeight)
			if err != nil {
				m.logger.Debug("no more blocks to check at this time", "height", nextHeight, "error", err)
				// No more blocks to check at this time
				break
			}

			headerHash := header.Hash().String()
			dataHash := data.DACommitment().String()

			if m.headerCache.IsDAIncluded(headerHash) && m.dataCache.IsDAIncluded(dataHash) {
				// Both header and data are DA-included, so we can advance the height
				if err := m.incrementDAIncludedHeight(ctx); err != nil {
					break
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
func (m *Manager) incrementDAIncludedHeight(ctx context.Context) error {
	currentHeight := m.GetDAIncludedHeight()
	newHeight := currentHeight + 1
	if m.daIncludedHeight.CompareAndSwap(currentHeight, newHeight) {
		heightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(heightBytes, newHeight)
		m.logger.Debug("setting DA included height", "height", newHeight)
		err := m.store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		if err != nil {
			m.logger.Error("failed to set DA included height", "height", newHeight, "error", err)
			return err
		}
		m.logger.Debug("setting final", "height", newHeight)
		err = m.exec.SetFinal(ctx, newHeight)
		if err != nil {
			m.logger.Error("failed to set final", "height", newHeight, "error", err)
			return err
		}
	}
	return nil
}
