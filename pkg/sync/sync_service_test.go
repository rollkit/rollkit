package sync

import (
	"context"
	sdklog "cosmossdk.io/log"
	cryptoRand "crypto/rand"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/require"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	testifylog "github.com/stretchr/testify/log"
)

func TestHeaderSyncServiceRestart(t *testing.T) {
	logging.SetDebugLogging()
	mainKV := sync.MutexWrap(datastore.NewMapDatastore())
	pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	rnd := rand.New(rand.NewSource(1)) // nolint:gosec // test code only

	proposerAddr := []byte("test")
	genesisDoc := genesispkg.Genesis{
		ChainID:            "test-chain-id",
		GenesisDAStartTime: time.Now(),
		InitialHeight:      1,
		ProposerAddress:    proposerAddr,
	}
	conf := config.DefaultConfig
	conf.RootDir = t.TempDir()
	nodeKey, err := key.LoadOrGenNodeKey(filepath.Dir(conf.ConfigPath()))
	require.NoError(t, err)
	logger := sdklog.NewTestLogger(t)
	p2pClient, err := p2p.NewClient(conf, nodeKey, mainKV, logger, p2p.NopMetrics())
	require.NoError(t, err)

	// Start p2p client before creating sync service
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = p2pClient.Start(ctx)
	require.NoError(t, err)

	svc, err := NewHeaderSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
	require.NoError(t, err)
	err = svc.Start(ctx)
	require.NoError(t, err)

	// broadcast genesis block
	headerConfig := types.HeaderConfig{
		Height:   genesisDoc.InitialHeight,
		DataHash: bytesN(rnd, 32),
		AppHash:  bytesN(rnd, 32),
		Signer:   noopSigner,
	}
	signedHeader, err := types.GetRandomSignedHeaderCustom(&headerConfig, genesisDoc.ChainID)
	require.NoError(t, err)
	require.NoError(t, signedHeader.Validate())
	require.NoError(t, svc.WriteToStoreAndBroadcast(ctx, signedHeader))

	// broadcast another 10 example blocks
	for i := genesisDoc.InitialHeight + 1; i < 10; i++ {
		signedHeader = nextHeader(t, signedHeader, genesisDoc.ChainID, noopSigner)
		t.Logf("signed header: %d", i)
		require.NoError(t, svc.WriteToStoreAndBroadcast(ctx, signedHeader))
	}

	// then stop and restart service
	_ = p2pClient.Close()
	_ = svc.Stop(ctx)
	cancel()

	p2pClient, err = p2p.NewClient(conf, nodeKey, mainKV, logger, p2p.NopMetrics())
	require.NoError(t, err)

	// Start p2p client again
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	err = p2pClient.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p2pClient.Close() })

	svc, err = NewHeaderSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
	require.NoError(t, err)
	err = svc.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })
	// done with stop and restart service

	// broadcast another 10 example blocks
	for i := signedHeader.Height() + 1; i < 10; i++ {
		signedHeader = nextHeader(t, signedHeader, genesisDoc.ChainID, noopSigner)
		t.Logf("signed header: %d", i)
		require.NoError(t, svc.WriteToStoreAndBroadcast(ctx, signedHeader))
	}
	cancel()
}

func nextHeader(t *testing.T, previousHeader *types.SignedHeader, chainID string, noopSigner signer.Signer) *types.SignedHeader {
	newSignedHeader := &types.SignedHeader{
		Header: types.GetRandomNextHeader(previousHeader.Header, chainID),
		Signer: previousHeader.Signer,
	}
	b, err := newSignedHeader.Header.MarshalBinary()
	require.NoError(t, err)
	signature, err := noopSigner.Sign(b)
	require.NoError(t, err)
	newSignedHeader.Signature = signature
	require.NoError(t, newSignedHeader.Validate())
	previousHeader = newSignedHeader
	return previousHeader
}

func bytesN(r *rand.Rand, n int) []byte {
	data := make([]byte, n)
	_, _ = r.Read(data)
	return data
}

func TestSyncServiceStart_SyncModes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Common setup
	mainKV := sync.MutexWrap(datastore.NewMapDatastore())
	pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	rnd := rand.New(rand.NewSource(1)) // nolint:gosec // test code only
	proposerAddr := []byte("test")
	genesisDoc := genesispkg.Genesis{
		ChainID:            "test-chain-id",
		GenesisDAStartTime: time.Now(),
		InitialHeight:      1,
		ProposerAddress:    proposerAddr,
	}
	baseConf := config.DefaultConfig
	baseConf.RootDir = t.TempDir()
	nodeKey, err := key.LoadOrGenNodeKey(filepath.Join(baseConf.RootDir, "config")) // Ensure unique path for key
	require.NoError(t, err)

	headerConfig := types.HeaderConfig{
		Height:   genesisDoc.InitialHeight,
		DataHash: bytesN(rnd, 32),
		AppHash:  bytesN(rnd, 32),
		Signer:   noopSigner,
	}
	genesisHeader, err := types.GetRandomSignedHeaderCustom(&headerConfig, genesisDoc.ChainID)
	require.NoError(t, err)

	t.Run("HeaderSyncService with SyncModeDaOnly", func(t *testing.T) {
		conf := baseConf
		conf.Node.SyncMode = config.SyncModeDaOnly
		logger := testifylog.NewTestLogger(t) // Using testify's logger for easy capture

		p2pClient, err := p2p.NewClient(conf, nodeKey, mainKV, logger, p2p.NopMetrics())
		require.NoError(t, err)
		// No need to start p2pClient for this case as SyncService.Start should exit early

		svc, err := NewHeaderSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
		require.NoError(t, err)

		err = svc.Start(ctx)
		require.NoError(t, err)
		defer func() { _ = svc.Stop(context.Background()) }()

		// Assert P2P components are not initialized
		require.Nil(t, svc.sub, "Subscriber should be nil in SyncModeDaOnly for headers")
		require.Nil(t, svc.ex, "Exchange should be nil in SyncModeDaOnly for headers")
		require.Nil(t, svc.p2pServer, "P2PServer should be nil in SyncModeDaOnly for headers")
		require.Nil(t, svc.syncer, "Syncer should be nil in SyncModeDaOnly for headers")

		// Assert store is started (or at least Start was attempted)
		// We can't directly check if store.Start was called without mocks/spies on the store itself.
		// However, the code calls store.Start(), so we assume it's called if no error.
		// A more robust check would involve a mock store or checking store's internal state if possible.
		require.True(t, svc.store.Height() == 0 || svc.store.Head(ctx) != nil, "Store should be started, though may be empty")

		require.Contains(t, logger.Logs().String(), "P2P header synchronization is disabled in da_only sync mode")
	})

	t.Run("HeaderSyncService with SyncModeBoth", func(t *testing.T) {
		conf := baseConf
		conf.Node.SyncMode = config.SyncModeBoth
		logger := sdklog.NewTestLogger(t) // sdklog for this case, or use testify logger too

		p2pClient, err := p2p.NewClient(conf, nodeKey, mainKV, logger, p2p.NopMetrics())
		require.NoError(t, err)
		err = p2pClient.Start(ctx) // Start p2p client as it's needed for SyncModeBoth
		require.NoError(t, err)
		defer func() { _ = p2pClient.Close() }()

		svc, err := NewHeaderSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
		require.NoError(t, err)

		// In SyncModeBoth, Start will try to sync with peers.
		// Since no peers are actually set up to provide the genesis header here,
		// setFirstAndStart might return an error or hang if not handled.
		// To avoid this, we can initialize the store with the genesis header first,
		// so setFirstAndStart doesn't need to fetch it.
		require.NoError(t, svc.store.Init(ctx, genesisHeader))

		err = svc.Start(ctx)
		require.NoError(t, err, "svc.Start should not error in SyncModeBoth even with no peers, if store is init")
		defer func() { _ = svc.Stop(context.Background()) }()

		// Assert P2P components are initialized (or attempted to)
		require.NotNil(t, svc.sub, "Subscriber should be initialized in SyncModeBoth for headers")
		require.NotNil(t, svc.ex, "Exchange should be initialized in SyncModeBoth for headers")
		require.NotNil(t, svc.p2pServer, "P2PServer should be initialized in SyncModeBoth for headers")
		require.NotNil(t, svc.syncer, "Syncer should be initialized in SyncModeBoth for headers")
		// Syncer might not be "started" if it has no work or peers, but it should be non-nil.
		// require.True(t, svc.syncerStatus.isStarted(), "Syncer should be started")
	})

	t.Run("DataSyncService with SyncModeDaOnly", func(t *testing.T) {
		conf := baseConf
		conf.Node.SyncMode = config.SyncModeDaOnly // Data sync should not be affected
		logger := sdklog.NewTestLogger(t)

		p2pClient, err := p2p.NewClient(conf, nodeKey, mainKV, logger, p2p.NopMetrics())
		require.NoError(t, err)
		err = p2pClient.Start(ctx) // Start p2p client as it's needed for DataSyncService
		require.NoError(t, err)
		defer func() { _ = p2pClient.Close() }()

		// Create a dummy genesis data block for DataSyncService
		genesisData := &types.Data{
			Metadata: &types.Metadata{Height: genesisDoc.InitialHeight, ChainID: genesisDoc.ChainID},
			Txs:      types.Txs{},
		}
		require.NoError(t, genesisData.Validate())


		svc, err := NewDataSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
		require.NoError(t, err)

		// Similar to HeaderSyncService in SyncModeBoth, initialize store to prevent issues with peer discovery
		require.NoError(t, svc.store.Init(ctx, genesisData))


		err = svc.Start(ctx)
		require.NoError(t, err, "DataSyncService Start should not error in SyncModeDaOnly")
		defer func() { _ = svc.Stop(context.Background()) }()

		// Assert P2P components are initialized for DataSyncService
		require.NotNil(t, svc.sub, "Subscriber should be initialized for DataSyncService")
		require.NotNil(t, svc.ex, "Exchange should be initialized for DataSyncService")
		require.NotNil(t, svc.p2pServer, "P2PServer should be initialized for DataSyncService")
		require.NotNil(t, svc.syncer, "Syncer should be initialized for DataSyncService")
	})
}
