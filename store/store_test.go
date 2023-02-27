package store

import (
	"context"
	"math/rand"
	"os"
	"testing"

	ds "github.com/ipfs/go-datastore"
	abcitypes "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	tmstate "github.com/tendermint/tendermint/proto/tendermint/state"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
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
			ds, _ := NewDefaultInMemoryKVStore()
			bstore := New(context.Background(), ds)
			assert.Equal(uint64(0), bstore.Height())

			for _, block := range c.blocks {
				err := bstore.SaveBlock(block, &types.Commit{})
				bstore.SetHeight(uint64(block.Header.Height()))
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
		// TODO(tzdybal): this test needs extra handling because of lastCommits
		//{"blocks out of order", []*types.Block{
		//	getRandomBlock(2, 20),
		//	getRandomBlock(3, 30),
		//	getRandomBlock(4, 100),
		//	getRandomBlock(5, 10),
		//	getRandomBlock(1, 10),
		//}},
	}

	tmpDir, err := os.MkdirTemp("", "rollkit_test")
	require.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tmpDir)
		if err != nil {
			t.Log("failed to remove temporary directory", err)
		}
	}()

	mKV, _ := NewDefaultInMemoryKVStore()
	dKV, _ := NewDefaultKVStore(tmpDir, "db", "test")
	for _, kv := range []ds.TxnDatastore{mKV, dKV} {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assert := assert.New(t)
				require := require.New(t)

				bstore := New(context.Background(), kv)

				lastCommit := &types.Commit{}
				for _, block := range c.blocks {
					commit := &types.Commit{Height: uint64(block.Header.Height()), HeaderHash: block.Header.Hash()}
					block.LastCommit = *lastCommit
					err := bstore.SaveBlock(block, commit)
					require.NoError(err)
					lastCommit = commit
				}

				for _, expected := range c.blocks {
					block, err := bstore.LoadBlock(uint64(expected.Header.Height()))
					assert.NoError(err)
					assert.NotNil(block)
					assert.Equal(expected, block)
					assert.Equal(expected.Header.Height()-1, int64(block.LastCommit.Height))
					assert.Equal(expected.LastCommit.Height, block.LastCommit.Height)
					assert.Equal(expected.LastCommit.HeaderHash, block.LastCommit.HeaderHash)

					commit, err := bstore.LoadCommit(uint64(expected.Header.Height()))
					assert.NoError(err)
					assert.NotNil(commit)
					assert.Equal(uint64(expected.Header.Height()), commit.Height)
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

	validatorSet := getRandomValidatorSet()

	ctx := context.Background()
	kv, _ := NewDefaultInMemoryKVStore()
	s1 := New(ctx, kv)
	expectedHeight := uint64(10)
	err := s1.UpdateState(types.State{
		LastBlockHeight: int64(expectedHeight),
		NextValidators:  validatorSet,
		Validators:      validatorSet,
		LastValidators:  validatorSet,
	})
	assert.NoError(err)

	s2 := New(ctx, kv)
	_, err = s2.LoadState()
	assert.NoError(err)

	assert.Equal(expectedHeight, s2.Height())
}

func TestBlockResponses(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	kv, _ := NewDefaultInMemoryKVStore()
	s := New(context.Background(), kv)

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
			BaseHeader: types.BaseHeader{
				Height: height,
			},
			AggregatorsHash: make([]byte, 32),
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
	size := rand.Int()%100 + 100 //nolint:gosec
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data) //nolint:gosec
	return data
}

// TODO(tzdybal): extract to some common place
func getRandomValidatorSet() *tmtypes.ValidatorSet {
	pubKey := ed25519.GenPrivKey().PubKey()
	return &tmtypes.ValidatorSet{
		Proposer: &tmtypes.Validator{PubKey: pubKey, Address: pubKey.Address()},
		Validators: []*tmtypes.Validator{
			{PubKey: pubKey, Address: pubKey.Address()},
		},
	}
}
