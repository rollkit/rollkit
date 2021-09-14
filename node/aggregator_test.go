package node

import (
	"context"
	"crypto/rand"
	mrand "math/rand"
	"testing"
	"time"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/mocks"
	"github.com/celestiaorg/optimint/p2p"
	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/store"
)

func TestAggregatorMode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	anotherKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	aggregatorConfig := config.AggregatorConfig{
		BlockTime:   500 * time.Millisecond,
		NamespaceID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	node, err := NewNode(context.Background(), config.NodeConfig{DALayer: "mock", Aggregator: true, AggregatorConfig: aggregatorConfig}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())

	pid, err := peer.IDFromPrivateKey(anotherKey)
	require.NoError(err)
	ctx, cancel := context.WithCancel(context.TODO())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				node.incomingTxCh <- &p2p.GossipMessage{Data: []byte(time.Now().String()), From: pid}
				time.Sleep(time.Duration(mrand.Uint32()%20) * time.Millisecond)
			}
		}
	}()
	time.Sleep(5 * time.Second)
	cancel()
}

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

	emptyStore := store.New(store.NewInMemoryKVStore())

	fullStore := store.New(store.NewInMemoryKVStore())
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
	conf := config.AggregatorConfig{
		BlockTime:   10 * time.Second,
		NamespaceID: [8]byte{1, 2, 3, 4, 5, 6, 7, 8},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			agg, err := newAggregator(key, conf, c.genesis, c.store, nil, nil, nil, log.TestingLogger())
			assert.NoError(err)
			assert.NotNil(agg)
			assert.Equal(c.expectedChainID, agg.lastState.ChainID)
			assert.Equal(c.expectedInitialHeight, agg.lastState.InitialHeight)
			assert.Equal(c.expectedLastBlockHeight, agg.lastState.LastBlockHeight)
		})
	}
}
