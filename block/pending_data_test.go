package block

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	mocksStore "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func TestNewPendingData_InitAndGetLastSubmittedDataHeight(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	// Simulate no metadata stored
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pd, err := NewPendingData(mockStore, logger)
	assert.NoError(t, err)
	assert.NotNil(t, pd)
	assert.Equal(t, uint64(0), pd.GetLastSubmittedDataHeight())
}

func TestPendingData_GetPendingData_AllCases(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pd, err := NewPendingData(mockStore, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Case: no data
	mockStore.On("Height", ctx).Return(uint64(0), nil).Once()
	pending, err := pd.GetPendingData()
	assert.NoError(t, err)
	assert.Nil(t, pending)

	// Case: all data submitted
	pd.lastSubmittedDataHeight.Store(5)
	mockStore.On("Height", ctx).Return(uint64(5), nil).Once()
	pending, err = pd.GetPendingData()
	assert.NoError(t, err)
	assert.Nil(t, pending)

	// Case: some pending data
	pd.lastSubmittedDataHeight.Store(2)
	mockStore.On("Height", ctx).Return(uint64(4), nil).Once()
	for i := uint64(3); i <= 4; i++ {
		data := &types.Data{Metadata: &types.Metadata{Height: i}}
		mockStore.On("GetBlockData", ctx, i).Return(nil, data, nil).Once()
	}
	pending, err = pd.GetPendingData()
	assert.NoError(t, err)
	assert.Len(t, pending, 2)
	assert.Equal(t, uint64(3), pending[0].Data.Height())
	assert.Equal(t, uint64(4), pending[1].Data.Height())

	// Case: error in store
	pd.lastSubmittedDataHeight.Store(4)
	mockStore.On("Height", ctx).Return(uint64(5), nil).Once()
	mockStore.On("GetBlockData", ctx, uint64(5)).Return(nil, nil, errors.New("err")).Once()
	pending, err = pd.GetPendingData()
	assert.Error(t, err)
}

func TestPendingData_isEmpty_numPendingData(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pd, err := NewPendingData(mockStore, logger)
	require.NoError(t, err)

	// isEmpty true
	pd.lastSubmittedDataHeight.Store(10)
	mockStore.On("Height", mock.Anything).Return(uint64(10), nil).Once()
	assert.True(t, pd.isEmpty())

	// isEmpty false
	pd.lastSubmittedDataHeight.Store(5)
	mockStore.On("Height", mock.Anything).Return(uint64(10), nil).Once()
	assert.False(t, pd.isEmpty())

	// numPendingData
	pd.lastSubmittedDataHeight.Store(3)
	mockStore.On("Height", mock.Anything).Return(uint64(7), nil).Once()
	assert.Equal(t, uint64(4), pd.numPendingData())
}

func TestPendingData_setLastSubmittedDataHeight(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pd, err := NewPendingData(mockStore, logger)
	require.NoError(t, err)

	ctx := context.Background()
	pd.lastSubmittedDataHeight.Store(2)
	// Should update
	mockStore.On("SetMetadata", ctx, LastSubmittedDataHeightKey, mock.Anything).Return(nil).Once()
	pd.setLastSubmittedDataHeight(ctx, 5)
	assert.Equal(t, uint64(5), pd.lastSubmittedDataHeight.Load())

	// Should not update (new <= old)
	pd.lastSubmittedDataHeight.Store(5)
	pd.setLastSubmittedDataHeight(ctx, 4)
	assert.Equal(t, uint64(5), pd.lastSubmittedDataHeight.Load())
}

func TestPendingData_init_with_existing_metadata(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	// Simulate existing metadata
	val := make([]byte, 8)
	binary.LittleEndian.PutUint64(val, 7)
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(val, nil).Once()
	pd := &PendingData{store: mockStore, logger: logger}
	err := pd.init()
	assert.NoError(t, err)
	assert.Equal(t, uint64(7), pd.lastSubmittedDataHeight.Load())
}

func TestPendingData_init_invalid_length(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return([]byte{1, 2}, nil).Once()
	pd := &PendingData{store: mockStore, logger: logger}
	err := pd.init()
	assert.Error(t, err)
}
