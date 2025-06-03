package block

import (
	"context"
	cryptoRand "crypto/rand"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	syncdb "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/pkg/store"
	rollkitSync "github.com/rollkit/rollkit/pkg/sync"
	"github.com/rollkit/rollkit/types"
)

func TestSlowConsumers(t *testing.T) {
	logging.SetDebugLogging()
	blockTime := 100 * time.Millisecond
	specs := map[string]struct {
		headerConsumerDelay time.Duration
		dataConsumerDelay   time.Duration
	}{
		"slow header consumer": {
			headerConsumerDelay: blockTime * 2,
			dataConsumerDelay:   0,
		},
		"slow data consumer": {
			headerConsumerDelay: 0,
			dataConsumerDelay:   blockTime * 2,
		},
		"both slow": {
			headerConsumerDelay: blockTime,
			dataConsumerDelay:   blockTime,
		},
		"both fast": {
			headerConsumerDelay: 0,
			dataConsumerDelay:   0,
		},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			dbm := syncdb.MutexWrap(ds.NewMapDatastore())
			ctx, cancel := context.WithCancel(t.Context())

			pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
			require.NoError(t, err)
			noopSigner, err := noop.NewNoopSigner(pk)
			require.NoError(t, err)

			manager, headerSync, dataSync := setupBlockManager(t, ctx, workDir, dbm, blockTime, noopSigner)
			var lastCapturedDataPayload *types.SignedData
			var lastCapturedHeaderPayload *types.SignedHeader
			manager.dataBroadcaster = capturingTailBroadcaster[*types.SignedData](spec.dataConsumerDelay, &lastCapturedDataPayload, dataSync)
			manager.headerBroadcaster = capturingTailBroadcaster[*types.SignedHeader](spec.headerConsumerDelay, &lastCapturedHeaderPayload, headerSync)

			blockTime := manager.config.Node.BlockTime.Duration
			aggCtx, aggCancel := context.WithCancel(ctx)
			errChan := make(chan error, 1)
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				manager.AggregationLoop(aggCtx, errChan)
				wg.Done()
			}()

			// wait for messages to pile up
			select {
			case err := <-errChan:
				require.NoError(t, err)
			case <-time.After(spec.dataConsumerDelay + spec.headerConsumerDelay + 3*blockTime):
			}
			aggCancel()
			wg.Wait() // await aggregation loop to finish
			t.Log("shutting down block manager")
			require.NoError(t, dataSync.Stop(ctx))
			require.NoError(t, headerSync.Stop(ctx))
			cancel()
			require.NotNil(t, lastCapturedHeaderPayload)
			require.NotNil(t, lastCapturedDataPayload)

			t.Log("restart with new block manager")
			ctx, cancel = context.WithCancel(t.Context())
			manager, headerSync, dataSync = setupBlockManager(t, ctx, workDir, dbm, blockTime, noopSigner)

			var firstCapturedDataPayload *types.SignedData
			var firstCapturedHeaderPayload *types.SignedHeader
			manager.dataBroadcaster = capturingHeadBroadcaster[*types.SignedData](0, &firstCapturedDataPayload, dataSync)
			manager.headerBroadcaster = capturingHeadBroadcaster[*types.SignedHeader](0, &firstCapturedHeaderPayload, headerSync)
			go manager.AggregationLoop(ctx, errChan)
			select {
			case err := <-errChan:
				require.NoError(t, err)
			case <-time.After(spec.dataConsumerDelay + spec.headerConsumerDelay + 3*blockTime):
			}
			cancel()
			require.NotNil(t, firstCapturedHeaderPayload)
			assert.InDelta(t, lastCapturedDataPayload.Height(), firstCapturedDataPayload.Height(), 1)
			require.NotNil(t, firstCapturedDataPayload)
			assert.InDelta(t, lastCapturedHeaderPayload.Height(), firstCapturedHeaderPayload.Height(), 1)
		})
	}
}

func capturingTailBroadcaster[T interface{ Height() uint64 }](waitDuration time.Duration, target *T, next ...broadcaster[T]) broadcaster[T] {
	var lastHeight uint64
	return broadcasterFn[T](func(ctx context.Context, payload T) error {
		if payload.Height() <= lastHeight {
			panic(fmt.Sprintf("got height %d, want %d", payload.Height(), lastHeight+1))
		}

		time.Sleep(waitDuration)
		lastHeight = payload.Height()
		*target = payload
		var err error
		for _, n := range next {
			err = errors.Join(n.WriteToStoreAndBroadcast(ctx, payload))
		}

		return err
	})
}

func capturingHeadBroadcaster[T interface{ Height() uint64 }](waitDuration time.Duration, target *T, next ...broadcaster[T]) broadcaster[T] {
	var once sync.Once
	return broadcasterFn[T](func(ctx context.Context, payload T) error {
		once.Do(func() {
			*target = payload
		})
		var err error
		for _, n := range next {
			err = errors.Join(n.WriteToStoreAndBroadcast(ctx, payload))
		}
		time.Sleep(waitDuration)
		return err
	})
}

type broadcasterFn[T any] func(ctx context.Context, payload T) error

func (b broadcasterFn[T]) WriteToStoreAndBroadcast(ctx context.Context, payload T) error {
	return b(ctx, payload)
}

func setupBlockManager(t *testing.T, ctx context.Context, workDir string, mainKV ds.Batching, blockTime time.Duration, signer signer.Signer) (*Manager, *rollkitSync.HeaderSyncService, *rollkitSync.DataSyncService) {
	t.Helper()
	nodeConfig := config.DefaultConfig
	nodeConfig.Node.BlockTime = config.DurationWrapper{Duration: blockTime}
	nodeConfig.RootDir = workDir
	nodeKey, err := key.LoadOrGenNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
	require.NoError(t, err)

	proposerAddr, err := signer.GetAddress()
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
	require.NoError(t, err)

	const RollkitPrefix = "0"
	ktds.Wrap(mainKV, ktds.PrefixTransform{Prefix: ds.NewKey(RollkitPrefix)})
	headerSyncService, err := rollkitSync.NewHeaderSyncService(mainKV, nodeConfig, genesisDoc, p2pClient, logger.With("module", "HeaderSyncService"))
	require.NoError(t, err)
	require.NoError(t, headerSyncService.Start(ctx))
	dataSyncService, err := rollkitSync.NewDataSyncService(mainKV, nodeConfig, genesisDoc, p2pClient, logger.With("module", "DataSyncService"))
	require.NoError(t, err)
	require.NoError(t, dataSyncService.Start(ctx))

	result, err := NewManager(
		ctx,
		signer,
		nodeConfig,
		genesisDoc,
		store.New(mainKV),
		&mockExecutor{},
		coresequencer.NewDummySequencer(),
		nil,
		logger.With("module", "BlockManager"),
		headerSyncService.Store(),
		dataSyncService.Store(),
		nil,
		nil,
		NopMetrics(),
		1.,
		1.,
	)
	require.NoError(t, err)
	return result, headerSyncService, dataSyncService
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

var rnd = rand.New(rand.NewSource(1)) //nolint:gosec // test code only

func bytesN(n int) []byte {
	data := make([]byte, n)
	_, _ = rnd.Read(data)
	return data
}
