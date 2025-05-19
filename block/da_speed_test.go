package block

import (
	"context"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	rollmocks "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func TestDASpeed(t *testing.T) {
	specs := map[string]struct {
		daDelay   time.Duration
		numBlocks int
	}{
		"Slow DA Layer": {
			daDelay:   500 * time.Millisecond,
			numBlocks: 5,
		},
		"Fast DA Layer": {
			daDelay:   0,
			numBlocks: 300,
		},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			daHeight := uint64(20)
			blockHeight := uint64(100)
			manager, mockDAClient := setupManagerForTest(t, daHeight)

			var receivedBlockCount atomic.Uint64
			ids, namespace := []coreda.ID{[]byte("dummy-id")}, []byte("placeholder")
			mockDAClient.
				On("GetIDs", mock.Anything, mock.Anything, namespace).
				Return(func(ctx context.Context, height uint64, namespace []byte) (*coreda.GetIDsResult, error) {
					return &coreda.GetIDsResult{IDs: ids, Timestamp: time.Now()}, nil
				})

			mockDAClient.
				On("Get", mock.Anything, ids, namespace).
				Return(func(ctx context.Context, ids []coreda.ID, namespace []byte) ([]coreda.Blob, error) {
					time.Sleep(spec.daDelay)
					// unique headers for cache misses
					n := receivedBlockCount.Add(1)
					hc := &types.HeaderConfig{Height: blockHeight + n - 1, Signer: manager.signer}
					header, err := types.GetRandomSignedHeaderCustom(hc, manager.genesis.ChainID)
					require.NoError(t, err)
					header.ProposerAddress = manager.genesis.ProposerAddress
					headerProto, err := header.ToProto()
					require.NoError(t, err)
					headerBytes, err := proto.Marshal(headerProto)
					require.NoError(t, err)
					return []coreda.Blob{headerBytes}, nil
				})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// when
			go manager.RetrieveLoop(ctx)
			go manager.HeaderStoreRetrieveLoop(ctx)
			go manager.DataStoreRetrieveLoop(ctx)
			go manager.SyncLoop(ctx, make(chan<- error))

			// then
			assert.Eventually(t, func() bool {
				return int(receivedBlockCount.Load()) >= spec.numBlocks
			}, 5*time.Second, 10*time.Millisecond)
			mockDAClient.AssertExpectations(t)
		})
	}
}

// setupManagerForTest initializes a Manager with mocked dependencies for testing.
func setupManagerForTest(t *testing.T, initialDAHeight uint64) (*Manager, *rollmocks.DA) {
	mockDAClient := rollmocks.NewDA(t)
	mockStore := rollmocks.NewStore(t)
	mockLogger := log.NewNopLogger()

	headerStore, _ := goheaderstore.NewStore[*types.SignedHeader](ds.NewMapDatastore())
	dataStore, _ := goheaderstore.NewStore[*types.Data](ds.NewMapDatastore())

	mockStore.On("GetState", mock.Anything).Return(types.State{DAHeight: initialDAHeight}, nil).Maybe()
	mockStore.On("Height", mock.Anything).Return(initialDAHeight, nil).Maybe()
	mockStore.On("SetMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	src := rand.Reader
	pk, _, err := crypto.GenerateEd25519Key(src)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)

	addr, err := noopSigner.GetAddress()
	require.NoError(t, err)

	blockTime := 1 * time.Second
	// setup with non-buffered channels that would block on slow consumers
	manager := &Manager{
		store: mockStore,
		config: config.Config{
			Node: config.NodeConfig{BlockTime: config.DurationWrapper{Duration: blockTime}},
			DA:   config.DAConfig{BlockTime: config.DurationWrapper{Duration: blockTime}},
		},
		genesis:       genesis.Genesis{ProposerAddress: addr},
		daHeight:      new(atomic.Uint64),
		headerInCh:    make(chan NewHeaderEvent),
		headerStore:   headerStore,
		dataInCh:      make(chan NewDataEvent),
		dataStore:     dataStore,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		headerStoreCh: make(chan struct{}),
		dataStoreCh:   make(chan struct{}),
		retrieveCh:    make(chan struct{}),
		logger:        mockLogger,
		lastStateMtx:  new(sync.RWMutex),
		da:            mockDAClient,
		signer:        noopSigner,
	}
	manager.daIncludedHeight.Store(0)
	manager.daHeight.Store(initialDAHeight)
	return manager, mockDAClient
}
