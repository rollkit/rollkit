//go:build e2e

package e2e

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/rollkit/rollkit/pkg/rpc/client"
	"github.com/stretchr/testify/require"
)

var binaryPath string

func init() {
	flag.StringVar(&binaryPath, "binary", "testapp", "testapp binary")
}

func TestBasic(t *testing.T) {
	// scenario:
	// start an aggregator and a fullnode
	// when a tx for state update is executed
	// then the new state becomes available on the fullnode, too
	flag.Parse()
	var (
		workDir   = t.TempDir()
		node1Home = filepath.Join(workDir, "1")
		node2Home = filepath.Join(workDir, "2")
	)

	// Define and parse the binary flag locally in the test function.

	sut := NewSystemUnderTest(t)

	// start local da
	localDABinary := filepath.Join(filepath.Dir(binaryPath), "local-da")
	sut.StartNode(localDABinary)
	// Wait a moment for the local DA to initialize
	time.Sleep(500 * time.Millisecond)

	aggregatorPass := "12345678"
	// init aggregator
	output, err := sut.RunCmd(binaryPath,
		"init",
		"--home="+node1Home,
		"--chain_id=testing",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase="+aggregatorPass,
	)
	require.NoError(t, err, "failed to init aggregator", output)

	// start aggregator
	sut.StartNode(binaryPath,
		"start",
		"--home="+node1Home,
		"--chain_id=testing",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase="+aggregatorPass,
		"--rollkit.node.block_time=5ms",
		"--rollkit.da.block_time=15ms",
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 2*time.Second)

	// Give aggregator more time before starting the next node
	time.Sleep(1 * time.Second) // Increased wait time

	// Init the second node (full node)
	output, err = sut.RunCmd(binaryPath,
		"init",
		"--home="+node2Home,
	)
	require.NoError(t, err, "failed to init fullnode", output)

	// Copy genesis file from aggregator to full node
	MustCopyFile(t, filepath.Join(node1Home, "config", "genesis.json"), filepath.Join(node2Home, "config", "genesis.json"))

	// Start the full node
	node2RPC := "127.0.0.1:7332"
	node1P2PAddr := fmt.Sprintf("%s@127.0.0.1:26656", NodeID(t, node1Home))
	sut.StartNode(
		binaryPath,
		"start",
		"--home="+node2Home,
		fmt.Sprintf("--rollkit.p2p.peers=%s", node1P2PAddr), // Use the constructed P2P address
		"--rollkit.log.level=debug",
		fmt.Sprintf("--rollkit.rpc.address=%s", node2RPC), // Use the variable
	)

	sut.AwaitNodeUp(t, "http://"+node2RPC, 5*time.Second) // Increased timeout slightly
	t.Logf("Full node (node 2) is up.")

	asserNodeCaughtUp := func(c *client.Client, height uint64) {
		ctx, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		state, err := c.GetState(ctx)
		require.NoError(t, err)
		require.Greater(t, state.LastBlockHeight, height)
	}

	node1Client := client.NewClient("http://127.0.0.1:7331")
	require.NoError(t, err)
	asserNodeCaughtUp(node1Client, 1)

	// get latest height for node 1
	ctx, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	state, err := node1Client.GetState(ctx)
	require.NoError(t, err)

	node2Client := client.NewClient("http://" + node2RPC) // Use variable and prepend http://
	require.NotNil(t, node2Client, "Failed to create client for node 2")
	require.Eventually(t, func() bool {
		ctxNode2, doneNode2 := context.WithTimeout(context.Background(), time.Second)
		defer doneNode2()
		stateNode2, err := node2Client.GetState(ctxNode2)
		if err != nil {
			t.Logf("Error getting state from node 2: %v", err)
			return false
		}
		return stateNode2.LastBlockHeight >= state.LastBlockHeight
	}, 10*time.Second, 500*time.Millisecond, "Node 2 failed to catch up")

	// when a client TX for state update is executed
	// const myKey = "foo"
	// myValue := fmt.Sprintf("bar%d", time.Now().UnixNano())
	// tx := fmt.Sprintf("%s=%s", myKey, myValue)

	// ctx, done := context.WithTimeout(context.Background(), time.Second)
	// defer done()
	// result, err := node1Client.BroadcastTxCommit(ctx, tx)
	// require.NoError(t, err)
	// require.Equal(t, uint32(0), result.TxResult.Code, result.TxResult.Log)

	// then state is persisted
	// ctx, done = context.WithTimeout(context.Background(), 150*time.Millisecond)
	// defer done()
	// resQuery, err := node1Client.ABCIQuery(ctx, "/store", []byte(myKey))
	// require.NoError(t, err)
	// require.Equal(t, myValue, string(resQuery.Response.Value))

	// and state distributed to fullnode
	// require.Eventually(t, func() bool {
	// 	ctx, done := context.WithTimeout(context.Background(), 150*time.Millisecond)
	// 	defer done()
	// 	resQuery, err = node2Client.ABCIQuery(ctx, "/store", []byte(myKey))
	// 	require.NoError(t, err)
	// 	return myValue == string(resQuery.Response.Value)
	// }, time.Second, 5*time.Millisecond)
}
