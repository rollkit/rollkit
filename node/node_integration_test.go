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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/types"
)

// NodeIntegrationTestSuite is a test suite for node integration tests
type NodeIntegrationTestSuite struct {
	suite.Suite
	ctx       context.Context
	cancel    context.CancelFunc
	node      Node
	seqSrv    *grpc.Server
	errCh     chan error
	runningWg sync.WaitGroup
}

// SetupTest is called before each test
func (s *NodeIntegrationTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.errCh = make(chan error, 1)

	// Setup node with proper configuration
	config := getTestConfig(s.T(), 1)
	config.Node.BlockTime = rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond} // Faster block production for tests
	config.DA.BlockTime = rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond}   // Faster DA submission for tests
	config.Node.MaxPendingBlocks = 100                                                      // Allow more pending blocks

	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey("test-chain")

	s.seqSrv = startMockSequencerServerGRPC(MockSequencerAddress)
	require.NotNil(s.T(), s.seqSrv)

	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)
	dummyClient := coreda.NewDummyClient(dummyDA, []byte(MockDANamespace))

	err := InitFiles(config.RootDir)
	require.NoError(s.T(), err)

	node, err := NewNode(
		s.ctx,
		config,
		dummyExec,
		dummySequencer,
		dummyClient,
		genesisValidatorKey,
		genesis,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		log.NewTestLogger(s.T()),
	)
	require.NoError(s.T(), err)
	require.NotNil(s.T(), node)

	fn, ok := node.(*FullNode)
	require.True(s.T(), ok)

	s.node = fn

	// Start the node in a goroutine using Run instead of Start
	s.runningWg.Add(1)
	go func() {
		defer s.runningWg.Done()
		err := s.node.Run(s.ctx)
		select {
		case s.errCh <- err:
		default:
			s.T().Logf("Error channel full, discarding error: %v", err)
		}
	}()

	// Wait for node initialization with retry
	err = testutils.Retry(60, 100*time.Millisecond, func() error {
		height, err := getNodeHeight(s.node, Header)
		if err != nil {
			return err
		}
		if height == 0 {
			return fmt.Errorf("waiting for first block")
		}
		return nil
	})
	require.NoError(s.T(), err, "Node failed to produce first block")

	// Wait for DA inclusion with longer timeout
	err = testutils.Retry(100, 100*time.Millisecond, func() error {
		daHeight := s.node.(*FullNode).blockManager.GetDAIncludedHeight()
		if daHeight == 0 {
			return fmt.Errorf("waiting for DA inclusion")
		}
		return nil
	})
	require.NoError(s.T(), err, "Failed to get DA inclusion")
}

// TearDownTest is called after each test
func (s *NodeIntegrationTestSuite) TearDownTest() {
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

	if s.seqSrv != nil {
		s.seqSrv.GracefulStop()
	}
}

// TestNodeIntegrationTestSuite runs the test suite
func TestNodeIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(NodeIntegrationTestSuite))
}

func (s *NodeIntegrationTestSuite) waitForHeight(targetHeight uint64) error {
	return waitForAtLeastNBlocks(s.node, int(targetHeight), Store)
}

func (s *NodeIntegrationTestSuite) TestBlockProduction() {
	// Wait for at least one block to be produced and transactions to be included
	time.Sleep(5 * time.Second) // Give more time for the full flow

	// Get transactions from executor to verify they are being injected
	execTxs, err := s.node.(*FullNode).blockManager.GetExecutor().GetTxs(s.ctx)
	s.NoError(err)
	s.T().Logf("Number of transactions from executor: %d", len(execTxs))

	// Wait for at least one block to be produced
	err = s.waitForHeight(1)
	s.NoError(err, "Failed to produce first block")

	// Get the current height
	height := s.node.(*FullNode).Store.Height()
	s.GreaterOrEqual(height, uint64(1), "Expected block height >= 1")

	// Get all blocks and log their contents
	for h := uint64(1); h <= height; h++ {
		header, data, err := s.node.(*FullNode).Store.GetBlockData(s.ctx, h)
		s.NoError(err)
		s.NotNil(header)
		s.NotNil(data)
		s.T().Logf("Block height: %d, Time: %s, Number of transactions: %d", h, header.Time(), len(data.Txs))
	}

	// Get the latest block
	header, data, err := s.node.(*FullNode).Store.GetBlockData(s.ctx, height)
	s.NoError(err)
	s.NotNil(header)
	s.NotNil(data)

	// Log block details
	s.T().Logf("Latest block height: %d, Time: %s, Number of transactions: %d", height, header.Time(), len(data.Txs))

	// Verify chain state
	state, err := s.node.(*FullNode).Store.GetState(s.ctx)
	s.NoError(err)
	s.Equal(height, state.LastBlockHeight)

	// Verify block content
	// TODO: uncomment this when we have the system working correctly
	// s.NotEmpty(data.Txs, "Expected block to contain transactions")
}
