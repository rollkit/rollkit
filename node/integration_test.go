package node

import (
	"context"
	"crypto/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/mocks"
)

// TestTxGossipingAndAggregation setups a network of nodes, with single aggregator and multiple producers.
// Nodes should gossip transactions and aggregator node should produce blocks.
func TestTxGossipingAndAggregation(t *testing.T) {
	require := require.New(t)

	nodes, aggApp := createNodes(11, t)

	for _, n := range nodes {
		require.NoError(n.Start())
	}

	time.Sleep(1 * time.Second)

	for i := 1; i < len(nodes); i++ {
		data := strconv.Itoa(i) + time.Now().String()
		require.NoError(nodes[i].P2P.GossipTx(context.TODO(), []byte(data)))
	}

	time.Sleep(4 * time.Second)

	for _, n := range nodes {
		require.NoError(n.Stop())
	}

	aggApp.AssertNumberOfCalls(t, "DeliverTx", 10)
	aggApp.AssertExpectations(t)
}

func createNodes(num int, t *testing.T) ([]*Node, *mocks.Application) {
	t.Helper()

	// create keys first, as they are required for P2P connections
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}

	nodes := make([]*Node, num)
	var aggApp *mocks.Application
	nodes[0], aggApp = createNode(0, true, keys, t)
	for i := 1; i < num; i++ {
		nodes[i], _ = createNode(i, false, keys, t)
	}

	return nodes, aggApp
}

func createNode(n int, aggregator bool, keys []crypto.PrivKey, t *testing.T) (*Node, *mocks.Application) {
	t.Helper()
	require := require.New(t)
	// nodes will listen on consecutive ports on local interface
	// random connections to other nodes will be added
	startPort := 10000
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

	return node, app
}
