//go:build integration

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

	coreda "github.com/evstack/ev-node/core/da"
	coreexecutor "github.com/evstack/ev-node/core/execution"
	rollkitconfig "github.com/evstack/ev-node/pkg/config"

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
	s.T().Logf("Test configuration: BlockTime=%v, DABlockTime=%v, MaxPendingHeadersAndData=%d",
		config.Node.BlockTime.Duration, config.DA.BlockTime.Duration, config.Node.MaxPendingHeadersAndData)

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
	err = waitForFirstBlockToBeDAIncluded(s.node)
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
// It checks that blocks are produced, state is updated, and the injected transaction is included in one of the blocks.
func (s *FullNodeTestSuite) TestBlockProduction() {
	testTx := []byte("test transaction")
	s.executor.InjectTx(testTx)
	err := waitForAtLeastNBlocks(s.node, 5, Store)
	s.NoError(err, "Failed to produce more than 5 blocks")

	// Get the current height
	height, err := s.node.Store.Height(s.ctx)
	require.NoError(s.T(), err)
	s.GreaterOrEqual(height, uint64(5), "Expected block height >= 5")

	// Verify chain state
	state, err := s.node.Store.GetState(s.ctx)
	s.NoError(err)
	s.GreaterOrEqual(height, state.LastBlockHeight)

	foundTx := false
	for h := uint64(1); h <= height; h++ {
		// Get the block data for each height
		header, data, err := s.node.Store.GetBlockData(s.ctx, h)
		s.NoError(err)
		s.NotNil(header)
		s.NotNil(data)

		// Log block details
		s.T().Logf("Block height: %d, Time: %s, Number of transactions: %d", h, header.Time(), len(data.Txs))

		// Check if testTx is in this block
		for _, tx := range data.Txs {
			if string(tx) == string(testTx) {
				foundTx = true
				break
			}
		}
	}

	// Verify at least one block contains the test transaction
	s.True(foundTx, "Expected at least one block to contain the test transaction")
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

// TestGenesisInitialization checks that the node's state is correctly initialized from the genesis document.
// It asserts that the initial height and chain ID in the state match those in the genesis.
func (s *FullNodeTestSuite) TestGenesisInitialization() {
	require := require.New(s.T())

	// Verify genesis state
	state := s.node.blockManager.GetLastState()
	require.Equal(s.node.genesis.InitialHeight, state.InitialHeight)
	require.Equal(s.node.genesis.ChainID, state.ChainID)
}

// TestStateRecovery verifies that the node can recover its state after a restart.
// It would check that the block height after restart is at least as high as before.
func TestStateRecovery(t *testing.T) {

	require := require.New(t)

	// Set up one sequencer
	config := getTestConfig(t, 1)
	executor, sequencer, dac, p2pClient, ds, _, stopDAHeightTicker := createTestComponents(t, config)
	node, cleanup := createNodeWithCustomComponents(t, config, executor, sequencer, dac, p2pClient, ds, stopDAHeightTicker)
	defer cleanup()

	var runningWg sync.WaitGroup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the sequencer first
	startNodeInBackground(t, []*FullNode{node}, []context.Context{ctx}, &runningWg, 0)

	blocksToWaitFor := uint64(20)
	// Wait for the sequencer to produce at first block
	require.NoError(waitForAtLeastNBlocks(node, blocksToWaitFor, Store))

	// Get current state
	originalHeight, err := getNodeHeight(node, Store)
	require.NoError(err)
	require.GreaterOrEqual(originalHeight, blocksToWaitFor)

	// Stop the current node
	cancel()

	// Wait for the node to stop
	waitCh := make(chan struct{})
	go func() {
		runningWg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Node stopped successfully
	case <-time.After(2 * time.Second):
		t.Fatalf("Node did not stop gracefully within timeout")
	}

	// Create a new node instance using the same components
	executor, sequencer, dac, p2pClient, _, _, stopDAHeightTicker = createTestComponents(t, config)
	node, cleanup = createNodeWithCustomComponents(t, config, executor, sequencer, dac, p2pClient, ds, stopDAHeightTicker)
	defer cleanup()

	// Verify state persistence
	recoveredHeight, err := getNodeHeight(node, Store)
	require.NoError(err)
	require.GreaterOrEqual(recoveredHeight, originalHeight)
}

// TestMaxPendingHeadersAndData verifies that the sequencer will stop producing blocks when the maximum number of pending headers or data is reached.
// It reconfigures the node with a low max pending value, waits for block production, and checks the pending block count.
func TestMaxPendingHeadersAndData(t *testing.T) {
	require := require.New(t)
	// Reconfigure node with low max pending
	config := getTestConfig(t, 1)
	config.Node.MaxPendingHeadersAndData = 2

	// Set DA block time large enough to avoid header submission to DA layer
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Second}

	node, cleanup := createNodeWithCleanup(t, config)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runningWg sync.WaitGroup
	startNodeInBackground(t, []*FullNode{node}, []context.Context{ctx}, &runningWg, 0)

	// Wait blocks to be produced up to max pending
	numExtraBlocks := uint64(5)
	time.Sleep(time.Duration(config.Node.MaxPendingHeadersAndData+numExtraBlocks) * config.Node.BlockTime.Duration)

	// Verify that the node is not producing blocks beyond the max pending limit
	height, err := getNodeHeight(node, Store)
	require.NoError(err)
	require.LessOrEqual(height, config.Node.MaxPendingHeadersAndData)

	// Stop the node and wait for shutdown
	shutdownAndWait(t, []context.CancelFunc{cancel}, &runningWg, 5*time.Second)
}

// TestBatchQueueThrottlingWithDAFailure tests that when DA layer fails and MaxPendingHeadersAndData
// is reached, the system behaves correctly and doesn't run into resource exhaustion.
// This test uses the dummy sequencer but demonstrates the scenario that would occur
// with a real single sequencer having queue limits.
func TestBatchQueueThrottlingWithDAFailure(t *testing.T) {
	require := require.New(t)

	// Set up configuration with low limits to trigger throttling quickly
	config := getTestConfig(t, 1)
	config.Node.MaxPendingHeadersAndData = 3 // Low limit to quickly reach pending limit after DA failure
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond}
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 1 * time.Second} // Longer DA time to ensure blocks are produced first

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

	// Wait for at least 5 blocks to be produced before simulating DA failure
	require.NoError(waitForAtLeastNBlocks(node, 5, Store))
	t.Log("Initial 5 blocks produced successfully")

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
	// Allow at most 3 additional blocks due to timing and pending blocks in queue
	heightIncrease := finalHeight - heightAfterDAFailure
	require.LessOrEqual(heightIncrease, uint64(3),
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
