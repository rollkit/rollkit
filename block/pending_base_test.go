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

// TestPendingBase_InitAndGetLastSubmittedDataHeight verifies that pendingBase initializes correctly
// and returns the correct last submitted height when no metadata is present.
func TestPendingBase_InitAndGetLastSubmittedDataHeight(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pb, err := newPendingBase[*types.SignedData](mockStore, logger, LastSubmittedDataHeightKey, fetchSignedData)
	assert.NoError(t, err)
	assert.NotNil(t, pb)
	assert.Equal(t, uint64(0), pb.lastHeight.Load())
}

// TestPendingBase_GetPending_AllCases checks pendingBase.getPending for no data, all data submitted,
// some pending data, and error cases.
func TestPendingBase_GetPending_AllCases(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pb, err := newPendingBase[*types.SignedData](mockStore, logger, LastSubmittedDataHeightKey, fetchSignedData)
	require.NoError(t, err)

	ctx := context.Background()

	// Case: no data
	mockStore.On("Height", ctx).Return(uint64(0), nil).Once()
	pending, err := pb.getPending(ctx)
	assert.NoError(t, err)
	assert.Nil(t, pending)

	// Case: all data submitted
	pb.lastHeight.Store(5)
	mockStore.On("Height", ctx).Return(uint64(5), nil).Once()
	pending, err = pb.getPending(ctx)
	assert.NoError(t, err)
	assert.Nil(t, pending)

	// Case: some pending data
	pb.lastHeight.Store(2)
	mockStore.On("Height", ctx).Return(uint64(4), nil).Once()
	for i := uint64(3); i <= 4; i++ {
		data := &types.Data{Metadata: &types.Metadata{Height: i}}
		mockStore.On("GetBlockData", ctx, i).Return(nil, data, nil).Once()
	}
	pending, err = pb.getPending(ctx)
	assert.NoError(t, err)
	assert.Len(t, pending, 2)
	assert.Equal(t, uint64(3), pending[0].Data.Height())
	assert.Equal(t, uint64(4), pending[1].Data.Height())

	// Case: error in store
	pb.lastHeight.Store(4)
	mockStore.On("Height", ctx).Return(uint64(5), nil).Once()
	mockStore.On("GetBlockData", ctx, uint64(5)).Return(nil, nil, errors.New("err")).Once()
	pending, err = pb.getPending(ctx)
	assert.Error(t, err)
	assert.Nil(t, pending)
}

// TestPendingBase_isEmpty_numPending verifies isEmpty and numPending methods for correct behavior
// when there are no pending items, some pending items, and all items submitted.
func TestPendingBase_isEmpty_numPending(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pb, err := newPendingBase[*types.SignedData](mockStore, logger, LastSubmittedDataHeightKey, fetchSignedData)
	require.NoError(t, err)

	// isEmpty true
	pb.lastHeight.Store(10)
	mockStore.On("Height", mock.Anything).Return(uint64(10), nil).Once()
	assert.True(t, pb.isEmpty())

	// isEmpty false
	pb.lastHeight.Store(5)
	mockStore.On("Height", mock.Anything).Return(uint64(10), nil).Once()
	assert.False(t, pb.isEmpty())

	// numPending
	pb.lastHeight.Store(3)
	mockStore.On("Height", mock.Anything).Return(uint64(7), nil).Once()
	assert.Equal(t, uint64(4), pb.numPending())
}

// TestPendingBase_setLastSubmittedHeight checks that setLastSubmittedHeight updates the last height
// and persists it only when the new height is greater than the current one.
func TestPendingBase_setLastSubmittedHeight(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(nil, ds.ErrNotFound).Once()
	pb, err := newPendingBase[*types.SignedData](mockStore, logger, LastSubmittedDataHeightKey, fetchSignedData)
	require.NoError(t, err)

	ctx := context.Background()
	pb.lastHeight.Store(2)
	// Should update
	mockStore.On("SetMetadata", ctx, LastSubmittedDataHeightKey, mock.Anything).Return(nil).Once()
	pb.setLastSubmittedHeight(ctx, 5)
	assert.Equal(t, uint64(5), pb.lastHeight.Load())

	// Should not update (new <= old)
	pb.lastHeight.Store(5)
	pb.setLastSubmittedHeight(ctx, 4)
	assert.Equal(t, uint64(5), pb.lastHeight.Load())
}

// TestPendingBase_init_with_existing_metadata ensures that pendingBase initializes its last height
// from existing metadata in the store.
func TestPendingBase_init_with_existing_metadata(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	val := make([]byte, 8)
	binary.LittleEndian.PutUint64(val, 7)
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return(val, nil).Once()
	pb := &pendingBase[*types.SignedData]{store: mockStore, logger: logger, metaKey: LastSubmittedDataHeightKey, fetch: fetchSignedData}
	err := pb.init()
	assert.NoError(t, err)
	assert.Equal(t, uint64(7), pb.lastHeight.Load())
}

// TestPendingBase_init_invalid_length checks that pendingBase.init returns an error
// if the stored metadata for last height is not the expected length.
func TestPendingBase_init_invalid_length(t *testing.T) {
	mockStore := mocksStore.NewStore(t)
	logger := log.NewNopLogger()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedDataHeightKey).Return([]byte{1, 2}, nil).Once()
	pb := &pendingBase[*types.SignedData]{store: mockStore, logger: logger, metaKey: LastSubmittedDataHeightKey, fetch: fetchSignedData}
	err := pb.init()
	assert.Error(t, err)
}
