package node

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"
	"time"

	"cosmossdk.io/log"
	cmcfg "github.com/cometbft/cometbft/config"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
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
	ctx := context.Background()
	config := getTestConfig(1)

	// Generate genesis and keys
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "test-chain")
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(t, err)

	p2pKey := generateSingleKey()

	dummyExec := NewDummyExecutor()
	dummySequencer := NewDummySequencer()
	database := ds.NewMapDatastore()

	node, err := NewNode(
		ctx,
		config,
		dummyExec,
		dummySequencer,
		p2pKey,
		signingKey,
		genesis,
		database,
		DefaultMetricsProvider(cmcfg.DefaultInstrumentationConfig()),
		log.NewTestLogger(t),
	)
	require.NoError(t, err)

	cleanup := func() {
		if fn, ok := node.(*FullNode); ok {
			_ = fn.Stop(ctx)
		}
	}

	return node.(*FullNode), cleanup
}
