package node

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	mrand "math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"

	testutils "github.com/celestiaorg/utils/test"
)

func TestAggregatorMode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On(InitChain, mock.Anything).Return(abci.ResponseInitChain{})
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	app.On(BeginBlock, mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On(DeliverTx, mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On(EndBlock, mock.Anything).Return(abci.ResponseEndBlock{})
	app.On(Commit, mock.Anything).Return(abci.ResponseCommit{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner()
	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := newFullNode(ctx, config.NodeConfig{DALayer: "mock", Aggregator: true, BlockManagerConfig: blockManagerConfig}, key, signingKey, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	err = node.Start()
	assert.NoError(err)
	defer func() {
		require.NoError(node.Stop())
	}()
	assert.True(node.IsRunning())

	ctx, cancel = context.WithCancel(context.TODO())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Duration(mrand.Uint32()%20) * time.Millisecond) //nolint:gosec
			}
		}
	}()
}

// TestTxGossipingAndAggregation setups a network of nodes, with single aggregator and multiple producers.
// Nodes should gossip transactions and aggregator node should produce blocks.
func TestTxGossipingAndAggregation(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	clientNodes := 4
	nodes, apps := createAndStartNodes(clientNodes, t)
	aggApp := apps[0]
	apps = apps[1:]

	aggApp.AssertNumberOfCalls(t, DeliverTx, clientNodes)
	aggApp.AssertExpectations(t)

	for i, app := range apps {
		app.AssertNumberOfCalls(t, DeliverTx, clientNodes)
		app.AssertExpectations(t)

		// assert that we have most of the blocks from aggregator
		beginCnt := 0
		endCnt := 0
		commitCnt := 0
		for _, call := range app.Calls {
			switch call.Method {
			case BeginBlock:
				beginCnt++
			case EndBlock:
				endCnt++
			case Commit:
				commitCnt++
			}
		}
		aggregatorHeight := nodes[0].Store.Height()
		adjustedHeight := int(aggregatorHeight - 3) // 3 is completely arbitrary
		assert.GreaterOrEqual(beginCnt, adjustedHeight)
		assert.GreaterOrEqual(endCnt, adjustedHeight)
		assert.GreaterOrEqual(commitCnt, adjustedHeight)

		// assert that all blocks known to node are same as produced by aggregator
		for h := uint64(1); h <= nodes[i].Store.Height(); h++ {
			aggBlock, err := nodes[0].Store.LoadBlock(h)
			require.NoError(err)
			nodeBlock, err := nodes[i].Store.LoadBlock(h)
			require.NoError(err)
			assert.Equal(aggBlock, nodeBlock, fmt.Sprintf("height: %d", h))
		}
	}
}

func TestLazyAggregator(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On(InitChain, mock.Anything).Return(abci.ResponseInitChain{})
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	app.On(BeginBlock, mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On(DeliverTx, mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On(EndBlock, mock.Anything).Return(abci.ResponseEndBlock{})
	app.On(Commit, mock.Anything).Return(abci.ResponseCommit{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner()
	blockManagerConfig := config.BlockManagerConfig{
		// After the genesis header is published, the syncer is started
		// which takes little longer (due to initialization) and the syncer
		// tries to retrieve the genesis header and check that is it recent
		// (genesis header time is not older than current minus 1.5x blocktime)
		// to allow sufficient time for syncer initialization, we cannot set
		// the blocktime too short. in future, we can add a configuration
		// in go-header syncer initialization to not rely on blocktime, but the
		// config variable
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := NewNode(ctx, config.NodeConfig{
		DALayer:            "mock",
		Aggregator:         true,
		BlockManagerConfig: blockManagerConfig,
		LazyAggregator:     true,
	}, key, signingKey, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
	assert.False(node.IsRunning())
	assert.NoError(err)

	assert.NoError(node.Start())
	defer func() {
		require.NoError(node.Stop())
	}()
	assert.True(node.IsRunning())

	require.NoError(waitForFirstBlock(node.(*FullNode), Header))

	client := node.GetClient()

	_, err = client.BroadcastTxCommit(context.Background(), []byte{0, 0, 0, 1})
	assert.NoError(err)
	require.NoError(waitForAtLeastNBlocks(node, 2, Header))

	_, err = client.BroadcastTxCommit(context.Background(), []byte{0, 0, 0, 2})
	assert.NoError(err)
	require.NoError(waitForAtLeastNBlocks(node, 3, Header))

	_, err = client.BroadcastTxCommit(context.Background(), []byte{0, 0, 0, 3})
	assert.NoError(err)

	require.NoError(waitForAtLeastNBlocks(node, 4, Header))
}

// TestFastDASync verifies that nodes can sync DA blocks faster than the DA block time
func TestFastDASync(t *testing.T) {
	// Test setup, create require and contexts for aggregator and client nodes
	require := require.New(t)
	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set test params
	clientNodes := 2
	bmConfig := getBMConfig()
	// Set the DABlockTime to a large value to avoid test failures due to
	// slow CI machines
	bmConfig.DABlockTime = 1 * time.Second
	// Set BlockTime to 2x DABlockTime to ensure that the aggregator node is
	// producing DA blocks faster than rollup blocks. This is to force the
	// block syncing to align with hard confirmations.
	bmConfig.BlockTime = 2 * bmConfig.DABlockTime
	const numberOfBlocksToSyncTill = 5

	// Create the 2 nodes
	nodes, _ := createNodes(aggCtx, ctx, clientNodes, bmConfig, t)
	node1 := nodes[0]
	node2 := nodes[1]

	// Start node 1
	require.NoError(node1.Start())
	defer func() {
		require.NoError(node1.Stop())
	}()

	// Wait for node 1 to sync the first numberOfBlocksToSyncTill
	require.NoError(waitForAtLeastNBlocks(node1, numberOfBlocksToSyncTill, Store))

	// Now that node 1 has already synced, start the second node
	require.NoError(node2.Start())
	defer func() {
		require.NoError(node2.Stop())
	}()

	// Start and launch the timer in a go routine to ensure that the test
	// fails if the nodes do not sync before the timer expires
	ch := make(chan struct{})
	defer safeClose(ch)
	// After the first DA block time passes, the node should signal RetrieveLoop once, and it
	// should catch up to the latest block height pretty soon after.
	timer := time.NewTimer(1*bmConfig.DABlockTime + 250*time.Millisecond)
	go func() {
		select {
		case <-ch:
			// Channel closed before timer expired.
			return
		case <-timer.C:
			// Timer expired before channel closed.
			safeClose(ch)
			require.FailNow("nodes did not sync before DA Block time")
			return
		}
	}()

	// Check that the nodes are synced in a loop. We don't use the helper
	// function here so that we can catch if the channel is closed to exit
	// the test quickly.
	require.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
		select {
		case <-ch:
			require.FailNow("channel closed")
		default:
		}
		nHeight, err := getNodeHeight(node2, Store)
		if err != nil {
			return err
		}
		if nHeight >= uint64(numberOfBlocksToSyncTill) {
			return nil
		}
		return fmt.Errorf("expected height > %v, got %v", numberOfBlocksToSyncTill, nHeight)
	}))

	// Verify the nodes are synced
	require.NoError(verifyNodesSynced(node1, node2, Store))

	// Verify that the block we synced to is hard confirmed. This is to
	// ensure that the test is passing due to the DA syncing, since the P2P
	// block sync will sync quickly but the block won't be hard confirmed.
	block, err := node2.Store.LoadBlock(numberOfBlocksToSyncTill)
	require.NoError(err)
	require.True(node2.blockManager.GetHardConfirmation(block.Hash()))
}

// TestSingleAggregatorTwoFullNodesBlockSyncSpeed tests the scenario where the chain's block time is much faster than the DA's block time. In this case, the full nodes should be able to use block sync to sync blocks much faster than syncing from the DA layer, and the test should conclude within block time
func TestSingleAggregatorTwoFullNodesBlockSyncSpeed(t *testing.T) {
	require := require.New(t)
	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientNodes := 3
	bmConfig := getBMConfig()
	bmConfig.BlockTime = 1 * time.Second
	bmConfig.DABlockTime = 10 * time.Second
	const numberOfBlocksTSyncTill = 5

	ch := make(chan struct{})
	defer close(ch)
	timer := time.NewTimer(numberOfBlocksTSyncTill * bmConfig.BlockTime)

	go func() {
		select {
		case <-ch:
			// Channel closed before timer expired.
			return
		case <-timer.C:
			// Timer expired before channel closed.
			t.Error("nodes did not sync before DA Block time")
			return
		}
	}()
	nodes, _ := createNodes(aggCtx, ctx, clientNodes, bmConfig, t)

	node1 := nodes[0]
	node2 := nodes[1]
	node3 := nodes[2]

	require.NoError(node1.Start())
	defer func() {
		require.NoError(node1.Stop())
	}()
	require.NoError(waitForFirstBlock(node1, Store))
	require.NoError(node2.Start())
	defer func() {
		require.NoError(node2.Stop())
	}()
	require.NoError(node3.Start())
	defer func() {
		require.NoError(node3.Stop())
	}()

	require.NoError(waitForAtLeastNBlocks(node2, numberOfBlocksTSyncTill, Store))
	require.NoError(waitForAtLeastNBlocks(node3, numberOfBlocksTSyncTill, Store))

	require.NoError(verifyNodesSynced(node1, node2, Store))
	require.NoError(verifyNodesSynced(node1, node3, Store))
}

func TestBlockExchange(t *testing.T) {
	t.Run("SingleAggregatorSingleFullNode", func(t *testing.T) {
		testSingleAggregatorSingleFullNode(t, Block)
	})
	t.Run("SingleAggregatorSingleFullNode", func(t *testing.T) {
		testSingleAggregatorTwoFullNode(t, Block)
	})
	t.Run("SingleAggregatorSingleFullNode", func(t *testing.T) {
		testSingleAggregatorSingleFullNodeTrustedHash(t, Block)
	})
}

func TestHeaderExchange(t *testing.T) {
	t.Run("SingleAggregatorSingleFullNode", func(t *testing.T) {
		testSingleAggregatorSingleFullNode(t, Header)
	})
	t.Run("SingleAggregatorTwoFullNode", func(t *testing.T) {
		testSingleAggregatorTwoFullNode(t, Header)
	})
	t.Run("SingleAggregatorSingleFullNodeTrustedHash", func(t *testing.T) {
		testSingleAggregatorSingleFullNodeTrustedHash(t, Header)
	})
	t.Run("SingleAggregatorSingleFullNodeSingleLightNode", testSingleAggregatorSingleFullNodeSingleLightNode)
}

func testSingleAggregatorSingleFullNode(t *testing.T, source Source) {
	require := require.New(t)

	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientNodes := 2
	nodes, _ := createNodes(aggCtx, ctx, clientNodes, getBMConfig(), t)

	node1 := nodes[0]
	node2 := nodes[1]

	require.NoError(node1.Start())
	defer func() {
		require.NoError(node1.Stop())
	}()

	require.NoError(waitForFirstBlock(node1, source))
	require.NoError(node2.Start())

	defer func() {
		require.NoError(node2.Stop())
	}()

	require.NoError(waitForAtLeastNBlocks(node2, 2, source))
	require.NoError(verifyNodesSynced(node1, node2, source))
}

func testSingleAggregatorTwoFullNode(t *testing.T, source Source) {
	require := require.New(t)

	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientNodes := 3
	nodes, _ := createNodes(aggCtx, ctx, clientNodes, getBMConfig(), t)

	node1 := nodes[0]
	node2 := nodes[1]
	node3 := nodes[2]

	require.NoError(node1.Start())
	defer func() {
		require.NoError(node1.Stop())
	}()
	require.NoError(waitForFirstBlock(node1, source))
	require.NoError(node2.Start())
	defer func() {
		require.NoError(node2.Stop())
	}()
	require.NoError(node3.Start())
	defer func() {
		require.NoError(node3.Stop())
	}()

	require.NoError(waitForAtLeastNBlocks(node2, 2, source))
	require.NoError(waitForAtLeastNBlocks(node3, 2, source))
	require.NoError(verifyNodesSynced(node1, node2, source))
	require.NoError(verifyNodesSynced(node1, node3, source))
}

func testSingleAggregatorSingleFullNodeTrustedHash(t *testing.T, source Source) {
	require := require.New(t)

	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientNodes := 2
	nodes, _ := createNodes(aggCtx, ctx, clientNodes, getBMConfig(), t)

	node1 := nodes[0]
	node2 := nodes[1]

	require.NoError(node1.Start())
	defer func() {
		require.NoError(node1.Stop())
	}()

	require.NoError(waitForFirstBlock(node1, source))

	// Get the trusted hash from node1 and pass it to node2 config
	trustedHash, err := node1.hSyncService.HeaderStore().GetByHeight(aggCtx, 1)
	require.NoError(err)
	node2.nodeConfig.TrustedHash = trustedHash.Hash().String()
	require.NoError(node2.Start())
	defer func() {
		require.NoError(node2.Stop())
	}()

	require.NoError(waitForAtLeastNBlocks(node1, 2, source))
	require.NoError(verifyNodesSynced(node1, node2, source))
}

func testSingleAggregatorSingleFullNodeSingleLightNode(t *testing.T) {
	require := require.New(t)

	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	num := 3
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	_ = dalc.Start()
	defer func() {
		require.NoError(dalc.Stop())
	}()
	bmConfig := getBMConfig()
	sequencer, _ := createNode(aggCtx, 0, true, false, keys, bmConfig, t)
	fullNode, _ := createNode(ctx, 1, false, false, keys, bmConfig, t)

	sequencer.(*FullNode).dalc = dalc
	sequencer.(*FullNode).blockManager.SetDALC(dalc)
	fullNode.(*FullNode).dalc = dalc
	fullNode.(*FullNode).blockManager.SetDALC(dalc)

	lightNode, _ := createNode(ctx, 2, false, true, keys, bmConfig, t)

	require.NoError(sequencer.Start())
	defer func() {
		require.NoError(sequencer.Stop())
	}()
	require.NoError(fullNode.Start())
	defer func() {
		require.NoError(fullNode.Stop())
	}()
	require.NoError(lightNode.Start())
	defer func() {
		require.NoError(lightNode.Stop())
	}()

	require.NoError(waitForAtLeastNBlocks(sequencer.(*FullNode), 2, Header))
	require.NoError(verifyNodesSynced(sequencer, fullNode, Header))
	require.NoError(verifyNodesSynced(fullNode, lightNode, Header))
}

// Creates a starts the given number of client nodes along with an aggregator node. Uses the given flag to decide whether to have the aggregator produce malicious blocks.
func createAndStartNodes(clientNodes int, t *testing.T) ([]*FullNode, []*mocks.Application) {
	aggCtx, aggCancel := context.WithCancel(context.Background())
	defer aggCancel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodes, apps := createNodes(aggCtx, ctx, clientNodes+1, getBMConfig(), t)
	startNodes(nodes, apps, t)
	defer func() {
		for _, n := range nodes {
			assert.NoError(t, n.Stop())
		}
	}()
	return nodes, apps
}

// Starts the given nodes using the given wait group to synchronize them
// and wait for them to gossip transactions
func startNodes(nodes []*FullNode, apps []*mocks.Application, t *testing.T) {

	// Wait for aggregator node to publish the first block for full nodes to initialize header exchange service
	require.NoError(t, nodes[0].Start())
	require.NoError(t, waitForFirstBlock(nodes[0], Header))
	for i := 1; i < len(nodes); i++ {
		require.NoError(t, nodes[i].Start())
	}

	// wait for nodes to start up and establish connections; 1 second ensures that test pass even on CI.
	for i := 1; i < len(nodes); i++ {
		require.NoError(t, waitForAtLeastNBlocks(nodes[i], 2, Header))
	}

	for i := 1; i < len(nodes); i++ {
		data := strconv.Itoa(i) + time.Now().String()
		require.NoError(t, nodes[i].p2pClient.GossipTx(context.TODO(), []byte(data)))
	}

	timeout := time.NewTimer(time.Second * 30)
	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		// create a MockTester, to catch the Failed asserts from the Mock package
		m := MockTester{t: t}
		// We don't need to check any specific arguments to DeliverTx
		// so just use a function that returns "true" for matching the args
		matcher := mock.MatchedBy(func(i interface{}) bool { return true })
		err := testutils.Retry(300, 100*time.Millisecond, func() error {
			for i := 0; i < len(apps); i++ {
				if !apps[i].AssertCalled(m, DeliverTx, matcher) {
					return errors.New("DeliverTx hasn't been called yet")
				}
			}
			return nil
		})
		assert := assert.New(t)
		assert.NoError(err)
	}()
	select {
	case <-doneChan:
	case <-timeout.C:
		t.FailNow()
	}
}

// Creates the given number of nodes the given nodes using the given wait group to synchornize them
func createNodes(aggCtx, ctx context.Context, num int, bmConfig config.BlockManagerConfig, t *testing.T) ([]*FullNode, []*mocks.Application) {
	t.Helper()

	if aggCtx == nil {
		aggCtx = context.Background()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// create keys first, as they are required for P2P connections
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}

	nodes := make([]*FullNode, num)
	apps := make([]*mocks.Application, num)
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, test.NewFileLoggerCustom(t, test.TempLogFileName(t, "dalc")))
	_ = dalc.Start()
	node, app := createNode(aggCtx, 0, true, false, keys, bmConfig, t)
	apps[0] = app
	nodes[0] = node.(*FullNode)
	// use same, common DALC, so nodes can share data
	nodes[0].dalc = dalc
	nodes[0].blockManager.SetDALC(dalc)
	for i := 1; i < num; i++ {
		node, apps[i] = createNode(ctx, i, false, false, keys, bmConfig, t)
		nodes[i] = node.(*FullNode)
		nodes[i].dalc = dalc
		nodes[i].blockManager.SetDALC(dalc)
	}

	return nodes, apps
}

func createNode(ctx context.Context, n int, aggregator bool, isLight bool, keys []crypto.PrivKey, bmConfig config.BlockManagerConfig, t *testing.T) (Node, *mocks.Application) {
	t.Helper()
	require := require.New(t)
	// nodes will listen on consecutive ports on local interface
	// random connections to other nodes will be added
	startPort := 10000
	p2pConfig := config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+n),
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
	app.On(InitChain, mock.Anything).Return(abci.ResponseInitChain{})
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	app.On(BeginBlock, mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On(EndBlock, mock.Anything).Return(abci.ResponseEndBlock{})
	app.On(Commit, mock.Anything).Return(abci.ResponseCommit{})
	app.On(DeliverTx, mock.Anything).Return(abci.ResponseDeliverTx{})

	if ctx == nil {
		ctx = context.Background()
	}

	genesisValidators, signingKey := getGenesisValidatorSetWithSigner()
	genesis := &cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}
	// TODO: need to investigate why this needs to be done for light nodes
	genesis.InitialHeight = 1
	node, err := NewNode(
		ctx,
		config.NodeConfig{
			P2P:                p2pConfig,
			DALayer:            "mock",
			Aggregator:         aggregator,
			BlockManagerConfig: bmConfig,
			Light:              isLight,
		},
		keys[n],
		signingKey,
		proxy.NewLocalClientCreator(app),
		genesis,
		test.NewFileLoggerCustom(t, test.TempLogFileName(t, fmt.Sprintf("node%v", n))).With("node", n))
	require.NoError(err)
	require.NotNil(node)

	return node, app
}
