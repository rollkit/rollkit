package node

import (
	"context"
	"testing"
	"time"

	cmcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestSubmitBlocksToDA(t *testing.T) {
	require := require.New(t)

	// Setup node with mock executor from TestMain
	node, cleanup := setupTestNodeWithCleanup(t)
	defer cleanup()
	//startNodeWithCleanup(t, node)

	// Wait for the first block to be produced and submitted to DA
	err := waitForFirstBlock(node, Header)
	require.NoError(err)

	// Verify that block was submitted to DA
	height, err := getNodeHeight(node, Header)
	require.NoError(err)
	require.Greater(height, uint64(0))

	// Wait for a few more blocks to ensure continuous DA submission
	time.Sleep(3 * node.nodeConfig.BlockTime)
	newHeight, err := getNodeHeight(node, Header)
	require.NoError(err)
	require.Greater(newHeight, height)
}

func TestMaxPending(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Setup node with mock executor from TestMain
	config := getTestConfig()
	config.BlockManagerConfig.MaxPendingBlocks = 2 // Set low max pending for testing

	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)

	p2pKey := generateSingleKey()

	node, err := NewNode(ctx, config, p2pKey, signingKey, genesis, DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()), log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	fn, ok := node.(*FullNode)
	require.True(ok)

	startNodeWithCleanup(t, fn)

	// Wait for blocks to be produced up to max pending
	time.Sleep(time.Duration(config.BlockManagerConfig.MaxPendingBlocks+1) * config.BlockTime)

	// Verify that number of pending blocks doesn't exceed max
	height, err := getNodeHeight(fn, Header)
	require.NoError(err)
	require.LessOrEqual(height, config.BlockManagerConfig.MaxPendingBlocks)
}
