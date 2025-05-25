package node

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
)

// TestTxGossipingAndAggregation tests that transactions are gossiped and blocks are aggregated and synced across multiple nodes.
// It creates 4 nodes (1 sequencer, 3 full nodes), injects a transaction, waits for all nodes to sync, and asserts block equality.
func TestTxGossipingAndAggregation(t *testing.T) {
	config := getTestConfig(t, 1)

	numNodes := 4
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start only the aggregator first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the first block to be produced by the aggregator
	err := waitForFirstBlock(nodes[0], Header)
	require.NoError(t, err, "Failed to get node height")

	// Verify block manager is properly initialized
	require.NotNil(t, nodes[0].blockManager, "Block manager should be initialized")

	// Start the other nodes
	for i := 1; i < numNodes; i++ {
		startNodeInBackground(t, nodes, ctxs, &runningWg, i)
	}

	// Inject a transaction into the aggregator's executor
	executor := nodes[0].blockManager.GetExecutor().(*coreexecutor.DummyExecutor)
	executor.InjectTx([]byte("gossip tx"))

	blocksToWaitFor := uint64(5)
	// Wait for all nodes to reach at least 5 blocks
	for _, node := range nodes {
		require.NoError(t, waitForAtLeastNBlocks(node, blocksToWaitFor, Store))
	}

	// Shutdown all nodes and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)

	// Assert that all nodes have the same block up to height blocksToWaitFor
	assertAllNodesSynced(t, nodes, blocksToWaitFor)
}

// TestFastDASync verifies that a new node can quickly synchronize with the DA layer using fast sync.
//
// This test sets up two nodes with different block and DA block times. It starts the sequencer node, waits for it to produce and DA-include several blocks,
// then starts the syncing full node and measures how quickly it can catch up to the sequencer node's height. The test asserts that the syncing full node syncs within
// a small delta of the DA block time, and verifies that both nodes have identical block hashes and that all blocks are DA-included.
func TestFastDASync(t *testing.T) {
	require := require.New(t)

	// Set up two nodes with different block and DA block times
	config := getTestConfig(t, 1)
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond}
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 50 * time.Millisecond}

	nodes, cleanups := createNodesWithCleanup(t, 2, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(len(nodes))
	var runningWg sync.WaitGroup

	// Start only the first node
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the first node to produce a few blocks
	blocksToWaitFor := uint64(5)
	require.NoError(waitForAtLeastNDAIncludedHeight(nodes[0], blocksToWaitFor))

	// Now start the second node and time its sync
	startNodeInBackground(t, nodes, ctxs, &runningWg, 1)
	start := time.Now()
	// Wait for the second node to catch up to the first node
	require.NoError(waitForAtLeastNBlocks(nodes[1], blocksToWaitFor, Store))
	syncDuration := time.Since(start)

	// Ensure node syncs within a small delta of DA block time
	delta := 75 * time.Millisecond
	require.Less(syncDuration, config.DA.BlockTime.Duration+delta, "Block sync should be faster than DA block time")

	// Verify both nodes are synced and that the synced block is DA-included
	assertAllNodesSynced(t, nodes, blocksToWaitFor)

	// Cancel all node contexts to signal shutdown and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// TestSingleSequencerTwoFullNodesBlockSyncSpeed tests that block synchronization is not bottlenecked by DA block time.
//
// This test sets up three nodes (one sequencer and two full nodes) with a fast block time and a slow DA block time. It starts the sequencer first, waits for it to produce a block, then starts the full nodes. The test waits for all nodes to sync a set number of blocks, measures the total sync duration, and asserts that block sync completes within a reasonable multiple of the block time (not the DA block time). It also verifies that all nodes have identical block hashes up to the target height.
func TestSingleSequencerTwoFullNodesBlockSyncSpeed(t *testing.T) {
	require := require.New(t)

	// Set up three nodes: 1 sequencer, 2 full nodes
	config := getTestConfig(t, 1)
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond} // fast block time
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 10 * time.Second}         // slow DA block time

	numNodes := 3
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start only the sequencer first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the sequencer to produce at least one block
	require.NoError(waitForAtLeastNBlocks(nodes[0], 1, Store))

	// Now start the other nodes
	for i := 1; i < numNodes; i++ {
		startNodeInBackground(t, nodes, ctxs, &runningWg, i)
	}

	blocksToWaitFor := uint64(10)
	start := time.Now()

	// Wait for all nodes to reach the target block height
	for _, node := range nodes {
		require.NoError(waitForAtLeastNBlocks(node, blocksToWaitFor, Store))
	}
	totalDuration := time.Since(start)

	// The test should complete within a reasonable multiple of block time, not DA block time
	maxExpected := config.Node.BlockTime.Duration*time.Duration(blocksToWaitFor) + 200*time.Millisecond
	require.Less(totalDuration, maxExpected, "Block sync should not be bottlenecked by DA block time")

	// Assert that all nodes have the same block up to height blocksToWaitFor
	assertAllNodesSynced(t, nodes, blocksToWaitFor)

	// Cancel all node contexts to signal shutdown and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// TestSingleSequencerTwoFullNodesDAInclusion verifies DA inclusion when block time is much faster than DA block time.

// TestBlockExchange verifies block exchange between nodes.

/*
TODO:

	Details:
	Runs three sub-tests:
	Single aggregator, single full node.
	Single aggregator, two full nodes.
	Single aggregator, single full node with trusted hash.
	Each sub-test checks block exchange and synchronization.
	Goal: Ensures block exchange works in different network topologies.
*/
func (s *FullNodeTestSuite) TestBlockExchange() {
	s.T().Skip()
}

// TestHeaderExchange verifies header exchange between nodes.

/*
TODO:
Purpose: Tests header exchange scenarios.
	Details:
	Runs four sub-tests:
	Single aggregator, single full node.
	Single aggregator, two full nodes.
	Single aggregator, single full node with trusted hash.
	Single aggregator, single full node, single light node.
	Each sub-test checks header exchange and synchronization.
	Goal: Ensures header exchange works in different network topologies.
*/

// TestTwoRollupsInOneNamespace verifies that  two rollup chains in the same namespace.

/*
TODO:
	Details:
	Runs two cases: same chain ID and different chain IDs.
	For each, sets up two rollup networks, each with two nodes.
	Uses a shared mock DA client.
	Starts both aggregators, waits for blocks, then stops them and starts the full nodes.
	Verifies full nodes can sync blocks from DA.
	Goal: Ensures multiple rollups can coexist and sync in the same namespace.
*/

// testSingleAggregatorSingleFullNode verifies block sync and DA inclusion for a single aggregator and full node.

/*
TODO:
Details:

	Sets up one aggregator and one full node.
	Injects a transaction, waits for blocks, then checks DA inclusion.
	Verifies both nodes are synced and that the synced block is DA-included.
	Goal: Ensures single aggregator and full node can sync and DA inclusion works.

	Details:

Starts all nodes, waits for blocks, and verifies synchronization.
*/
func (s *FullNodeTestSuite) testSingleAggregatorSingleFullNode() {
	s.T().Skip()
}

// testSingleAggregatorTwoFullNodes verifies block sync and DA inclusion for a single aggregator and two full nodes.

/*
TODO:
Details:
Sets up one aggregator and two full nodes.
Injects a transaction, waits for blocks, then checks DA inclusion.
Verifies all nodes are synced and that the synced block is DA-included.
Goal: Ensures single aggregator and two full nodes can sync and DA inclusion works.
*/
func (s *FullNodeTestSuite) testSingleAggregatorTwoFullNodes() {
	s.T().Skip()
}

// testSingleAggregatorSingleFullNodeTrustedHash verifies block sync and DA inclusion for a single aggregator and full node with a trusted hash.

/*
TODO:
Details:

	Sets up one aggregator and one full node with a trusted hash.
	Injects a transaction, waits for blocks, then checks DA inclusion.
	Verifies both nodes are synced and that the synced block is DA-included.
	Goal: Ensures single aggregator and full node with trusted hash can sync and DA inclusion works.
*/
func (s *FullNodeTestSuite) testSingleAggregatorSingleFullNodeTrustedHash() {
	s.T().Skip()
}
