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

	"github.com/rollkit/rollkit/pkg/store"
	mocksStore "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// --- Generic test case struct and helpers for pendingBase tests ---
type pendingBaseTestCase[T any] struct {
	name     string
	key      string
	fetch    func(ctx context.Context, store store.Store, height uint64) (T, error)
	makeItem func(height uint64) T
	// For GetBlockData, returns (header, data, error) for a given height
	mockGetBlockData func(height uint64) (any, any, error)
}

func runPendingBase_InitAndGetLastSubmittedHeight[T any](t *testing.T, tc pendingBaseTestCase[T]) {
	t.Run(tc.name+"/InitAndGetLastSubmittedHeight", func(t *testing.T) {
		mockStore := mocksStore.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, tc.key).Return(nil, ds.ErrNotFound).Once()
		pb, err := newPendingBase(mockStore, logger, tc.key, tc.fetch)
		assert.NoError(t, err)
		assert.NotNil(t, pb)
		assert.Equal(t, uint64(0), pb.lastHeight.Load())
	})
}

func runPendingBase_GetPending_AllCases[T any](t *testing.T, tc pendingBaseTestCase[T]) {
	t.Run(tc.name+"/GetPending_AllCases", func(t *testing.T) {
		mockStore := mocksStore.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, tc.key).Return(nil, ds.ErrNotFound).Once()
		pb, err := newPendingBase(mockStore, logger, tc.key, tc.fetch)
		require.NoError(t, err)
		ctx := context.Background()

		// Case: no items
		mockStore.On("Height", ctx).Return(uint64(0), nil).Once()
		pending, err := pb.getPending(ctx)
		assert.NoError(t, err)
		assert.Nil(t, pending)

		// Case: all items submitted
		pb.lastHeight.Store(5)
		mockStore.On("Height", ctx).Return(uint64(5), nil).Once()
		pending, err = pb.getPending(ctx)
		assert.NoError(t, err)
		assert.Nil(t, pending)

		// Case: some pending items
		pb.lastHeight.Store(2)
		mockStore.On("Height", ctx).Return(uint64(4), nil).Once()
		for i := uint64(3); i <= 4; i++ {
			ret0, ret1, retErr := tc.mockGetBlockData(i)
			mockStore.On("GetBlockData", ctx, i).Return(ret0, ret1, retErr).Once()
		}
		pending, err = pb.getPending(ctx)
		assert.NoError(t, err)
		assert.Len(t, pending, 2)
		// Use reflection to call Height() for both types
		getHeight := func(item any) uint64 {
			return item.(interface{ Height() uint64 }).Height()
		}
		assert.Equal(t, uint64(3), getHeight(pending[0]))
		assert.Equal(t, uint64(4), getHeight(pending[1]))

		// Case: error in store
		pb.lastHeight.Store(4)
		mockStore.On("Height", ctx).Return(uint64(5), nil).Once()
		// For error case, always return error for height 5
		mockStore.On("GetBlockData", ctx, uint64(5)).Return(nil, nil, errors.New("err")).Once()
		pending, err = pb.getPending(ctx)
		assert.Error(t, err)
		assert.Empty(t, pending)
	})
}

func runPendingBase_isEmpty_numPending[T any](t *testing.T, tc pendingBaseTestCase[T]) {
	t.Run(tc.name+"/isEmpty_numPending", func(t *testing.T) {
		mockStore := mocksStore.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, tc.key).Return(nil, ds.ErrNotFound).Once()
		pb, err := newPendingBase(mockStore, logger, tc.key, tc.fetch)
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
	})
}

func runPendingBase_setLastSubmittedHeight[T any](t *testing.T, tc pendingBaseTestCase[T]) {
	t.Run(tc.name+"/setLastSubmittedHeight", func(t *testing.T) {
		mockStore := mocksStore.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, tc.key).Return(nil, ds.ErrNotFound).Once()
		pb, err := newPendingBase(mockStore, logger, tc.key, tc.fetch)
		require.NoError(t, err)

		ctx := context.Background()
		pb.lastHeight.Store(2)
		// Should update
		mockStore.On("SetMetadata", ctx, tc.key, mock.Anything).Return(nil).Once()
		pb.setLastSubmittedHeight(ctx, 5)
		assert.Equal(t, uint64(5), pb.lastHeight.Load())

		// Should not update (new <= old)
		pb.lastHeight.Store(5)
		pb.setLastSubmittedHeight(ctx, 4)
		assert.Equal(t, uint64(5), pb.lastHeight.Load())
	})
}

func runPendingBase_init_with_existing_metadata[T any](t *testing.T, tc pendingBaseTestCase[T]) {
	t.Run(tc.name+"/init_with_existing_metadata", func(t *testing.T) {
		mockStore := mocksStore.NewStore(t)
		logger := log.NewNopLogger()
		val := make([]byte, 8)
		binary.LittleEndian.PutUint64(val, 7)
		mockStore.On("GetMetadata", mock.Anything, tc.key).Return(val, nil).Once()
		pb := &pendingBase[T]{store: mockStore, logger: logger, metaKey: tc.key, fetch: tc.fetch}
		err := pb.init()
		assert.NoError(t, err)
		assert.Equal(t, uint64(7), pb.lastHeight.Load())
	})
}

func runPendingBase_init_invalid_length[T any](t *testing.T, tc pendingBaseTestCase[T]) {
	t.Run(tc.name+"/init_invalid_length", func(t *testing.T) {
		mockStore := mocksStore.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, tc.key).Return([]byte{1, 2}, nil).Once()
		pb := &pendingBase[T]{store: mockStore, logger: logger, metaKey: tc.key, fetch: tc.fetch}
		err := pb.init()
		assert.Error(t, err)
	})
}

func TestPendingBase_Generic(t *testing.T) {
	dataCase := pendingBaseTestCase[*types.Data]{
		name:  "Data",
		key:   LastSubmittedDataHeightKey,
		fetch: fetchData,
		makeItem: func(height uint64) *types.Data {
			return &types.Data{Metadata: &types.Metadata{Height: height}}
		},
		mockGetBlockData: func(height uint64) (any, any, error) {
			return nil, &types.Data{Metadata: &types.Metadata{Height: height}}, nil
		},
	}
	headerCase := pendingBaseTestCase[*types.SignedHeader]{
		name:  "Header",
		key:   LastSubmittedHeaderHeightKey,
		fetch: fetchSignedHeader,
		makeItem: func(height uint64) *types.SignedHeader {
			return &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: height}}}
		},
		mockGetBlockData: func(height uint64) (any, any, error) {
			return &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: height}}}, nil, nil
		},
	}

	cases := []any{dataCase, headerCase}

	for _, c := range cases {
		switch tc := c.(type) {
		case pendingBaseTestCase[*types.Data]:
			runPendingBase_InitAndGetLastSubmittedHeight(t, tc)
			runPendingBase_GetPending_AllCases(t, tc)
			runPendingBase_isEmpty_numPending(t, tc)
			runPendingBase_setLastSubmittedHeight(t, tc)
			runPendingBase_init_with_existing_metadata(t, tc)
			runPendingBase_init_invalid_length(t, tc)
		case pendingBaseTestCase[*types.SignedHeader]:
			runPendingBase_InitAndGetLastSubmittedHeight(t, tc)
			runPendingBase_GetPending_AllCases(t, tc)
			runPendingBase_isEmpty_numPending(t, tc)
			runPendingBase_setLastSubmittedHeight(t, tc)
			runPendingBase_init_with_existing_metadata(t, tc)
			runPendingBase_init_invalid_length(t, tc)
		}
	}
}
