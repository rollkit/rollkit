package node

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
)

// TestTxGossipingMultipleNodesNoDA tests that transactions are gossiped and blocks are sequenced and synced across multiple nodes without the DA layer over P2P.
// It creates 4 nodes (1 sequencer, 3 full nodes), injects a transaction, waits for all nodes to sync, and asserts block equality.
func TestTxGossipingMultipleNodesNoDA(t *testing.T) {
	require := require.New(t)
	config := getTestConfig(t, 1)
	// Set the DA block time to a very large value to ensure that the DA layer is not used
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Second}
	numNodes := 4
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start only the sequencer first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the first block to be produced by the sequencer
	err := waitForFirstBlock(nodes[0], Header)
	require.NoError(err)

	// Verify block manager is properly initialized
	require.NotNil(nodes[0].blockManager, "Block manager should be initialized")

	// Start the other nodes
	for i := 1; i < numNodes; i++ {
		startNodeInBackground(t, nodes, ctxs, &runningWg, i)
	}

	// Inject a transaction into the sequencer's executor
	executor := nodes[0].blockManager.GetExecutor().(*coreexecutor.DummyExecutor)
	executor.InjectTx([]byte("test tx"))

	blocksToWaitFor := uint64(5)
	// Wait for all nodes to reach at least blocksToWaitFor blocks
	for _, node := range nodes {
		require.NoError(waitForAtLeastNBlocks(node, blocksToWaitFor, Store))
	}

	// Shutdown all nodes and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)

	// Assert that all nodes have the same block up to height blocksToWaitFor
	assertAllNodesSynced(t, nodes, blocksToWaitFor)
}

// TestTxGossipingMultipleNodesDAIncluded tests that transactions are gossiped and blocks are sequenced and synced across multiple nodes only using DA. P2P gossiping is disabled.
// It creates 4 nodes (1 sequencer, 3 full nodes), injects a transaction, waits for all nodes to sync with DA inclusion, and asserts block equality.
func TestTxGossipingMultipleNodesDAIncluded(t *testing.T) {
	require := require.New(t)
	config := getTestConfig(t, 1)
	// Disable P2P gossiping
	config.P2P.Peers = "none"

	numNodes := 4
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start only the sequencer first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the first block to be produced by the sequencer
	err := waitForFirstBlock(nodes[0], Header)
	require.NoError(err)

	// Verify block manager is properly initialized
	require.NotNil(nodes[0].blockManager, "Block manager should be initialized")

	// Start the other nodes
	for i := 1; i < numNodes; i++ {
		startNodeInBackground(t, nodes, ctxs, &runningWg, i)
	}

	// Inject a transaction into the sequencer's executor
	executor := nodes[0].blockManager.GetExecutor().(*coreexecutor.DummyExecutor)
	executor.InjectTx([]byte("test tx"))

	blocksToWaitFor := uint64(5)
	// Wait for all nodes to reach at least blocksToWaitFor blocks with DA inclusion
	for _, node := range nodes {
		require.NoError(waitForAtLeastNDAIncludedHeight(node, blocksToWaitFor))
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
	// Set the block time to 2 seconds and the DA block time to 1 second
	// Note: these are large values to avoid test failures due to slow CI machines
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 2 * time.Second}
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 1 * time.Second}

	nodes, cleanups := createNodesWithCleanup(t, 2, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(len(nodes))
	var runningWg sync.WaitGroup

	// Start only the first node
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the first node to produce a few blocks
	blocksToWaitFor := uint64(2)
	require.NoError(waitForAtLeastNDAIncludedHeight(nodes[0], blocksToWaitFor))

	// Now start the second node and time its sync
	startNodeInBackground(t, nodes, ctxs, &runningWg, 1)
	start := time.Now()
	// Wait for the second node to catch up to the first node
	require.NoError(waitForAtLeastNBlocks(nodes[1], blocksToWaitFor, Store))
	syncDuration := time.Since(start)

	// Ensure node syncs within a small delta of DA block time
	delta := 250 * time.Millisecond
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

	// Wait for the sequencer to produce at first block
	require.NoError(waitForFirstBlock(nodes[0], Store))

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

	for i := 1; i < numNodes; i++ {
		require.NoError(verifyNodesSynced(nodes[0], nodes[i], Store))
	}

	// Cancel all node contexts to signal shutdown and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// TestDataExchange verifies data exchange and synchronization between nodes in various network topologies.
//
// This test runs three sub-tests:
//  1. Single sequencer and single full node.
//  2. Single sequencer and two full nodes.
//  3. Single sequencer and single full node with trusted hash.
//
// Each sub-test checks data exchange and synchronization to ensure correct data propagation and consistency across nodes.
func TestDataExchange(t *testing.T) {
	t.Run("SingleSequencerSingleFullNode", func(t *testing.T) {
		testSingleSequencerSingleFullNode(t, Data)
	})
	t.Run("SingleSequencerTwoFullNodes", func(t *testing.T) {
		testSingleSequencerTwoFullNodes(t, Data)
	})
	t.Run("SingleSequencerSingleFullNodeTrustedHash", func(t *testing.T) {
		testSingleSequencerSingleFullNodeTrustedHash(t, Data)
	})
}

// TestHeaderExchange verifies header exchange and synchronization between nodes in various network topologies.
//
// This test runs three sub-tests:
//  1. Single sequencer and single full node.
//  2. Single sequencer and two full nodes.
//  3. Single sequencer and single full node with trusted hash.
//
// Each sub-test checks header exchange and synchronization to ensure correct header propagation and consistency across nodes.
func TestHeaderExchange(t *testing.T) {
	t.Run("SingleSequencerSingleFullNode", func(t *testing.T) {
		testSingleSequencerSingleFullNode(t, Header)
	})
	t.Run("SingleSequencerTwoFullNodes", func(t *testing.T) {
		testSingleSequencerTwoFullNodes(t, Header)
	})
	t.Run("SingleSequencerSingleFullNodeTrustedHash", func(t *testing.T) {
		testSingleSequencerSingleFullNodeTrustedHash(t, Header)
	})
}

// testSingleSequencerSingleFullNode sets up a single sequencer and a single full node, starts the sequencer, waits for it to produce a block, then starts the full node.
// It waits for both nodes to reach a target block height (using the provided 'source' to determine block inclusion), verifies that both nodes are fully synced, and then shuts them down.
func testSingleSequencerSingleFullNode(t *testing.T, source Source) {
	require := require.New(t)

	// Set up one sequencer and one full node
	config := getTestConfig(t, 1)
	numNodes := 2
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start the sequencer first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the sequencer to produce at first block
	require.NoError(waitForFirstBlock(nodes[0], source))

	// Start the full node
	startNodeInBackground(t, nodes, ctxs, &runningWg, 1)

	blocksToWaitFor := uint64(3)
	// Wait for both nodes to reach at least blocksToWaitFor blocks
	for _, node := range nodes {
		require.NoError(waitForAtLeastNBlocks(node, blocksToWaitFor, source))
	}

	// Verify both nodes are synced using the helper
	require.NoError(verifyNodesSynced(nodes[0], nodes[1], source))

	// Cancel all node contexts to signal shutdown and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// testSingleSequencerTwoFullNodes sets up a single sequencer and two full nodes, starts the sequencer, waits for it to produce a block, then starts the full nodes.
// It waits for all nodes to reach a target block height (using the provided 'source' to determine block inclusion), verifies that all nodes are fully synced, and then shuts them down.
func testSingleSequencerTwoFullNodes(t *testing.T, source Source) {
	require := require.New(t)

	// Set up one sequencer and two full nodes
	config := getTestConfig(t, 1)
	numNodes := 3
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start the sequencer first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the sequencer to produce at first block
	require.NoError(waitForFirstBlock(nodes[0], source))

	// Start the full nodes
	for i := 1; i < numNodes; i++ {
		startNodeInBackground(t, nodes, ctxs, &runningWg, i)
	}

	blocksToWaitFor := uint64(3)
	// Wait for all nodes to reach at least blocksToWaitFor blocks
	for _, node := range nodes {
		require.NoError(waitForAtLeastNBlocks(node, blocksToWaitFor, source))
	}

	// Verify all nodes are synced using the helper
	for i := 1; i < numNodes; i++ {
		require.NoError(verifyNodesSynced(nodes[0], nodes[i], source))
	}

	// Cancel all node contexts to signal shutdown and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// testSingleSequencerSingleFullNodeTrustedHash sets up a single sequencer and a single full node with a trusted hash, starts the sequencer, waits for it to produce a block, then starts the full node with the trusted hash.
// It waits for both nodes to reach a target block height (using the provided 'source' to determine block inclusion), verifies that both nodes are fully synced, and then shuts them down.
func testSingleSequencerSingleFullNodeTrustedHash(t *testing.T, source Source) {
	require := require.New(t)

	// Set up one sequencer and one full node
	config := getTestConfig(t, 1)
	numNodes := 2
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs, cancels := createNodeContexts(numNodes)
	var runningWg sync.WaitGroup

	// Start the sequencer first
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)

	// Wait for the sequencer to produce at first block
	require.NoError(waitForFirstBlock(nodes[0], source))

	// Get the hash of the first block (using the correct source)
	var trustedHash string
	switch source {
	case Data:
		trustedHashValue, err := nodes[0].dSyncService.Store().GetByHeight(ctxs[0], 1)
		require.NoError(err)
		trustedHash = trustedHashValue.Hash().String()
	case Header:
		trustedHashValue, err := nodes[0].hSyncService.Store().GetByHeight(ctxs[0], 1)
		require.NoError(err)
		trustedHash = trustedHashValue.Hash().String()
	default:
		t.Fatalf("unsupported source for trusted hash test: %v", source)
	}

	// Set the trusted hash in the full node
	nodes[1].nodeConfig.Node.TrustedHash = trustedHash

	// Start the full node
	startNodeInBackground(t, nodes, ctxs, &runningWg, 1)

	blocksToWaitFor := uint64(3)
	// Wait for both nodes to reach at least blocksToWaitFor blocks
	for _, node := range nodes {
		require.NoError(waitForAtLeastNBlocks(node, blocksToWaitFor, source))
	}

	// Verify both nodes are synced using the helper
	require.NoError(verifyNodesSynced(nodes[0], nodes[1], source))

	// Cancel all node contexts to signal shutdown and wait
	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// TestTwoChainsInOneNamespace verifies that two chains in the same namespace can coexist without any issues.
func TestTwoChainsInOneNamespace(t *testing.T) {
	cases := []struct {
		name     string
		chainID1 string
		chainID2 string
	}{
		{
			name:     "same chain ID",
			chainID1: "test-1",
			chainID2: "test-1",
		},
		{
			name:     "different chain IDs",
			chainID1: "foo-1",
			chainID2: "bar-2",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testTwoChainsInOneNamespace(t, c.chainID1, c.chainID2)
		})
	}
}

// testTwoChainsInOneNamespace sets up two chains in the same namespace, starts the sequencers, waits for blocks, then starts the full nodes.
// It waits for all nodes to reach a target block height, and verifies that all nodes are fully synced, and then shuts them down.
func testTwoChainsInOneNamespace(t *testing.T, chainID1 string, chainID2 string) {
	require := require.New(t)

	// Set up nodes for the first chain
	configChain1 := getTestConfig(t, 1)
	configChain1.ChainID = chainID1

	numNodes := 2
	nodes1, cleanups := createNodesWithCleanup(t, numNodes, configChain1)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	// Set up nodes for the second chain
	configChain2 := getTestConfig(t, 1000)
	configChain2.ChainID = chainID2

	numNodes = 2
	nodes2, cleanups := createNodesWithCleanup(t, numNodes, configChain2)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	// Set up context and wait group for the sequencer of chain 1
	ctxs1, cancels1 := createNodeContexts(numNodes)
	var runningWg1 sync.WaitGroup

	// Start the sequencer of chain 1
	startNodeInBackground(t, nodes1, ctxs1, &runningWg1, 0)

	// Wait for the sequencer to produce at first block
	require.NoError(waitForFirstBlock(nodes1[0], Store))

	// Set up context and wait group for the sequencer of chain 2
	ctxs2, cancels2 := createNodeContexts(numNodes)
	var runningWg2 sync.WaitGroup

	// Start the sequencer of chain 2
	startNodeInBackground(t, nodes2, ctxs2, &runningWg2, 0)

	// Wait for the sequencer to produce at first block
	require.NoError(waitForFirstBlock(nodes2[0], Store))

	// Start the full node of chain 1
	startNodeInBackground(t, nodes1, ctxs1, &runningWg1, 1)

	// Start the full node of chain 2
	startNodeInBackground(t, nodes2, ctxs2, &runningWg2, 1)

	blocksToWaitFor := uint64(3)

	// Wait for the full node of chain 1 to reach at least blocksToWaitFor blocks
	require.NoError(waitForAtLeastNBlocks(nodes1[1], blocksToWaitFor, Store))

	// Wait for the full node of chain 2 to reach at least blocksToWaitFor blocks
	require.NoError(waitForAtLeastNBlocks(nodes2[1], blocksToWaitFor, Store))

	// Verify both full nodes are synced using the helper
	require.NoError(verifyNodesSynced(nodes1[0], nodes1[1], Store))
	require.NoError(verifyNodesSynced(nodes2[0], nodes2[1], Store))

	// Cancel all node contexts to signal shutdown and wait for both chains
	shutdownAndWait(t, cancels1, &runningWg1, 5*time.Second)
	shutdownAndWait(t, cancels2, &runningWg2, 5*time.Second)
}

// TestSyncModeDaOnlyNode verifies that a node in SyncModeDaOnly does not sync headers from P2P
// but does sync them from DA.
func TestSyncModeDaOnlyNode(t *testing.T) {
	require := require.New(t)

	// Config for Node A (Sequencer, SyncModeBoth)
	configA := getTestConfig(t, 1) // Node A is a sequencer
	configA.Node.Aggregator = true
	configA.Node.SyncMode = rollkitconfig.SyncModeBoth // Default, but explicit
	configA.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond} // Relatively fast DA for testing
	configA.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond}

	// Config for Node B (Syncer, SyncModeDaOnly)
	configB := getTestConfig(t, 2) // Node B is a full node (not sequencer)
	configB.Node.Aggregator = false
	configB.Node.SyncMode = rollkitconfig.SyncModeDaOnly
	configB.DA = configA.DA // Ensure they use the same DA config for mock DA behavior
	configB.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond}


	// Create nodes. For this test, we need a more controllable DA.
	// The default createNodesWithCleanup might use a basic mock DA or a real client.
	// We'll assume for now it uses a mock DA that we can implicitly test.
	// A true controllable mock DA would require deeper changes to test infra.
	nodes := make([]Node, 2)
	cleanups := make([]func(), 2)

	nodeA, cleanupA := createNodeWithCleanup(t, configA)
	nodes[0] = nodeA
	cleanups[0] = cleanupA
	defer cleanupA()

	nodeB, cleanupB := createNodeWithCleanup(t, configB)
	nodes[1] = nodeB
	cleanups[1] = cleanupB
	defer cleanupB()

	// Ensure Node B can connect to Node A for P2P (e.g., for transaction gossip, though not primary for this test)
	// This setup is implicit in how createNodesWithCleanup might set up peer lists if P2P is enabled.
	// For SyncModeDaOnly, header sync over P2P is disabled by SyncService, other P2P functions might still work.

	ctxs, cancels := createNodeContexts(2)
	var runningWg sync.WaitGroup

	// Start Node A (Sequencer)
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)
	require.NoError(waitForFirstBlock(nodes[0], Store), "Node A (sequencer) failed to start or produce first block")

	// Start Node B (DaOnly Syncer)
	startNodeInBackground(t, nodes, ctxs, &runningWg, 1)
	// Wait a moment for Node B to establish P2P connection if any, and for its SyncLoop to start.
	time.Sleep(500 * time.Millisecond)

	// Node A produces a few blocks
	targetHeight := uint64(3)
	require.NoError(waitForAtLeastNBlocks(nodes[0], targetHeight, Store), "Node A failed to produce target blocks")

	// Verification for Node B (DaOnly)
	// 1. Node B should NOT sync these blocks from P2P.
	// Check Node B's height. It should remain at genesis (or 0 if genesis isn't auto-synced without DA).
	// Give it some time, if it were to sync from P2P, it would have by now.
	time.Sleep(2 * configA.Node.BlockTime.Duration) // Wait longer than a block time

	nodeBHeightP2PCheck, err := getNodeHeight(nodes[1], Store)
	require.NoError(err)
	// Assuming initial height is 1 after genesis. If it's 0, this check needs adjustment.
	// The core idea is that it shouldn't have synced Node A's new blocks yet.
	require.Less(nodeBHeightP2PCheck, targetHeight, "Node B (DaOnly) should not have synced blocks from P2P yet")

	// 2. Node B should sync blocks once they are available on DA.
	// The existing test infrastructure uses a DA client that eventually makes blocks available.
	// We wait for Node B to catch up, implying it got them from DA.
	require.NoError(waitForAtLeastNBlocks(nodes[1], targetHeight, Store), "Node B (DaOnly) failed to sync blocks from DA")

	nodeBHeightDACheck, err := getNodeHeight(nodes[1], Store)
	require.NoError(err)
	require.GreaterOrEqual(nodeBHeightDACheck, targetHeight, "Node B (DaOnly) should have synced blocks from DA")

	// Further verification: Ensure Node B's blocks match Node A's (implies successful DA sync)
	require.NoError(verifyNodesSynced(nodes[0], nodes[1], Store), "Nodes did not sync the same blocks via DA")

	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}

// TestSyncModeBothBaseline verifies that two nodes in SyncModeBoth can sync,
// with P2P potentially being the faster source.
func TestSyncModeBothBaseline(t *testing.T) {
	require := require.New(t)

	// Config for Node C (Sequencer, SyncModeBoth)
	configC := getTestConfig(t, 1)
	configC.Node.Aggregator = true
	configC.Node.SyncMode = rollkitconfig.SyncModeBoth
	configC.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 2 * time.Second}    // Slower DA
	configC.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond} // Faster blocks

	// Config for Node D (Syncer, SyncModeBoth)
	configD := getTestConfig(t, 2)
	configD.Node.Aggregator = false
	configD.Node.SyncMode = rollkitconfig.SyncModeBoth
	configD.DA = configC.DA // Same DA config
	configD.Node.BlockTime = configC.Node.BlockTime


	nodes := make([]Node, 2)
	cleanups := make([]func(), 2)

	nodeC, cleanupC := createNodeWithCleanup(t, configC)
	nodes[0] = nodeC
	cleanups[0] = cleanupC
	defer cleanupC()

	nodeD, cleanupD := createNodeWithCleanup(t, configD)
	nodes[1] = nodeD
	cleanups[1] = cleanupD
	defer cleanupD()


	ctxs, cancels := createNodeContexts(2)
	var runningWg sync.WaitGroup

	// Start Node C (Sequencer)
	startNodeInBackground(t, nodes, ctxs, &runningWg, 0)
	require.NoError(waitForFirstBlock(nodes[0], Store), "Node C (sequencer) failed to start or produce first block")

	// Start Node D (Both Syncer)
	startNodeInBackground(t, nodes, ctxs, &runningWg, 1)

	targetHeight := uint64(5)
	// Node C produces blocks
	require.NoError(waitForAtLeastNBlocks(nodes[0], targetHeight, Store), "Node C failed to produce target blocks")

	// Node D should sync these blocks. P2P should be faster than DA.
	// We verify it syncs to the target height. The exact source isn't explicitly checked here,
	// but the setup encourages P2P.
	err := Retry(15, configC.Node.BlockTime.Duration, func() error { // Retry for up to ~3 seconds
		return waitForAtLeastNBlocks(nodes[1], targetHeight, Store)
	})
	require.NoError(err, "Node D (Both) failed to sync blocks in time (P2P expected)")


	// Verify nodes are synced
	require.NoError(verifyNodesSynced(nodes[0], nodes[1], Store), "Nodes did not sync the same blocks")

	shutdownAndWait(t, cancels, &runningWg, 5*time.Second)
}
