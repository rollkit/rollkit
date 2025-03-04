package node

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"cosmossdk.io/log"
	cmcfg "github.com/cometbft/cometbft/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
)

// generateSingleKey generates a single Ed25519 key for testing
func generateSingleKey() crypto.PrivKey {
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		panic(err)
	}
	return key
}

func getTestConfig(n int) config.NodeConfig {
	startPort := 10000
	return config.NodeConfig{
		Aggregator:       true,
		DAAddress:        MockDAAddress,
		DANamespace:      MockDANamespace,
		ExecutorAddress:  MockExecutorAddress,
		SequencerAddress: MockSequencerAddress,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime:     500 * time.Millisecond,
			LazyBlockTime: 5 * time.Second,
		},
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+n),
		},
	}
}

func setupTestNodeWithCleanup(t *testing.T) (*FullNode, func()) {
	// Create a cancellable context instead of using background context
	ctx, cancel := context.WithCancel(context.Background())
	config := getTestConfig(1)

	// Generate genesis and keys
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(t, err)

	p2pKey := generateSingleKey()

	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()

	node, err := NewNode(
		ctx,
		config,
		dummyExec,
		dummySequencer,
		p2pKey,
		signingKey,
		genesis,
		DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()),
		log.NewTestLogger(t),
	)
	require.NoError(t, err)

	// Update cleanup to cancel the context instead of calling Stop
	cleanup := func() {
		// Cancel the context to stop the node
		cancel()
	}

	return node.(*FullNode), cleanup
}
