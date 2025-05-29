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
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"

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
	s.GreaterOrEqual(state.LastBlockHeight, height)

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

// TestMaxPendingHeaders verifies that the sequencer will stop producing blocks when the maximum number of pending headers is reached.
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
	startNodeInBackground(t, []*FullNode{node}, []context.Context{ctx}, &runningWg, 0)

	// Wait blocks to be produced up to max pending
	numExtraHeaders := uint64(5)
	time.Sleep(time.Duration(config.Node.MaxPendingHeaders+numExtraHeaders) * config.Node.BlockTime.Duration)

	// Verify that number of pending blocks doesn't exceed max
	height, err := getNodeHeight(node, Store)
	require.NoError(err)
	require.LessOrEqual(height, config.Node.MaxPendingHeaders)

	// Stop the node and wait for shutdown
	shutdownAndWait(t, []context.CancelFunc{cancel}, &runningWg, 5*time.Second)
}
