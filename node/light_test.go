package node

import (
	"context"
	"testing"
	"time"

	ds "github.com/ipfs/go-datastore"
	ds_sync "github.com/ipfs/go-datastore/sync"
	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/p2p"
	p2p_key "github.com/evstack/ev-node/pkg/p2p/key"
)

// TestLightNodeLifecycle tests the light node's lifecycle.
func TestLightNodeLifecycle(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	conf := config.Config{
		RootDir: t.TempDir(),
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/0",
		},
		RPC: config.RPCConfig{
			Address: "127.0.0.1:0",
		},
	}
	gen := genesis.Genesis{
		ChainID: "test-chain",
	}
	p2pKey, err := p2p_key.GenerateNodeKey()
	require.NoError(err)

	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL")
	p2pMetrics := p2p.NopMetrics()

	db := ds_sync.MutexWrap(ds.NewMapDatastore())

	p2pClient, err := p2p.NewClient(conf, p2pKey, db, logger, p2pMetrics)
	require.NoError(err)

	ln, err := newLightNode(conf, gen, p2pClient, db, logger)
	require.NoError(err)
	require.NotNil(ln)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runResult := make(chan error, 1)
	go func() {
		runResult <- ln.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)

	cancel()

	// Wait for the Run method to return.
	select {
	case err := <-runResult:
		assert.ErrorIs(err, context.Canceled, "Expected context.Canceled error")
	case <-time.After(5 * time.Second):
		t.Fatal("Node did not stop gracefully within the timeout")
	}
}
