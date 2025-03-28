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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			ds, _ := NewDefaultInMemoryKVStore()
			bstore := New(ds)
			assert.Equal(uint64(0), bstore.Height())

			for i, header := range c.headers {
				data := c.data[i]
				err := bstore.SaveBlockData(ctx, header, data, &types.Signature{})
				bstore.SetHeight(ctx, header.Height())
				assert.NoError(err)
			}

			assert.Equal(c.expected, bstore.Height())
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, kv := range []ds.Batching{mKV, dKV} {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assert := assert.New(t)
				require := require.New(t)

				bstore := New(kv)

				for i, header := range c.headers {
					data := c.data[i]
					signature := &header.Signature
					err := bstore.SaveBlockData(ctx, header, data, signature)
					require.NoError(err)
				}

				for i, expectedHeader := range c.headers {
					expectedData := c.data[i]
					header, data, err := bstore.GetBlockData(ctx, expectedHeader.Height())
					assert.NoError(err)
					assert.NotNil(header)
					assert.NotNil(data)
					assert.Equal(expectedHeader, header)
					assert.Equal(expectedData, data)

					signature, err := bstore.GetSignature(ctx, expectedHeader.Height())
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir := t.TempDir()

	kv, err := NewDefaultKVStore(tmpDir, "test", "test")
	require.NoError(err)

	s1 := New(kv)
	expectedHeight := uint64(10)
	err = s1.UpdateState(ctx, types.State{
		LastBlockHeight: expectedHeight,
	})
	assert.NoError(err)

	err = s1.Close()
	assert.NoError(err)

	kv, err = NewDefaultKVStore(tmpDir, "test", "test")
	require.NoError(err)

	s2 := New(kv)
	assert.NoError(err)

	state2, err := s2.GetState(ctx)
	assert.NoError(err)

	err = s2.Close()
	assert.NoError(err)

	assert.Equal(expectedHeight, state2.LastBlockHeight)
}

func TestMetadata(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kv, err := NewDefaultInMemoryKVStore()
	require.NoError(err)
	s := New(kv)

	getKey := func(i int) string {
		return fmt.Sprintf("key %d", i)
	}
	getValue := func(i int) []byte {
		return []byte(fmt.Sprintf("value %d", i))
	}

	const n = 5
	for i := 0; i < n; i++ {
		require.NoError(s.SetMetadata(ctx, getKey(i), getValue(i)))
	}

	for i := 0; i < n; i++ {
		value, err := s.GetMetadata(ctx, getKey(i))
		require.NoError(err)
		require.Equal(getValue(i), value)
	}

	v, err := s.GetMetadata(ctx, "unused key")
	require.Error(err)
	require.Nil(v)
}
