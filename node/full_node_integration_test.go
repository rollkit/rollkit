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

	cmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
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
	"github.com/rollkit/rollkit/da"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"

	testutils "github.com/celestiaorg/utils/test"
)

func prepareProposalResponse(_ context.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	return &abci.ResponsePrepareProposal{
		Txs: req.Txs,
	}, nil
}

func TestAggregatorMode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	blockManagerConfig := config.BlockManagerConfig{
		BlockTime: 1 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := newFullNode(ctx, config.NodeConfig{DAAddress: MockDAAddress, DANamespace: MockDANamespace, Aggregator: true, BlockManagerConfig: blockManagerConfig}, key, signingKey, proxy.NewLocalClientCreator(app), genesisDoc, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	startNodeWithCleanup(t, node)

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
	aggCtx := context.Background()
	ctx := context.Background()
	nodes, apps := createNodes(aggCtx, ctx, clientNodes+1, getBMConfig(), t)
	startNodes(nodes, apps, t)
	defer func() {
		for _, n := range nodes {
			assert.NoError(n.Stop())
		}
	}()

	// wait for nodes to start up and sync up till numBlocksToWaitFor
	numBlocksToWaitFor := 5
	for i := 1; i < len(nodes); i++ {
		require.NoError(waitForAtLeastNBlocks(nodes[i], numBlocksToWaitFor, Store))
	}

	// Cancel all the nodes before checking the calls to ABCI methods were done correctly
	// Can't stop here, because we need access to Store to test the state.
	for _, node := range nodes {
		node.Cancel()
	}

	// Now that the nodes are cancelled, it should be safe to access the mock
	// calls outside the mutex controlled methods.
	//
	// The reason we do this is because in the beginning of the test, we
	// check that we have produced at least N blocks, which means we could
	// have over produced. So when checking the calls, we also want to check
	// that we called FinalizeBlock at least N times.
	aggApp := apps[0]
	apps = apps[1:]

	checkCalls := func(app *mocks.Application, numBlocks int) error {
		calls := app.Calls
		numCalls := 0
		for _, call := range calls {
			if call.Method == "FinalizeBlock" {
				numCalls++
			}
		}
		if numBlocks > numCalls {
			return fmt.Errorf("expected at least %d calls to FinalizeBlock, got %d calls", numBlocks, numCalls)
		}
		return nil
	}

	require.NoError(checkCalls(aggApp, numBlocksToWaitFor))
	aggApp.AssertExpectations(t)

	for i, app := range apps {
		require.NoError(checkCalls(app, numBlocksToWaitFor))
		app.AssertExpectations(t)

		// assert that all blocks known to node are same as produced by aggregator
		for h := uint64(1); h <= nodes[i].Store.Height(); h++ {
			aggBlock, err := nodes[0].Store.GetBlock(ctx, h)
			require.NoError(err)
			nodeBlock, err := nodes[i].Store.GetBlock(ctx, h)
			require.NoError(err)
			assert.Equal(aggBlock, nodeBlock, fmt.Sprintf("height: %d", h))
		}
	}
}

func TestLazyAggregator(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	blockManagerConfig := config.BlockManagerConfig{
		// After the genesis header is published, the syncer is started
		// which takes little longer (due to initialization) and the syncer
		// tries to retrieve the genesis header and check that is it recent
		// (genesis header time is not older than current minus 1.5x blocktime)
		// to allow sufficient time for syncer initialization, we cannot set
		// the blocktime too short. in future, we can add a configuration
		// in go-header syncer initialization to not rely on blocktime, but the
		// config variable
		BlockTime: 1 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := NewNode(ctx, config.NodeConfig{
		DAAddress:          MockDAAddress,
		DANamespace:        MockDANamespace,
		Aggregator:         true,
		BlockManagerConfig: blockManagerConfig,
		LazyAggregator:     true,
	}, key, signingKey, proxy.NewLocalClientCreator(app), genesisDoc, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), log.TestingLogger())
	assert.False(node.IsRunning())
	assert.NoError(err)

	startNodeWithCleanup(t, node)
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
	// block syncing to align with DA inclusions.
	bmConfig.BlockTime = 2 * bmConfig.DABlockTime
	const numberOfBlocksToSyncTill = 5

	// Create the 2 nodes
	nodes, _ := createNodes(aggCtx, ctx, clientNodes, bmConfig, t)

	node1 := nodes[0]
	node2 := nodes[1]

	// Start node 1
	startNodeWithCleanup(t, node1)

	// Wait for node 1 to sync the first numberOfBlocksToSyncTill
	require.NoError(waitForAtLeastNBlocks(node1, numberOfBlocksToSyncTill, Store))

	// Now that node 1 has already synced, start the second node
	startNodeWithCleanup(t, node2)

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

	// Verify that the block we synced to is DA included. This is to
	// ensure that the test is passing due to the DA syncing, since the P2P
	// block sync will sync quickly but the block won't be DA included.
	block, err := node2.Store.GetBlock(ctx, numberOfBlocksToSyncTill)
	require.NoError(err)
	require.True(node2.blockManager.IsDAIncluded(block.Hash()))
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

	startNodeWithCleanup(t, node1)
	require.NoError(waitForFirstBlock(node1, Store))

	startNodeWithCleanup(t, node2)
	startNodeWithCleanup(t, node3)

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

func TestSubmitBlocksToDA(t *testing.T) {
	require := require.New(t)

	clientNodes := 1
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodes, _ := createNodes(
		ctx,
		context.Background(),
		clientNodes,
		config.BlockManagerConfig{
			DABlockTime: 20 * time.Millisecond,
			BlockTime:   10 * time.Millisecond,
		},
		t,
	)
	seq := nodes[0]
	startNodeWithCleanup(t, seq)

	numberOfBlocksToSyncTill := seq.Store.Height()

	//Make sure all produced blocks made it to DA
	for i := uint64(1); i <= numberOfBlocksToSyncTill; i++ {
		require.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
			block, err := seq.Store.GetBlock(ctx, i)
			if err != nil {
				return err
			}
			if !seq.blockManager.IsDAIncluded(block.Hash()) {
				return fmt.Errorf("block %d not DA included", block.Height())
			}
			return nil
		}))
	}
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

	startNodeWithCleanup(t, node1)
	require.NoError(waitForFirstBlock(node1, source))

	startNodeWithCleanup(t, node2)
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

	startNodeWithCleanup(t, node1)
	require.NoError(waitForFirstBlock(node1, source))

	startNodeWithCleanup(t, node2)
	startNodeWithCleanup(t, node3)

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

	startNodeWithCleanup(t, node1)
	require.NoError(waitForFirstBlock(node1, source))

	// Get the trusted hash from node1 and pass it to node2 config
	trustedHash, err := node1.hSyncService.HeaderStore().GetByHeight(aggCtx, 1)
	require.NoError(err)

	node2.nodeConfig.TrustedHash = trustedHash.Hash().String()
	startNodeWithCleanup(t, node2)

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
	dalc := getMockDA(t)
	bmConfig := getBMConfig()
	sequencer, _ := createAndConfigureNode(aggCtx, 0, true, false, keys, bmConfig, dalc, t)
	fullNode, _ := createAndConfigureNode(ctx, 1, false, false, keys, bmConfig, dalc, t)
	lightNode, _ := createNode(ctx, 2, false, true, keys, bmConfig, t)

	startNodeWithCleanup(t, sequencer)
	startNodeWithCleanup(t, fullNode)
	startNodeWithCleanup(t, lightNode)

	require.NoError(waitForAtLeastNBlocks(sequencer.(*FullNode), 2, Header))
	require.NoError(verifyNodesSynced(sequencer, fullNode, Header))
	require.NoError(verifyNodesSynced(fullNode, lightNode, Header))
}

func getMockApplication() *mocks.Application {
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	return app
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
		// We don't need to check any specific arguments to FinalizeBlock
		// so just use a function that returns "true" for matching the args
		matcher := mock.MatchedBy(func(i interface{}) bool { return true })
		err := testutils.Retry(300, 100*time.Millisecond, func() error {
			for i := 0; i < len(apps); i++ {
				if !apps[i].AssertCalled(m, "FinalizeBlock", matcher, matcher) {
					return errors.New("FinalizeBlock hasn't been called yet")
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
	dalc := getMockDA(t)
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
		p2pConfig.Seeds += "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+r) + "/p2p/" + id.Loggable()["peerID"].(string) + ","
	}
	p2pConfig.Seeds = strings.TrimSuffix(p2pConfig.Seeds, ",")

	app := getMockApplication()

	if ctx == nil {
		ctx = context.Background()
	}

	pubkeyBytes, err := keys[0].GetPublic().Raw()
	require.NoError(err)
	var pubkey ed25519.PubKey = pubkeyBytes
	genesisValidators := []cmtypes.GenesisValidator{
		{
			Address: pubkey.Address(),
			PubKey:  pubkey,
			Power:   int64(1),
			Name:    "sequencer",
		},
	}

	genesis := &cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}
	// TODO: need to investigate why this needs to be done for light nodes
	genesis.InitialHeight = 1
	node, err := NewNode(
		ctx,
		config.NodeConfig{
			DAAddress:          MockDAAddress,
			DANamespace:        MockDANamespace,
			P2P:                p2pConfig,
			Aggregator:         aggregator,
			BlockManagerConfig: bmConfig,
			Light:              isLight,
		},
		keys[n],
		keys[n],
		proxy.NewLocalClientCreator(app),
		genesis,
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
		test.NewFileLoggerCustom(t, test.TempLogFileName(t, fmt.Sprintf("node%v", n))).With("node", n))
	require.NoError(err)
	require.NotNil(node)

	return node, app
}

func createAndConfigureNode(ctx context.Context, n int, aggregator bool, isLight bool, keys []crypto.PrivKey, bmConfig config.BlockManagerConfig, dalc *da.DAClient, t *testing.T) (Node, *mocks.Application) {
	t.Helper()
	node, app := createNode(ctx, n, aggregator, isLight, keys, bmConfig, t)
	node.(*FullNode).dalc = dalc
	node.(*FullNode).blockManager.SetDALC(dalc)

	return node, app
}
