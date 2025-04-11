//go:build e2e

package e2e

import (
	"context"
	"flag"
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
		// node2Home = filepath.Join(workDir, "2")
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

	// // copy genesis to target home2
	// output, err = sut.RunCmd(binaryPath,
	// 	"init",
	// 	"--home="+node2Home,
	// )
	// require.NoError(t, err, "failed to init fullnode", output)
	// MustCopyFile(t, filepath.Join(node1Home, "config", "genesis.json"), filepath.Join(node2Home, "config", "genesis.json"))
	// sut.StartNode(
	// 	binaryPath,
	// 	"start",
	// 	"--home="+node2Home,
	// 	fmt.Sprintf("--rollkit.p2p.seeds=%s@127.0.0.1:26656", NodeID(t, node1Home)),
	// 	"--rollkit.log.level=debug",
	// 	"--rollkit.rpc.address=127.0.0.1:7332",
	// )
	// sut.AwaitNodeUp(t, "http://127.0.0.1:7332", 2*time.Second)

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

	// get latest height
	// ctx, done := context.WithTimeout(context.Background(), time.Second)
	// defer done()
	// state, err := node1Client.GetState(ctx)
	// require.NoError(t, err)

	// node2Client := client.NewClient("http://127.0.0.1:16657")
	// require.NoError(t, err)
	// asserNodeCaughtUp(node2Client, state.LastBlockHeight)

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
