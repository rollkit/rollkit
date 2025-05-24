package node

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	remote_signer "github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/types"
)

func getTestConfig(t *testing.T, n int) rollkitconfig.Config {
	// Use a higher base port to reduce chances of conflicts with system services
	startPort := 40000 + n*100 // Spread port ranges further apart
	return rollkitconfig.Config{
		RootDir: t.TempDir(),
		Node: rollkitconfig.NodeConfig{
			Aggregator:        true,
			BlockTime:         rollkitconfig.DurationWrapper{Duration: 100 * time.Millisecond},
			MaxPendingHeaders: 100,
			LazyBlockInterval: rollkitconfig.DurationWrapper{Duration: 5 * time.Second},
		},
		DA: rollkitconfig.DAConfig{
			BlockTime: rollkitconfig.DurationWrapper{Duration: 200 * time.Millisecond},
			Address:   MockDAAddress,
			Namespace: MockDANamespace,
		},
		P2P: rollkitconfig.P2PConfig{
			ListenAddress: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", startPort+n),
		},
	}
}

func setupTestNodeWithCleanup(t *testing.T, config rollkitconfig.Config) (*FullNode, func()) {
	// Create a cancellable context instead of using background context
	ctx, cancel := context.WithCancel(context.Background())

	// Generate genesis and keys
	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(t, err)

	executor, sequencer, dac, p2pClient, ds := createTestComponents(t)

	nodeKey, err := InitFiles(config.RootDir)
	require.NoError(t, err)

	node, err := NewNode(
		ctx,
		config,
		executor,
		sequencer,
		dac,
		remoteSigner,
		*nodeKey,
		p2pClient,
		genesis,
		ds,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		log.NewTestLogger(t),
		types.ValidatorHasher(nil),
	)
	require.NoError(t, err)

	// Update cleanup to cancel the context instead of calling Stop
	cleanup := func() {
		// Cancel the context to stop the node
		cancel()
	}

	return node.(*FullNode), cleanup
}
