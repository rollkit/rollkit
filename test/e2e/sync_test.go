//go:build e2e

package e2e

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	nodeclient "github.com/rollkit/rollkit/pkg/rpc/client"
	"github.com/stretchr/testify/require"
)

func TestFullNodeSyncsFromAggregator(t *testing.T) {
	// scenario:
	// start an aggregator and a fullnode
	// when a tx for state update is executed
	// then the new state becomes available on the fullnode, too
	flag.Parse()
	var (
		workDir   = t.TempDir()
		node1Home = filepath.Join(workDir, "agg")
		node2Home = filepath.Join(workDir, "full")
	)

	sut := NewSystemUnderTest(t)

	// Start local DA if needed
	localDABinary := filepath.Join(filepath.Dir(binaryPath), "local-da")
	sut.StartNode(localDABinary)
	time.Sleep(500 * time.Millisecond)

	// Init aggregator
	output, err := sut.RunCmd(binaryPath,
		"init",
		"--home="+node1Home,
		"--chain_id=sync-test",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase=12345678",
	)
	require.NoError(t, err, "failed to init aggregator", output)

	// Start aggregator with block time 100ms
	sut.StartNode(binaryPath,
		"start",
		"--home="+node1Home,
		"--chain_id=sync-test",
		"--rollkit.node.aggregator",
		"--rollkit.signer.passphrase=12345678",
		"--rollkit.node.block_time=100ms",
		"--rollkit.da.block_time=200ms",
		"--kv-endpoint=127.0.0.1:9090",
	)

	node1RPC := "http://127.0.0.1:7331"
	sut.AwaitNodeUp(t, node1RPC, 2*time.Second)
	t.Log("Aggregator node is up.")

	node1Client := nodeclient.NewClient(node1RPC)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	info, err := node1Client.GetNetInfo(ctx)
	cancel()

	require.NoError(t, err)
	t.Log(info.ListenAddresses[0])

	// Wait for a few blocks to be produced
	time.Sleep(1 * time.Second)

	// Init full node
	output, err = sut.RunCmd(binaryPath,
		"init",
		"--chain_id=sync-test",
		"--home="+node2Home,
	)
	require.NoError(t, err, "failed to init fullnode", output)

	// Copy genesis file from aggregator to full node
	MustCopyFile(t, filepath.Join(node1Home, "config", "genesis.json"), filepath.Join(node2Home, "config", "genesis.json"))

	// Start full node
	node2RPC := "127.0.0.1:7332"
	node2P2P := "/ip4/0.0.0.0/tcp/7677"
	sut.StartNode(
		binaryPath,
		"start",
		"--home="+node2Home,
		"--rollkit.log.level=debug",
		"--rollkit.p2p.listen_address="+node2P2P,
		"--rollkit.p2p.peers="+info.ListenAddresses[0],
		fmt.Sprintf("--rollkit.rpc.address=%s", node2RPC),
	)
	sut.AwaitNodeUp(t, "http://"+node2RPC, 2*time.Second)
	t.Log("Full node is up.")

	// Wait for the full node to sync
	fullNodeClient := nodeclient.NewClient("http://" + node2RPC)
	var synced bool
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		state, err := fullNodeClient.GetState(ctx)
		cancel()
		if err == nil && state.LastBlockHeight > 10 {
			synced = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.True(t, synced, "Full node did not sync to aggregator's latest block")

}
