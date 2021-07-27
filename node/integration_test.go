package node

import (
	"context"
	"crypto/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/mocks"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestTxGossipingAndAggregation setups a network of nodes, with single aggregator and multiple producers.
// Nodes should gossip transactions and aggregator node should produce blocks.
func TestTxGossipingAndAggregation(t *testing.T) {
	nodes := createNodes(11, t)

	for _, n := range nodes {
		n.Start()
	}

	time.Sleep(250 * time.Millisecond)

	for i := 1; i < len(nodes); i++ {
		data := strconv.Itoa(i) + time.Now().String()
		nodes[i].P2P.GossipTx(context.TODO(), []byte(data))
	}

	time.Sleep(1 * time.Second)

	for _, n := range nodes {
		n.Stop()
	}
}

func createNodes(num int, t *testing.T) []*Node {
	t.Helper()

	// create keys first, as they are required for P2P connections
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}

	nodes := make([]*Node, num)
	nodes[0] = createNode(0, true, keys, t)
	for i := 1; i < num; i++ {
		nodes[i] = createNode(i, false, keys, t)
	}

	return nodes
}

func createNode(n int, aggregator bool, keys []crypto.PrivKey, t *testing.T) *Node {
	t.Helper()
	require := require.New(t)
	// nodes will listen on consecutive ports on local interface
	// random connections to other nodes will be added
	startPort := 9000
	p2pConfig := config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+n),
	}
	aggConfig := config.AggregatorConfig{
		BlockTime:   200 * time.Millisecond,
		NamespaceID: [8]byte{8, 7, 6, 5, 4, 3, 2, 1},
	}
	for i := 0; i < len(keys); i++ {
		if i == n {
			continue
		}
		r := i
		id, err := peer.IDFromPrivateKey(keys[r])
		require.NoError(err)
		p2pConfig.Seeds += "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+r) + "/p2p/" + id.Pretty() + ","
	}
	p2pConfig.Seeds = strings.TrimSuffix(p2pConfig.Seeds, ",")

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	if aggregator {
		app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
		app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
		app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
		app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	}

	node, err := NewNode(
		context.Background(),
		config.NodeConfig{
			P2P:              p2pConfig,
			DALayer:          "mock",
			Aggregator:       aggregator,
			AggregatorConfig: aggConfig,
		},
		keys[n],
		proxy.NewLocalClientCreator(app),
		&types.GenesisDoc{ChainID: "test"},
		log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	return node
}
