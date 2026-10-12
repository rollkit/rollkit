package store

import (
	"context"
	"errors"
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/evstack/ev-node/pkg/telemetry/testutil"
	"github.com/evstack/ev-node/types"
)

type tracingMockStore struct {
	heightFn           func(ctx context.Context) (uint64, error)
	getBlockDataFn     func(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error)
	getBlockByHashFn   func(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error)
	getSignatureFn     func(ctx context.Context, height uint64) (*types.Signature, error)
	getSignatureByHash func(ctx context.Context, hash []byte) (*types.Signature, error)
	getHeaderFn        func(ctx context.Context, height uint64) (*types.SignedHeader, error)
	getStateFn         func(ctx context.Context) (types.State, error)
	getStateAtHeightFn func(ctx context.Context, height uint64) (types.State, error)
	getMetadataFn      func(ctx context.Context, key string) ([]byte, error)

	setMetadataFn         func(ctx context.Context, key string, value []byte) error
	batchMetadataFn       func(ctx context.Context, puts []MetadataKV, deletes []string) error
	deleteMetadataFn      func(ctx context.Context, key string) error
	rollbackFn            func(ctx context.Context, height uint64, aggregator bool) error
	pruneBlocksFn         func(ctx context.Context, height uint64) error
	deleteStateAtHeightFn func(ctx context.Context, height uint64) error
	newBatchFn            func(ctx context.Context) (Batch, error)
}

func (m *tracingMockStore) Height(ctx context.Context) (uint64, error) {
	if m.heightFn != nil {
		return m.heightFn(ctx)
	}
	return 0, nil
}

func (m *tracingMockStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	if m.getBlockDataFn != nil {
		return m.getBlockDataFn(ctx, height)
	}
	return nil, nil, nil
}

func (m *tracingMockStore) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	if m.getBlockByHashFn != nil {
		return m.getBlockByHashFn(ctx, hash)
	}
	return nil, nil, nil
}

func (m *tracingMockStore) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	if m.getSignatureFn != nil {
		return m.getSignatureFn(ctx, height)
	}
	return nil, nil
}

func (m *tracingMockStore) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	if m.getSignatureByHash != nil {
		return m.getSignatureByHash(ctx, hash)
	}
	return nil, nil
}

func (m *tracingMockStore) GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error) {
	if m.getHeaderFn != nil {
		return m.getHeaderFn(ctx, height)
	}
	return nil, nil
}

func (m *tracingMockStore) GetState(ctx context.Context) (types.State, error) {
	if m.getStateFn != nil {
		return m.getStateFn(ctx)
	}
	return types.State{}, nil
}

func (m *tracingMockStore) GetStateAtHeight(ctx context.Context, height uint64) (types.State, error) {
	if m.getStateAtHeightFn != nil {
		return m.getStateAtHeightFn(ctx, height)
	}
	return types.State{}, nil
}

func (m *tracingMockStore) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	if m.getMetadataFn != nil {
		return m.getMetadataFn(ctx, key)
	}
	return nil, nil
}

func (m *tracingMockStore) SetMetadata(ctx context.Context, key string, value []byte) error {
	if m.setMetadataFn != nil {
		return m.setMetadataFn(ctx, key, value)
	}
	return nil
}

func (m *tracingMockStore) BatchMetadata(ctx context.Context, puts []MetadataKV, deletes []string) error {
	if m.batchMetadataFn != nil {
		return m.batchMetadataFn(ctx, puts, deletes)
	}
	return nil
}

func (m *tracingMockStore) DeleteMetadata(ctx context.Context, key string) error {
	if m.deleteMetadataFn != nil {
		return m.deleteMetadataFn(ctx, key)
	}
	return nil
}

func (m *tracingMockStore) Rollback(ctx context.Context, height uint64, aggregator bool) error {
	if m.rollbackFn != nil {
		return m.rollbackFn(ctx, height, aggregator)
	}
	return nil
}

func (m *tracingMockStore) PruneBlocks(ctx context.Context, height uint64) error {
	if m.pruneBlocksFn != nil {
		return m.pruneBlocksFn(ctx, height)
	}
	return nil
}

func (m *tracingMockStore) PruneStates(ctx context.Context, height uint64) error {
	return nil
}

func (m *tracingMockStore) DeleteStateAtHeight(ctx context.Context, height uint64) error {
	if m.deleteStateAtHeightFn != nil {
		return m.deleteStateAtHeightFn(ctx, height)
	}
	return nil
}

func (m *tracingMockStore) Close() error {
	return nil
}

func (m *tracingMockStore) NewBatch(ctx context.Context) (Batch, error) {
	if m.newBatchFn != nil {
		return m.newBatchFn(ctx)
	}
	return &tracingMockBatch{}, nil
}

type tracingMockBatch struct {
	saveBlockDataFn func(header *types.SignedHeader, data *types.Data, signature *types.Signature) error
	setHeightFn     func(height uint64) error
	updateStateFn   func(state types.State) error
	commitFn        func() error
	putFn           func(key ds.Key, value []byte) error
	deleteFn        func(key ds.Key) error
}

func (b *tracingMockBatch) SaveBlockData(header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	if b.saveBlockDataFn != nil {
		return b.saveBlockDataFn(header, data, signature)
	}
	return nil
}

func (b *tracingMockBatch) SaveBlockDataFromBytes(header *types.SignedHeader, _, _ []byte, signature *types.Signature) error {
	// Delegate to SaveBlockData for test mocking purposes.
	return b.SaveBlockData(header, nil, signature)
}

func (b *tracingMockBatch) SetHeight(height uint64) error {
	if b.setHeightFn != nil {
		return b.setHeightFn(height)
	}
	return nil
}

func (b *tracingMockBatch) UpdateState(state types.State) error {
	if b.updateStateFn != nil {
		return b.updateStateFn(state)
	}
	return nil
}

func (b *tracingMockBatch) Commit() error {
	if b.commitFn != nil {
		return b.commitFn()
	}
	return nil
}

func (b *tracingMockBatch) Put(key ds.Key, value []byte) error {
	if b.putFn != nil {
		return b.putFn(key, value)
	}
	return nil
}

func (b *tracingMockBatch) Delete(key ds.Key) error {
	if b.deleteFn != nil {
		return b.deleteFn(key)
	}
	return nil
}

func setupStoreTrace(t *testing.T, inner Store) (Store, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)
	return WithTracingStore(inner), sr
}

func TestTracedStore_Height_Success(t *testing.T) {
	mock := &tracingMockStore{
		heightFn: func(ctx context.Context) (uint64, error) {
			return 100, nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	height, err := store.Height(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), height)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "Store.Height", span.Name())
	require.Equal(t, codes.Unset, span.Status().Code)

	attrs := span.Attributes()
	testutil.RequireAttribute(t, attrs, "height", int64(100))
}

func TestTracedStore_Height_Error(t *testing.T) {
	expectedErr := errors.New("height error")
	mock := &tracingMockStore{
		heightFn: func(ctx context.Context) (uint64, error) {
			return 0, expectedErr
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	_, err := store.Height(ctx)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, codes.Error, span.Status().Code)
}

func TestTracedStore_GetBlockData_Success(t *testing.T) {
	expectedHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 50}}}
	expectedData := &types.Data{}
	mock := &tracingMockStore{
		getBlockDataFn: func(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
			return expectedHeader, expectedData, nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	header, data, err := store.GetBlockData(ctx, 50)
	require.NoError(t, err)
	require.Equal(t, expectedHeader, header)
	require.Equal(t, expectedData, data)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "Store.GetBlockData", span.Name())

	attrs := span.Attributes()
	testutil.RequireAttribute(t, attrs, "height", int64(50))
}

func TestTracedStore_GetState_Success(t *testing.T) {
	expectedState := types.State{LastBlockHeight: 200}
	mock := &tracingMockStore{
		getStateFn: func(ctx context.Context) (types.State, error) {
			return expectedState, nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	state, err := store.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedState, state)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "Store.GetState", span.Name())

	attrs := span.Attributes()
	testutil.RequireAttribute(t, attrs, "state.height", int64(200))
}

func TestTracedStore_Rollback_Success(t *testing.T) {
	mock := &tracingMockStore{
		rollbackFn: func(ctx context.Context, height uint64, aggregator bool) error {
			return nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	err := store.Rollback(ctx, 50, true)
	require.NoError(t, err)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "Store.Rollback", span.Name())

	attrs := span.Attributes()
	testutil.RequireAttribute(t, attrs, "height", int64(50))
	testutil.RequireAttribute(t, attrs, "aggregator", true)
}

func TestTracedStore_NewBatch_Success(t *testing.T) {
	mock := &tracingMockStore{
		newBatchFn: func(ctx context.Context) (Batch, error) {
			return &tracingMockBatch{}, nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NotNil(t, batch)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	require.Equal(t, "Store.NewBatch", span.Name())
}

func TestTracedBatch_Commit_Success(t *testing.T) {
	mock := &tracingMockStore{
		newBatchFn: func(ctx context.Context) (Batch, error) {
			return &tracingMockBatch{}, nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)

	err = batch.Commit()
	require.NoError(t, err)

	spans := sr.Ended()
	require.Len(t, spans, 2)
	require.Equal(t, "Store.NewBatch", spans[0].Name())
	require.Equal(t, "Batch.Commit", spans[1].Name())
}

func TestTracedBatch_SaveBlockData_Success(t *testing.T) {
	mock := &tracingMockStore{
		newBatchFn: func(ctx context.Context) (Batch, error) {
			return &tracingMockBatch{}, nil
		},
	}
	store, sr := setupStoreTrace(t, mock)
	ctx := context.Background()

	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)

	header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 100}}}
	data := &types.Data{}
	sig := &types.Signature{}

	err = batch.SaveBlockData(header, data, sig)
	require.NoError(t, err)

	spans := sr.Ended()
	require.Len(t, spans, 2)
	require.Equal(t, "Store.NewBatch", spans[0].Name())
	require.Equal(t, "Batch.SaveBlockData", spans[1].Name())

	attrs := spans[1].Attributes()
	testutil.RequireAttribute(t, attrs, "height", int64(100))
}
