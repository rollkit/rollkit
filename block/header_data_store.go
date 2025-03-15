package block

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rollkit/rollkit/types"
)

// HeaderStoreRetrieveLoop is responsible for retrieving headers from the Header Store.
func (m *Manager) HeaderStoreRetrieveLoop(ctx context.Context) {
	lastHeaderStoreHeight := uint64(0)
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
			daHeight := atomic.LoadUint64(&m.daHeight)
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
				if !m.isUsingExpectedCentralizedSequencer(header) {
					continue
				}
				m.logger.Debug("header retrieved from p2p header sync", "headerHeight", header.Height(), "daHeight", daHeight)
				m.headerInCh <- NewHeaderEvent{header, daHeight}

				// Process the header
				err = m.ProcessHeader(ctx, header)
				if err != nil {
					m.logger.Error("failed to process header", "height", header.Height(), "err", err)
					continue
				}
			}
		}
		lastHeaderStoreHeight = headerStoreHeight
	}
}

// DataStoreRetrieveLoop is responsible for retrieving data from the Data Store.
func (m *Manager) DataStoreRetrieveLoop(ctx context.Context) {
	lastDataStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.dataStoreCh:
		}
		dataStoreHeight := m.dataStore.Height()
		if dataStoreHeight > lastDataStoreHeight {
			dataItems, err := m.getDataFromDataStore(ctx, lastDataStoreHeight+1, dataStoreHeight)
			if err != nil {
				m.logger.Error("failed to get data from Data Store", "lastDataHeight", lastDataStoreHeight, "dataStoreHeight", dataStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.daHeight)
			for _, data := range dataItems {
				// Check for shut down event prior to logging
				// and sending data to dataInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				m.logger.Debug("data retrieved from p2p data sync", "dataHeight", data.Metadata.Height, "daHeight", daHeight)
				m.dataInCh <- NewDataEvent{data, daHeight}

				// Process the data
				err = m.ProcessData(ctx, data)
				if err != nil {
					m.logger.Error("failed to process data", "height", data.Metadata.Height, "err", err)
					continue
				}
			}
		}
		lastDataStoreHeight = dataStoreHeight
	}
}

func (m *Manager) getHeadersFromHeaderStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.SignedHeader, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}

	headers := make([]*types.SignedHeader, 0, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		// Adapt to go-header-store interface which doesn't take a context
		header, err := m.headerStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		headers = append(headers, header)
	}
	return headers, nil
}

func (m *Manager) getDataFromDataStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.Data, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}

	data := make([]*types.Data, 0, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		// Adapt to go-header-store interface which doesn't take a context
		d, err := m.dataStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		data = append(data, d)
	}
	return data, nil
}
