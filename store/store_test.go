package store

import (
	"context"
	"os"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	ds "github.com/ipfs/go-datastore"

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			ds, _ := NewDefaultInMemoryKVStore()
			bstore := New(ctx, ds)
			assert.Equal(uint64(0), bstore.Height())

			for _, block := range c.blocks {
				err := bstore.SaveBlock(block, &types.Commit{})
				bstore.SetHeight(uint64(block.SignedHeader.Header.Height()))
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, kv := range []ds.TxnDatastore{mKV, dKV} {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assert := assert.New(t)
				require := require.New(t)

				bstore := New(ctx, kv)

				lastCommit := &types.Commit{}
				for _, block := range c.blocks {
					commit := &types.Commit{}
					block.SignedHeader.Commit = *lastCommit
					block.SignedHeader.Validators = types.GetRandomValidatorSet()
					err := bstore.SaveBlock(block, commit)
					require.NoError(err)
					lastCommit = commit
				}

				for _, expected := range c.blocks {
					block, err := bstore.LoadBlock(uint64(expected.SignedHeader.Header.Height()))
					assert.NoError(err)
					assert.NotNil(block)
					assert.Equal(expected, block)

					commit, err := bstore.LoadCommit(uint64(expected.SignedHeader.Header.Height()))
					assert.NoError(err)
					assert.NotNil(commit)
				}
			})
		}
	}
}

func TestRestart(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	validatorSet := types.GetRandomValidatorSet()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kv, _ := NewDefaultInMemoryKVStore()
	s := New(ctx, kv)

	expected := &cmstate.ABCIResponses{
		BeginBlock: &abcitypes.ResponseBeginBlock{
			Events: []abcitypes.Event{{
				Type: "test",
				Attributes: []abcitypes.EventAttribute{{
					Key:   string("foo"),
					Value: string("bar"),
					Index: false,
				}},
			}},
		},
		DeliverTxs: nil,
		EndBlock: &abcitypes.ResponseEndBlock{
			ValidatorUpdates: nil,
			ConsensusParamUpdates: &cmproto.ConsensusParams{
				Block: &cmproto.BlockParams{
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
