//go:build e2e

package e2e

import (
	"flag"
	"path/filepath"
	"testing"
	"time"
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
	// start aggregator
	sut.StartNode(binaryPath,
		"start",
		"--home="+node1Home,
		"--node.sequencer_rollup_id=testing",
		"--node.aggregator",
		"--node.block_time=5ms",
		"--da.block_time=15ms",
	)
	sut.AwaitNodeUp(t, "tcp://127.0.0.1:7331", 2*time.Second)

	// // copy genesis to target home2
	// MustCopyFile(t, filepath.Join(node1Home, "config", "genesis.json"), filepath.Join(node2Home, "config", "genesis.json"))
	// sut.StartNode(
	// 	binaryPath,
	// 	"start",
	// 	"--home="+node2Home,
	// 	"--node.sequencer_rollup_id=testing",
	// 	fmt.Sprintf("--p2p.seeds=%s@127.0.0.1:26656", NodeID(t, node1Home)),
	// 	"--block_time=5ms",
	// 	"--da_block_time=15ms",
	// 	"--log_level=debug",
	// )
	// sut.AwaitNodeUp(t, "tcp://127.0.0.1:16657", 2*time.Second)

	// asserNodeCaughtUp := func(c *rpchttp.HTTP) {
	// 	ctx, done := context.WithTimeout(context.Background(), time.Second)
	// 	defer done()
	// 	status, err := c.Status(ctx)
	// 	require.NoError(t, err)
	// 	require.False(t, status.SyncInfo.CatchingUp)
	// }
	// node1Client, err := rpchttp.New("tcp://localhost:26657", "tcp://localhost:26657"+"/websocket")
	// require.NoError(t, err)
	// asserNodeCaughtUp(node1Client)

	// node2Client, err := rpchttp.New("tcp://localhost:16657", "tcp://localhost:16657"+"/websocket")
	// require.NoError(t, err)
	// asserNodeCaughtUp(node2Client)

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
