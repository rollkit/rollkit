package block

import (
	"context"
	"fmt"

	"github.com/rollkit/rollkit/types"
)

// HeaderStoreRetrieveLoop is responsible for retrieving headers from the Header Store.
func (m *Manager) HeaderStoreRetrieveLoop(ctx context.Context) {
	// height is always > 0
	initialHeight, err := m.store.Height(ctx)
	if err != nil {
		m.logger.Error("failed to get initial store height for DataStoreRetrieveLoop", "error", err)
		return
	}
	lastHeaderStoreHeight := initialHeight
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.headerStoreCh:
		}
		headerStoreHeight := m.headerStore.Height()
		if headerStoreHeight > lastHeaderStoreHeight {
			headers, err := m.getHeadersFromHeaderStore(ctx, lastHeaderStoreHeight+1, headerStoreHeight)
			if err != nil {
				m.logger.Error("failed to get headers from Header Store", "lastHeaderHeight", lastHeaderStoreHeight, "headerStoreHeight", headerStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := m.daHeight.Load()
			for _, header := range headers {
				// Check for shut down event prior to logging
				// and sending header to headerInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				// early validation to reject junk headers
				if !m.isUsingExpectedSingleSequencer(header) {
					continue
				}
				m.logger.Debug("header retrieved from p2p header sync", "headerHeight", header.Height(), "daHeight", daHeight)
				m.headerInCh <- NewHeaderEvent{header, daHeight}
			}
		}
		lastHeaderStoreHeight = headerStoreHeight
	}
}

// DataStoreRetrieveLoop is responsible for retrieving data from the Data Store.
func (m *Manager) DataStoreRetrieveLoop(ctx context.Context) {
	// height is always > 0
	initialHeight, err := m.store.Height(ctx)
	if err != nil {
		m.logger.Error("failed to get initial store height for DataStoreRetrieveLoop", "error", err)
		return
	}
	lastDataStoreHeight := initialHeight
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.dataStoreCh:
		}
		dataStoreHeight := m.dataStore.Height()
		if dataStoreHeight > lastDataStoreHeight {
			data, err := m.getDataFromDataStore(ctx, lastDataStoreHeight+1, dataStoreHeight)
			if err != nil {
				m.logger.Error("failed to get data from Data Store", "lastDataStoreHeight", lastDataStoreHeight, "dataStoreHeight", dataStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := m.daHeight.Load()
			for _, d := range data {
				// Check for shut down event prior to logging
				// and sending header to dataInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				//TODO: remove junk if possible
				m.logger.Debug("data retrieved from p2p data sync", "dataHeight", d.Metadata.Height, "daHeight", daHeight)
				m.dataInCh <- NewDataEvent{d, daHeight}
			}
		}
		lastDataStoreHeight = dataStoreHeight
	}
}

func (m *Manager) getHeadersFromHeaderStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.SignedHeader, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	headers := make([]*types.SignedHeader, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		header, err := m.headerStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		headers[i-startHeight] = header
	}
	return headers, nil
}

func (m *Manager) getDataFromDataStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.SignedData, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	data := make([]*types.SignedData, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		d, err := m.dataStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		data[i-startHeight] = d
	}
	return data, nil
}
