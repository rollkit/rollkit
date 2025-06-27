//go:build integration

package node

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
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
	executor.InjectTx([]byte("test tx 1"))
	executor.InjectTx([]byte("test tx 2"))
	executor.InjectTx([]byte("test tx 3"))

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

// TestBatchQueueThrottlingWithDAFailure tests that when DA layer fails and MaxPendingHeadersAndData
// is reached, the system behaves correctly and doesn't run into resource exhaustion.
// This test uses the dummy sequencer but demonstrates the scenario that would occur
// with a real single sequencer having queue limits.
func TestBatchQueueThrottlingWithDAFailure(t *testing.T) {
	require := require.New(t)

	// Set up configuration with low limits to trigger throttling quickly
	config := getTestConfig(t, 1)
	config.Node.MaxPendingHeadersAndData = 2  // Low limit to quickly reach pending limit
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond}
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond}

	// Create test components
	executor, sequencer, dummyDA, p2pClient, ds, _, stopDAHeightTicker := createTestComponents(t, config)
	defer stopDAHeightTicker()

	// Cast executor to DummyExecutor so we can inject transactions
	dummyExecutor, ok := executor.(*coreexecutor.DummyExecutor)
	require.True(ok, "Expected DummyExecutor implementation")

	// Cast dummyDA to our enhanced version so we can make it fail
	dummyDAImpl, ok := dummyDA.(*coreda.DummyDA)
	require.True(ok, "Expected DummyDA implementation")

	// Create node with components
	node, cleanup := createNodeWithCustomComponents(t, config, executor, sequencer, dummyDAImpl, p2pClient, ds, func() {})
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runningWg sync.WaitGroup
	startNodeInBackground(t, []*FullNode{node}, []context.Context{ctx}, &runningWg, 0)

	// Wait for the node to start producing blocks
	require.NoError(waitForFirstBlock(node, Store))

	// Inject some initial transactions to get the system working
	for i := 0; i < 5; i++ {
		dummyExecutor.InjectTx([]byte(fmt.Sprintf("initial-tx-%d", i)))
	}

	// Wait for some initial blocks to be produced and verify normal operation
	require.NoError(waitForAtLeastNBlocks(node, 2, Store))
	t.Log("Initial blocks produced successfully")

	// Get the current height before DA failure
	initialHeight, err := getNodeHeight(node, Store)
	require.NoError(err)
	t.Logf("Height before DA failure: %d", initialHeight)

	// Simulate DA layer going down
	t.Log("Simulating DA layer failure")
	dummyDAImpl.SetSubmitFailure(true)

	// Continue injecting transactions - this tests the behavior when:
	// 1. DA layer is down (can't submit blocks to DA)
	// 2. MaxPendingHeadersAndData limit is reached (stops block production)
	// 3. Reaper continues trying to submit transactions
	// In a real single sequencer, this would fill the batch queue and eventually return ErrQueueFull
	go func() {
		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				dummyExecutor.InjectTx([]byte(fmt.Sprintf("tx-after-da-failure-%d", i)))
				time.Sleep(10 * time.Millisecond) // Inject faster than block time
			}
		}
	}()

	// Wait for the pending headers/data to reach the MaxPendingHeadersAndData limit
	// This should cause block production to stop
	time.Sleep(3 * config.Node.BlockTime.Duration)

	// Verify that block production has stopped due to MaxPendingHeadersAndData
	heightAfterDAFailure, err := getNodeHeight(node, Store)
	require.NoError(err)
	t.Logf("Height after DA failure: %d", heightAfterDAFailure)

	// Wait a bit more and verify height didn't increase significantly
	time.Sleep(5 * config.Node.BlockTime.Duration)
	finalHeight, err := getNodeHeight(node, Store)
	require.NoError(err)
	t.Logf("Final height: %d", finalHeight)

	// The height should not have increased much due to MaxPendingHeadersAndData limit
	// Allow at most 1 additional block due to timing
	heightIncrease := finalHeight - heightAfterDAFailure
	require.LessOrEqual(heightIncrease, uint64(1), 
		"Height should not increase significantly when DA is down and MaxPendingHeadersAndData limit is reached")

	t.Logf("Successfully demonstrated that MaxPendingHeadersAndData prevents runaway block production when DA fails")
	t.Logf("Height progression: initial=%d, after_DA_failure=%d, final=%d", 
		initialHeight, heightAfterDAFailure, finalHeight)

	// This test demonstrates the scenario described in the PR:
	// - DA layer goes down (SetSubmitFailure(true))
	// - Block production stops when MaxPendingHeadersAndData limit is reached
	// - Reaper continues injecting transactions (would fill batch queue in real single sequencer)
	// - In a real single sequencer with queue limits, this would eventually return ErrQueueFull
	//   preventing unbounded resource consumption

	t.Log("NOTE: This test uses DummySequencer. In a real deployment with SingleSequencer,")
	t.Log("the batch queue would fill up and return ErrQueueFull, providing backpressure.")

	// Shutdown
	shutdownAndWait(t, []context.CancelFunc{cancel}, &runningWg, 5*time.Second)
}
