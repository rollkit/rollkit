package node

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tendermint/tendermint/crypto/encoding"

	abciconv "github.com/rollkit/rollkit/conv/abci"
	"github.com/rollkit/rollkit/types"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/p2p"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/conv"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/mocks"

	tmcrypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/store"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

var mockTxProcessingTime = 10 * time.Millisecond

var genesisValidatorKey = ed25519.GenPrivKey()

// TODO: accept argument for number of validators / proposer index
func getRandomValidatorSet() *tmtypes.ValidatorSet {
	pubKey := ed25519.GenPrivKey().PubKey()
	return &tmtypes.ValidatorSet{
		Proposer: &tmtypes.Validator{PubKey: pubKey, Address: pubKey.Address()},
		Validators: []*tmtypes.Validator{
			{PubKey: pubKey, Address: pubKey.Address()},
		},
	}
}

// TODO: use n and return n validators
func getGenesisValidatorSetWithSigner(n int) ([]tmtypes.GenesisValidator, crypto.PrivKey) {
	nodeKey := &p2p.NodeKey{
		PrivKey: genesisValidatorKey,
	}
	signingKey, _ := conv.GetNodeKey(nodeKey)
	pubKey := genesisValidatorKey.PubKey()

	genesisValidators := []tmtypes.GenesisValidator{
		{Address: pubKey.Address(), PubKey: pubKey, Power: int64(100), Name: "gen #1"},
	}
	return genesisValidators, signingKey
}

func createGenesisValidators(numNodes int, appCreator func(require *require.Assertions, vKeyToRemove tmcrypto.PrivKey, wg *sync.WaitGroup) *mocks.Application, require *require.Assertions, wg *sync.WaitGroup) *FullClient {
	vKeys := make([]tmcrypto.PrivKey, numNodes)
	apps := make([]*mocks.Application, numNodes)
	nodes := make([]*FullNode, numNodes)

	genesisValidators := make([]tmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		genesisValidators[i] = tmtypes.GenesisValidator{Address: vKeys[i].PubKey().Address(), PubKey: vKeys[i].PubKey(), Power: int64(i + 100), Name: fmt.Sprintf("gen #%d", i)}
		apps[i] = appCreator(require, vKeys[0], wg)
		wg.Add(1)
	}

	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, err := store.NewDefaultInMemoryKVStore()
	require.Nil(err)
	err = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	require.Nil(err)
	err = dalc.Start()
	require.Nil(err)

	for i := 0; i < len(nodes); i++ {
		nodeKey := &p2p.NodeKey{
			PrivKey: vKeys[i],
		}
		signingKey, err := conv.GetNodeKey(nodeKey)
		require.NoError(err)
		nodes[i], err = newFullNode(
			context.Background(),
			config.NodeConfig{
				DALayer:    "mock",
				Aggregator: true,
				BlockManagerConfig: config.BlockManagerConfig{
					BlockTime:   1 * time.Second,
					DABlockTime: 100 * time.Millisecond,
				},
			},
			signingKey,
			signingKey,
			proxy.NewLocalClientCreator(apps[i]),
			&tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators},
			log.TestingLogger(),
		)
		require.NoError(err)
		require.NotNil(nodes[i])

		// use same, common DALC, so nodes can share data
		nodes[i].dalc = dalc
		nodes[i].blockManager.SetDALC(dalc)
	}

	rpc := NewFullClient(nodes[0])
	require.NotNil(rpc)

	for i := 0; i < len(nodes); i++ {
		err := nodes[i].Start()
		require.NoError(err)
	}
	return rpc
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	return getRandomBlockWithProposer(height, nTxs, getRandomBytes(20))
}

func getRandomBlockWithProposer(height uint64, nTxs int, proposerAddr []byte) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				Version:         types.Version{Block: types.InitStateVersion.Consensus.Block},
				ProposerAddress: proposerAddr,
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.SignedHeader.Header.AppHash = getRandomBytes(32)

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	tmprotoLC, err := tmtypes.CommitFromProto(&tmproto.Commit{})
	if err != nil {
		return nil
	}
	lastCommitHash := make(types.Hash, 32)
	copy(lastCommitHash, tmprotoLC.Hash().Bytes())
	block.SignedHeader.Header.LastCommitHash = lastCommitHash

	block.SignedHeader.Validators = getRandomValidatorSet()

	return block
}

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = crand.Read(data)
	return data
}

// From state/indexer/block/kv/kv_test
func indexBlocks(t *testing.T, rpc *FullClient, heights []int64) {
	t.Helper()

	for _, h := range heights {
		require.NoError(t, rpc.node.BlockIndexer.Index(tmtypes.EventDataNewBlockHeader{
			Header: tmtypes.Header{Height: h},
			ResultBeginBlock: abci.ResponseBeginBlock{
				Events: []abci.Event{
					{
						Type: "begin_event",
						Attributes: []abci.EventAttribute{
							{
								Key:   []byte("proposer"),
								Value: []byte("FCAA001"),
								Index: true,
							},
						},
					},
				},
			},
			ResultEndBlock: abci.ResponseEndBlock{
				Events: []abci.Event{
					{
						Type: "end_event",
						Attributes: []abci.EventAttribute{
							{
								Key:   []byte("foo"),
								Value: []byte(fmt.Sprintf("%d", h)),
								Index: true,
							},
						},
					},
				},
			},
		}))
	}

}

func getBlockMeta(rpc *FullClient, n int64) *tmtypes.BlockMeta {
	b, err := rpc.node.Store.LoadBlock(uint64(n))
	if err != nil {
		return nil
	}
	bmeta, err := abciconv.ToABCIBlockMeta(b)
	if err != nil {
		return nil
	}

	return bmeta
}

func getRPC(t *testing.T) (*mocks.Application, *FullClient) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	node, err := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, signingKey, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewFullClient(node)
	require.NotNil(rpc)

	return app, rpc
}

func checkValSet(rpc *FullClient, assert *assert.Assertions, h int64, expectedValCount int) {
	vals, err := rpc.Validators(context.Background(), &h, nil, nil)
	assert.NoError(err)
	assert.NotNil(vals)
	assert.EqualValues(expectedValCount, vals.Total)
	assert.Len(vals.Validators, expectedValCount)
	assert.EqualValues(vals.BlockHeight, h)
}

func checkValSetLatest(rpc *FullClient, assert *assert.Assertions, lastBlockHeight int64, expectedValCount int) {
	vals, err := rpc.Validators(context.Background(), nil, nil, nil)
	assert.NoError(err)
	assert.NotNil(vals)
	assert.EqualValues(expectedValCount, vals.Total)
	assert.Len(vals.Validators, expectedValCount)
	assert.GreaterOrEqual(vals.BlockHeight, lastBlockHeight)
}

func createApp(require *require.Assertions, vKeyToRemove tmcrypto.PrivKey, wg *sync.WaitGroup) *mocks.Application {
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})
	app.On("GenerateFraudProof", mock.Anything).Return(abci.ResponseGenerateFraudProof{})

	pbValKey, err := encoding.PubKeyToProto(vKeyToRemove.PubKey())
	require.NoError(err)

	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Times(2)
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{ValidatorUpdates: []abci.ValidatorUpdate{{PubKey: pbValKey, Power: 0}}}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{ValidatorUpdates: []abci.ValidatorUpdate{{PubKey: pbValKey, Power: 100}}}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Times(5)
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Run(func(args mock.Arguments) {
		wg.Done()
	}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	return app
}

// Creates a starts the given number of client nodes along with an aggregator node. Uses the given flag to decide whether to have the aggregator produce malicious blocks.
func createAndStartNodes(clientNodes int, isMalicious bool, t *testing.T) ([]*FullNode, []*mocks.Application) {
	var wg sync.WaitGroup
	aggCtx, aggCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	nodes, apps := createNodes(aggCtx, ctx, clientNodes+1, isMalicious, &wg, t)
	startNodes(nodes, &wg, t)
	aggCancel()
	time.Sleep(100 * time.Millisecond)
	for _, n := range nodes {
		require.NoError(t, n.Stop())
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
	return nodes, apps
}

// Starts the given nodes using the given wait group to synchronize them
// and wait for them to gossip transactions
func startNodes(nodes []*FullNode, wg *sync.WaitGroup, t *testing.T) {
	numNodes := len(nodes)
	wg.Add((numNodes) * (numNodes - 1))

	// Wait for aggregator node to publish the first block for full nodes to initialize header exchange service
	require.NoError(t, nodes[0].Start())
	time.Sleep(1 * time.Second)
	for i := 1; i < len(nodes); i++ {
		require.NoError(t, nodes[i].Start())
	}

	// wait for nodes to start up and establish connections; 1 second ensures that test pass even on CI.
	time.Sleep(1 * time.Second)

	for i := 1; i < len(nodes); i++ {
		data := strconv.Itoa(i) + time.Now().String()
		require.NoError(t, nodes[i].P2P.GossipTx(context.TODO(), []byte(data)))
	}

	timeout := time.NewTimer(time.Second * 30)
	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		wg.Wait()
	}()
	select {
	case <-doneChan:
	case <-timeout.C:
		t.FailNow()
	}
}

// Creates the given number of nodes the given nodes using the given wait group to synchornize them
func createNodes(aggCtx, ctx context.Context, num int, isMalicious bool, wg *sync.WaitGroup, t *testing.T) ([]*FullNode, []*mocks.Application) {
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
		keys[i], _, _ = crypto.GenerateEd25519Key(crand.Reader)
	}

	nodes := make([]*FullNode, num)
	apps := make([]*mocks.Application, num)
	dalc := &mockda.DataAvailabilityLayerClient{}
	ds, _ := store.NewDefaultInMemoryKVStore()
	_ = dalc.Init([8]byte{}, nil, ds, log.TestingLogger())
	_ = dalc.Start()
	node, app := createNode(aggCtx, 0, isMalicious, true, false, keys, wg, t)
	apps[0] = app
	nodes[0] = node.(*FullNode)
	// use same, common DALC, so nodes can share data
	nodes[0].dalc = dalc
	nodes[0].blockManager.SetDALC(dalc)
	for i := 1; i < num; i++ {
		node, apps[i] = createNode(ctx, i, isMalicious, false, false, keys, wg, t)
		nodes[i] = node.(*FullNode)
		nodes[i].dalc = dalc
		nodes[i].blockManager.SetDALC(dalc)
	}

	return nodes, apps
}

func createNode(ctx context.Context, n int, isMalicious bool, aggregator bool, isLight bool, keys []crypto.PrivKey, wg *sync.WaitGroup, t *testing.T) (Node, *mocks.Application) {
	t.Helper()
	require := require.New(t)
	// nodes will listen on consecutive ports on local interface
	// random connections to other nodes will be added
	startPort := 10000
	p2pConfig := config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/" + strconv.Itoa(startPort+n),
	}
	bmConfig := config.BlockManagerConfig{
		DABlockTime: 100 * time.Millisecond,
		BlockTime:   1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		NamespaceID: types.NamespaceID{8, 7, 6, 5, 4, 3, 2, 1},
		FraudProofs: true,
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
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	maliciousAppHash := []byte{9, 8, 7, 6}
	nonMaliciousAppHash := []byte{1, 2, 3, 4}
	if isMalicious && aggregator {
		app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{AppHash: maliciousAppHash})
	} else {
		app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{AppHash: nonMaliciousAppHash})
	}

	if isMalicious && !aggregator {
		app.On("GenerateFraudProof", mock.Anything).Return(abci.ResponseGenerateFraudProof{FraudProof: &abci.FraudProof{}})
	}
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{}).Run(func(args mock.Arguments) {
		wg.Done()
	})
	if ctx == nil {
		ctx = context.Background()
	}

	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	genesis := &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}
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
		log.TestingLogger().With("node", n))
	require.NoError(err)
	require.NotNil(node)

	return node, app
}
