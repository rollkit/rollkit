package block

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/da"
	mockda "github.com/celestiaorg/optimint/da/mock"
	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/store"
)

func TestInitialState(t *testing.T) {
	genesis := &types.GenesisDoc{
		ChainID:       "genesis id",
		InitialHeight: 100,
	}
	sampleState := state.State{
		ChainID:         "state id",
		InitialHeight:   123,
		LastBlockHeight: 128,
	}

	emptyStore := store.New(store.NewDefaultInMemoryKVStore())

	fullStore := store.New(store.NewDefaultInMemoryKVStore())
	err := fullStore.UpdateState(sampleState)
	require.NoError(t, err)

	cases := []struct {
		name                    string
		store                   store.Store
		genesis                 *types.GenesisDoc
		expectedInitialHeight   int64
		expectedLastBlockHeight int64
		expectedChainID         string
	}{
		{
			name:                    "empty store",
			store:                   emptyStore,
			genesis:                 genesis,
			expectedInitialHeight:   genesis.InitialHeight,
			expectedLastBlockHeight: 0,
			expectedChainID:         genesis.ChainID,
		},
		{
			name:                    "state in store",
			store:                   fullStore,
			genesis:                 genesis,
			expectedInitialHeight:   sampleState.InitialHeight,
			expectedLastBlockHeight: sampleState.LastBlockHeight,
			expectedChainID:         sampleState.ChainID,
		},
	}

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	conf := config.BlockManagerConfig{
		BlockTime:   10 * time.Second,
		NamespaceID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			logger := log.TestingLogger()
			dalc := getMockDALC(logger)
			agg, err := NewManager(key, conf, c.genesis, c.store, nil, nil, dalc, nil, logger)
			assert.NoError(err)
			assert.NotNil(agg)
			assert.Equal(c.expectedChainID, agg.lastState.ChainID)
			assert.Equal(c.expectedInitialHeight, agg.lastState.InitialHeight)
			assert.Equal(c.expectedLastBlockHeight, agg.lastState.LastBlockHeight)
		})
	}
}

func getMockDALC(logger log.Logger) da.DataAvailabilityLayerClient {
	dalc := &mockda.MockDataAvailabilityLayerClient{}
	_ = dalc.Init(nil, nil, logger)
	_ = dalc.Start()
	return dalc
}
