//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	nodeclient "github.com/rollkit/rollkit/pkg/rpc/client"
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
		"--kv-endpoint=127.0.0.1:9090",
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 2*time.Second)

	// Give aggregator more time before starting the next node
	time.Sleep(1 * time.Second) // Increased wait time

	// Init the second node (full node)
	output, err = sut.RunCmd(binaryPath,
		"init",
		"--chain_id=testing",
		"--home="+node2Home,
	)
	require.NoError(t, err, "failed to init fullnode", output)

	// Copy genesis file from aggregator to full node
	MustCopyFile(t, filepath.Join(node1Home, "config", "genesis.json"), filepath.Join(node2Home, "config", "genesis.json"))

	// Start the full node
	node2RPC := "127.0.0.1:7332"
	node2P2P := "/ip4/0.0.0.0/tcp/7676"
	sut.StartNode(
		binaryPath,
		"start",
		"--home="+node2Home,
		"--rollkit.log.level=debug",
		"--rollkit.p2p.listen_address="+node2P2P,
		fmt.Sprintf("--rollkit.rpc.address=%s", node2RPC),
	)

	sut.AwaitNodeUp(t, "http://"+node2RPC, 2*time.Second)
	t.Logf("Full node (node 2) is up.")

	// when a client TX for state update is executed
	const myKey = "foo"
	myValue := fmt.Sprintf("bar%d", time.Now().UnixNano())
	tx := fmt.Sprintf("%s=%s", myKey, myValue)
	kvStoreEndpoint := "http://127.0.0.1:9090/tx" // Assuming this is the endpoint based on init flag

	ctx, done := context.WithTimeout(context.Background(), 5*time.Second) // Increased timeout for HTTP request
	defer done()

	// Submit transaction via HTTP POST to the KV store endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kvStoreEndpoint, bytes.NewBufferString(tx))
	require.NoError(t, err, "failed to create http request")
	req.Header.Set("Content-Type", "text/plain") // Or application/octet-stream depending on server expectation

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err, "failed to send http request")
	defer resp.Body.Close()

	require.Equal(t, http.StatusAccepted, resp.StatusCode, "transaction not accepted by kv store")
	t.Logf("Transaction '%s' submitted successfully via HTTP to %s", tx, kvStoreEndpoint)

	// wait for the transaction to be processed
	time.Sleep(2 * time.Second)

	// verify a block has been produced
	c := nodeclient.NewClient("http://127.0.0.1:7331")
	require.NoError(t, err)

	ctx, done = context.WithTimeout(context.Background(), time.Second)
	defer done()
	state, err := c.GetState(ctx)
	require.NoError(t, err)
	require.Greater(t, state.LastBlockHeight, uint64(1))
}

func TestNodeRestartPersistence(t *testing.T) {
	flag.Parse()
	var (
		workDir  = t.TempDir()
		nodeHome = filepath.Join(workDir, "node1")
	)

	sut := NewSystemUnderTest(t)

	// Start local DA if needed
	localDABinary := filepath.Join(filepath.Dir(binaryPath), "local-da")
	sut.StartNode(localDABinary)
	time.Sleep(500 * time.Millisecond)

	// Init node
	output, err := sut.RunCmd(binaryPath,
		"init",
		"--home="+nodeHome,
		"--chain_id=testing",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase=12345678",
	)
	require.NoError(t, err, "failed to init node", output)

	// Start node
	sut.StartNode(binaryPath,
		"start",
		"--home="+nodeHome,
		"--chain_id=testing",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase=12345678",
		"--rollkit.node.block_time=5ms",
		"--rollkit.da.block_time=15ms",
		"--kv-endpoint=127.0.0.1:9090",
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 2*time.Second)
	t.Log("Node started and is up.")

	c := nodeclient.NewClient("http://127.0.0.1:7331")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, err := c.GetState(ctx)
	require.NoError(t, err)
	require.Greater(t, state.LastBlockHeight, uint64(1))

	// Wait for a block to be produced
	time.Sleep(1 * time.Second)

	// Shutdown node
	sut.Shutdown()
	t.Log("Node stopped.")

	// Wait a moment to ensure shutdown
	time.Sleep(500 * time.Millisecond)

	// Restart node
	sut.StartNode(binaryPath,
		"start",
		"--home="+nodeHome,
		"--chain_id=testing",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase=12345678",
		"--rollkit.node.block_time=5ms",
		"--rollkit.da.block_time=15ms",
		"--kv-endpoint=127.0.0.1:9090",
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 2*time.Second)
	t.Log("Node restarted and is up.")

	// Wait for a block to be produced after restart
	time.Sleep(1 * time.Second)

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	state2, err := c.GetState(ctx2)
	defer cancel()
	require.NoError(t, err)
	require.Greater(t, state2.LastBlockHeight, state.LastBlockHeight)
}
