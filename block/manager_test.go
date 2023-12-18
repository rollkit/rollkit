package block

import (
	"context"
	crand "crypto/rand"
	"testing"
	"time"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

func TestInitialState(t *testing.T) {
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
	fullStore.SaveBlock(b, &b.SignedHeader.Commit)
	err := fullStore.UpdateState(sampleState)
	require.NoError(err)

	es3, _ := store.NewDefaultInMemoryKVStore()
	overrideStore := store.New(ctx, es3)
	overrideStore.UpdateState(sampleState)
	b.SignedHeader.AppHash = types.GetRandomBytes(32)
	overrideStore.SaveBlock(b, &b.SignedHeader.Commit)

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
