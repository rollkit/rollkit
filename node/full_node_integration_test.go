package node

import (
	"context"
	"crypto/rand"
	"fmt"
	mrand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/config"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/mocks"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

func TestAggregatorMode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	anotherKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}
	node, err := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock", Aggregator: true, BlockManagerConfig: blockManagerConfig}, key, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())

	pid, err := peer.IDFromPrivateKey(anotherKey)
	require.NoError(err)
	ctx, cancel := context.WithCancel(context.TODO())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				node.incomingTxCh <- &p2p.GossipMessage{Data: []byte(time.Now().String()), From: pid}
				time.Sleep(time.Duration(mrand.Uint32()%20) * time.Millisecond) //nolint:gosec
			}
		}
	}()
	time.Sleep(3 * time.Second)
	cancel()
}

// TestTxGossipingAndAggregation setups a network of nodes, with single aggregator and multiple producers.
// Nodes should gossip transactions and aggregator node should produce blocks.
func TestTxGossipingAndAggregation(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	clientNodes := 4
	nodes, apps := createAndStartNodes(clientNodes, false, t)
	aggApp := apps[0]
	apps = apps[1:]

	aggApp.AssertNumberOfCalls(t, "DeliverTx", clientNodes)
	aggApp.AssertExpectations(t)

	for i, app := range apps {
		app.AssertNumberOfCalls(t, "DeliverTx", clientNodes)
		app.AssertExpectations(t)

		// assert that we have most of the blocks from aggregator
		beginCnt := 0
		endCnt := 0
		commitCnt := 0
		for _, call := range app.Calls {
			switch call.Method {
			case "BeginBlock":
				beginCnt++
			case "EndBlock":
				endCnt++
			case "Commit":
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
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}

	node, err := NewNode(context.Background(), config.NodeConfig{
		DALayer:            "mock",
		Aggregator:         true,
		BlockManagerConfig: blockManagerConfig,
		LazyAggregator:     true,
	}, key, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
	assert.False(node.IsRunning())
	assert.NoError(err)
	err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())

	require.NoError(err)

	// delay to ensure it waits for block 1 to be built
	time.Sleep(1 * time.Second)

	client := node.GetClient()

	_, err = client.BroadcastTxCommit(context.Background(), []byte{0, 0, 0, 1})
	assert.NoError(err)
	time.Sleep(2 * time.Second)
	assert.Equal(node.(*FullNode).Store.Height(), uint64(2))

	_, err = client.BroadcastTxCommit(context.Background(), []byte{0, 0, 0, 2})
	assert.NoError(err)
	time.Sleep(2 * time.Second)
	assert.Equal(node.(*FullNode).Store.Height(), uint64(3))

	_, err = client.BroadcastTxCommit(context.Background(), []byte{0, 0, 0, 3})
	assert.NoError(err)
	time.Sleep(2 * time.Second)
	assert.Equal(node.(*FullNode).Store.Height(), uint64(4))

}

func TestHeaderExchange(t *testing.T) {
	testSingleAggreatorSingleFullNode(t)
	testSingleAggreatorTwoFullNode(t)
	testSingleAggreatorSingleFullNodeTrustedHash(t)
	testSingleAggreatorSingleFullNodeSingleLightNode(t)
}

func testSingleAggreatorSingleFullNode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	clientNodes := 1
	nodes, _ := createNodes(aggCtx, ctx, clientNodes+1, false, &wg, t)

	node1 := nodes[0]
	node2 := nodes[1]

	require.NoError(node1.Start())
	time.Sleep(2 * time.Second) // wait for more than 1 blocktime for syncer to work
	require.NoError(node2.Start())

	time.Sleep(3 * time.Second)

	n1h := node1.hExService.headerStore.Height()
	aggCancel()
	require.NoError(node1.Stop())

	time.Sleep(3 * time.Second)

	n2h := node2.hExService.headerStore.Height()
	cancel()
	require.NoError(node2.Stop())

	assert.Equal(n1h, n2h, "heights must match")
}

func testSingleAggreatorTwoFullNode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	clientNodes := 2
	nodes, _ := createNodes(aggCtx, ctx, clientNodes+1, false, &wg, t)

	node1 := nodes[0]
	node2 := nodes[1]
	node3 := nodes[2]

	require.NoError(node1.Start())
	time.Sleep(2 * time.Second) // wait for more than 1 blocktime for syncer to work
	require.NoError(node2.Start())
	require.NoError(node3.Start())

	time.Sleep(3 * time.Second)

	n1h := node1.hExService.headerStore.Height()
	aggCancel()
	require.NoError(node1.Stop())

	time.Sleep(3 * time.Second)

	n2h := node2.hExService.headerStore.Height()
	cancel()
	require.NoError(node2.Stop())

	n3h := node3.hExService.headerStore.Height()
	require.NoError(node3.Stop())

	assert.Equal(n1h, n2h, "heights must match")
	assert.Equal(n1h, n3h, "heights must match")
}

func testSingleAggreatorSingleFullNodeTrustedHash(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	clientNodes := 1
	nodes, _ := createNodes(aggCtx, ctx, clientNodes+1, false, &wg, t)

	node1 := nodes[0]
	node2 := nodes[1]

	require.NoError(node1.Start())
	time.Sleep(2 * time.Second) // wait for more than 1 blocktime for syncer to work
	// Get the trusted hash from node1 and pass it to node2 config
	trustedHash, err := node1.hExService.headerStore.GetByHeight(aggCtx, 1)
	require.NoError(err)
	node2.conf.TrustedHash = trustedHash.Hash().String()
	require.NoError(node2.Start())

	time.Sleep(3 * time.Second)

	n1h := node1.hExService.headerStore.Height()
	aggCancel()
	require.NoError(node1.Stop())

	time.Sleep(3 * time.Second)

	n2h := node2.hExService.headerStore.Height()
	cancel()
	require.NoError(node2.Stop())

	assert.Equal(n1h, n2h, "heights must match")
}

func testSingleAggreatorSingleFullNodeSingleLightNode(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())

	num := 3
	keys := make([]crypto.PrivKey, num)
	for i := 0; i < num; i++ {
		keys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
	}
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	_ = dalc.Start()
	sequencer, _ := createNode(aggCtx, 0, false, true, false, keys, &wg, t)
	fullNode, _ := createNode(ctx, 1, false, false, false, keys, &wg, t)

	sequencer.(*FullNode).dalc = dalc
	sequencer.(*FullNode).blockManager.SetDALC(dalc)
	fullNode.(*FullNode).dalc = dalc
	fullNode.(*FullNode).blockManager.SetDALC(dalc)

	lightNode, _ := createNode(ctx, 2, false, false, true, keys, &wg, t)

	require.NoError(sequencer.Start())
	require.NoError(fullNode.Start())
	require.NoError(lightNode.Start())

	time.Sleep(3 * time.Second)

	n1h := sequencer.(*FullNode).hExService.headerStore.Height()
	aggCancel()
	require.NoError(sequencer.Stop())

	time.Sleep(3 * time.Second)

	n2h := fullNode.(*FullNode).hExService.headerStore.Height()
	n3h := lightNode.(*LightNode).hExService.headerStore.Height()
	cancel()
	require.NoError(fullNode.Stop())
	require.NoError(lightNode.Stop())

	assert.Equal(n1h, n2h, "heights must match")
	assert.Equal(n1h, n3h, "heights must match")
}

func testSingleAggreatorSingleFullNodeFraudProofGossip(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	clientNodes := 1
	nodes, apps := createNodes(aggCtx, ctx, clientNodes+1, true, &wg, t)

	for _, app := range apps {
		app.On("VerifyFraudProof", mock.Anything).Return(abci.ResponseVerifyFraudProof{Success: true}).Run(func(args mock.Arguments) {
			wg.Done()
		}).Once()
	}

	aggNode := nodes[0]
	fullNode := nodes[1]

	wg.Add(clientNodes + 1)
	require.NoError(aggNode.Start())
	time.Sleep(2 * time.Second)
	require.NoError(fullNode.Start())

	wg.Wait()
	// aggregator should have 0 GenerateFraudProof calls and 1 VerifyFraudProof calls
	apps[0].AssertNumberOfCalls(t, "GenerateFraudProof", 0)
	apps[0].AssertNumberOfCalls(t, "VerifyFraudProof", 1)
	// fullnode should have 1 GenerateFraudProof calls and 1 VerifyFraudProof calls
	apps[1].AssertNumberOfCalls(t, "GenerateFraudProof", 1)
	apps[1].AssertNumberOfCalls(t, "VerifyFraudProof", 1)

	n1Frauds, err := aggNode.fraudService.Get(aggCtx, types.StateFraudProofType)
	require.NoError(err)
	aggCancel()
	require.NoError(aggNode.Stop())

	n2Frauds, err := fullNode.fraudService.Get(aggCtx, types.StateFraudProofType)
	require.NoError(err)
	cancel()
	require.NoError(fullNode.Stop())

	assert.Equal(len(n1Frauds), 1, "number of fraud proofs received via gossip should be 1")
	assert.Equal(len(n2Frauds), 1, "number of fraud proofs received via gossip should be 1")
	assert.Equal(n1Frauds, n2Frauds, "the received fraud proofs after gossip must match")
}

func testSingleAggreatorTwoFullNodeFraudProofSync(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	clientNodes := 2
	nodes, apps := createNodes(aggCtx, ctx, clientNodes+1, true, &wg, t)

	for _, app := range apps {
		app.On("VerifyFraudProof", mock.Anything).Return(abci.ResponseVerifyFraudProof{Success: true}).Run(func(args mock.Arguments) {
			wg.Done()
		}).Once()
	}

	aggNode := nodes[0]
	fullNode1 := nodes[1]
	fullNode2 := nodes[2]

	wg.Add(clientNodes)
	require.NoError(aggNode.Start())
	time.Sleep(2 * time.Second)
	require.NoError(fullNode1.Start())

	wg.Wait()
	// aggregator should have 0 GenerateFraudProof calls and 1 VerifyFraudProof calls
	apps[0].AssertNumberOfCalls(t, "GenerateFraudProof", 0)
	apps[0].AssertNumberOfCalls(t, "VerifyFraudProof", 1)
	// fullnode1 should have 1 GenerateFraudProof calls and 1 VerifyFraudProof calls
	apps[1].AssertNumberOfCalls(t, "GenerateFraudProof", 1)
	apps[1].AssertNumberOfCalls(t, "VerifyFraudProof", 1)

	n1Frauds, err := aggNode.fraudService.Get(aggCtx, types.StateFraudProofType)
	require.NoError(err)

	n2Frauds, err := fullNode1.fraudService.Get(aggCtx, types.StateFraudProofType)
	require.NoError(err)
	assert.Equal(n1Frauds, n2Frauds, "number of fraud proofs gossiped between nodes must match")

	wg.Add(1)
	// delay start node3 such that it can sync the fraud proof from peers, instead of listening to gossip
	require.NoError(fullNode2.Start())

	wg.Wait()
	// fullnode2 should have 1 GenerateFraudProof calls and 1 VerifyFraudProof calls
	apps[2].AssertNumberOfCalls(t, "GenerateFraudProof", 1)
	apps[2].AssertNumberOfCalls(t, "VerifyFraudProof", 1)

	n3Frauds, err := fullNode2.fraudService.Get(aggCtx, types.StateFraudProofType)
	require.NoError(err)
	assert.Equal(n1Frauds, n3Frauds, "number of fraud proofs gossiped between nodes must match")

	aggCancel()
	require.NoError(aggNode.Stop())
	cancel()
	require.NoError(fullNode1.Stop())
	require.NoError(fullNode2.Stop())
}

func TestFraudProofService(t *testing.T) {
	testSingleAggreatorSingleFullNodeFraudProofGossip(t)
	testSingleAggreatorTwoFullNodeFraudProofSync(t)
}

// TODO: rewrite this integration test to accommodate gossip/halting mechanism of full nodes after fraud proof generation (#693)
// TestFraudProofTrigger setups a network of nodes, with single malicious aggregator and multiple producers.
// Aggregator node should produce malicious blocks, nodes should detect fraud, and generate fraud proofs
/* func TestFraudProofTrigger(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	clientNodes := 4
	nodes, apps := createAndStartNodes(clientNodes, true, t)
	aggApp := apps[0]
	apps = apps[1:]

	aggApp.AssertNumberOfCalls(t, "DeliverTx", clientNodes)
	aggApp.AssertExpectations(t)

	for i, app := range apps {
		//app.AssertNumberOfCalls(t, "DeliverTx", clientNodes)
		app.AssertExpectations(t)

		// assert that we have most of the blocks from aggregator
		beginCnt := 0
		endCnt := 0
		commitCnt := 0
		generateFraudProofCnt := 0
		for _, call := range app.Calls {
			switch call.Method {
			case "BeginBlock":
				beginCnt++
			case "EndBlock":
				endCnt++
			case "Commit":
				commitCnt++
			case "GenerateFraudProof":
				generateFraudProofCnt++
			}
		}
		aggregatorHeight := nodes[0].Store.Height()
		adjustedHeight := int(aggregatorHeight - 3) // 3 is completely arbitrary
		assert.GreaterOrEqual(beginCnt, adjustedHeight)
		assert.GreaterOrEqual(endCnt, adjustedHeight)
		assert.GreaterOrEqual(commitCnt, adjustedHeight)

		// GenerateFraudProof should have been called on each call to
		// BeginBlock, DeliverTx, and EndBlock so the sum of their counts
		// should be equal.
		// Note: The value of clientNodes represents number of calls to DeliverTx
		assert.Equal(beginCnt+clientNodes+endCnt, generateFraudProofCnt)

		// assert that all blocks known to node are same as produced by aggregator
		for h := uint64(1); h <= nodes[i].Store.Height(); h++ {
			nodeBlock, err := nodes[i].Store.LoadBlock(h)
			require.NoError(err)
			aggBlock, err := nodes[0].Store.LoadBlock(h)
			require.NoError(err)
			assert.Equal(aggBlock, nodeBlock)
		}
	}
} */
