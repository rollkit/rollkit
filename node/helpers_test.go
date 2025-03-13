package node

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"

	rollkitconfig "github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
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

func getTestConfig(n int) rollkitconfig.Config {
	startPort := 10000
	return rollkitconfig.Config{
		Node: rollkitconfig.NodeConfig{
			Aggregator:       true,
			ExecutorAddress:  MockExecutorAddress,
			SequencerAddress: MockSequencerAddress,
			BlockTime:        500 * time.Millisecond,
			LazyBlockTime:    5 * time.Second,
		},
		DA: rollkitconfig.DAConfig{
			Address:   MockDAAddress,
			Namespace: MockDANamespace,
		},
		P2P: rollkitconfig.P2PConfig{
			ListenAddress: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", startPort+n),
		},
	}
}

func setupTestNodeWithCleanup(t *testing.T) (*FullNode, func()) {
	// Create a cancellable context instead of using background context
	ctx, cancel := context.WithCancel(context.Background())
	config := getTestConfig(1)

	// Generate genesis and keys
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey("test-chain")

	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000)
	dummyClient := coreda.NewDummyClient(dummyDA, []byte(MockDANamespace))

	node, err := NewNode(
		ctx,
		config,
		dummyExec,
		dummySequencer,
		dummyClient,
		genesisValidatorKey,
		genesis,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
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
