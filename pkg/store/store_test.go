package store

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/types"
)

// mockBatchingDatastore is a mock implementation of ds.Batching for testing error cases.
type mockBatchingDatastore struct {
	ds.Batching          // Embed ds.Batching directly
	mockBatch            *mockBatch
	putError             error
	getError             error // General get error, used if getErrors is empty
	batchError           error
	commitError          error
	unmarshalErrorOnCall int     // New field: 0 for no unmarshal error, 1 for first Get, 2 for second Get, etc.
	getCallCount         int     // Tracks number of Get calls
	getErrors            []error // Specific errors for sequential Get calls
	getMetadataError     error   // Specific error for GetMetadata calls
}

// mockBatch is a mock implementation of ds.Batch for testing error cases.
type mockBatch struct {
	ds.Batch
	putError    error
	commitError error
}

func (m *mockBatchingDatastore) Put(ctx context.Context, key ds.Key, value []byte) error {
	if m.putError != nil {
		return m.putError
	}
	return m.Batching.Put(ctx, key, value)
}

func (m *mockBatchingDatastore) Get(ctx context.Context, key ds.Key) ([]byte, error) {
	// Check for specific metadata error for DA included height key
	if m.getMetadataError != nil && key.String() == "/m/d" {
		return nil, m.getMetadataError
	}

	m.getCallCount++
	if len(m.getErrors) >= m.getCallCount && m.getErrors[m.getCallCount-1] != nil {
		return nil, m.getErrors[m.getCallCount-1]
	}
	if m.getError != nil { // Fallback to general getError if sequential not set
		return nil, m.getError
	}
	if m.unmarshalErrorOnCall == m.getCallCount { // Trigger unmarshal error on specific call
		return []byte("malformed data"), nil
	}
	return m.Batching.Get(ctx, key)
}

func (m *mockBatchingDatastore) Batch(ctx context.Context) (ds.Batch, error) {
	if m.batchError != nil {
		return nil, m.batchError
	}
	baseBatch, err := m.Batching.Batch(ctx)
	if err != nil {
		return nil, err
	}
	m.mockBatch = &mockBatch{Batch: baseBatch}
	m.mockBatch.putError = m.putError // Propagate put error to batch
	m.mockBatch.commitError = m.commitError
	return m.mockBatch, nil
}

func (m *mockBatch) Put(ctx context.Context, key ds.Key, value []byte) error {
	if m.putError != nil {
		return m.putError
	}
	return m.Batch.Put(ctx, key, value)
}

func (m *mockBatch) Commit(ctx context.Context) error {
	if m.commitError != nil {
		return m.commitError
	}
	return m.Batch.Commit(ctx)
}

func TestStoreHeight(t *testing.T) {
	t.Parallel()
	chainID := "TestStoreHeight"
	header1, data1 := types.GetRandomBlock(1, 0, chainID)
	header2, data2 := types.GetRandomBlock(1, 0, chainID)
	header3, data3 := types.GetRandomBlock(2, 0, chainID)
	header4, data4 := types.GetRandomBlock(2, 0, chainID)
	header5, data5 := types.GetRandomBlock(3, 0, chainID)
	header6, data6 := types.GetRandomBlock(1, 0, chainID)
	header7, data7 := types.GetRandomBlock(1, 0, chainID)
	header8, data8 := types.GetRandomBlock(9, 0, chainID)
	header9, data9 := types.GetRandomBlock(10, 0, chainID)
	cases := []struct {
		name     string
		headers  []*types.SignedHeader
		data     []*types.Data
		expected uint64
	}{
		{"single block", []*types.SignedHeader{header1}, []*types.Data{data1}, 1},
		{"two consecutive blocks", []*types.SignedHeader{header2, header3}, []*types.Data{data2, data3}, 2},
		{"blocks out of order", []*types.SignedHeader{header4, header5, header6}, []*types.Data{data4, data5, data6}, 3},
		{"with a gap", []*types.SignedHeader{header7, header8, header9}, []*types.Data{data7, data8, data9}, 10},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			ds, _ := NewTestInMemoryKVStore()
			bstore := New(ds)
			height, err := bstore.Height(t.Context())
			assert.NoError(err)
			assert.Equal(uint64(0), height)

			for i, header := range c.headers {
				data := c.data[i]
				batch, err := bstore.NewBatch(t.Context())
				require.NoError(t, err)
				err = batch.SaveBlockData(header, data, &types.Signature{})
				require.NoError(t, err)
				err = batch.SetHeight(header.Height())
				require.NoError(t, err)
				err = batch.Commit()
				require.NoError(t, err)
			}

			height, err = bstore.Height(t.Context())
			assert.NoError(err)
			assert.Equal(c.expected, height)
		})
	}
}

func TestStoreLoad(t *testing.T) {
	t.Parallel()
	chainID := "TestStoreLoad"
	header1, data1 := types.GetRandomBlock(1, 10, chainID)
	header2, data2 := types.GetRandomBlock(1, 10, chainID)
	header3, data3 := types.GetRandomBlock(2, 20, chainID)
	cases := []struct {
		name    string
		headers []*types.SignedHeader
		data    []*types.Data
	}{
		{"single block", []*types.SignedHeader{header1}, []*types.Data{data1}},
		{"two consecutive blocks", []*types.SignedHeader{header2, header3}, []*types.Data{data2, data3}},
		// TODO(tzdybal): this test needs extra handling because of lastCommits
		//{"blocks out of order", []*types.Block{
		//	getRandomBlock(2, 20),
		//	getRandomBlock(3, 30),
		//	getRandomBlock(4, 100),
		//	getRandomBlock(5, 10),
		//	getRandomBlock(1, 10),
		//}},
	}

	tmpDir := t.TempDir()

	mKV, _ := NewTestInMemoryKVStore()
	dKV, _ := NewDefaultKVStore(tmpDir, "db", "test")
	for _, kv := range []ds.Batching{mKV, dKV} {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assert := assert.New(t)
				require := require.New(t)

				bstore := New(kv)

				for i, header := range c.headers {
					data := c.data[i]
					signature := &header.Signature
					batch, err := bstore.NewBatch(t.Context())
					require.NoError(err)
					err = batch.SaveBlockData(header, data, signature)
					require.NoError(err)
					err = batch.Commit()
					require.NoError(err)
				}

				for i, expectedHeader := range c.headers {
					expectedData := c.data[i]
					header, data, err := bstore.GetBlockData(t.Context(), expectedHeader.Height())
					assert.NoError(err)
					assert.NotNil(header)
					assert.NotNil(data)
					assert.Equal(expectedHeader, header)
					assert.Equal(expectedData, data)

					signature, err := bstore.GetSignature(t.Context(), expectedHeader.Height())
					assert.NoError(err)
					assert.NotNil(signature)
				}
			})
		}
	}
}

func TestRestart(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)
	require := require.New(t)

	tmpDir := t.TempDir()

	kv, err := NewDefaultKVStore(tmpDir, "test", "test")
	require.NoError(err)

	s1 := New(kv)
	expectedHeight := uint64(10)
	batch, err := s1.NewBatch(t.Context())
	require.NoError(err)
	err = batch.SetHeight(expectedHeight)
	require.NoError(err)
	err = batch.UpdateState(types.State{
		LastBlockHeight: expectedHeight,
	})
	assert.NoError(err)
	err = batch.Commit()
	assert.NoError(err)

	err = s1.Close()
	assert.NoError(err)

	kv, err = NewDefaultKVStore(tmpDir, "test", "test")
	require.NoError(err)

	s2 := New(kv)
	assert.NoError(err)

	state2, err := s2.GetState(t.Context())
	assert.NoError(err)

	err = s2.Close()
	assert.NoError(err)

	assert.Equal(expectedHeight, state2.LastBlockHeight)
}

func TestSetHeightError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// Simulate an error when putting data
	mockErr := fmt.Errorf("mock put error")
	mockDs, _ := NewTestInMemoryKVStore()
	mockBatchingDs := &mockBatchingDatastore{Batching: mockDs, putError: mockErr}
	s := New(mockBatchingDs)

	batch, err := s.NewBatch(t.Context())
	require.NoError(err)
	err = batch.SetHeight(10)
	require.Error(err)
	require.Contains(err.Error(), mockErr.Error())
}

func TestHeightErrors(t *testing.T) {
	t.Parallel()

	type cfg struct {
		name      string
		mock      *mockBatchingDatastore
		expectSub string
	}
	mockErr := func(msg string) error { return fmt.Errorf("mock %s error", msg) }

	// badHeightBytes triggers decodeHeight length check
	badHeightBytes := []byte("bad")

	inMem := mustNewInMem()

	cases := []cfg{
		{
			name:      "store get error",
			mock:      &mockBatchingDatastore{Batching: mustNewInMem(), getError: mockErr("get height")},
			expectSub: "get height",
		},
		{
			name:      "unmarshal height error",
			mock:      &mockBatchingDatastore{Batching: inMem},
			expectSub: "invalid height length",
		},
	}

	// pre-seed the second case once
	_ = cases[1].mock.Put(context.Background(), ds.NewKey(getHeightKey()), badHeightBytes)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := New(tc.mock)

			h, err := s.Height(t.Context())
			require.ErrorContains(t, err, tc.expectSub)
			require.Equal(t, uint64(0), h)
		})
	}
}

func TestMetadata(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	kv, err := NewTestInMemoryKVStore()
	require.NoError(err)
	s := New(kv)

	getKey := func(i int) string {
		return fmt.Sprintf("key %d", i)
	}
	getValue := func(i int) []byte {
		return fmt.Appendf(nil, "value %d", i)
	}

	const n = 5
	for i := range n {
		require.NoError(s.SetMetadata(t.Context(), getKey(i), getValue(i)))
	}

	for i := range n {
		value, err := s.GetMetadata(t.Context(), getKey(i))
		require.NoError(err)
		require.Equal(getValue(i), value)
	}

	v, err := s.GetMetadata(t.Context(), "unused key")
	require.Error(err)
	require.Nil(v)
}

func TestGetBlockDataErrors(t *testing.T) {
	t.Parallel()
	chainID := "TestGetBlockDataErrors"
	header, _ := types.GetRandomBlock(1, 0, chainID)
	headerBlob, _ := header.MarshalBinary()

	mockErr := func(msg string) error { return fmt.Errorf("mock %s error", msg) }

	type cfg struct {
		name      string
		mock      *mockBatchingDatastore
		prepare   func(context.Context, *mockBatchingDatastore)
		expectSub string
	}
	cases := []cfg{
		{
			name:      "header get error",
			mock:      &mockBatchingDatastore{Batching: mustNewInMem(), getError: mockErr("get header")},
			prepare:   func(_ context.Context, _ *mockBatchingDatastore) {}, // nothing to pre-seed
			expectSub: "load block header",
		},
		{
			name: "header unmarshal error",
			mock: &mockBatchingDatastore{Batching: mustNewInMem(), unmarshalErrorOnCall: 1},
			prepare: func(ctx context.Context, m *mockBatchingDatastore) {
				_ = m.Put(ctx, ds.NewKey(getHeaderKey(header.Height())), []byte("garbage"))
			},
			expectSub: "unmarshal block header",
		},
		{
			name: "data get error",
			mock: &mockBatchingDatastore{Batching: mustNewInMem(), getErrors: []error{nil, mockErr("get data")}},
			prepare: func(ctx context.Context, m *mockBatchingDatastore) {
				_ = m.Put(ctx, ds.NewKey(getHeaderKey(header.Height())), headerBlob)
			},
			expectSub: "failed to load block data",
		},
		{
			name: "data unmarshal error",
			mock: &mockBatchingDatastore{Batching: mustNewInMem(), unmarshalErrorOnCall: 2},
			prepare: func(ctx context.Context, m *mockBatchingDatastore) {
				_ = m.Put(ctx, ds.NewKey(getHeaderKey(header.Height())), headerBlob)
				_ = m.Put(ctx, ds.NewKey(getDataKey(header.Height())), []byte("garbage"))
			},
			expectSub: "failed to unmarshal block data",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := New(tc.mock)
			tc.prepare(t.Context(), tc.mock)

			_, _, err := s.GetBlockData(t.Context(), header.Height())
			require.ErrorContains(t, err, tc.expectSub)
		})
	}
}

func TestSaveBlockDataErrors(t *testing.T) {
	t.Parallel()

	type errConf struct {
		name      string
		mock      *mockBatchingDatastore
		expectSub string
	}
	chainID := "TestSaveBlockDataErrors"
	header, data := types.GetRandomBlock(1, 0, chainID)
	signature := &types.Signature{}

	mockErr := func(msg string) error { return fmt.Errorf("mock %s error", msg) }

	cases := []errConf{
		{
			name:      "batch creation error",
			mock:      &mockBatchingDatastore{Batching: mustNewInMem(), batchError: mockErr("batch")},
			expectSub: "failed to create a new batch",
		},
		{
			name:      "header put error",
			mock:      &mockBatchingDatastore{Batching: mustNewInMem(), putError: mockErr("put header")},
			expectSub: "failed to put header blob",
		},
		{
			name:      "commit error",
			mock:      &mockBatchingDatastore{Batching: mustNewInMem(), commitError: mockErr("commit")},
			expectSub: "failed to commit batch",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := New(tc.mock)

			batch, err := s.NewBatch(t.Context())
			if tc.expectSub == "failed to create a new batch" {
				require.ErrorContains(t, err, tc.expectSub)
				return
			}
			require.NoError(t, err)
			err = batch.SaveBlockData(header, data, signature)
			if tc.expectSub == "failed to put header blob" {
				require.ErrorContains(t, err, tc.expectSub)
				return
			}
			require.NoError(t, err)
			err = batch.Commit()
			require.ErrorContains(t, err, tc.expectSub)
		})
	}
}

// mustNewInMem is a test-helper that panics if the in-memory KV store cannot be created.
func mustNewInMem() ds.Batching {
	m, err := NewTestInMemoryKVStore()
	if err != nil {
		panic(err)
	}
	return m
}

func TestGetBlockByHashError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	hash := []byte("test_hash")

	// Simulate getHeightByHash error
	mockErr := fmt.Errorf("mock get height by hash error")
	mockDs, _ := NewTestInMemoryKVStore()
	mockBatchingDs := &mockBatchingDatastore{Batching: mockDs, getError: mockErr} // This will cause getHeightByHash to fail
	s := New(mockBatchingDs)

	_, _, err := s.GetBlockByHash(t.Context(), hash)
	require.Error(err)
	require.Contains(err.Error(), mockErr.Error())
	require.Contains(err.Error(), "failed to load height from index")
}

func TestGetSignatureByHashError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	hash := []byte("test_hash")

	// Simulate getHeightByHash error
	mockErr := fmt.Errorf("mock get height by hash error")
	mockDs, _ := NewTestInMemoryKVStore()
	mockBatchingDs := &mockBatchingDatastore{Batching: mockDs, getError: mockErr} // This will cause getHeightByHash to fail
	s := New(mockBatchingDs)

	_, err := s.GetSignatureByHash(t.Context(), hash)
	require.Error(err)
	require.Contains(err.Error(), mockErr.Error())
	require.Contains(err.Error(), "failed to load hash from index")
}

func TestGetSignatureError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	height := uint64(1)

	// Simulate get error for signature data
	mockErrGet := fmt.Errorf("mock get signature error")
	mockDsGet, _ := NewTestInMemoryKVStore()
	mockBatchingDsGet := &mockBatchingDatastore{Batching: mockDsGet, getError: mockErrGet}
	sGet := New(mockBatchingDsGet)

	_, err := sGet.GetSignature(t.Context(), height)
	require.Error(err)
	require.Contains(err.Error(), mockErrGet.Error())
	require.Contains(err.Error(), fmt.Sprintf("failed to retrieve signature from height %v", height))
}

func TestUpdateStateError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	state := types.State{} // Empty state for testing

	// Simulate ToProto error (not directly mockable on types.State, but can be simulated if ToProto was part of an interface)
	// For now, we assume types.State.ToProto() doesn't fail unless the underlying data is invalid.

	// Simulate proto.Marshal error (hard to simulate with valid state, but can be if state contains unmarshalable fields)

	// Simulate put error
	mockErrPut := fmt.Errorf("mock put state error")
	mockDsPut, _ := NewTestInMemoryKVStore()
	mockBatchingDsPut := &mockBatchingDatastore{Batching: mockDsPut, putError: mockErrPut}
	sPut := New(mockBatchingDsPut)

	batch, err := sPut.NewBatch(t.Context())
	require.NoError(err)
	err = batch.UpdateState(state)
	require.Error(err)
	require.Contains(err.Error(), mockErrPut.Error())
}

func TestDeleteStateAtHeight(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	kv, err := NewTestInMemoryKVStore()
	require.NoError(err)

	store := New(kv)

	batch, err := store.NewBatch(t.Context())
	require.NoError(err)
	require.NoError(batch.SetHeight(1))
	require.NoError(batch.UpdateState(types.State{LastBlockHeight: 1}))
	require.NoError(batch.Commit())

	require.NoError(store.(*DefaultStore).DeleteStateAtHeight(t.Context(), 1))

	_, err = store.GetStateAtHeight(t.Context(), 1)
	require.ErrorIs(err, ds.ErrNotFound)
}

func TestPruneStates(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	ctx := t.Context()

	kv, err := NewTestInMemoryKVStore()
	require.NoError(err)
	store := New(kv)

	for h := uint64(1); h <= 10; h++ {
		batch, err := store.NewBatch(ctx)
		require.NoError(err)
		require.NoError(batch.SetHeight(h))
		require.NoError(batch.UpdateState(types.State{LastBlockHeight: h}))
		require.NoError(batch.Commit())
	}

	require.NoError(store.PruneStates(ctx, 4))
	// Lower or equal heights are no-ops.
	require.NoError(store.PruneStates(ctx, 2))
	require.NoError(store.PruneStates(ctx, 4))
	// Resumes from the recorded marker.
	require.NoError(store.PruneStates(ctx, 6))

	for h := uint64(1); h <= 6; h++ {
		_, err := store.GetStateAtHeight(ctx, h)
		require.ErrorIs(err, ds.ErrNotFound, "state at height %d should be pruned", h)
	}
	for h := uint64(7); h <= 10; h++ {
		_, err := store.GetStateAtHeight(ctx, h)
		require.NoError(err, "state at height %d should be kept", h)
	}

	meta, err := store.GetMetadata(ctx, LastPrunedStateHeightKey)
	require.NoError(err)
	require.Equal(uint64(6), binary.LittleEndian.Uint64(meta))

	// Block height and current state are untouched.
	height, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(uint64(10), height)
	state, err := store.GetState(ctx)
	require.NoError(err)
	require.Equal(uint64(10), state.LastBlockHeight)
}

func TestGetStateError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// Simulate get error
	mockErrGet := fmt.Errorf("mock get state error")
	mockDsGet, _ := NewTestInMemoryKVStore()
	mockBatchingDsGet := &mockBatchingDatastore{Batching: mockDsGet, getError: mockErrGet}
	sGet := New(mockBatchingDsGet)

	_, err := sGet.GetState(t.Context())
	require.Error(err)
	require.Contains(err.Error(), mockErrGet.Error())

	// Simulate proto.Unmarshal error
	mockDsUnmarshal, _ := NewTestInMemoryKVStore()
	mockBatchingDsUnmarshal := &mockBatchingDatastore{Batching: mockDsUnmarshal, unmarshalErrorOnCall: 3}
	sUnmarshal := New(mockBatchingDsUnmarshal)

	// Put some data that will cause unmarshal error
	height := uint64(1)
	batch, err := sUnmarshal.NewBatch(t.Context())
	require.NoError(err)
	err = batch.SetHeight(height)
	require.NoError(err)
	err = batch.Commit()
	require.NoError(err)

	err = mockBatchingDsUnmarshal.Put(t.Context(), ds.NewKey(getStateAtHeightKey(height)), []byte("invalid state proto"))
	require.NoError(err)

	_, err = sUnmarshal.GetState(t.Context())
	require.Error(err)
	require.Contains(err.Error(), "failed to unmarshal state from protobuf")
}

func TestSetMetadataError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	key := "test_key"
	value := []byte("test_value")

	// Simulate put error
	mockErrPut := fmt.Errorf("mock put metadata error")
	mockDsPut, _ := NewTestInMemoryKVStore()
	mockBatchingDsPut := &mockBatchingDatastore{Batching: mockDsPut, putError: mockErrPut}
	sPut := New(mockBatchingDsPut)

	err := sPut.SetMetadata(t.Context(), key, value)
	require.Error(err)
	require.Contains(err.Error(), mockErrPut.Error())
	require.Contains(err.Error(), fmt.Sprintf("failed to set metadata for key '%s'", key))
}

func TestGetMetadataError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	key := "test_key"

	// Simulate get error
	mockErrGet := fmt.Errorf("mock get metadata error")
	mockDsGet, _ := NewTestInMemoryKVStore()
	mockBatchingDsGet := &mockBatchingDatastore{Batching: mockDsGet, getError: mockErrGet}
	sGet := New(mockBatchingDsGet)

	_, err := sGet.GetMetadata(t.Context(), key)
	require.Error(err)
	require.Contains(err.Error(), mockErrGet.Error())
	require.Contains(err.Error(), fmt.Sprintf("failed to get metadata for key '%s'", key))
}

func TestGetHeader(t *testing.T) {
	t.Parallel()
	chainID := "TestGetHeader"
	header, _ := types.GetRandomBlock(1, 0, chainID)
	headerBlob, _ := header.MarshalBinary()

	mockErr := func(msg string) error { return fmt.Errorf("mock %s error", msg) }

	cases := map[string]struct {
		mock    *mockBatchingDatastore
		prepare func(context.Context, *mockBatchingDatastore)
		expErr  string
	}{
		"all good": {
			mock: &mockBatchingDatastore{Batching: mustNewInMem()},
			prepare: func(ctx context.Context, db *mockBatchingDatastore) {
				_ = db.Put(ctx, ds.NewKey(getHeaderKey(header.Height())), headerBlob)
			},
		},
		"db error": {
			mock:    &mockBatchingDatastore{Batching: mustNewInMem(), getError: mockErr("get header")},
			prepare: func(_ context.Context, _ *mockBatchingDatastore) {}, // nothing to pre-seed
			expErr:  "load block header",
		},
		"unmarshal error": {
			mock: &mockBatchingDatastore{Batching: mustNewInMem(), unmarshalErrorOnCall: 1},
			prepare: func(ctx context.Context, m *mockBatchingDatastore) {
				_ = m.Put(ctx, ds.NewKey(getHeaderKey(header.Height())), []byte("garbage"))
			},
			expErr: "unmarshal block header",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := New(tc.mock)
			tc.prepare(t.Context(), tc.mock)

			gotHeader, gotErr := s.GetHeader(t.Context(), header.Height())
			if tc.expErr != "" {
				require.Error(t, gotErr)
				require.ErrorContains(t, gotErr, tc.expErr)
				return
			}
			require.NoError(t, gotErr)
			assert.Equal(t, header, gotHeader)
		})
	}
}

// TestRollback verifies that rollback successfully removes blocks and updates height
func TestRollback(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	store := New(mustNewInMem())

	// Setup: create and save multiple blocks
	chainID := "test-rollback"
	maxHeight := uint64(10)

	for h := uint64(1); h <= maxHeight; h++ {
		header, data := types.GetRandomBlock(h, 2, chainID)
		sig := &header.Signature

		batch, err := store.NewBatch(ctx)
		require.NoError(err)
		err = batch.SaveBlockData(header, data, sig)
		require.NoError(err)
		err = batch.SetHeight(h)
		require.NoError(err)

		// Create and update state for this height
		state := types.State{
			ChainID:         chainID,
			InitialHeight:   1,
			LastBlockHeight: h,
			LastBlockTime:   header.Time(),
			AppHash:         header.AppHash,
		}
		err = batch.UpdateState(state)
		require.NoError(err)
		err = batch.Commit()
		require.NoError(err)
	}

	// Verify initial state
	height, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(maxHeight, height)

	// Verify all blocks exist
	for h := uint64(1); h <= maxHeight; h++ {
		_, _, err := store.GetBlockData(ctx, h)
		require.NoError(err, "block at height %d should exist", h)
	}

	// Execute rollback to height 7
	rollbackToHeight := uint64(7)
	err = store.Rollback(ctx, rollbackToHeight, true)
	require.NoError(err)

	// Verify new height
	newHeight, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(rollbackToHeight, newHeight)

	// Verify blocks exist only up to rollback height
	for h := uint64(1); h <= rollbackToHeight; h++ {
		_, _, err := store.GetBlockData(ctx, h)
		require.NoError(err, "block at height %d should still exist after rollback", h)
	}

	// Verify blocks above rollback height are removed
	for h := rollbackToHeight + 1; h <= maxHeight; h++ {
		_, _, err := store.GetBlockData(ctx, h)
		require.Error(err, "block at height %d should be removed after rollback", h)
	}

	// Verify state is rolled back
	state, err := store.GetState(ctx)
	require.NoError(err)
	require.Equal(rollbackToHeight, state.LastBlockHeight)
}

func TestPruneBlocks_RemovesOldBlockDataOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)

	s := New(ds).(*DefaultStore)

	// create and store a few blocks with headers, data, signatures, state, and per-height DA metadata
	batch, err := s.NewBatch(ctx)
	require.NoError(t, err)

	var lastState types.State
	for h := uint64(1); h <= 5; h++ {
		header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: h}}}
		data := &types.Data{}
		sig := types.Signature([]byte{byte(h)})

		require.NoError(t, batch.SaveBlockData(header, data, &sig))

		// fake state snapshot per height
		lastState = types.State{LastBlockHeight: h}
		require.NoError(t, batch.UpdateState(lastState))

		// store fake DA metadata per height
		hDaKey := GetHeightToDAHeightHeaderKey(h)
		dDaKey := GetHeightToDAHeightDataKey(h)
		bz := make([]byte, 8)
		binary.LittleEndian.PutUint64(bz, h+100) // arbitrary DA height
		require.NoError(t, s.SetMetadata(ctx, hDaKey, bz))
		require.NoError(t, s.SetMetadata(ctx, dDaKey, bz))
	}
	require.NoError(t, batch.SetHeight(5))
	require.NoError(t, batch.Commit())

	// prune everything up to height 3
	require.NoError(t, s.PruneBlocks(ctx, 3))

	// old block data should be gone
	for h := uint64(1); h <= 3; h++ {
		_, _, err := s.GetBlockData(ctx, h)
		assert.Error(t, err, "expected block data at height %d to be pruned", h)
	}

	// recent block data should remain
	for h := uint64(4); h <= 5; h++ {
		_, _, err := s.GetBlockData(ctx, h)
		assert.NoError(t, err, "expected block data at height %d to be kept", h)
	}

	// state snapshots are not pruned by PruneBlocks
	for h := uint64(1); h <= 5; h++ {
		st, err := s.GetStateAtHeight(ctx, h)
		assert.NoError(t, err, "expected state at height %d to remain", h)
		assert.Equal(t, h, st.LastBlockHeight)
	}

	// per-height DA metadata for pruned heights should be gone
	for h := uint64(1); h <= 3; h++ {
		hDaKey := GetHeightToDAHeightHeaderKey(h)
		dDaKey := GetHeightToDAHeightDataKey(h)
		_, err := s.GetMetadata(ctx, hDaKey)
		assert.Error(t, err, "expected header DA metadata at height %d to be pruned", h)
		_, err = s.GetMetadata(ctx, dDaKey)
		assert.Error(t, err, "expected data DA metadata at height %d to be pruned", h)
	}

	// per-height DA metadata for unpruned heights should remain
	for h := uint64(4); h <= 5; h++ {
		hDaKey := GetHeightToDAHeightHeaderKey(h)
		dDaKey := GetHeightToDAHeightDataKey(h)
		_, err := s.GetMetadata(ctx, hDaKey)
		assert.NoError(t, err, "expected header DA metadata at height %d to remain", h)
		_, err = s.GetMetadata(ctx, dDaKey)
		assert.NoError(t, err, "expected data DA metadata at height %d to remain", h)
	}
}

// TestRollbackToSameHeight verifies that rollback to same height is a no-op
func TestRollbackToSameHeight(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	store := New(mustNewInMem())

	// Setup: create one block
	chainID := "test-rollback-same"
	height := uint64(5)
	header, data := types.GetRandomBlock(height, 2, chainID)
	sig := &header.Signature

	batch, err := store.NewBatch(ctx)
	require.NoError(err)
	err = batch.SaveBlockData(header, data, sig)
	require.NoError(err)
	err = batch.SetHeight(height)
	require.NoError(err)
	err = batch.Commit()
	require.NoError(err)

	// Execute rollback to same height
	err = store.Rollback(ctx, height, true)
	require.Error(err)

	// Verify height unchanged
	newHeight, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(height, newHeight)

	// Verify block still exists
	_, _, err = store.GetBlockData(ctx, height)
	require.NoError(err)
}

// TestRollbackToHigherHeight verifies that rollback to higher height is a no-op
func TestRollbackToHigherHeight(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	store := New(mustNewInMem())

	// Setup: create one block
	chainID := "test-rollback-higher"
	currentHeight := uint64(5)
	header, data := types.GetRandomBlock(currentHeight, 2, chainID)
	sig := &header.Signature

	batch, err := store.NewBatch(ctx)
	require.NoError(err)
	err = batch.SaveBlockData(header, data, sig)
	require.NoError(err)
	err = batch.SetHeight(currentHeight)
	require.NoError(err)
	err = batch.Commit()
	require.NoError(err)

	// Execute rollback to higher height
	rollbackToHeight := uint64(10)
	err = store.Rollback(ctx, rollbackToHeight, true)
	require.Error(err)

	// Verify height unchanged
	newHeight, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(currentHeight, newHeight)

	// Verify block still exists
	_, _, err = store.GetBlockData(ctx, currentHeight)
	require.NoError(err)
}

// TestRollbackBatchError verifies that rollback handles batch creation errors
func TestRollbackBatchError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	mock := &mockBatchingDatastore{
		Batching:   mustNewInMem(),
		batchError: errors.New("batch creation failed"),
	}
	store := New(mock)

	err := store.Rollback(ctx, uint64(5), true)
	require.Error(err)
	require.Contains(err.Error(), "failed to create a new batch")
}

// TestRollbackHeightError verifies that rollback handles height retrieval errors
func TestRollbackHeightError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	mock := &mockBatchingDatastore{
		Batching: mustNewInMem(),
		getError: errors.New("height retrieval failed"),
	}
	store := New(mock)

	err := store.Rollback(ctx, uint64(5), true)
	require.Error(err)
	require.Contains(err.Error(), "failed to get current height")
}

// TestRollbackDAIncludedHeightValidation verifies DA included height validation during rollback
func TestRollbackDAIncludedHeightValidation(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// Test case 1: Rollback to height below DA included height should fail
	t.Run("rollback below DA included height fails as aggregator", func(t *testing.T) {
		ctx := context.Background()
		store := New(mustNewInMem())

		// Setup: create and save multiple blocks
		chainID := "test-rollback-da-fail"
		maxHeight := uint64(10)

		for h := uint64(1); h <= maxHeight; h++ {
			header, data := types.GetRandomBlock(h, 2, chainID)
			sig := &header.Signature

			batch, err := store.NewBatch(ctx)
			require.NoError(err)
			err = batch.SaveBlockData(header, data, sig)
			require.NoError(err)
			err = batch.SetHeight(h)
			require.NoError(err)

			// Create and update state for this height
			state := types.State{
				ChainID:         chainID,
				InitialHeight:   1,
				LastBlockHeight: h,
				LastBlockTime:   header.Time(),
				AppHash:         header.AppHash,
			}
			err = batch.UpdateState(state)
			require.NoError(err)
			err = batch.Commit()
			require.NoError(err)
		}

		// Set DA included height to 8
		daIncludedHeight := uint64(8)
		heightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(heightBytes, daIncludedHeight)
		err := store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		require.NoError(err)

		// Rollback to height below DA included height should fail
		err = store.Rollback(ctx, uint64(6), true)
		require.Error(err)
		require.Contains(err.Error(), "DA included height is greater than the rollback height: cannot rollback a finalized height")
	})

	// Test case 2: Rollback to height below DA included height should succeed as sync node
	t.Run("rollback below DA included succeed as sync node", func(t *testing.T) {
		ctx := context.Background()
		store := New(mustNewInMem())

		// Setup: create and save multiple blocks
		chainID := "test-rollback-da-sync-success"
		maxHeight := uint64(10)

		for h := uint64(1); h <= maxHeight; h++ {
			header, data := types.GetRandomBlock(h, 2, chainID)
			sig := &header.Signature

			batch, err := store.NewBatch(ctx)
			require.NoError(err)
			err = batch.SaveBlockData(header, data, sig)
			require.NoError(err)
			err = batch.SetHeight(h)
			require.NoError(err)

			// Create and update state for this height
			state := types.State{
				ChainID:         chainID,
				InitialHeight:   1,
				LastBlockHeight: h,
				LastBlockTime:   header.Time(),
				AppHash:         header.AppHash,
			}
			err = batch.UpdateState(state)
			require.NoError(err)
			err = batch.Commit()
			require.NoError(err)
		}

		// Set DA included height to 8
		daIncludedHeight := uint64(8)
		heightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(heightBytes, daIncludedHeight)
		err := store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		require.NoError(err)

		// Rollback to height below DA included height should fail
		err = store.Rollback(ctx, uint64(6), false)
		require.NoError(err)

		// Verify height was rolled back to 6
		currentHeight, err := store.Height(ctx)
		require.NoError(err)
		require.Equal(uint64(6), currentHeight)
	})

	// Test case 2: Rollback to height equal to DA included height should succeed
	t.Run("rollback to DA included height succeeds", func(t *testing.T) {
		ctx := context.Background()
		store := New(mustNewInMem())

		// Setup: create and save multiple blocks
		chainID := "test-rollback-da-equal"
		maxHeight := uint64(10)

		for h := uint64(1); h <= maxHeight; h++ {
			header, data := types.GetRandomBlock(h, 2, chainID)
			sig := &header.Signature

			batch, err := store.NewBatch(ctx)
			require.NoError(err)
			err = batch.SaveBlockData(header, data, sig)
			require.NoError(err)
			err = batch.SetHeight(h)
			require.NoError(err)

			// Create and update state for this height
			state := types.State{
				ChainID:         chainID,
				InitialHeight:   1,
				LastBlockHeight: h,
				LastBlockTime:   header.Time(),
				AppHash:         header.AppHash,
			}
			err = batch.UpdateState(state)
			require.NoError(err)
			err = batch.Commit()
			require.NoError(err)
		}

		// Set DA included height to 8
		daIncludedHeight := uint64(8)
		heightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(heightBytes, daIncludedHeight)
		err := store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		require.NoError(err)

		// Rollback to height equal to DA included height should succeed
		err = store.Rollback(ctx, uint64(8), true)
		require.NoError(err)

		// Verify height was rolled back to 8
		currentHeight, err := store.Height(ctx)
		require.NoError(err)
		require.Equal(uint64(8), currentHeight)
	})

	// Test case 3: Rollback to height above DA included height should succeed
	t.Run("rollback above DA included height succeeds", func(t *testing.T) {
		ctx := context.Background()
		store := New(mustNewInMem())

		// Setup: create and save multiple blocks
		chainID := "test-rollback-da-above"
		maxHeight := uint64(10)

		for h := uint64(1); h <= maxHeight; h++ {
			header, data := types.GetRandomBlock(h, 2, chainID)
			sig := &header.Signature

			batch, err := store.NewBatch(ctx)
			require.NoError(err)
			err = batch.SaveBlockData(header, data, sig)
			require.NoError(err)
			err = batch.SetHeight(h)
			require.NoError(err)

			// Create and update state for this height
			state := types.State{
				ChainID:         chainID,
				InitialHeight:   1,
				LastBlockHeight: h,
				LastBlockTime:   header.Time(),
				AppHash:         header.AppHash,
			}
			err = batch.UpdateState(state)
			require.NoError(err)
			err = batch.Commit()
			require.NoError(err)
		}

		// Set DA included height to 8
		daIncludedHeight := uint64(8)
		heightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(heightBytes, daIncludedHeight)
		err := store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		require.NoError(err)

		// Rollback to height above DA included height should succeed
		err = store.Rollback(ctx, uint64(9), true)
		require.NoError(err)

		// Verify height was rolled back to 9
		currentHeight, err := store.Height(ctx)
		require.NoError(err)
		require.Equal(uint64(9), currentHeight)
	})
}

// TestRollbackDAIncludedHeightNotSet verifies rollback works when DA included height is not set
func TestRollbackDAIncludedHeightNotSet(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	store := New(mustNewInMem())

	// Setup: create and save multiple blocks
	chainID := "test-rollback-da-notset"
	maxHeight := uint64(5)

	for h := uint64(1); h <= maxHeight; h++ {
		header, data := types.GetRandomBlock(h, 2, chainID)
		sig := &header.Signature

		batch, err := store.NewBatch(ctx)
		require.NoError(err)
		err = batch.SaveBlockData(header, data, sig)
		require.NoError(err)
		err = batch.SetHeight(h)
		require.NoError(err)

		// Create and update state for this height
		state := types.State{
			ChainID:         chainID,
			InitialHeight:   1,
			LastBlockHeight: h,
			LastBlockTime:   header.Time(),
			AppHash:         header.AppHash,
		}
		err = batch.UpdateState(state)
		require.NoError(err)
		err = batch.Commit()
		require.NoError(err)
	}

	// Don't set DA included height - it should not exist
	// Rollback should succeed since no DA included height is set
	err := store.Rollback(ctx, uint64(3), true)
	require.NoError(err)

	// Verify height was rolled back to 3
	currentHeight, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(uint64(3), currentHeight)
}

// TestRollbackDAIncludedHeightInvalidLength verifies rollback works with invalid DA included height data
func TestRollbackDAIncludedHeightInvalidLength(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	store := New(mustNewInMem())

	// Setup: create and save multiple blocks
	chainID := "test-rollback-da-invalid"
	maxHeight := uint64(5)

	for h := uint64(1); h <= maxHeight; h++ {
		header, data := types.GetRandomBlock(h, 2, chainID)
		sig := &header.Signature

		batch, err := store.NewBatch(ctx)
		require.NoError(err)
		err = batch.SaveBlockData(header, data, sig)
		require.NoError(err)
		err = batch.SetHeight(h)
		require.NoError(err)

		// Create and update state for this height
		state := types.State{
			ChainID:         chainID,
			InitialHeight:   1,
			LastBlockHeight: h,
			LastBlockTime:   header.Time(),
			AppHash:         header.AppHash,
		}
		err = batch.UpdateState(state)
		require.NoError(err)
		err = batch.Commit()
		require.NoError(err)
	}

	// Set DA included height with invalid length (not 8 bytes)
	invalidHeightData := []byte{1, 2, 3, 4} // only 4 bytes
	err := store.SetMetadata(ctx, DAIncludedHeightKey, invalidHeightData)
	require.NoError(err)

	// Rollback should succeed since invalid length data is ignored
	err = store.Rollback(ctx, uint64(3), true)
	require.NoError(err)

	// Verify height was rolled back to 3
	currentHeight, err := store.Height(ctx)
	require.NoError(err)
	require.Equal(uint64(3), currentHeight)
}

// TestRollbackDAIncludedHeightGetMetadataError verifies rollback handles GetMetadata errors for DA included height
func TestRollbackDAIncludedHeightGetMetadataError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx := context.Background()
	mock := &mockBatchingDatastore{
		Batching: mustNewInMem(),
	}
	store := New(mock)

	// Setup: create one block to ensure height > rollback target
	header, data := types.GetRandomBlock(uint64(2), 2, "test-chain")
	sig := &header.Signature
	batch, err := store.NewBatch(ctx)
	require.NoError(err)
	err = batch.SaveBlockData(header, data, sig)
	require.NoError(err)
	err = batch.SetHeight(uint64(2))
	require.NoError(err)

	// Create and update state for this height
	state := types.State{
		ChainID:         "test-chain",
		InitialHeight:   1,
		LastBlockHeight: 2,
		LastBlockTime:   header.Time(),
		AppHash:         header.AppHash,
	}
	err = batch.UpdateState(state)
	require.NoError(err)
	err = batch.Commit()
	require.NoError(err)

	// Configure mock to return error when getting DA included height metadata
	mock.getMetadataError = errors.New("metadata retrieval failed")

	// Rollback should fail due to GetMetadata error
	err = store.Rollback(ctx, uint64(1), true)
	require.Error(err)
	require.Contains(err.Error(), "failed to get DA included height")
	require.Contains(err.Error(), "metadata retrieval failed")
}
