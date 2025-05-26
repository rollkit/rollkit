package block

import (
	"context"
	"cosmossdk.io/log"
	cryptoRand "crypto/rand"
	"fmt"
	"github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	syncdb "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/pkg/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"math/rand"
	"path/filepath"
	"testing"
	"time"
)

func TestSlowConsumers(t *testing.T) {
	blockTime := 100 * time.Millisecond
	specs := map[string]struct {
		headerConsumerDelay time.Duration
		dataConsumerDelay   time.Duration
	}{
		"slow header consumer": {
			headerConsumerDelay: blockTime,
			dataConsumerDelay:   0,
		},
		"slow data consumer": {
			headerConsumerDelay: 0,
			dataConsumerDelay:   blockTime,
		},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			var lastCapturedHeaderHeight uint64
			var lastCapturedDataHeight uint64

			dbm := syncdb.MutexWrap(datastore.NewMapDatastore())
			ctx, cancel := context.WithCancel(t.Context())

			manager, headerSync, dataSync := setupBlockManager(t, ctx, dbm, blockTime)
			go seqConsumer(ctx, manager.DataCh, spec.dataConsumerDelay, &lastCapturedDataHeight)
			blockTime := manager.config.Node.BlockTime.Duration
			go seqConsumer(ctx, manager.HeaderCh, spec.headerConsumerDelay, &lastCapturedHeaderHeight)
			errChan := make(chan error, 1)
			go manager.AggregationLoop(ctx, errChan)

			// wait for messages to pile up
			select {
			case err := <-errChan:
				require.NoError(t, err)
			case <-time.After(3 * blockTime):
			}
			t.Log("shutting down block manager")
			headerSync.Stop(ctx)
			dataSync.Stop(ctx)
			cancel()

			t.Log("restart with new block manager")
			ctx, cancel = context.WithCancel(t.Context())
			manager, headerSync, dataSync = setupBlockManager(t, ctx, dbm, blockTime)
			go manager.AggregationLoop(ctx, errChan)

			// expect to continue from last state
			gotHeader := <-manager.HeaderCh
			assert.Equal(t, lastCapturedHeaderHeight+1, gotHeader.Height())
			gotData := <-manager.DataCh
			assert.Equal(t, lastCapturedDataHeight+1, gotData.Height())
		})
	}
}

func setupBlockManager(t *testing.T, ctx context.Context, mainKV ds.Batching, blockTime time.Duration) (*Manager, *sync.HeaderSyncService, *sync.DataSyncService) {
	t.Helper()
	nodeConfig := config.DefaultConfig
	nodeConfig.Node.BlockTime = config.DurationWrapper{Duration: blockTime}
	nodeConfig.RootDir = t.TempDir()
	nodeKey, err := key.LoadOrGenNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
	require.NoError(t, err)

	pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	proposerAddr, err := noopSigner.GetAddress()
	require.NoError(t, err)
	genesisDoc := genesispkg.Genesis{
		ChainID:            "test-chain-id",
		GenesisDAStartTime: time.Now(),
		InitialHeight:      1,
		ProposerAddress:    proposerAddr,
	}

	logger := log.NewTestLogger(t)
	p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, mainKV, logger, p2p.NopMetrics())
	require.NoError(t, err)

	// Start p2p client before creating sync service
	err = p2pClient.Start(ctx)

	const RollkitPrefix = "0"
	ktds.Wrap(mainKV, ktds.PrefixTransform{Prefix: ds.NewKey(RollkitPrefix)})
	headerSyncService, err := sync.NewHeaderSyncService(mainKV, nodeConfig, genesisDoc, p2pClient, logger.With("module", "HeaderSyncService"))
	require.NoError(t, err)
	require.NoError(t, headerSyncService.Start(ctx))
	dataSyncService, err := sync.NewDataSyncService(mainKV, nodeConfig, genesisDoc, p2pClient, logger.With("module", "DataSyncService"))
	require.NoError(t, err)
	require.NoError(t, dataSyncService.Start(ctx))

	result, err := NewManager(
		ctx,
		noopSigner,
		nodeConfig,
		genesisDoc,
		store.New(mainKV),
		&mockExecutor{},
		coresequencer.NewDummySequencer(),
		nil,
		logger.With("module", "BlockManager"),
		headerSyncService.Store(),
		dataSyncService.Store(),
		NopMetrics(),
		1.,
		1.,
	)
	require.NoError(t, err)
	return result, headerSyncService, dataSyncService
}

func seqConsumer[T interface{ Height() uint64 }](ctx context.Context, chn <-chan T, sleepTime time.Duration, lastCapturedHeaderHeight *uint64) {
	select {
	case <-ctx.Done():
	case h := <-chn:
		time.Sleep(sleepTime)
		gotHeight := h.Height()
		if gotHeight != *lastCapturedHeaderHeight+1 {
			panic(fmt.Sprintf("got height %d, want %d", gotHeight, *lastCapturedHeaderHeight+1))
		}
		*lastCapturedHeaderHeight = gotHeight
	}
}

type mockExecutor struct{}

func (m mockExecutor) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) (stateRoot []byte, maxBytes uint64, err error) {
	return bytesN(32), 10_000, nil
}

func (m mockExecutor) GetTxs(ctx context.Context) ([][]byte, error) {
	panic("implement me")
}

func (m mockExecutor) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (updatedStateRoot []byte, maxBytes uint64, err error) {
	return bytesN(32), 10_000, nil

}

func (m mockExecutor) SetFinal(ctx context.Context, blockHeight uint64) error {
	return nil
}

var rnd = rand.New(rand.NewSource(1))

func bytesN(n int) []byte {
	data := make([]byte, n)
	_, _ = rnd.Read(data)
	return data
}
