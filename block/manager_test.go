package block

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/da"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

func TestInitialState(t *testing.T) {
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
	}
	sampleState := types.State{
		ChainID:         "state id",
		InitialHeight:   123,
		LastBlockHeight: 128,
		LastValidators:  types.GetRandomValidatorSet(),
		Validators:      types.GetRandomValidatorSet(),
		NextValidators:  types.GetRandomValidatorSet(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(ctx, es)

	es2, _ := store.NewDefaultInMemoryKVStore()
	fullStore := store.New(ctx, es2)
	err := fullStore.UpdateState(sampleState)
	require.NoError(t, err)

	cases := []struct {
		name                    string
		store                   store.Store
		genesis                 *cmtypes.GenesisDoc
		expectedInitialHeight   uint64
		expectedLastBlockHeight uint64
		expectedChainID         string
	}{
		{
			name:                    "empty_store",
			store:                   emptyStore,
			genesis:                 genesis,
			expectedInitialHeight:   uint64(genesis.InitialHeight),
			expectedLastBlockHeight: 0,
			expectedChainID:         genesis.ChainID,
		},
		{
			name:                    "state_in_store",
			store:                   fullStore,
			genesis:                 genesis,
			expectedInitialHeight:   uint64(sampleState.InitialHeight),
			expectedLastBlockHeight: uint64(sampleState.LastBlockHeight),
			expectedChainID:         sampleState.ChainID,
		},
	}

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	conf := config.BlockManagerConfig{
		BlockTime:   10 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			logger := test.NewFileLoggerCustom(t, test.TempLogFileName(t, c.name))
			dalc := getMockDALC(logger)
			defer func() {
				require.NoError(t, dalc.Stop())
			}()
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

func getMockDALC(logger log.Logger) da.DataAvailabilityLayerClient {
	dalc := &mockda.DataAvailabilityLayerClient{}
	_ = dalc.Init([8]byte{}, nil, nil, logger)
	_ = dalc.Start()
	return dalc
}

func TestGetHardConfirmation(t *testing.T) {
	require := require.New(t)

	// Create a minimalistic block manager
	m := &Manager{
		blockCache: NewBlockCache(),
	}
	hash := types.Hash([]byte("hash"))

	// GetHardConfirmation should return false for unseen hash
	require.False(m.GetHardConfirmation(hash))

	// Set the hash as hard confirmed and verify GetHardConfirmation returns
	// true
	m.blockCache.setHardConfirmed(hash.String())
	require.True(m.GetHardConfirmation(hash))
}
