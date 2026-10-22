package syncing

import (
	"context"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/block/internal/cache"
	"github.com/evstack/ev-node/block/internal/common"
	"github.com/evstack/ev-node/pkg/config"
	datypes "github.com/evstack/ev-node/pkg/da/types"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/store"
	testmocks "github.com/evstack/ev-node/test/mocks"
	extmocks "github.com/evstack/ev-node/test/mocks/external"
	"github.com/evstack/ev-node/types"
)

const daHeightOffset = 100

func BenchmarkSyncerIO(b *testing.B) {
	cases := map[string]struct {
		heights    uint64
		shuffledTx bool
		daDelay    time.Duration
		execDelay  time.Duration
		p2pEnabled bool
		p2pDelay   time.Duration
	}{
		"slow producer": {heights: 100, daDelay: 200 * time.Microsecond, execDelay: 0, p2pDelay: 0, p2pEnabled: false},
		"slow consumer": {heights: 100, daDelay: 0, execDelay: 200 * time.Microsecond, p2pDelay: 0, p2pEnabled: false},
	}
	for name, spec := range cases {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				fixt := newBenchFixture(b, spec.heights, spec.shuffledTx, spec.daDelay, spec.execDelay, true)

				// run both loops
				ctx := b.Context()
				go fixt.s.processLoop(ctx)

				// Create a DAFollower to drive DA retrieval.
				daClient, eventCh := setupMockDAClient(b)
				follower := NewDAFollower(DAFollowerConfig{
					Client:        daClient,
					Retriever:     fixt.s.daRetriever,
					Logger:        zerolog.Nop(),
					EventSink:     fixt.s,
					Namespace:     []byte("ns"),
					StartDAHeight: fixt.s.daRetrieverHeight.Load(),
					DABlockTime:   0,
				}).(*daFollower)
				follower.Start(ctx)
				eventCh <- datypes.SubscriptionEvent{Height: spec.heights + daHeightOffset}

				fixt.s.startSyncWorkers(ctx)

				require.Eventually(b, func() bool {
					processedHeight, _ := fixt.s.store.Height(ctx)
					return processedHeight == spec.heights
				}, 5*time.Second, 50*time.Microsecond)
				fixt.s.cancel()
				fixt.s.wg.Wait()

				// Ensure clean end-state per iteration - verify no pending events remain
				for i := uint64(1); i <= spec.heights; i++ {
					event := fixt.s.cache.GetNextPendingEvent(i)
					require.Nil(b, event, "expected no pending event at height %d", i)
				}
				require.Len(b, fixt.s.heightInCh, 0)

				assert.Equal(b, spec.heights+daHeightOffset, fixt.s.daRetrieverHeight)
				gotStoreHeight, err := fixt.s.store.Height(ctx)
				require.NoError(b, err)
				assert.Equal(b, spec.heights, gotStoreHeight)
			}
		})
	}
}

type benchFixture struct {
	s      *Syncer
	st     store.Store
	cm     cache.CacheManager
	cancel context.CancelFunc
}

func newBenchFixture(b *testing.B, totalHeights uint64, shuffledTx bool, daDelay, execDelay time.Duration, includeP2P bool) *benchFixture {
	b.Helper()
	ctx, cancel := context.WithCancel(b.Context())

	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(b, err)

	addr, pub, signer := buildSyncTestSigner(b)
	cfg := config.DefaultConfig()
	// keep P2P ticker dormant unless we manually inject P2P events
	cfg.Node.BlockTime = config.DurationWrapper{Duration: 1}
	gen := genesis.Genesis{ChainID: "bchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr, DAStartHeight: daHeightOffset}

	mockExec := testmocks.NewMockExecutor(b)
	// if execDelay > 0, sleep on ExecuteTxs to simulate slow consumer
	mockExec.On("ExecuteTxs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			if execDelay > 0 {
				time.Sleep(execDelay)
			}
		}).
		Return([]byte("app"), nil).Maybe()

	// Build syncer with mocks
	s := NewSyncer(
		st,
		mockExec,
		nil, // DA injected via mock retriever below
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		nil, // headerStore not used; we inject P2P directly to channel when needed
		nil, // dataStore not used
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(b, s.initializeState())
	s.ctx, s.cancel = ctx, cancel
	s.lifecycleState = syncerLifecycleStarted

	// prepare height events to emit
	heightEvents := make([]common.DAHeightEvent, totalHeights)
	for i := range totalHeights {
		blockHeight, daHeight := i+gen.InitialHeight, i+daHeightOffset
		_, sh := makeSignedHeaderBytes(b, gen.ChainID, blockHeight, addr, pub, signer, nil, nil, nil)
		d := &types.Data{Metadata: &types.Metadata{ChainID: gen.ChainID, Height: blockHeight, Time: uint64(time.Now().UnixNano())}}
		heightEvents[i] = common.DAHeightEvent{Header: sh, Data: d, DaHeight: daHeight}
	}
	if shuffledTx {
		rand.New(rand.NewPCG(1, 2)).Shuffle(len(heightEvents), func(i, j int) { //nolint:gosec // false positive
			heightEvents[i], heightEvents[j] = heightEvents[j], heightEvents[i]
		})
	}

	// Mock DA retriever to emit exactly totalHeights events, then HFF and cancel
	daR := NewMockDARetriever(b)

	for i := range totalHeights {
		daHeight := i + daHeightOffset
		daR.On("RetrieveFromDA", mock.Anything, daHeight).
			Run(func(_ mock.Arguments) {
				if daDelay > 0 {
					time.Sleep(daDelay)
				}
			}).
			Return([]common.DAHeightEvent{heightEvents[i]}, nil).Once()
	}
	// after last, return height-from-future and stop when queue drains
	daR.On("RetrieveFromDA", mock.Anything, totalHeights+daHeightOffset).
		Return(nil, common.ErrHeightFromFutureStr).Maybe()

	// Attach mocks
	s.daRetriever = daR
	mockP2P := newMockp2pHandler(b) // not used directly in this benchmark path
	mockP2P.On("SetProcessedHeight", mock.Anything).Return().Maybe()
	s.p2pHandler = mockP2P
	headerP2PStore := extmocks.NewMockStore[*types.P2PSignedHeader](b)
	s.headerStore = headerP2PStore
	dataP2PStore := extmocks.NewMockStore[*types.P2PData](b)
	s.dataStore = dataP2PStore
	return &benchFixture{s: s, st: st, cm: cm, cancel: cancel}
}
