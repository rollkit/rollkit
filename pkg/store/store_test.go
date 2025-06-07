package store

import (
	"context"
	"fmt"
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
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
			ds, _ := NewDefaultInMemoryKVStore()
			bstore := New(ds)
			height, err := bstore.Height(t.Context())
			assert.NoError(err)
			assert.Equal(uint64(0), height)

			for i, header := range c.headers {
				data := c.data[i]
				err := bstore.SaveBlockData(t.Context(), header, data, &types.Signature{})
				require.NoError(t, err)
				err = bstore.SetHeight(t.Context(), header.Height())
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

	mKV, _ := NewDefaultInMemoryKVStore()
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
					err := bstore.SaveBlockData(t.Context(), header, data, signature)
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
	err = s1.UpdateState(t.Context(), types.State{
		LastBlockHeight: expectedHeight,
	})
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
	mockDs, _ := NewDefaultInMemoryKVStore()
	mockBatchingDs := &mockBatchingDatastore{Batching: mockDs, putError: mockErr}
	s := New(mockBatchingDs)

	err := s.SetHeight(t.Context(), 10)
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
		tc := tc
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

	kv, err := NewDefaultInMemoryKVStore()
	require.NoError(err)
	s := New(kv)

	getKey := func(i int) string {
		return fmt.Sprintf("key %d", i)
	}
	getValue := func(i int) []byte {
		return fmt.Appendf(nil, "value %d", i)
	}

	const n = 5
	for i := 0; i < n; i++ {
		require.NoError(s.SetMetadata(t.Context(), getKey(i), getValue(i)))
	}

	for i := 0; i < n; i++ {
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
			expectSub: "failed to load block header",
		},
		{
			name: "header unmarshal error",
			mock: &mockBatchingDatastore{Batching: mustNewInMem(), unmarshalErrorOnCall: 1},
			prepare: func(ctx context.Context, m *mockBatchingDatastore) {
				_ = m.Put(ctx, ds.NewKey(getHeaderKey(header.Height())), []byte("garbage"))
			},
			expectSub: "failed to unmarshal block header",
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

			err := s.SaveBlockData(t.Context(), header, data, signature)
			require.ErrorContains(t, err, tc.expectSub)
		})
	}
}

// mustNewInMem is a test-helper that panics if the in-memory KV store cannot be created.
func mustNewInMem() ds.Batching {
	m, err := NewDefaultInMemoryKVStore()
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
	mockDs, _ := NewDefaultInMemoryKVStore()
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
	mockDs, _ := NewDefaultInMemoryKVStore()
	mockBatchingDs := &mockBatchingDatastore{Batching: mockDs, getError: mockErr} // This will cause getHeightByHash to fail
	s := New(mockBatchingDs)

	_, err := s.GetSignatureByHash(t.Context(), hash)
	require.Error(err)
	require.Contains(err.Error(), mockErr.Error())
	require.Contains(err.Error(), "failed to load height from index")
}

func TestGetSignatureError(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	height := uint64(1)

	// Simulate get error for signature data
	mockErrGet := fmt.Errorf("mock get signature error")
	mockDsGet, _ := NewDefaultInMemoryKVStore()
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
	mockDsPut, _ := NewDefaultInMemoryKVStore()
	mockBatchingDsPut := &mockBatchingDatastore{Batching: mockDsPut, putError: mockErrPut}
	sPut := New(mockBatchingDsPut)

	err := sPut.UpdateState(t.Context(), state)
	require.Error(err)
	require.Contains(err.Error(), mockErrPut.Error())
}

func TestGetStateError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// Simulate get error
	mockErrGet := fmt.Errorf("mock get state error")
	mockDsGet, _ := NewDefaultInMemoryKVStore()
	mockBatchingDsGet := &mockBatchingDatastore{Batching: mockDsGet, getError: mockErrGet}
	sGet := New(mockBatchingDsGet)

	_, err := sGet.GetState(t.Context())
	require.Error(err)
	require.Contains(err.Error(), mockErrGet.Error())
	require.Contains(err.Error(), "failed to retrieve state")

	// Simulate proto.Unmarshal error
	mockDsUnmarshal, _ := NewDefaultInMemoryKVStore()
	mockBatchingDsUnmarshal := &mockBatchingDatastore{Batching: mockDsUnmarshal, unmarshalErrorOnCall: 1}
	sUnmarshal := New(mockBatchingDsUnmarshal)

	// Put some data that will cause unmarshal error
	err = mockBatchingDsUnmarshal.Put(t.Context(), ds.NewKey(getStateKey()), []byte("invalid state proto"))
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
	mockDsPut, _ := NewDefaultInMemoryKVStore()
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
	mockDsGet, _ := NewDefaultInMemoryKVStore()
	mockBatchingDsGet := &mockBatchingDatastore{Batching: mockDsGet, getError: mockErrGet}
	sGet := New(mockBatchingDsGet)

	_, err := sGet.GetMetadata(t.Context(), key)
	require.Error(err)
	require.Contains(err.Error(), mockErrGet.Error())
	require.Contains(err.Error(), fmt.Sprintf("failed to get metadata for key '%s'", key))
}
