package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	testutils "github.com/celestiaorg/utils/test"
	cmcfg "github.com/cometbft/cometbft/config"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
)

// FullNodeTestSuite is a test suite for full node integration tests
type FullNodeTestSuite struct {
	suite.Suite
	ctx       context.Context
	cancel    context.CancelFunc
	node      *FullNode
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
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.errCh = make(chan error, 1)

	// Setup node with proper configuration
	config := getTestConfig(1)
	config.BlockTime = 100 * time.Millisecond        // Faster block production for tests
	config.DABlockTime = 200 * time.Millisecond      // Faster DA submission for tests
	config.BlockManagerConfig.MaxPendingBlocks = 100 // Allow more pending blocks
	config.Aggregator = true                         // Enable aggregator mode

	// Add debug logging for configuration
	s.T().Logf("Test configuration: BlockTime=%v, DABlockTime=%v, MaxPendingBlocks=%d",
		config.BlockTime, config.DABlockTime, config.BlockManagerConfig.MaxPendingBlocks)

	// Create genesis with current time
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	genesis.GenesisTime = time.Now() // Set genesis time to now
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(s.T(), err)

	p2pKey := generateSingleKey()

	dummyDA := coreda.NewDummyDA(100_000)
	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()

	node, err := NewNode(
		s.ctx,
		config,
		dummyExec,
		dummySequencer,
		dummyDA,
		p2pKey,
		signingKey,
		genesis,
		DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()),
		log.NewTestLogger(s.T()),
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), node)

	fn, ok := node.(*FullNode)
	require.True(s.T(), ok)

	s.node = fn

	// Start the node in a goroutine using Run instead of Start
	s.startNodeInBackground(s.node)

	// Wait for the node to start and initialize DA connection
	time.Sleep(2 * time.Second)

	// Verify that the node is running and producing blocks
	height, err := getNodeHeight(s.node, Header)
	require.NoError(s.T(), err, "Failed to get node height")
	require.Greater(s.T(), height, uint64(0), "Node should have produced at least one block")

	// Wait for DA inclusion with retry
	err = testutils.Retry(30, 100*time.Millisecond, func() error {
		daHeight := s.node.blockManager.GetDAIncludedHeight()
		if daHeight == 0 {
			return fmt.Errorf("waiting for DA inclusion")
		}
		return nil
	})
	require.NoError(s.T(), err, "Failed to get DA inclusion")

	// Wait for additional blocks to be produced
	time.Sleep(500 * time.Millisecond)

	// Additional debug info after node start
	initialHeight := s.node.Store.Height()
	s.T().Logf("Node started - Initial block height: %d", initialHeight)
	s.T().Logf("DA client initialized: %v", s.node.blockManager.DALCInitialized())

	// Wait longer for height to stabilize and log intermediate values
	for i := 0; i < 5; i++ {
		time.Sleep(200 * time.Millisecond)
		currentHeight := s.node.Store.Height()
		s.T().Logf("Current height during stabilization: %d", currentHeight)
	}

	// Get final height after stabilization period
	finalHeight := s.node.Store.Height()
	s.T().Logf("Final setup height: %d", finalHeight)

	// Store the stable height for test use
	s.node.blockManager.SetLastState(s.node.blockManager.GetLastState())

	// Log additional state information
	s.T().Logf("Last submitted height: %d", s.node.blockManager.PendingHeaders().GetLastSubmittedHeight())
	s.T().Logf("DA included height: %d", s.node.blockManager.GetDAIncludedHeight())

	// Verify sequencer client is working
	err = testutils.Retry(30, 100*time.Millisecond, func() error {
		if s.node.blockManager.SeqClient() == nil {
			return fmt.Errorf("sequencer client not initialized")
		}
		return nil
	})
	require.NoError(s.T(), err, "Sequencer client initialization failed")
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

func (s *FullNodeTestSuite) TestSubmitBlocksToDA() {
	require := require.New(s.T())

	// Get initial state
	initialDAHeight := s.node.blockManager.GetDAIncludedHeight()
	initialHeight := s.node.Store.Height()

	// Check if block manager is properly initialized
	s.T().Log("=== Block Manager State ===")
	pendingHeaders, err := s.node.blockManager.PendingHeaders().GetPendingHeaders()
	require.NoError(err)
	s.T().Logf("Initial Pending Headers: %d", len(pendingHeaders))
	s.T().Logf("Last Submitted Height: %d", s.node.blockManager.PendingHeaders().GetLastSubmittedHeight())

	// Verify sequencer is working
	s.T().Log("=== Sequencer Check ===")
	require.NotNil(s.node.blockManager.SeqClient(), "Sequencer client should be initialized")

	// Monitor batch retrieval
	s.T().Log("=== Monitoring Batch Retrieval ===")
	for i := 0; i < 5; i++ {
		time.Sleep(200 * time.Millisecond)
		// We can't directly check batch queue size, but we can monitor block production
		currentHeight := s.node.Store.Height()
		s.T().Logf("Current height after batch check %d: %d", i, currentHeight)
	}

	// Try to trigger block production explicitly
	s.T().Log("=== Attempting to Trigger Block Production ===")
	// Force a state update to trigger block production
	currentState := s.node.blockManager.GetLastState()
	currentState.LastBlockTime = time.Now().Add(-2 * s.node.nodeConfig.BlockTime)
	s.node.blockManager.SetLastState(currentState)

	// Monitor after trigger
	for i := 0; i < 5; i++ {
		time.Sleep(200 * time.Millisecond)
		currentHeight := s.node.Store.Height()
		currentDAHeight := s.node.blockManager.GetDAIncludedHeight()
		pendingHeaders, _ := s.node.blockManager.PendingHeaders().GetPendingHeaders()
		s.T().Logf("Post-trigger check %d - Height: %d, DA Height: %d, Pending: %d",
			i, currentHeight, currentDAHeight, len(pendingHeaders))
	}

	// Final assertions with more detailed error messages
	finalDAHeight := s.node.blockManager.GetDAIncludedHeight()
	finalHeight := s.node.Store.Height()

	require.Greater(finalHeight, initialHeight, "Block height should have increased")
	require.Greater(finalDAHeight, initialDAHeight, "DA height should have increased")
}

func (s *FullNodeTestSuite) TestDAInclusion() {
	require := require.New(s.T())

	// Get initial height and DA height
	initialHeight, err := getNodeHeight(s.node, Header)
	require.NoError(err, "Failed to get initial height")
	initialDAHeight := s.node.blockManager.GetDAIncludedHeight()

	s.T().Logf("=== Initial State ===")
	s.T().Logf("Block height: %d, DA height: %d", initialHeight, initialDAHeight)
	s.T().Logf("Is proposer: %v", s.node.blockManager.IsProposer())
	s.T().Logf("DA client initialized: %v", s.node.blockManager.DALCInitialized())
	s.T().Logf("Aggregator enabled: %v", s.node.nodeConfig.Aggregator)

	// Monitor state changes in shorter intervals
	s.T().Log("=== Monitoring State Changes ===")
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		currentHeight := s.node.Store.Height()
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
		currentHeight := s.node.Store.Height()
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

	// Final state logging
	s.T().Log("=== Final State ===")
	finalHeight := s.node.Store.Height()
	pendingHeaders, _ := s.node.blockManager.PendingHeaders().GetPendingHeaders()
	s.T().Logf("Final Height: %d", finalHeight)
	s.T().Logf("Final DA Height: %d", finalDAHeight)
	s.T().Logf("Final Pending Headers: %d", len(pendingHeaders))

	// Assertions
	require.NoError(err, "DA height did not increase")
	require.Greater(finalHeight, initialHeight, "Block height should increase")
	require.Greater(finalDAHeight, initialDAHeight, "DA height should increase")
}

func (s *FullNodeTestSuite) TestMaxPending() {
	require := require.New(s.T())

	// First, stop the current node by cancelling its context
	s.cancel()

	// Create a new context for the new node
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Reset error channel
	s.errCh = make(chan error, 1)

	// Reconfigure node with low max pending
	config := getTestConfig(1)
	config.BlockManagerConfig.MaxPendingBlocks = 2

	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)

	p2pKey := generateSingleKey()

	dummyDA := coreda.NewDummyDA(100_000)
	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()

	node, err := NewNode(
		s.ctx,
		config,
		dummyExec,
		dummySequencer,
		dummyDA,
		p2pKey,
		signingKey,
		genesis,
		DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()),
		log.NewTestLogger(s.T()),
	)
	require.NoError(err)
	require.NotNil(node)

	fn, ok := node.(*FullNode)
	require.True(ok)

	s.node = fn

	// Start the node using Run in a goroutine
	s.startNodeInBackground(s.node)

	// Wait blocks to be produced up to max pending
	time.Sleep(time.Duration(config.BlockManagerConfig.MaxPendingBlocks+1) * config.BlockTime)

	// Verify that number of pending blocks doesn't exceed max
	height, err := getNodeHeight(s.node, Header)
	require.NoError(err)
	require.LessOrEqual(height, config.BlockManagerConfig.MaxPendingBlocks)
}

func (s *FullNodeTestSuite) TestGenesisInitialization() {
	require := require.New(s.T())

	// Verify genesis state
	state := s.node.blockManager.GetLastState()
	require.Equal(s.node.genesis.InitialHeight, int64(state.InitialHeight))
	require.Equal(s.node.genesis.ChainID, state.ChainID)
}

func (s *FullNodeTestSuite) TestStateRecovery() {
	s.T().Skip("skipping state recovery test, we need to reuse the same database, when we use in memory it starts fresh each time")
	require := require.New(s.T())

	// Get current state
	originalHeight, err := getNodeHeight(s.node, Store)
	require.NoError(err)

	// Wait for some blocks
	time.Sleep(2 * s.node.nodeConfig.BlockTime)

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
		s.T().Log("Warning: Node did not stop gracefully within timeout")
	}

	// Create a new context
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.errCh = make(chan error, 1)

	// Create a NEW node instance instead of reusing the old one
	config := getTestConfig(1)
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	p2pKey := generateSingleKey()

	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000)

	node, err := NewNode(
		s.ctx,
		config,
		dummyExec,
		dummySequencer,
		dummyDA,
		p2pKey,
		signingKey,
		genesis,
		DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()),
		log.NewTestLogger(s.T()),
	)
	require.NoError(err)

	fn, ok := node.(*FullNode)
	require.True(ok)

	// Replace the old node with the new one
	s.node = fn

	// Start the new node
	s.startNodeInBackground(s.node)

	// Wait a bit after restart
	time.Sleep(s.node.nodeConfig.BlockTime)

	// Verify state persistence
	recoveredHeight, err := getNodeHeight(s.node, Store)
	require.NoError(err)
	require.GreaterOrEqual(recoveredHeight, originalHeight)
}

func (s *FullNodeTestSuite) TestInvalidDAConfig() {
	require := require.New(s.T())

	// Create a node with invalid DA configuration
	invalidConfig := getTestConfig(1)
	invalidConfig.DAAddress = "invalid://invalid-address:1234" // Use an invalid URL scheme

	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)

	p2pKey := generateSingleKey()

	dummyDA := coreda.NewDummyDA(100_000)
	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()

	// Attempt to create a node with invalid DA config
	node, err := NewNode(
		s.ctx,
		invalidConfig,
		dummyExec,
		dummySequencer,
		dummyDA,
		p2pKey,
		signingKey,
		genesis,
		DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()),
		log.NewTestLogger(s.T()),
	)

	// Verify that node creation fails with appropriate error
	require.Error(err, "Expected error when creating node with invalid DA config")
	require.Contains(err.Error(), "unknown url scheme", "Expected error related to invalid URL scheme")
	require.Nil(node, "Node should not be created with invalid DA config")
}
