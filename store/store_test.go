package store

import (
	"math/rand"
	"os"
	"testing"

	abcitypes "github.com/tendermint/tendermint/abci/types"
	tmstate "github.com/tendermint/tendermint/proto/tendermint/state"

	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreHeight(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		blocks   []*types.Block
		expected uint64
	}{
		{"single block", []*types.Block{getRandomBlock(1, 0)}, 1},
		{"two consecutive blocks", []*types.Block{
			getRandomBlock(1, 0),
			getRandomBlock(2, 0),
		}, 2},
		{"blocks out of order", []*types.Block{
			getRandomBlock(2, 0),
			getRandomBlock(3, 0),
			getRandomBlock(1, 0),
		}, 3},
		{"with a gap", []*types.Block{
			getRandomBlock(1, 0),
			getRandomBlock(9, 0),
			getRandomBlock(10, 0),
		}, 10},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			bstore := New(NewDefaultInMemoryKVStore())
			assert.Equal(uint64(0), bstore.Height())

			for _, block := range c.blocks {
				err := bstore.SaveBlock(block, &types.Commit{})
				assert.NoError(err)
			}

			assert.Equal(c.expected, bstore.Height())
		})
	}
}

func TestStoreLoad(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		blocks []*types.Block
	}{
		{"single block", []*types.Block{getRandomBlock(1, 10)}},
		{"two consecutive blocks", []*types.Block{
			getRandomBlock(1, 10),
			getRandomBlock(2, 20),
		}},
		{"blocks out of order", []*types.Block{
			getRandomBlock(2, 20),
			getRandomBlock(3, 30),
			getRandomBlock(4, 100),
			getRandomBlock(5, 10),
			getRandomBlock(1, 10),
		}},
	}

	tmpDir, err := os.MkdirTemp("", "optimint_test")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Log("failed to remove temporary directory", err)
		}
	}()

	for _, kv := range []KVStore{NewDefaultInMemoryKVStore(), NewDefaultKVStore(tmpDir, "db", "test")} {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assert := assert.New(t)
				require := require.New(t)

				bstore := New(kv)

				for _, block := range c.blocks {
					err := bstore.SaveBlock(block, &types.Commit{Height: block.Header.Height, HeaderHash: block.Header.Hash()})
					require.NoError(err)
				}

				for _, expected := range c.blocks {
					block, err := bstore.LoadBlock(expected.Header.Height)
					assert.NoError(err)
					assert.NotNil(block)
					assert.Equal(expected, block)

					commit, err := bstore.LoadCommit(expected.Header.Height)
					assert.NoError(err)
					assert.NotNil(commit)
					assert.Equal(expected.Header.Height, commit.Height)
					headerHash := expected.Header.Hash()
					assert.Equal(headerHash, commit.HeaderHash)
				}
			})
		}
	}
}

func TestRestart(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	kv := NewDefaultInMemoryKVStore()
	s1 := New(kv)
	expectedHeight := uint64(10)
	//block := getRandomBlock(expectedHeight, 10)
	//err := s1.SaveBlock(block, &types.Commit{Height: block.Header.Height, HeaderHash: block.Header.Hash()})
	err := s1.UpdateState(state.State{
		LastBlockHeight: int64(expectedHeight),
	})
	assert.NoError(err)

	s2 := New(kv)
	_, err = s2.LoadState()
	assert.NoError(err)

	assert.Equal(expectedHeight, s2.Height())
}

func TestBlockResponses(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	kv := NewDefaultInMemoryKVStore()
	s := New(kv)

	expected := &tmstate.ABCIResponses{
		BeginBlock: &abcitypes.ResponseBeginBlock{
			Events: []abcitypes.Event{{
				Type: "test",
				Attributes: []abcitypes.EventAttribute{{
					Key:   []byte("foo"),
					Value: []byte("bar"),
					Index: false,
				}},
			}},
		},
		DeliverTxs: nil,
		EndBlock: &abcitypes.ResponseEndBlock{
			ValidatorUpdates: nil,
			ConsensusParamUpdates: &abcitypes.ConsensusParams{
				Block: &abcitypes.BlockParams{
					MaxBytes: 12345,
					MaxGas:   678909876,
				},
			},
		},
	}

	err := s.SaveBlockResponses(1, expected)
	assert.NoError(err)

	resp, err := s.LoadBlockResponses(123)
	assert.Error(err)
	assert.Nil(resp)

	resp, err = s.LoadBlockResponses(1)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Equal(expected, resp)
}

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

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
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
