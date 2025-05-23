package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	coreexecutor "github.com/rollkit/rollkit/core/execution"

	testutils "github.com/celestiaorg/utils/test"
)

// FullNodeTestSuite is a test suite for full node integration tests
type FullNodeTestSuite struct {
	suite.Suite
	ctx       context.Context
	cancel    context.CancelFunc
	node      *FullNode
	executor  *coreexecutor.DummyExecutor
	errCh     chan error
	runningWg sync.WaitGroup
}

// startNodeInBackground starts the given node in a background goroutine
// and adds to the wait group for proper cleanup
func (s *FullNodeTestSuite) startNodeInBackground(node *FullNode) {
	s.runningWg.Add(1)
	go func() {
		defer s.runningWg.Done()
		err := node.Run(s.ctx)
		select {
		case s.errCh <- err:
		default:
			s.T().Logf("Error channel full, discarding error: %v", err)
		}
	}()
}

// SetupTest initializes the test context, creates a test node, and starts it in the background.
// It also verifies that the node is running, producing blocks, and properly initialized.
func (s *FullNodeTestSuite) SetupTest() {
	require := require.New(s.T())
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.errCh = make(chan error, 1)

	// Setup a test node
	config := getTestConfig(s.T(), 1)

	// Add debug logging for configuration
	s.T().Logf("Test configuration: BlockTime=%v, DABlockTime=%v, MaxPendingHeaders=%d",
		config.Node.BlockTime.Duration, config.DA.BlockTime.Duration, config.Node.MaxPendingHeaders)

	node, cleanup := createNodeWithCleanup(s.T(), config)
	s.T().Cleanup(func() {
		cleanup()
	})

	s.node = node

	s.executor = node.blockManager.GetExecutor().(*coreexecutor.DummyExecutor)

	// Start the node in a goroutine using Run instead of Start
	s.startNodeInBackground(s.node)

	// Verify that the node is running and producing blocks
	err := waitForFirstBlock(s.node, Header)
	require.NoError(err, "Failed to get node height")

	// Wait for the first block to be DA included
	err = waitForFirstBlockToBeDAIncludedHeight(s.node)
	require.NoError(err, "Failed to get DA inclusion")

	// Verify sequencer client is working
	err = testutils.Retry(30, 100*time.Millisecond, func() error {
		if s.node.blockManager.SeqClient() == nil {
			return fmt.Errorf("sequencer client not initialized")
		}
		return nil
	})
	require.NoError(err, "Sequencer client initialization failed")

	// Verify block manager is properly initialized
	require.NotNil(s.node.blockManager, "Block manager should be initialized")
}

// TearDownTest cancels the test context and waits for the node to stop, ensuring proper cleanup after each test.
// It also checks for any errors that occurred during node shutdown.
func (s *FullNodeTestSuite) TearDownTest() {
	if s.cancel != nil {
		s.cancel() // Cancel context to stop the node

		// Wait for the node to stop with a timeout
		waitCh := make(chan struct{})
		go func() {
			s.runningWg.Wait()
			close(waitCh)
		}()

		select {
		case <-waitCh:
			// Node stopped successfully
		case <-time.After(5 * time.Second):
			s.T().Log("Warning: Node did not stop gracefully within timeout")
		}

		// Check for any errors
		select {
		case err := <-s.errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				s.T().Logf("Error stopping node in teardown: %v", err)
			}
		default:
			// No error
		}
	}
}

// TestFullNodeTestSuite runs the FullNodeTestSuite using testify's suite runner.
// This is the entry point for running all integration tests in this suite.
func TestFullNodeTestSuite(t *testing.T) {
	suite.Run(t, new(FullNodeTestSuite))
}

// TestBlockProduction verifies block production and state after injecting a transaction.
// It checks that blocks are produced, state is updated, and transactions are included in blocks.
func (s *FullNodeTestSuite) TestBlockProduction() {
	s.executor.InjectTx([]byte("test transaction"))
	err := waitForAtLeastNBlocks(s.node, 5, Store)
	s.NoError(err, "Failed to produce second block")

	// Get the current height
	height, err := s.node.Store.Height(s.ctx)
	require.NoError(s.T(), err)
	s.GreaterOrEqual(height, uint64(5), "Expected block height >= 5")

	// Get the latest block
	header, data, err := s.node.Store.GetBlockData(s.ctx, height)
	s.NoError(err)
	s.NotNil(header)
	s.NotNil(data)

	// Log block details
	s.T().Logf("Latest block height: %d, Time: %s, Number of transactions: %d", height, header.Time(), len(data.Txs))

	// Verify chain state
	state, err := s.node.Store.GetState(s.ctx)
	s.NoError(err)
	s.GreaterOrEqual(height, state.LastBlockHeight)

	// Verify block content
	s.NotEmpty(data.Txs, "Expected block to contain transactions")
}

// TestSubmitBlocksToDA verifies that blocks produced by the node are properly submitted to the Data Availability (DA) layer.
// It injects a transaction, waits for several blocks to be produced and DA-included, and asserts that all blocks are DA included.
func (s *FullNodeTestSuite) TestSubmitBlocksToDA() {
	s.executor.InjectTx([]byte("test transaction"))
	n := uint64(5)
	err := waitForAtLeastNBlocks(s.node, n, Store)
	s.NoError(err, "Failed to produce second block")
	err = waitForAtLeastNDAIncludedHeight(s.node, n)
	s.NoError(err, "Failed to get DA inclusion")
	// Verify that all blocks are DA included
	for height := uint64(1); height <= n; height++ {
		ok, err := s.node.blockManager.IsDAIncluded(s.ctx, height)
		require.NoError(s.T(), err)
		require.True(s.T(), ok, "Block at height %d is not DA included", height)
	}
}

// TestTxGossipingAndAggregation tests that transactions are gossiped and blocks are aggregated and synced across multiple nodes.
// It creates 4 nodes (1 aggregator, 3 full nodes), injects a transaction, waits for all nodes to sync, and asserts block equality.
func (s *FullNodeTestSuite) TestTxGossipingAndAggregation() {
	// First, stop the current node by cancelling its context
	s.cancel()

	// Create a new context for the new node
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Reset error channel
	s.errCh = make(chan error, 1)

	require := require.New(s.T())
	config := getTestConfig(s.T(), 1)

	numNodes := 2
	nodes, cleanups := createNodesWithCleanup(s.T(), numNodes, config)
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()

	s.node = nodes[0]

	// Start all nodes in background
	for _, node := range nodes {
		s.startNodeInBackground(node)
	}

	// Inject a transaction into the aggregator's executor
	executor := nodes[0].blockManager.GetExecutor().(*coreexecutor.DummyExecutor)
	executor.InjectTx([]byte("gossip tx"))

	// Wait for all nodes to reach at least 3 blocks
	for _, node := range nodes {
		require.NoError(waitForAtLeastNBlocks(node, 3, Store))
	}

	// Assert that all nodes have the same block at height 1 and 2
	for height := uint64(1); height <= 2; height++ {
		var refHash []byte
		for i, node := range nodes {
			header, _, err := node.Store.GetBlockData(context.Background(), height)
			require.NoError(err)
			if i == 0 {
				refHash = header.Hash()
			} else {
				s.Equal(refHash, header.Hash(), "Block hash mismatch at height %d between node 0 and node %d", height, i)
			}
		}
	}
}

// TestMaxPendingHeaders verifies that the node will stop producing blocks when the maximum number of pending headers is reached.
// It reconfigures the node with a low max pending value, waits for block production, and checks the pending block count.
func (s *FullNodeTestSuite) TestMaxPendingHeaders() {
	require := require.New(s.T())

	// First, stop the current node by cancelling its context
	s.cancel()

	// Create a new context for the new node
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Reset error channel
	s.errCh = make(chan error, 1)

	// Reconfigure node with low max pending
	config := getTestConfig(s.T(), 1)
	config.Node.MaxPendingHeaders = 2

	node, cleanup := createNodeWithCleanup(s.T(), config)
	defer cleanup()

	s.node = node

	// Start the node using Run in a goroutine
	s.startNodeInBackground(s.node)

	// Wait blocks to be produced up to max pending
	time.Sleep(time.Duration(config.Node.MaxPendingHeaders+1) * config.Node.BlockTime.Duration)

	// Verify that number of pending blocks doesn't exceed max
	height, err := getNodeHeight(s.node, Header)
	require.NoError(err)
	require.LessOrEqual(height, config.Node.MaxPendingHeaders)
}

// TestGenesisInitialization checks that the node's state is correctly initialized from the genesis document.
// It asserts that the initial height and chain ID in the state match those in the genesis.
func (s *FullNodeTestSuite) TestGenesisInitialization() {
	require := require.New(s.T())

	// Verify genesis state
	state := s.node.blockManager.GetLastState()
	require.Equal(s.node.genesis.InitialHeight, state.InitialHeight)
	require.Equal(s.node.genesis.ChainID, state.ChainID)
}

// TestStateRecovery (skipped) is intended to verify that the node can recover its state after a restart.
// It would check that the block height after restart is at least as high as before, but is skipped due to in-memory DB.
func (s *FullNodeTestSuite) TestStateRecovery() {
	s.T().Skip("skipping state recovery test, we need to reuse the same database, when we use in memory it starts fresh each time")
	require := require.New(s.T())

	// Get current state
	originalHeight, err := getNodeHeight(s.node, Store)
	require.NoError(err)

	// Wait for some blocks
	err = waitForAtLeastNBlocks(s.node, 5, Store)
	require.NoError(err)

	// Stop the current node
	s.cancel()

	// Wait for the node to stop
	waitCh := make(chan struct{})
	go func() {
		s.runningWg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Node stopped successfully
	case <-time.After(2 * time.Second):
		s.T().Fatalf("Node did not stop gracefully within timeout")
	}

	// Create a new context
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.errCh = make(chan error, 1)

	config := getTestConfig(s.T(), 1)
	// Create a new node instance instead of reusing the old one
	node, cleanup := createNodeWithCleanup(s.T(), config)
	defer cleanup()

	// Replace the old node with the new one
	s.node = node

	// Start the new node
	s.startNodeInBackground(s.node)

	// Wait a bit after restart
	time.Sleep(s.node.nodeConfig.Node.BlockTime.Duration)

	// Verify state persistence
	recoveredHeight, err := getNodeHeight(s.node, Store)
	require.NoError(err)
	require.GreaterOrEqual(recoveredHeight, originalHeight)
}

// TestFastDASync verifies that the node can sync with the DA layer using fast sync.
// It creates a new node, injects a transaction, waits for it to be DA-included, and asserts that the node is in sync.

/*
TODO:

	Details:
	Sets up two nodes with different block and DA block times.
	Starts one node, waits for it to sync several blocks, then starts the second node.
	Uses a timer to ensure the second node syncs quickly.
	Verifies both nodes are synced and that the synced block is DA-included.
	Goal: Ensures block sync is faster than DA block time and DA inclusion is verified.
*/
func (s *FullNodeTestSuite) TestFastDASync() {
	s.T().Skip()
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
func (s *FullNodeTestSuite) TestSingleAggregatorTwoFullNodesBlockSyncSpeed() {
	s.T().Skip()
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
