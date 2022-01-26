package client

import (
	"context"
	crand "crypto/rand"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/libs/log"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/mocks"
	"github.com/celestiaorg/optimint/node"
	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/types"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

var mockTxProcessingTime = 10 * time.Millisecond

func TestConnectionGetters(t *testing.T) {
	assert := assert.New(t)

	_, rpc := getRPC(t)
	assert.NotNil(rpc.consensus())
	assert.NotNil(rpc.mempool())
	assert.NotNil(rpc.snapshot())
	assert.NotNil(rpc.query())
}

func TestInfo(t *testing.T) {
	assert := assert.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("Info", mock.Anything).Return(expectedInfo)

	info, err := rpc.ABCIInfo(context.Background())
	assert.NoError(err)
	assert.Equal(expectedInfo, info.Response)
}

func TestCheckTx(t *testing.T) {
	assert := assert.New(t)

	expectedTx := []byte("tx data")

	mockApp, rpc := getRPC(t)
	mockApp.On("CheckTx", abci.RequestCheckTx{Tx: expectedTx}).Once().Return(abci.ResponseCheckTx{})

	res, err := rpc.CheckTx(context.Background(), expectedTx)
	assert.NoError(err)
	assert.NotNil(res)
	mockApp.AssertExpectations(t)
}

func TestBroadcastTxAsync(t *testing.T) {
	assert := assert.New(t)

	expectedTx := []byte("tx data")

	mockApp, rpc := getRPC(t)
	mockApp.On("CheckTx", abci.RequestCheckTx{Tx: expectedTx}).Return(abci.ResponseCheckTx{})

	err := rpc.node.Start()
	require.NoError(t, err)

	res, err := rpc.BroadcastTxAsync(context.Background(), expectedTx)
	assert.NoError(err)
	assert.NotNil(res)
	assert.Empty(res.Code)
	assert.Empty(res.Data)
	assert.Empty(res.Log)
	assert.Empty(res.Codespace)
	assert.NotEmpty(res.Hash)
	mockApp.AssertExpectations(t)

	err = rpc.node.Stop()
	require.NoError(t, err)
}

func TestBroadcastTxSync(t *testing.T) {
	assert := assert.New(t)

	expectedTx := []byte("tx data")
	expectedResponse := abci.ResponseCheckTx{
		Code:      1,
		Data:      []byte("data"),
		Log:       "log",
		Info:      "info",
		GasWanted: 0,
		GasUsed:   0,
		Events:    nil,
		Codespace: "space",
	}

	mockApp, rpc := getRPC(t)

	err := rpc.node.Start()
	require.NoError(t, err)

	mockApp.On("CheckTx", abci.RequestCheckTx{Tx: expectedTx}).Return(expectedResponse)

	res, err := rpc.BroadcastTxSync(context.Background(), expectedTx)
	assert.NoError(err)
	assert.NotNil(res)
	assert.Equal(expectedResponse.Code, res.Code)
	assert.Equal(bytes.HexBytes(expectedResponse.Data), res.Data)
	assert.Equal(expectedResponse.Log, res.Log)
	assert.Equal(expectedResponse.Codespace, res.Codespace)
	assert.NotEmpty(res.Hash)
	mockApp.AssertExpectations(t)

	err = rpc.node.Stop()
	require.NoError(t, err)
}

func TestBroadcastTxCommit(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	expectedTx := []byte("tx data")
	expectedCheckResp := abci.ResponseCheckTx{
		Code:      abci.CodeTypeOK,
		Data:      []byte("data"),
		Log:       "log",
		Info:      "info",
		GasWanted: 0,
		GasUsed:   0,
		Events:    nil,
		Codespace: "space",
	}
	expectedDeliverResp := abci.ResponseDeliverTx{
		Code:      0,
		Data:      []byte("foo"),
		Log:       "bar",
		Info:      "baz",
		GasWanted: 100,
		GasUsed:   10,
		Events:    nil,
		Codespace: "space",
	}

	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.BeginBlock(abci.RequestBeginBlock{})
	mockApp.On("CheckTx", abci.RequestCheckTx{Tx: expectedTx}).Return(expectedCheckResp)

	// in order to broadcast, the node must be started
	err := rpc.node.Start()
	require.NoError(err)

	go func() {
		time.Sleep(mockTxProcessingTime)
		err := rpc.node.EventBus().PublishEventTx(tmtypes.EventDataTx{TxResult: abci.TxResult{
			Height: 1,
			Index:  0,
			Tx:     expectedTx,
			Result: expectedDeliverResp,
		}})
		require.NoError(err)
	}()

	res, err := rpc.BroadcastTxCommit(context.Background(), expectedTx)
	assert.NoError(err)
	require.NotNil(res)
	assert.Equal(expectedCheckResp, res.CheckTx)
	assert.Equal(expectedDeliverResp, res.DeliverTx)
	mockApp.AssertExpectations(t)

	err = rpc.node.Stop()
	require.NoError(err)
}

func TestGetBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	mockApp.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

	err := rpc.node.Start()
	require.NoError(err)

	err = rpc.node.Store.SaveBlock(getRandomBlock(1, 10), &types.Commit{})
	require.NoError(err)

	blockResp, err := rpc.Block(context.Background(), nil)
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)

	err = rpc.node.Stop()
	require.NoError(err)
}

func TestGetBlockByHash(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	mockApp.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

	err := rpc.node.Start()
	require.NoError(err)

	block := getRandomBlock(1, 10)
	err = rpc.node.Store.SaveBlock(block, &types.Commit{})
	require.NoError(err)

	blockHash := block.Header.Hash()
	blockResp, err := rpc.BlockByHash(context.Background(), blockHash[:])
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)

	err = rpc.node.Stop()
	require.NoError(err)
}

func TestUnconfirmedTxs(t *testing.T) {
	tx1 := tmtypes.Tx("tx1")
	tx2 := tmtypes.Tx("another tx")

	cases := []struct {
		name               string
		txs                []tmtypes.Tx
		expectedCount      int
		expectedTotal      int
		expectedTotalBytes int
	}{
		{"no txs", nil, 0, 0, 0},
		{"one tx", []tmtypes.Tx{tx1}, 1, 1, len(tx1)},
		{"two txs", []tmtypes.Tx{tx1, tx2}, 2, 2, len(tx1) + len(tx2)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			mockApp, rpc := getRPC(t)
			mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
			mockApp.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})

			err := rpc.node.Start()
			require.NoError(err)

			for _, tx := range c.txs {
				res, err := rpc.BroadcastTxAsync(context.Background(), tx)
				assert.NoError(err)
				assert.NotNil(res)
			}

			numRes, err := rpc.NumUnconfirmedTxs(context.Background())
			assert.NoError(err)
			assert.NotNil(numRes)
			assert.EqualValues(c.expectedCount, numRes.Count)
			assert.EqualValues(c.expectedTotal, numRes.Total)
			assert.EqualValues(c.expectedTotalBytes, numRes.TotalBytes)

			limit := -1
			txRes, err := rpc.UnconfirmedTxs(context.Background(), &limit)
			assert.NoError(err)
			assert.NotNil(txRes)
			assert.EqualValues(c.expectedCount, txRes.Count)
			assert.EqualValues(c.expectedTotal, txRes.Total)
			assert.EqualValues(c.expectedTotalBytes, txRes.TotalBytes)
			assert.Len(txRes.Txs, c.expectedCount)
		})
	}
}

func TestUnconfirmedTxsLimit(t *testing.T) {
	t.Skip("Test disabled because of known bug")
	// there's a bug in mempool implementation - count should be 1
	// TODO(tzdybal): uncomment after resolving https://github.com/celestiaorg/optimint/issues/191

	assert := assert.New(t)
	require := require.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})

	err := rpc.node.Start()
	require.NoError(err)

	tx1 := tmtypes.Tx("tx1")
	tx2 := tmtypes.Tx("another tx")

	res, err := rpc.BroadcastTxAsync(context.Background(), tx1)
	assert.NoError(err)
	assert.NotNil(res)

	res, err = rpc.BroadcastTxAsync(context.Background(), tx2)
	assert.NoError(err)
	assert.NotNil(res)

	limit := 1
	txRes, err := rpc.UnconfirmedTxs(context.Background(), &limit)
	assert.NoError(err)
	assert.NotNil(txRes)
	assert.EqualValues(1, txRes.Count)
	assert.EqualValues(2, txRes.Total)
	assert.EqualValues(len(tx1)+len(tx2), txRes.TotalBytes)
	assert.Len(txRes.Txs, limit)
	assert.Contains(txRes.Txs, tx1)
	assert.NotContains(txRes.Txs, tx2)
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		Header: types.Header{
			Height:          height,
			Version:         types.Version{Block: state.InitStateVersion.Consensus.Block},
			ProposerAddress: getRandomBytes(20),
		},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	copy(block.Header.AppHash[:], getRandomBytes(32))

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/celestiaorg/optimint/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	tmprotoLC, err := tmtypes.CommitFromProto(&tmproto.Commit{})
	if err != nil {
		return nil
	}
	copy(block.Header.LastCommitHash[:], tmprotoLC.Hash())

	return block
}

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data)
	return data
}

func getRPC(t *testing.T) (*mocks.Application, *Client) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	node, err := node.NewNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewClient(node)
	require.NotNil(rpc)

	return app, rpc
}

func TestMempool2Nodes(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", abci.RequestCheckTx{Tx: []byte("bad")}).Return(abci.ResponseCheckTx{Code: 1})
	app.On("CheckTx", abci.RequestCheckTx{Tx: []byte("good")}).Return(abci.ResponseCheckTx{Code: 0})
	key1, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	key2, _, _ := crypto.GenerateEd25519Key(crand.Reader)

	id1, err := peer.IDFromPrivateKey(key1)
	require.NoError(err)

	node1, err := node.NewNode(context.Background(), config.NodeConfig{
		DALayer: "mock",
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9001",
		},
	}, key1, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	node2, err := node.NewNode(context.Background(), config.NodeConfig{
		DALayer: "mock",
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9002",
			Seeds:         "/ip4/127.0.0.1/tcp/9001/p2p/" + id1.Pretty(),
		},
	}, key2, proxy.NewLocalClientCreator(app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	err = node1.Start()
	require.NoError(err)

	err = node2.Start()
	require.NoError(err)

	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	local := NewClient(node1)
	require.NotNil(local)

	// broadcast the bad Tx, this should not be propogated or added to the local mempool
	resp, err := local.BroadcastTxSync(ctx, []byte("bad"))
	assert.NoError(err)
	assert.NotNil(resp)
	// broadcast the good Tx, this should be propogated and added to the local mempool
	resp, err = local.BroadcastTxSync(ctx, []byte("good"))
	assert.NoError(err)
	assert.NotNil(resp)
	// broadcast the good Tx again in the same block, this should not be propogated and
	// added to the local mempool
	resp, err = local.BroadcastTxSync(ctx, []byte("good"))
	assert.Error(err)
	assert.Nil(resp)

	txAvailable := node2.Mempool.TxsAvailable()
	select {
	case <-txAvailable:
	case <-ctx.Done():
	}

	assert.Equal(node2.Mempool.TxsBytes(), int64(len("good")))
}
