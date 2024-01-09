package block

import (
	"context"
	"testing"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

func TestInitialStateClean(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 1,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(ctx, es)
	s, err := getInitialState(emptyStore, genesis)
	require.Equal(int64(s.LastBlockHeight), genesis.InitialHeight-1)
	require.NoError(err)
	require.Equal(uint64(genesis.InitialHeight), s.InitialHeight)
}

func TestInitialStateStored(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 1,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "myChain",
		InitialHeight:   1,
		LastBlockHeight: 100,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(ctx, es)
	store.UpdateState(sampleState)
	s, err := getInitialState(store, genesis)
	require.Equal(int64(s.LastBlockHeight), int64(100))
	require.NoError(err)
	require.Equal(s.InitialHeight, uint64(1))
}

func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 2,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "myChain",
		InitialHeight:   1,
		LastBlockHeight: 0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(ctx, es)
	store.UpdateState(sampleState)
	_, err := getInitialState(store, genesis)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")
}

func TestInitialStateUnexpectedStoredBlock(t *testing.T) {
	require := require.New(t)
	genesisDoc, privKey := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 100,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(ctx, es)
	b, _ := types.GetRandomBlockWithKey(100, 1, privKey)
	store.SaveBlock(b, &b.SignedHeader.Commit)
	_, err := getInitialState(store, genesis)
	require.EqualError(err, "unexpected stored block at genesis.InitialHeight (100)")
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
