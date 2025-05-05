package node

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	testutils "github.com/celestiaorg/utils/test"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/rollkit/rollkit/block"
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
	s.executor.InjectTx([]byte("test transaction"))
	n := uint64(5)
	err := waitForAtLeastNBlocks(s.node, n, Store)
	s.NoError(err, "Failed to produce second block")
	waitForAtLeastNDAIncludedHeight(s.node, n)

	// Verify that all blocks are DA included
	for height := uint64(1); height <= n; height++ {
		header, data, err := s.node.Store.GetBlockData(s.ctx, height)
		require.NoError(s.T(), err)

		headerHash := header.Hash()
		dataHash := data.DACommitment()

		isHeaderDAIncluded := s.node.blockManager.HeaderCache().IsDAIncluded(headerHash.String())
		isDataDAIncluded := s.node.blockManager.DataCache().IsDAIncluded(dataHash.String()) || bytes.Equal(dataHash, block.DataHashForEmptyTxs)

		require.True(s.T(), isHeaderDAIncluded, "Header at height %d is not DA included", height)
		require.True(s.T(), isDataDAIncluded, "Data at height %d is not DA included", height)
	}
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
