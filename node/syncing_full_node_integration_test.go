package node

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
)

// TestTxGossipingAndAggregation tests that transactions are gossiped and blocks are aggregated and synced across multiple nodes.
// It creates 4 nodes (1 aggregator, 3 full nodes), injects a transaction, waits for all nodes to sync, and asserts block equality.
func TestTxGossipingAndAggregation(t *testing.T) {
	config := getTestConfig(t, 1)

	numNodes := 4
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs := make([]context.Context, numNodes)
	cancelFuncs := make([]context.CancelFunc, numNodes)
	var runningWg sync.WaitGroup

	// Create a context and cancel function for each node
	for i := 0; i < numNodes; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		cancelFuncs[i] = cancel
	}

	// Start only nodes[0] (aggregator) first
	runningWg.Add(1)
	go func(node *FullNode, ctx context.Context) {
		defer runningWg.Done()
		err := node.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error running node 0: %v", err)
		}
	}(nodes[0], ctxs[0])

	// Wait for the first block to be produced by the aggregator
	err := waitForFirstBlock(nodes[0], Header)
	require.NoError(t, err, "Failed to get node height")

	// Verify block manager is properly initialized
	require.NotNil(t, nodes[0].blockManager, "Block manager should be initialized")

	// Now start the other nodes
	for i := 1; i < numNodes; i++ {
		runningWg.Add(1)
		go func(node *FullNode, ctx context.Context, idx int) {
			defer runningWg.Done()
			err := node.Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("Error running node %d: %v", idx, err)
			}
		}(nodes[i], ctxs[i], i)
	}

	// Inject a transaction into the aggregator's executor
	executor := nodes[0].blockManager.GetExecutor().(*coreexecutor.DummyExecutor)
	executor.InjectTx([]byte("gossip tx"))

	blocksToWaitFor := uint64(5)
	// Wait for all nodes to reach at least 5 blocks
	for _, node := range nodes {
		require.NoError(t, waitForAtLeastNBlocks(node, blocksToWaitFor, Store))
	}

	// Cancel all node contexts to signal shutdown
	for _, cancel := range cancelFuncs {
		cancel()
	}

	// Wait for all nodes to stop, with a timeout
	waitCh := make(chan struct{})
	go func() {
		runningWg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Nodes stopped successfully
	case <-time.After(5 * time.Second):
		t.Log("Warning: Not all nodes stopped gracefully within timeout")
	}

	// Assert that all nodes have the same block up to height blocksToWaitFor
	for height := uint64(1); height <= blocksToWaitFor; height++ {
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

// TestMaxPendingHeaders verifies that the node will stop producing blocks when the maximum number of pending headers is reached.
// It reconfigures the node with a low max pending value, waits for block production, and checks the pending block count.
func TestMaxPendingHeaders(t *testing.T) {
	require := require.New(t)

	// Reconfigure node with low max pending
	config := getTestConfig(t, 1)
	config.Node.MaxPendingHeaders = 2

	// Set DA block time large enough to avoid header submission to DA layer
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 20 * time.Second}

	node, cleanup := createNodeWithCleanup(t, config)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runningWg sync.WaitGroup
	runningWg.Add(1)
	go func() {
		defer runningWg.Done()
		err := node.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error running node: %v", err)
		}
	}()

	// Wait blocks to be produced up to max pending
	time.Sleep(time.Duration(config.Node.MaxPendingHeaders+5) * config.Node.BlockTime.Duration)

	// Verify that number of pending blocks doesn't exceed max
	height, err := getNodeHeight(node, Store)
	require.NoError(err)
	require.LessOrEqual(height, config.Node.MaxPendingHeaders)

	// Stop the node and wait for shutdown
	cancel()
	waitCh := make(chan struct{})
	go func() {
		runningWg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Node stopped successfully
	case <-time.After(5 * time.Second):
		t.Log("Warning: Node did not stop gracefully within timeout")
	}
}

// TestFastDASync verifies that a new node can quickly synchronize with the Data Availability (DA) layer using fast sync.
//
// This test sets up two nodes with different block and DA block times. It starts the first node, waits for it to produce and DA-include several blocks,
// then starts the second node and measures how quickly it can catch up to the first node's height. The test asserts that the second node syncs within
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

	ctxs := make([]context.Context, len(nodes))
	cancelFuncs := make([]context.CancelFunc, len(nodes))
	var runningWg sync.WaitGroup

	// Create a context and cancel function for each node
	for i := 0; i < len(nodes); i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		cancelFuncs[i] = cancel
	}

	// Start only the first node
	runningWg.Add(1)
	go func(node *FullNode, ctx context.Context) {
		defer runningWg.Done()
		err := node.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error running node 0: %v", err)
		}
	}(nodes[0], ctxs[0])

	// Wait for the first node to produce a few blocks
	blocksToWaitFor := uint64(5)
	require.NoError(waitForAtLeastNDAIncludedHeight(nodes[0], blocksToWaitFor))

	// Now start the second node and time its sync
	runningWg.Add(1)
	go func(node *FullNode, ctx context.Context) {
		defer runningWg.Done()
		err := node.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error running node 1: %v", err)
		}
	}(nodes[1], ctxs[1])
	start := time.Now()
	// Wait for the second node to catch up to the first node
	require.NoError(waitForAtLeastNBlocks(nodes[1], blocksToWaitFor, Store))
	syncDuration := time.Since(start)

	// Ensure node syncs within a small delta of DA block time
	delta := 75 * time.Millisecond
	require.Less(syncDuration, config.DA.BlockTime.Duration+delta, "Block sync should be faster than DA block time")

	// Verify both nodes are synced and that the synced block is DA-included
	for height := uint64(1); height <= blocksToWaitFor; height++ {
		// Both nodes should have the same block hash
		header0, _, err := nodes[0].Store.GetBlockData(context.Background(), height)
		require.NoError(err)
		header1, _, err := nodes[1].Store.GetBlockData(context.Background(), height)
		require.NoError(err)
		require.EqualValues(header0.Hash(), header1.Hash(), "Block hash mismatch at height %d", height)
	}

	// Cancel all node contexts to signal shutdown
	for _, cancel := range cancelFuncs {
		cancel()
	}

	// Wait for all nodes to stop, with a timeout
	waitCh := make(chan struct{})
	go func() {
		runningWg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Nodes stopped successfully
	case <-time.After(5 * time.Second):
		t.Log("Warning: Not all nodes stopped gracefully within timeout")
	}
}

// TestSingleAggregatorTwoFullNodesBlockSyncSpeed verifies block sync speed when block time is much faster than DA block time.

/*
TODO:

	Details:
	Sets up three nodes with fast block time and slow DA block time.
	Starts all nodes and waits for them to sync a set number of blocks.
	Uses a timer to ensure the test completes within the block time.
	Verifies all nodes are synced.
	Goal: Ensures block sync is not bottlenecked by DA block time.
*/
func TestSingleAggregatorTwoFullNodesBlockSyncSpeed(t *testing.T) {
	require := require.New(t)

	// Set up three nodes: 1 aggregator, 2 full nodes
	config := getTestConfig(t, 1)
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond} // fast block time
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 10 * time.Second}         // slow DA block time

	numNodes := 3
	nodes, cleanups := createNodesWithCleanup(t, numNodes, config)
	for _, cleanup := range cleanups {
		defer cleanup()
	}

	ctxs := make([]context.Context, numNodes)
	cancelFuncs := make([]context.CancelFunc, numNodes)
	var runningWg sync.WaitGroup

	for i := 0; i < numNodes; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ctxs[i] = ctx
		cancelFuncs[i] = cancel
	}

	// Start only the first node (aggregator) first
	runningWg.Add(1)
	go func(node *FullNode, ctx context.Context) {
		defer runningWg.Done()
		err := node.Run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error running node 0: %v", err)
		}
	}(nodes[0], ctxs[0])

	// Wait for the first node to produce at least one block
	require.NoError(waitForAtLeastNBlocks(nodes[0], 1, Store))

	// Now start the other nodes
	for i := 1; i < numNodes; i++ {
		runningWg.Add(1)
		go func(node *FullNode, ctx context.Context, idx int) {
			defer runningWg.Done()
			err := node.Run(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Logf("Error running node %d: %v", idx, err)
			}
		}(nodes[i], ctxs[i], i)
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
	for height := uint64(1); height <= blocksToWaitFor; height++ {
		var refHash []byte
		for i, node := range nodes {
			header, _, err := node.Store.GetBlockData(context.Background(), height)
			require.NoError(err)
			if i == 0 {
				refHash = header.Hash()
			} else {
				headerHash := header.Hash()
				require.EqualValues(refHash, headerHash, "Block hash mismatch at height %d between node 0 and node %d", height, i)
			}
		}
	}

	// Cancel all node contexts to signal shutdown
	for _, cancel := range cancelFuncs {
		cancel()
	}

	// Wait for all nodes to stop, with a timeout
	waitCh := make(chan struct{})
	go func() {
		runningWg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Nodes stopped successfully
	case <-time.After(5 * time.Second):
		t.Log("Warning: Not all nodes stopped gracefully within timeout")
	}
}

// TestSingleAggregatorTwoFullNodesDAInclusion verifies DA inclusion when block time is much faster than DA block time.

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
