//nolint:unused
package node

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"

	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	remote_signer "github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/types"
)

const (
	// MockDAAddress is the address used by the mock gRPC service
	// NOTE: this should be unique per test package to avoid
	// "bind: listen address already in use" because multiple packages
	// are tested in parallel
	MockDAAddress = "grpc://localhost:7990"

	// MockDANamespace is a sample namespace used by the mock DA client
	MockDANamespace = "00000000000000000000000000000000000000000000000000deadbeef"

	// MockExecutorAddress is a sample address used by the mock executor
	MockExecutorAddress = "127.0.0.1:40041"
)

// createTestComponents creates test components for node initialization
func createTestComponents(t *testing.T, config rollkitconfig.Config) (coreexecutor.Executor, coresequencer.Sequencer, coreda.DA, *p2p.Client, datastore.Batching, *key.NodeKey, func()) {
	executor := coreexecutor.NewDummyExecutor()
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0, config.DA.BlockTime.Duration)
	dummyDA.StartHeightTicker()

	stopDAHeightTicker := func() {
		dummyDA.StopHeightTicker()
	}

	// Create genesis and keys for P2P client
	_, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	p2pKey := &key.NodeKey{
		PrivKey: genesisValidatorKey,
		PubKey:  genesisValidatorKey.GetPublic(),
	}
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL")
	p2pClient, err := p2p.NewClient(config, p2pKey, dssync.MutexWrap(datastore.NewMapDatastore()), logger, p2p.NopMetrics())
	require.NoError(t, err)
	require.NotNil(t, p2pClient)
	ds := dssync.MutexWrap(datastore.NewMapDatastore())

	return executor, sequencer, dummyDA, p2pClient, ds, p2pKey, stopDAHeightTicker
}

func getTestConfig(t *testing.T, n int) rollkitconfig.Config {
	// Use a higher base port to reduce chances of conflicts with system services
	startPort := 40000 // Spread port ranges further apart
	return rollkitconfig.Config{
		RootDir: t.TempDir(),
		Node: rollkitconfig.NodeConfig{
			Aggregator:               true,
			BlockTime:                rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond},
			MaxPendingHeadersAndData: 1000,
			LazyBlockInterval:        rollkitconfig.DurationWrapper{Duration: 5 * time.Second},
		},
		DA: rollkitconfig.DAConfig{
			BlockTime: rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond},
			Address:   MockDAAddress,
			Namespace: MockDANamespace,
		},
		P2P: rollkitconfig.P2PConfig{
			ListenAddress: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", startPort+n),
		},
		RPC: rollkitconfig.RPCConfig{
			Address: fmt.Sprintf("127.0.0.1:%d", 8000+n),
		},
		ChainID:         "test-chain",
		Instrumentation: &rollkitconfig.InstrumentationConfig{},
	}
}

// newTestNode is a private helper that creates a node and returns it with a unified cleanup function.
func newTestNode(
	t *testing.T,
	config rollkitconfig.Config,
	executor coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dac coreda.DA,
	p2pClient *p2p.Client,
	ds datastore.Batching,
	stopDAHeightTicker func(),
) (*FullNode, func()) {
	ctx, cancel := context.WithCancel(context.Background())

	// Generate genesis and keys
	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey(config.ChainID)
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(t, err)

	node, err := NewNode(
		ctx,
		config,
		executor,
		sequencer,
		dac,
		remoteSigner,
		p2pClient,
		genesis,
		ds,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logging.Logger("test"),
		nil,
	)
	require.NoError(t, err)

	cleanup := func() {
		cancel()
		if stopDAHeightTicker != nil {
			stopDAHeightTicker()
		}
	}

	return node.(*FullNode), cleanup
}

func createNodeWithCleanup(t *testing.T, config rollkitconfig.Config) (*FullNode, func()) {
	executor, sequencer, dac, p2pClient, ds, _, stopDAHeightTicker := createTestComponents(t, config)
	return newTestNode(t, config, executor, sequencer, dac, p2pClient, ds, stopDAHeightTicker)
}

func createNodeWithCustomComponents(
	t *testing.T,
	config rollkitconfig.Config,
	executor coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dac coreda.DA,
	p2pClient *p2p.Client,
	ds datastore.Batching,
	stopDAHeightTicker func(),
) (*FullNode, func()) {
	return newTestNode(t, config, executor, sequencer, dac, p2pClient, ds, stopDAHeightTicker)
}

// Creates the given number of nodes the given nodes using the given wait group to synchronize them
func createNodesWithCleanup(t *testing.T, num int, config rollkitconfig.Config) ([]*FullNode, []func()) {
	t.Helper()
	require := require.New(t)

	nodes := make([]*FullNode, num)
	cleanups := make([]func(), num)
	// Create a cancellable context instead of using background context
	aggCtx, aggCancel := context.WithCancel(context.Background())

	// Generate genesis and keys
	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey(config.ChainID)
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(err)

	aggListenAddress := config.P2P.ListenAddress
	aggPeers := config.P2P.Peers
	executor, sequencer, dac, p2pClient, ds, aggP2PKey, stopDAHeightTicker := createTestComponents(t, config)
	aggPeerID, err := peer.IDFromPrivateKey(aggP2PKey.PrivKey)
	require.NoError(err)

	aggNode, err := NewNode(
		aggCtx,
		config,
		executor,
		sequencer,
		dac,
		remoteSigner,
		p2pClient,
		genesis,
		ds,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logging.Logger("test"),
		nil,
	)
	require.NoError(err)

	// Update cleanup to cancel the context instead of calling Stop
	cleanup := func() {
		// Cancel the context to stop the node
		aggCancel()
		stopDAHeightTicker()
	}

	nodes[0], cleanups[0] = aggNode.(*FullNode), cleanup
	config.Node.Aggregator = false
	peersList := []string{}
	if aggPeers != "none" {
		aggPeerAddress := fmt.Sprintf("%s/p2p/%s", aggListenAddress, aggPeerID.Loggable()["peerID"].(string))
		peersList = append(peersList, aggPeerAddress)
	}
	for i := 1; i < num; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		if aggPeers != "none" {
			config.P2P.Peers = strings.Join(peersList, ",")
		}
		config.P2P.ListenAddress = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", 40001+i)
		config.RPC.Address = fmt.Sprintf("127.0.0.1:%d", 8001+i)
		executor, sequencer, _, p2pClient, _, nodeP2PKey, stopDAHeightTicker := createTestComponents(t, config)
		node, err := NewNode(
			ctx,
			config,
			executor,
			sequencer,
			dac,
			nil,
			p2pClient,
			genesis,
			dssync.MutexWrap(datastore.NewMapDatastore()),
			DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
			logging.Logger("test"),
			nil,
		)
		require.NoError(err)
		// Update cleanup to cancel the context instead of calling Stop
		cleanup := func() {
			// Cancel the context to stop the node
			cancel()
			stopDAHeightTicker()
		}
		nodes[i], cleanups[i] = node.(*FullNode), cleanup
		nodePeerID, err := peer.IDFromPrivateKey(nodeP2PKey.PrivKey)
		require.NoError(err)
		peersList = append(peersList, fmt.Sprintf("%s/p2p/%s", config.P2P.ListenAddress, nodePeerID.Loggable()["peerID"].(string)))
	}

	return nodes, cleanups
}

// Helper to create N contexts and cancel functions
func createNodeContexts(n int) ([]context.Context, []context.CancelFunc) {
	ctxs := make([]context.Context, n)
	cancels := make([]context.CancelFunc, n)
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		cancels[i] = cancel
	}
	return ctxs, cancels
}

// Helper to start a single node in a goroutine and add to wait group
func startNodeInBackground(t *testing.T, nodes []*FullNode, ctxs []context.Context, wg *sync.WaitGroup, idx int) {
	wg.Add(1)
	go func(node *FullNode, ctx context.Context, idx int) {
		defer wg.Done()
		err := node.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error running node %d: %v", idx, err)
		}
	}(nodes[idx], ctxs[idx], idx)
}

// Helper to cancel all contexts and wait for goroutines with timeout
func shutdownAndWait(t *testing.T, cancels []context.CancelFunc, wg *sync.WaitGroup, timeout time.Duration) {
	for _, cancel := range cancels {
		cancel()
	}
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()
	select {
	case <-waitCh:
		// Nodes stopped successfully
	case <-time.After(timeout):
		t.Log("Warning: Not all nodes stopped gracefully within timeout")
	}
}

// Helper to check that all nodes are synced up to a given height (all block hashes match for all heights up to maxHeight)
func assertAllNodesSynced(t *testing.T, nodes []*FullNode, maxHeight uint64) {
	t.Helper()
	for height := uint64(1); height <= maxHeight; height++ {
		var refHash []byte
		for i, node := range nodes {
			header, _, err := node.Store.GetBlockData(context.Background(), height)
			require.NoError(t, err)
			if i == 0 {
				refHash = header.Hash()
			} else {
				headerHash := header.Hash()
				require.EqualValues(t, refHash, headerHash, "Block hash mismatch at height %d between node 0 and node %d", height, i)
			}
		}
	}
}

func verifyNodesSynced(node1, syncingNode Node, source Source) error {
	return testutils.Retry(300, 100*time.Millisecond, func() error {
		sequencerHeight, err := getNodeHeight(node1, source)
		if err != nil {
			return err
		}
		syncingHeight, err := getNodeHeight(syncingNode, source)
		if err != nil {
			return err
		}
		if sequencerHeight >= syncingHeight {
			return nil
		}
		return fmt.Errorf("nodes not synced: sequencer at height %v, syncing node at height %v", sequencerHeight, syncingHeight)
	})
}
