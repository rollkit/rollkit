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
	genesisValidators, _ := types.GetGenesisValidatorSetWithSigner()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisValidators,
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
	genesisValidators, _ := types.GetGenesisValidatorSetWithSigner()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisValidators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	overrideStore := store.New(ctx, es)
	b, _ := types.GetRandomBlockWithKey(100, 1)
	overrideStore.SaveBlock(b, &b.SignedHeader.Commit)
	_, err := getInitialState(overrideStore, genesis)
	require.EqualError(err, "store does not match genesis. were you trying to hard-fork?")
}

func TestInitialStateStoreMatchingGenesis(t *testing.T) {
	require := require.New(t)
	genesisValidators, _ := types.GetGenesisValidatorSetWithSigner()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisValidators,
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
	b, _ := types.GetRandomBlockWithKey(100, 1)
	b.SignedHeader.AppHash = genesis.AppHash.Bytes()
	matchingStore.SaveBlock(b, &b.SignedHeader.Commit)
	matchingStore.UpdateState(sampleState)
	s, err := getInitialState(matchingStore, genesis)
	require.NoError(err)
	require.Equal(uint64(sampleState.InitialHeight), s.InitialHeight)
}

func TestInitialErrorNoSavedState(t *testing.T) {
	require := require.New(t)
	genesisValidators, _ := types.GetGenesisValidatorSetWithSigner()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisValidators,
		AppHash:       []byte("app hash"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	matchingStore := store.New(ctx, es)
	b, _ := types.GetRandomBlockWithKey(100, 1)
	b.SignedHeader.AppHash = genesis.AppHash.Bytes()
	matchingStore.SaveBlock(b, &b.SignedHeader.Commit)
	_, err := getInitialState(matchingStore, genesis)
	require.EqualError(err, "store contains blocks, but no saved state.")
}

/*func TestInitialState(t *testing.T) {
	require := require.New(t)
	genesisValidators, _ := types.GetGenesisValidatorSetWithSigner()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
		Validators:    genesisValidators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "state id",
		InitialHeight:   123,
		LastBlockHeight: 128,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(ctx, es)

	es2, _ := store.NewDefaultInMemoryKVStore()
	fullStore := store.New(ctx, es2)
	b, _ := types.GetRandomBlockWithKey(100, 1)
	b.SignedHeader.AppHash = genesis.AppHash.Bytes()
	err := fullStore.SaveBlock(b, &b.SignedHeader.Commit)
	require.NoError(err)
	err = fullStore.UpdateState(sampleState)
	require.NoError(err)

	es3, _ := store.NewDefaultInMemoryKVStore()
	overrideStore := store.New(ctx, es3)
	err = overrideStore.UpdateState(sampleState)
	require.NoError(err)
	b.SignedHeader.AppHash = types.GetRandomBytes(32)
	err = overrideStore.SaveBlock(b, &b.SignedHeader.Commit)
	require.NoError(err)

	cases := []struct {
		name                    string
		store                   store.Store
		genesis                 *cmtypes.GenesisDoc
		expectedInitialHeight   uint64
		expectedLastBlockHeight uint64
		expectedChainID         string
	}{
		// When the store is empty, it should always load state from the supplied genesis.
		{
			name:                    "empty_store",
			store:                   emptyStore,
			genesis:                 genesis,
			expectedInitialHeight:   uint64(genesis.InitialHeight),
			expectedLastBlockHeight: 0,
			expectedChainID:         genesis.ChainID,
		},
		// Unless the user-supplied genesis contradicts a past block, trust the last saved state.
		{
			name:                    "state_in_store",
			store:                   fullStore,
			genesis:                 genesis,
			expectedInitialHeight:   uint64(sampleState.InitialHeight),
			expectedLastBlockHeight: uint64(sampleState.LastBlockHeight),
			expectedChainID:         sampleState.ChainID,
		},
		// if the user-supplied genesis DOES contradict a past block, assume we're hard-forking, and trust the user.
		{
			name:                    "genesis_overrides",
			store:                   overrideStore,
			genesis:                 genesis,
			expectedInitialHeight:   uint64(genesis.InitialHeight),
			expectedLastBlockHeight: 0,
			expectedChainID:         genesis.ChainID,
		},
	}

	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	conf := config.BlockManagerConfig{
		BlockTime: 10 * time.Second,
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			logger := test.NewFileLoggerCustom(t, test.TempLogFileName(t, c.name))
			dalc := &da.DAClient{DA: goDATest.NewDummyDA(), Logger: logger}
			agg, err := NewManager(key, conf, c.genesis, c.store, nil, nil, dalc, nil, logger, nil)
			assert.NoError(err)
			assert.NotNil(agg)
			agg.lastStateMtx.RLock()
			assert.Equal(c.expectedChainID, agg.lastState.ChainID)
			assert.Equal(c.expectedInitialHeight, agg.lastState.InitialHeight)
			assert.Equal(c.expectedLastBlockHeight, agg.lastState.LastBlockHeight)
			agg.lastStateMtx.RUnlock()
		})
	}
}*/

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
