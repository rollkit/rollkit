package block

import (
	"context"
	"testing"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

func TestInitialStateNoBlock(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(ctx, es)
	s, err := getInitialState(emptyStore, genesis)
	require.NoError(err)
	require.Equal(uint64(genesis.InitialHeight), s.InitialHeight)
}

func TestInitialStateOverrideError(t *testing.T) {
	require := require.New(t)
	genesisDoc, privKey := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	overrideStore := store.New(ctx, es)
	b, _ := types.GetRandomBlockWithKey(100, 1, privKey)
	err := overrideStore.SaveBlock(b, &b.SignedHeader.Commit)
	require.NoError(err)
	_, err = getInitialState(overrideStore, genesis)
	require.EqualError(err, "store does not match genesis. were you trying to hard-fork?")
}

func TestInitialStateStoreMatchingGenesis(t *testing.T) {
	require := require.New(t)
	genesisDoc, privKey := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "state id",
		InitialHeight:   100,
		LastBlockHeight: 128,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	matchingStore := store.New(ctx, es)
	b, _ := types.GetRandomBlockWithKey(100, 1, privKey)
	b.SignedHeader.AppHash = genesis.AppHash.Bytes()
	err := matchingStore.SaveBlock(b, &b.SignedHeader.Commit)
	require.NoError(err)
	err = matchingStore.UpdateState(sampleState)
	require.NoError(err)
	s, err := getInitialState(matchingStore, genesis)
	require.NoError(err)
	require.Equal(sampleState.InitialHeight, s.InitialHeight)
}

func TestInitialErrorNoSavedState(t *testing.T) {
	require := require.New(t)
	genesisDoc, privKey := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	matchingStore := store.New(ctx, es)
	b, _ := types.GetRandomBlockWithKey(100, 1, privKey)
	b.SignedHeader.AppHash = genesis.AppHash.Bytes()
	err := matchingStore.SaveBlock(b, &b.SignedHeader.Commit)
	require.NoError(err)
	_, err = getInitialState(matchingStore, genesis)
	require.EqualError(err, "store contains blocks, but no saved state.")
}

func TestIsDAIncluded(t *testing.T) {
	require := require.New(t)

	// Create a minimalistic block manager
	m := &Manager{
		blockCache: NewBlockCache(),
	}
	hash := types.Hash([]byte("hash"))

	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(hash))

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.blockCache.setDAIncluded(hash.String())
	require.True(m.IsDAIncluded(hash))
}
