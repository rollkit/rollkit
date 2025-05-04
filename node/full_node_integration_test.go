package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
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

func (s *FullNodeTestSuite) SetupTest() {
	require := require.New(s.T())
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.errCh = make(chan error, 1)

	// Setup a test node
	config := getTestConfig(s.T(), 1)

	// Add debug logging for configuration
	s.T().Logf("Test configuration: BlockTime=%v, DABlockTime=%v, MaxPendingBlocks=%d",
		config.Node.BlockTime.Duration, config.DA.BlockTime.Duration, config.Node.MaxPendingBlocks)

	node, cleanup := setupTestNodeWithCleanup(s.T(), config)
	defer cleanup()

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

// TestFullNodeTestSuite runs the test suite
func TestFullNodeTestSuite(t *testing.T) {
	suite.Run(t, new(FullNodeTestSuite))
}

func (s *FullNodeTestSuite) TestBlockProduction() {
	s.executor.InjectTx([]byte("test transaction"))
	err := waitForAtLeastNBlocks(s.node, 5, Store)
	s.NoError(err, "Failed to produce second block")

	// Get the current height
	height, err := s.node.Store.Height(s.ctx)
	require.NoError(s.T(), err)
	s.GreaterOrEqual(height, uint64(5), "Expected block height >= 5")

	// Get all blocks and log their contents
	for h := uint64(1); h <= height; h++ {
		header, data, err := s.node.Store.GetBlockData(s.ctx, h)
		s.NoError(err)
		s.NotNil(header)
		s.NotNil(data)
		s.T().Logf("Block height: %d, Time: %s, Number of transactions: %d", h, header.Time(), len(data.Txs))
	}

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
	s.Equal(height, state.LastBlockHeight)

	// Verify block content
	s.NotEmpty(data.Txs, "Expected block to contain transactions")
}

// TestSubmitBlocksToDA tests the submission of blocks to the DA
func (s *FullNodeTestSuite) TestSubmitBlocksToDA() {
	require := require.New(s.T())

	// Get initial state
	initialHeight, err := getNodeHeight(s.node, Header)
	require.NoError(err)

	// Check if block manager is properly initialized
	s.T().Log("=== Block Manager State ===")
	pendingHeaders, err := s.node.blockManager.PendingHeaders().GetPendingHeaders()
	require.NoError(err)
	s.T().Logf("Initial Pending Headers: %d", len(pendingHeaders))
	s.T().Logf("Last Submitted Height: %d", s.node.blockManager.PendingHeaders().GetLastSubmittedHeight())

	// Verify sequencer is working
	s.T().Log("=== Sequencer Check ===")
	require.NotNil(s.node.blockManager.SeqClient(), "Sequencer client should be initialized")

	s.executor.InjectTx([]byte("dummy transaction"))

	// Monitor batch retrieval
	s.T().Log("=== Monitoring Batch Retrieval ===")
	err = waitForAtLeastNBlocks(s.node, 5, Store)
	require.NoError(err, "Failed to produce additional blocks")

	// Final assertions with more detailed error messages
	finalHeight, err := s.node.Store.Height(s.ctx)
	require.NoError(err)

	require.Greater(finalHeight, initialHeight, "Block height should have increased")
}

func (s *FullNodeTestSuite) TestDAInclusion() {
	s.T().Skip("skipping DA inclusion test")
	// this test currently thinks DAIncludedHeight is returning a DA height, but it's actually returning a block height
	require := require.New(s.T())

	// Get initial height and DA height
	initialHeight, err := getNodeHeight(s.node, Header)
	require.NoError(err, "Failed to get initial height")
	initialDAHeight := s.node.blockManager.GetDAIncludedHeight()

	s.T().Logf("=== Initial State ===")
	s.T().Logf("Block height: %d, DA height: %d", initialHeight, initialDAHeight)
	s.T().Logf("Aggregator enabled: %v", s.node.nodeConfig.Node.Aggregator)

	s.executor.InjectTx([]byte("dummy transaction"))

	// Monitor state changes in shorter intervals
	s.T().Log("=== Monitoring State Changes ===")
	for i := range 10 {
		time.Sleep(200 * time.Millisecond)
		currentHeight, err := s.node.Store.Height(s.ctx)
		require.NoError(err)
		currentDAHeight := s.node.blockManager.GetDAIncludedHeight()
		pendingHeaders, _ := s.node.blockManager.PendingHeaders().GetPendingHeaders()
		lastSubmittedHeight := s.node.blockManager.PendingHeaders().GetLastSubmittedHeight()

		s.T().Logf("Iteration %d:", i)
		s.T().Logf("  - Height: %d", currentHeight)
		s.T().Logf("  - DA Height: %d", currentDAHeight)
		s.T().Logf("  - Pending Headers: %d", len(pendingHeaders))
		s.T().Logf("  - Last Submitted Height: %d", lastSubmittedHeight)
	}

	s.T().Log("=== Checking DA Height Increase ===")
	// Use shorter retry period with more frequent checks
	var finalDAHeight uint64
	err = testutils.Retry(30, 200*time.Millisecond, func() error {
		currentDAHeight := s.node.blockManager.GetDAIncludedHeight()
		currentHeight, err := s.node.Store.Height(s.ctx)
		require.NoError(err)
		pendingHeaders, _ := s.node.blockManager.PendingHeaders().GetPendingHeaders()

		s.T().Logf("Retry check - DA Height: %d, Block Height: %d, Pending: %d",
			currentDAHeight, currentHeight, len(pendingHeaders))

		if currentDAHeight <= initialDAHeight {
			return fmt.Errorf("waiting for DA height to increase from %d (current: %d)",
				initialDAHeight, currentDAHeight)
		}
		finalDAHeight = currentDAHeight
		return nil
	})
	require.NoError(err, "DA height did not increase")

	// Final state logging
	s.T().Log("=== Final State ===")
	finalHeight, err := s.node.Store.Height(s.ctx)
	require.NoError(err)
	pendingHeaders, _ := s.node.blockManager.PendingHeaders().GetPendingHeaders()
	s.T().Logf("Final Height: %d", finalHeight)
	s.T().Logf("Final DA Height: %d", finalDAHeight)
	s.T().Logf("Final Pending Headers: %d", len(pendingHeaders))

	// Assertions
	require.NoError(err, "DA height did not increase")
	require.Greater(finalHeight, initialHeight, "Block height should increase")
	require.Greater(finalDAHeight, initialDAHeight, "DA height should increase")
}

// TestMaxPending tests that the node will stop producing blocks when the limit is reached
func (s *FullNodeTestSuite) TestMaxPending() {
	require := require.New(s.T())

	// First, stop the current node by cancelling its context
	s.cancel()

	// Create a new context for the new node
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Reset error channel
	s.errCh = make(chan error, 1)

	// Reconfigure node with low max pending
	config := getTestConfig(s.T(), 1)
	config.Node.MaxPendingBlocks = 2

	node, cleanup := setupTestNodeWithCleanup(s.T(), config)
	defer cleanup()

	s.node = node

	// Start the node using Run in a goroutine
	s.startNodeInBackground(s.node)

	// Wait blocks to be produced up to max pending
	time.Sleep(time.Duration(config.Node.MaxPendingBlocks+1) * config.Node.BlockTime.Duration)

	// Verify that number of pending blocks doesn't exceed max
	height, err := getNodeHeight(s.node, Header)
	require.NoError(err)
	require.LessOrEqual(height, config.Node.MaxPendingBlocks)
}

func (s *FullNodeTestSuite) TestGenesisInitialization() {
	require := require.New(s.T())

	// Verify genesis state
	state := s.node.blockManager.GetLastState()
	require.Equal(s.node.genesis.InitialHeight, state.InitialHeight)
	require.Equal(s.node.genesis.ChainID, state.ChainID)
}

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
	node, cleanup := setupTestNodeWithCleanup(s.T(), config)
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
