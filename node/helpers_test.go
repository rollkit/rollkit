package node

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"

	cmcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/types"
)

// getMockDA creates a mock DA client for testing
func getMockDA(t *testing.T) *da.DAClient {
	namespace := make([]byte, len(MockDANamespace)/2)
	_, err := hex.Decode(namespace, []byte(MockDANamespace))
	require.NoError(t, err)
	return da.NewDAClient(goDATest.NewDummyDA(), -1, -1, namespace, nil, log.TestingLogger())
}

// generateSingleKey generates a single Ed25519 key for testing
func generateSingleKey() crypto.PrivKey {
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		panic(err)
	}
	return key
}

func getTestConfig() config.NodeConfig {
	return config.NodeConfig{
		DAAddress:        MockDAAddress,
		DANamespace:      MockDANamespace,
		ExecutorAddress:  MockExecutorAddress,
		SequencerAddress: MockSequencerAddress,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime:        100 * time.Millisecond,
			DABlockTime:      200 * time.Millisecond,
			DAStartHeight:    0,
			DAMempoolTTL:     100,
			MaxPendingBlocks: 100,
			LazyAggregator:   false,
		},
	}
}

func setupTestNodeWithCleanup(t *testing.T) (*FullNode, func()) {
	ctx := context.Background()
	config := getTestConfig()

	// Generate genesis and keys
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(t, err)

	p2pKey := generateSingleKey()

	node, err := NewNode(ctx, config, p2pKey, signingKey, genesis, DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()), log.TestingLogger())
	require.NoError(t, err)

	cleanup := func() {
		if fn, ok := node.(*FullNode); ok {
			_ = fn.Stop()
		}
	}

	//startNodeWithCleanup(t, node)

	return node.(*FullNode), cleanup
}

// TestHelpers verifies that helper functions work correctly
func TestHelpers(t *testing.T) {
	t.Run("getTestConfig returns valid config", func(t *testing.T) {
		cfg := getTestConfig()
		require.Equal(t, MockDAAddress, cfg.DAAddress)
		require.Equal(t, MockDANamespace, cfg.DANamespace)
		require.Equal(t, MockExecutorAddress, cfg.ExecutorAddress)
		require.Equal(t, MockSequencerAddress, cfg.SequencerAddress)
	})

	t.Run("setupTestNode creates working node", func(t *testing.T) {
		node, cleanup := setupTestNodeWithCleanup(t)
		defer cleanup()
		require.NotNil(t, node)
		require.False(t, node.IsRunning())
	})

	t.Run("startNodeWithCleanup works correctly", func(t *testing.T) {
		node, cleanup := setupTestNodeWithCleanup(t)
		defer cleanup()
		startNodeWithCleanup(t, node)
		require.True(t, node.IsRunning())
		require.NoError(t, node.Stop())
		require.False(t, node.IsRunning())
	})

	t.Run("getMockDA returns valid client", func(t *testing.T) {
		client := getMockDA(t)
		require.NotNil(t, client)
	})
}
