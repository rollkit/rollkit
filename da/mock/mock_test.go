package mock

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/log/test"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

var (
	dalcPrefix                 = []byte{1}
	dalcKV     *store.PrefixKV = store.NewPrefixKV(store.NewInMemoryKVStore(), dalcPrefix)
)

func TestLifecycle(t *testing.T) {
	var da da.DataAvailabilityLayerClient = &MockDataAvailabilityLayerClient{}
	dalcKV := store.NewInMemoryKVStore()

	require := require.New(t)

	err := da.Init([]byte{}, dalcKV, &test.TestLogger{T: t})
	require.NoError(err)

	err = da.Start()
	require.NoError(err)

	err = da.Stop()
	require.NoError(err)
}

func TestMockDALC(t *testing.T) {
	var dalc da.DataAvailabilityLayerClient = &MockDataAvailabilityLayerClient{}
	dalcKV := store.NewInMemoryKVStore()

	require := require.New(t)
	assert := assert.New(t)

	err := dalc.Init([]byte{}, dalcKV, &test.TestLogger{T: t})
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// only blocks b1 and b2 will be submitted to DA
	b1 := getRandomBlock(1, 10)
	b2 := getRandomBlock(2, 10)
	b3 := getRandomBlock(1, 10)

	resp := dalc.SubmitBlock(b1)
	assert.Equal(da.StatusSuccess, resp.Code)

	resp = dalc.SubmitBlock(b2)
	assert.Equal(da.StatusSuccess, resp.Code)

	check := dalc.CheckBlockAvailability(&b1.Header)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	check = dalc.CheckBlockAvailability(&b2.Header)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	// this block was never submitted to DA
	check = dalc.CheckBlockAvailability(&b3.Header)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.False(check.DataAvailable)
}

func TestRetrieve(t *testing.T) {
	mock := &MockDataAvailabilityLayerClient{}
	var dalc da.DataAvailabilityLayerClient = mock
	var retriever da.BlockRetriever = mock

	dalcKV := store.NewInMemoryKVStore()

	require := require.New(t)
	assert := assert.New(t)

	err := dalc.Init([]byte{}, dalcKV, &test.TestLogger{T: t})
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	for i := uint64(0); i < 100; i++ {
		b := getRandomBlock(i, rand.Int()%20)
		resp := dalc.SubmitBlock(b)
		assert.Equal(da.StatusSuccess, resp.Code)

		ret := retriever.RetrieveBlock(i)
		assert.Equal(da.StatusSuccess, ret.Code)
		assert.Equal(b, ret.Block)
	}
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		Header: types.Header{
			Height: height,
		},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	copy(block.Header.AppHash[:], getRandomBytes(32))

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(https://github.com/celestiaorg/optimint/issues/143)
	// This is a hack to get around equality checks on serialization round trips
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	return block
}

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data)
	return data
}
