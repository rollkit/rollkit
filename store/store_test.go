package store

import (
	"context"
	"fmt"
	"os"
	"testing"

	abcitypes "github.com/cometbft/cometbft/abci/types"
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
		{"single block", []*types.Block{types.GetRandomBlock(1, 0)}, 1},
		{"two consecutive blocks", []*types.Block{
			types.GetRandomBlock(1, 0),
			types.GetRandomBlock(2, 0),
		}, 2},
		{"blocks out of order", []*types.Block{
			types.GetRandomBlock(2, 0),
			types.GetRandomBlock(3, 0),
			types.GetRandomBlock(1, 0),
		}, 3},
		{"with a gap", []*types.Block{
			types.GetRandomBlock(1, 0),
			types.GetRandomBlock(9, 0),
			types.GetRandomBlock(10, 0),
		}, 10},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			ds, _ := NewDefaultInMemoryKVStore()
			bstore := New(ds)
			assert.Equal(uint64(0), bstore.Height())

			for _, block := range c.blocks {
				err := bstore.SaveBlock(ctx, block, &types.Commit{})
				bstore.SetHeight(ctx, block.Height())
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
		{"single block", []*types.Block{types.GetRandomBlock(1, 10)}},
		{"two consecutive blocks", []*types.Block{
			types.GetRandomBlock(1, 10),
			types.GetRandomBlock(2, 20),
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

				bstore := New(kv)

				lastCommit := &types.Commit{}
				for _, block := range c.blocks {
					commit := &types.Commit{}
					block.SignedHeader.Commit = *lastCommit
					block.SignedHeader.Validators = types.GetRandomValidatorSet()
					err := bstore.SaveBlock(ctx, block, commit)
					require.NoError(err)
					lastCommit = commit
				}

				for _, expected := range c.blocks {
					block, err := bstore.GetBlock(ctx, expected.Height())
					assert.NoError(err)
					assert.NotNil(block)
					assert.Equal(expected, block)

					commit, err := bstore.GetCommit(ctx, expected.Height())
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
	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tmpDir, err := os.MkdirTemp("", t.Name())
	require.NoError(err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

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

func TestBlockResponses(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	kv, _ := NewDefaultInMemoryKVStore()
	s := New(kv)

	expected := &abcitypes.ResponseFinalizeBlock{
		Events: []abcitypes.Event{{
			Type: "test",
			Attributes: []abcitypes.EventAttribute{{
				Key:   string("foo"),
				Value: string("bar"),
				Index: false,
			}},
		}},
		TxResults:        nil,
		ValidatorUpdates: nil,
		ConsensusParamUpdates: &cmproto.ConsensusParams{
			Block: &cmproto.BlockParams{
				MaxBytes: 12345,
				MaxGas:   678909876,
			},
		},
	}

	err := s.SaveBlockResponses(ctx, 1, expected)
	assert.NoError(err)

	resp, err := s.GetBlockResponses(ctx, 123)
	assert.Error(err)
	assert.Nil(resp)

	resp, err = s.GetBlockResponses(ctx, 1)
	assert.NoError(err)
	assert.NotNil(resp)
	assert.Equal(expected, resp)
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

func TestExtendedCommits(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	kv, err := NewDefaultInMemoryKVStore()
	require.NoError(err)
	s := New(kv)

	// reading before saving returns error
	commit, err := s.GetExtendedCommit(ctx, 1)
	require.Error(err)
	require.ErrorIs(err, ds.ErrNotFound)
	require.Nil(commit)

	expected := &abcitypes.ExtendedCommitInfo{
		Round: 123,
		Votes: []abcitypes.ExtendedVoteInfo{{
			Validator: abcitypes.Validator{
				Address: types.GetRandomBytes(20),
				Power:   123,
			},
			VoteExtension:      []byte("extended"),
			ExtensionSignature: []byte("just trust me bro"),
			BlockIdFlag:        0,
		}},
	}

	err = s.SaveExtendedCommit(ctx, 10, expected)
	require.NoError(err)
	commit, err = s.GetExtendedCommit(ctx, 10)
	require.NoError(err)
	require.Equal(expected, commit)
}
