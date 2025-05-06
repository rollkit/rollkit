package node

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
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

	// MockSequencerAddress is a sample address used by the mock sequencer
	MockSequencerAddress = "127.0.0.1:50051"

	// MockExecutorAddress is a sample address used by the mock executor
	MockExecutorAddress = "127.0.0.1:40041"
)

// createTestComponents creates test components for node initialization
func createTestComponents(t *testing.T) (coreexecutor.Executor, coresequencer.Sequencer, coreda.DA, *p2p.Client, datastore.Batching) {
	executor := coreexecutor.NewDummyExecutor()
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)

	// Create genesis and keys for P2P client
	_, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	nodeKey := &key.NodeKey{
		PrivKey: genesisValidatorKey,
		PubKey:  genesisValidatorKey.GetPublic(),
	}
	p2pClient, err := p2p.NewClient(rollkitconfig.DefaultConfig, nodeKey, dssync.MutexWrap(datastore.NewMapDatastore()), log.NewNopLogger(), p2p.NopMetrics())
	require.NoError(t, err)
	require.NotNil(t, p2pClient)
	ds := dssync.MutexWrap(datastore.NewMapDatastore())

	return executor, sequencer, dummyDA, p2pClient, ds
}

func getTestConfig(t *testing.T, n int) rollkitconfig.Config {
	// Use a higher base port to reduce chances of conflicts with system services
	startPort := 40000 + n*100 // Spread port ranges further apart
	return rollkitconfig.Config{
		RootDir: t.TempDir(),
		Node: rollkitconfig.NodeConfig{
			Aggregator:        true,
			BlockTime:         rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond},
			MaxPendingHeaders: 100,
			LazyBlockInterval: rollkitconfig.DurationWrapper{Duration: 5 * time.Second},
		},
		DA: rollkitconfig.DAConfig{
			BlockTime: rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond},
			Address:   MockDAAddress,
			Namespace: MockDANamespace,
		},
		P2P: rollkitconfig.P2PConfig{
			ListenAddress: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", startPort+n),
		},
	}
}

func createNodeWithCleanup(t *testing.T, config rollkitconfig.Config) (*FullNode, func()) {
	// Create a cancellable context instead of using background context
	ctx, cancel := context.WithCancel(context.Background())

	// Generate genesis and keys
	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(t, err)

	executor, sequencer, dac, p2pClient, ds := createTestComponents(t)

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
		log.NewTestLogger(t),
	)
	require.NoError(t, err)

	// Update cleanup to cancel the context instead of calling Stop
	cleanup := func() {
		// Cancel the context to stop the node
		cancel()
	}

	return node.(*FullNode), cleanup
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
	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(err)

	executor, sequencer, dac, p2pClient, ds := createTestComponents(t)

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
		log.NewTestLogger(t),
	)
	require.NoError(err)

	// Update cleanup to cancel the context instead of calling Stop
	cleanup := func() {
		// Cancel the context to stop the node
		aggCancel()
	}

	nodes[0], cleanups[0] = aggNode.(*FullNode), cleanup
	config.Node.Aggregator = false
	for i := 1; i < num; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		executor, sequencer, _, p2pClient, ds = createTestComponents(t)
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
			log.NewTestLogger(t),
		)
		require.NoError(err)
		// Update cleanup to cancel the context instead of calling Stop
		cleanup := func() {
			// Cancel the context to stop the node
			cancel()
		}
		nodes[i], cleanups[i] = node.(*FullNode), cleanup
	}

	return nodes, cleanups
}
