package node

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	remote_signer "github.com/rollkit/rollkit/pkg/remote_signer/noop"
	"github.com/rollkit/rollkit/types"
)

func getTestConfig(t *testing.T, n int) rollkitconfig.Config {
	startPort := 10000
	return rollkitconfig.Config{
		RootDir: t.TempDir(),
		Node: rollkitconfig.NodeConfig{
			Aggregator:       true,
			ExecutorAddress:  MockExecutorAddress,
			SequencerAddress: MockSequencerAddress,
			BlockTime:        rollkitconfig.DurationWrapper{Duration: 500 * time.Millisecond},
			LazyBlockTime:    rollkitconfig.DurationWrapper{Duration: 5 * time.Second},
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
	config := getTestConfig(t, 1)

	// Generate genesis and keys
	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(t, err)

	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)
	dummyClient := coreda.NewDummyClient(dummyDA, []byte(MockDANamespace))

	err = InitFiles(config.RootDir)
	require.NoError(t, err)

	node, err := NewNode(
		ctx,
		config,
		dummyExec,
		dummySequencer,
		dummyClient,
		remoteSigner,
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
