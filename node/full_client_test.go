package node

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	tconfig "github.com/tendermint/tendermint/config"
	tmcrypto "github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/encoding"
	"github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/p2p"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"
	"github.com/tendermint/tendermint/version"

	"github.com/rollkit/rollkit/config"
	abciconv "github.com/rollkit/rollkit/conv/abci"
	"github.com/rollkit/rollkit/mocks"
	"github.com/rollkit/rollkit/types"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

var mockTxProcessingTime = 10 * time.Millisecond

func TestConnectionGetter(t *testing.T) {
	assert := assert.New(t)

	_, rpc := getRPC(t)
	assert.NotNil(rpc.appClient())
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

func TestGenesisChunked(t *testing.T) {
	assert := assert.New(t)

	genDoc := &tmtypes.GenesisDoc{
		ChainID:       "test",
		InitialHeight: int64(1),
		AppHash:       []byte("test hash"),
		Validators: []tmtypes.GenesisValidator{
			{Address: bytes.HexBytes{}, Name: "test", Power: 1, PubKey: ed25519.GenPrivKey().PubKey()},
		},
	}

	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	privKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	n, _ := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock"}, privKey, signingKey, abcicli.NewLocalClient(nil, mockApp), genDoc, log.TestingLogger())

	rpc := NewFullClient(n)

	var expectedID uint = 2
	gc, err := rpc.GenesisChunked(context.Background(), expectedID)
	assert.Error(err)
	assert.Nil(gc)

	err = rpc.node.Start()
	require.NoError(t, err)

	expectedID = 0
	gc2, err := rpc.GenesisChunked(context.Background(), expectedID)
	gotID := gc2.ChunkNumber
	assert.NoError(err)
	assert.NotNil(gc2)
	assert.Equal(int(expectedID), gotID)

	gc3, err := rpc.GenesisChunked(context.Background(), 5)
	assert.Error(err)
	assert.Nil(gc3)
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

	block := getRandomBlock(1, 10)
	err = rpc.node.Store.SaveBlock(block, &types.Commit{})
	rpc.node.Store.SetHeight(uint64(block.Header.Height()))
	require.NoError(err)

	blockResp, err := rpc.Block(context.Background(), nil)
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)

	err = rpc.node.Stop()
	require.NoError(err)
}

func TestGetCommit(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

	blocks := []*types.Block{getRandomBlock(1, 5), getRandomBlock(2, 6), getRandomBlock(3, 8), getRandomBlock(4, 10)}

	err := rpc.node.Start()
	require.NoError(err)

	for _, b := range blocks {
		err = rpc.node.Store.SaveBlock(b, &types.Commit{Height: uint64(b.Header.Height())})
		rpc.node.Store.SetHeight(uint64(b.Header.Height()))
		require.NoError(err)
	}
	t.Run("Fetch all commits", func(t *testing.T) {
		for _, b := range blocks {
			h := b.Header.Height()
			commit, err := rpc.Commit(context.Background(), &h)
			require.NoError(err)
			require.NotNil(commit)
			assert.Equal(b.Header.Height(), commit.Height)
		}
	})

	t.Run("Fetch commit for nil height", func(t *testing.T) {
		commit, err := rpc.Commit(context.Background(), nil)
		require.NoError(err)
		require.NotNil(commit)
		assert.Equal(blocks[3].Header.Height(), commit.Height)
	})

	err = rpc.node.Stop()
	require.NoError(err)
}

func TestBlockSearch(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

	heights := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, h := range heights {
		block := getRandomBlock(uint64(h), 5)
		err := rpc.node.Store.SaveBlock(block, &types.Commit{
			Height:     uint64(h),
			HeaderHash: block.Header.Hash(),
		})
		require.NoError(err)
	}
	indexBlocks(t, rpc, heights)

	tests := []struct {
		query      string
		page       int
		perPage    int
		totalCount int
		orderBy    string
	}{
		{
			query:      "block.height >= 1 AND end_event.foo <= 5",
			page:       1,
			perPage:    5,
			totalCount: 5,
			orderBy:    "asc",
		},
		{
			query:      "block.height >= 2 AND end_event.foo <= 10",
			page:       1,
			perPage:    3,
			totalCount: 9,
			orderBy:    "desc",
		},
		{
			query:      "begin_event.proposer = 'FCAA001' AND end_event.foo <= 5",
			page:       1,
			perPage:    5,
			totalCount: 5,
			orderBy:    "asc",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.query, func(t *testing.T) {
			result, err := rpc.BlockSearch(context.Background(), test.query, &test.page, &test.perPage, test.orderBy)
			require.NoError(err)
			assert.Equal(test.totalCount, result.TotalCount)
			assert.Len(result.Blocks, test.perPage)
		})

	}
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
	abciBlock, err := abciconv.ToABCIBlock(block)
	require.NoError(err)

	height := block.Header.Height()
	retrievedBlock, err := rpc.Block(context.Background(), &height)
	require.NoError(err)
	require.NotNil(retrievedBlock)
	assert.Equal(abciBlock, retrievedBlock.Block)
	assert.Equal(abciBlock.Hash(), retrievedBlock.Block.Hash())

	blockHash := block.Header.Hash()
	blockResp, err := rpc.BlockByHash(context.Background(), blockHash[:])
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)

	err = rpc.node.Stop()
	require.NoError(err)
}

func TestTx(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	node, err := newFullNode(context.Background(), config.NodeConfig{
		DALayer:    "mock",
		Aggregator: true,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 200 * time.Millisecond,
		}},
		key, signingKey, abcicli.NewLocalClient(nil, mockApp),
		&tmtypes.GenesisDoc{ChainID: "test"},
		log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewFullClient(node)
	require.NotNil(rpc)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	mockApp.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	mockApp.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	mockApp.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})
	mockApp.On("GenerateFraudProof", mock.Anything).Return(abci.ResponseGenerateFraudProof{})

	err = rpc.node.Start()
	require.NoError(err)

	tx1 := tmtypes.Tx("tx1")
	res, err := rpc.BroadcastTxSync(context.Background(), tx1)
	assert.NoError(err)
	assert.NotNil(res)

	time.Sleep(1 * time.Second)

	resTx, errTx := rpc.Tx(context.Background(), res.Hash, true)
	assert.NoError(errTx)
	assert.NotNil(resTx)
	assert.EqualValues(tx1, resTx.Tx)
	assert.EqualValues(res.Hash, resTx.Hash)

	tx2 := tmtypes.Tx("tx2")
	assert.Panics(func() {
		resTx, errTx := rpc.Tx(context.Background(), tx2.Hash(), true)
		assert.Nil(resTx)
		assert.Error(errTx)
	})
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

func TestConsensusState(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, rpc := getRPC(t)
	require.NotNil(rpc)

	resp1, err := rpc.ConsensusState(context.Background())
	assert.Nil(resp1)
	assert.ErrorIs(err, ErrConsensusStateNotAvailable)

	resp2, err := rpc.DumpConsensusState(context.Background())
	assert.Nil(resp2)
	assert.ErrorIs(err, ErrConsensusStateNotAvailable)
}

func TestBlockchainInfo(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	mockApp, rpc := getRPC(t)
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

	heights := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, h := range heights {
		block := getRandomBlock(uint64(h), 5)
		err := rpc.node.Store.SaveBlock(block, &types.Commit{
			Height:     uint64(h),
			HeaderHash: block.Header.Hash(),
		})
		rpc.node.Store.SetHeight(uint64(block.Header.Height()))
		require.NoError(err)
	}

	tests := []struct {
		desc string
		min  int64
		max  int64
		exp  []*tmtypes.BlockMeta
		err  bool
	}{
		{
			desc: "min = 1 and max = 5",
			min:  1,
			max:  5,
			exp:  []*tmtypes.BlockMeta{getBlockMeta(rpc, 1), getBlockMeta(rpc, 5)},
			err:  false,
		}, {
			desc: "min height is 0",
			min:  0,
			max:  10,
			exp:  []*tmtypes.BlockMeta{getBlockMeta(rpc, 1), getBlockMeta(rpc, 10)},
			err:  false,
		}, {
			desc: "max height is out of range",
			min:  0,
			max:  15,
			exp:  []*tmtypes.BlockMeta{getBlockMeta(rpc, 1), getBlockMeta(rpc, 10)},
			err:  false,
		}, {
			desc: "negative min height",
			min:  -1,
			max:  11,
			exp:  nil,
			err:  true,
		}, {
			desc: "negative max height",
			min:  1,
			max:  -1,
			exp:  nil,
			err:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result, err := rpc.BlockchainInfo(context.Background(), test.min, test.max)
			if test.err {
				require.Error(err)
			} else {
				require.NoError(err)
				assert.Equal(result.LastHeight, heights[9])
				assert.Contains(result.BlockMetas, test.exp[0])
				assert.Contains(result.BlockMetas, test.exp[1])
			}

		})
	}
}

func TestValidatorSetHandling(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})
	app.On("GenerateFraudProof", mock.Anything).Return(abci.ResponseGenerateFraudProof{})

	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)

	vKeys := make([]tmcrypto.PrivKey, 4)
	genesisValidators := make([]tmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		genesisValidators[i] = tmtypes.GenesisValidator{Address: vKeys[i].PubKey().Address(), PubKey: vKeys[i].PubKey(), Power: int64(i + 100), Name: "one"}
	}

	pbValKey, err := encoding.PubKeyToProto(vKeys[0].PubKey())
	require.NoError(err)

	waitCh := make(chan interface{})

	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Times(5)
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{ValidatorUpdates: []abci.ValidatorUpdate{{PubKey: pbValKey, Power: 0}}}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{ValidatorUpdates: []abci.ValidatorUpdate{{PubKey: pbValKey, Power: 100}}}).Once()
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{}).Run(func(args mock.Arguments) {
		waitCh <- nil
	})

	node, err := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock", Aggregator: true, BlockManagerConfig: config.BlockManagerConfig{BlockTime: 10 * time.Millisecond}}, key, signingKey, abcicli.NewLocalClient(nil, app), &tmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewFullClient(node)
	require.NotNil(rpc)

	err = node.Start()
	require.NoError(err)

	<-waitCh

	// test first blocks
	for h := int64(1); h <= 6; h++ {
		vals, err := rpc.Validators(context.Background(), &h, nil, nil)
		assert.NoError(err)
		assert.NotNil(vals)
		assert.EqualValues(len(genesisValidators), vals.Total)
		assert.Len(vals.Validators, len(genesisValidators))
		assert.EqualValues(vals.BlockHeight, h)
	}

	// 6th EndBlock removes first validator from the list
	for h := int64(7); h <= 8; h++ {
		vals, err := rpc.Validators(context.Background(), &h, nil, nil)
		assert.NoError(err)
		assert.NotNil(vals)
		assert.EqualValues(len(genesisValidators)-1, vals.Total)
		assert.Len(vals.Validators, len(genesisValidators)-1)
		assert.EqualValues(vals.BlockHeight, h)
	}

	// 8th EndBlock adds validator back
	for h := int64(9); h <= 12; h++ {
		<-waitCh
		vals, err := rpc.Validators(context.Background(), &h, nil, nil)
		assert.NoError(err)
		assert.NotNil(vals)
		assert.EqualValues(len(genesisValidators), vals.Total)
		assert.Len(vals.Validators, len(genesisValidators))
		assert.EqualValues(vals.BlockHeight, h)
	}

	// check for "latest block"
	vals, err := rpc.Validators(context.Background(), nil, nil, nil)
	assert.NoError(err)
	assert.NotNil(vals)
	assert.EqualValues(len(genesisValidators), vals.Total)
	assert.Len(vals.Validators, len(genesisValidators))
	assert.GreaterOrEqual(vals.BlockHeight, int64(12))
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	return getRandomBlockWithProposer(height, nTxs, getRandomBytes(20))
}

func getRandomBlockWithProposer(height uint64, nTxs int, proposerAddr []byte) *types.Block {
	block := &types.Block{
		Header: types.Header{
			BaseHeader: types.BaseHeader{
				Height: height,
			},
			Version:         types.Version{Block: types.InitStateVersion.Consensus.Block},
			ProposerAddress: proposerAddr,
			AggregatorsHash: make([]byte, 32),
		},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.Header.AppHash = getRandomBytes(32)

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
	block.Header.LastCommitHash = lastCommitHash

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
	node, err := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, signingKey, abcicli.NewLocalClient(nil, app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewFullClient(node)
	require.NotNil(rpc)

	return app, rpc
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
func TestMempool2Nodes(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", abci.RequestCheckTx{Tx: []byte("bad")}).Return(abci.ResponseCheckTx{Code: 1})
	app.On("CheckTx", abci.RequestCheckTx{Tx: []byte("good")}).Return(abci.ResponseCheckTx{Code: 0})
	key1, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	key2, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey1, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey2, _, _ := crypto.GenerateEd25519Key(crand.Reader)

	id1, err := peer.IDFromPrivateKey(key1)
	require.NoError(err)

	node1, err := newFullNode(context.Background(), config.NodeConfig{
		DALayer: "mock",
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9001",
		},
	}, key1, signingKey1, abcicli.NewLocalClient(nil, app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	node2, err := newFullNode(context.Background(), config.NodeConfig{
		DALayer: "mock",
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9002",
			Seeds:         "/ip4/127.0.0.1/tcp/9001/p2p/" + id1.Pretty(),
		},
	}, key2, signingKey2, abcicli.NewLocalClient(nil, app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	err = node1.Start()
	require.NoError(err)

	err = node2.Start()
	require.NoError(err)

	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	local := NewFullClient(node1)
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

	assert.Equal(node2.Mempool.SizeBytes(), int64(len("good")))
}

func TestStatus(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)

	vKeys := make([]tmcrypto.PrivKey, 2)
	validators := make([]*tmtypes.Validator, len(vKeys))
	genesisValidators := make([]tmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		validators[i] = &tmtypes.Validator{
			Address:          vKeys[i].PubKey().Address(),
			PubKey:           vKeys[i].PubKey(),
			VotingPower:      int64(i + 100),
			ProposerPriority: int64(i),
		}
		genesisValidators[i] = tmtypes.GenesisValidator{
			Address: vKeys[i].PubKey().Address(),
			PubKey:  vKeys[i].PubKey(),
			Power:   int64(i + 100),
			Name:    "one",
		}
	}

	node, err := newFullNode(
		context.Background(),
		config.NodeConfig{
			DALayer: "mock",
			P2P: config.P2PConfig{
				ListenAddress: "/ip4/0.0.0.0/tcp/26656",
			},
			Aggregator: true,
			BlockManagerConfig: config.BlockManagerConfig{
				BlockTime: 10 * time.Millisecond,
			},
		},
		key,
		signingKey,
		abcicli.NewLocalClient(nil, app),
		&tmtypes.GenesisDoc{
			ChainID:    "test",
			Validators: genesisValidators,
		},
		log.TestingLogger(),
	)
	require.NoError(err)
	require.NotNil(node)

	validatorSet := tmtypes.NewValidatorSet(validators)
	err = node.Store.SaveValidators(1, validatorSet)
	require.NoError(err)
	err = node.Store.SaveValidators(2, validatorSet)
	require.NoError(err)
	err = node.Store.UpdateState(types.State{LastValidators: validatorSet, NextValidators: validatorSet, Validators: validatorSet})
	assert.NoError(err)

	rpc := NewFullClient(node)
	assert.NotNil(rpc)

	earliestBlock := getRandomBlockWithProposer(1, 1, validators[0].Address.Bytes())
	err = rpc.node.Store.SaveBlock(earliestBlock, &types.Commit{Height: uint64(earliestBlock.Header.Height())})
	rpc.node.Store.SetHeight(uint64(earliestBlock.Header.Height()))
	require.NoError(err)

	latestBlock := getRandomBlockWithProposer(2, 1, validators[1].Address.Bytes())
	err = rpc.node.Store.SaveBlock(latestBlock, &types.Commit{Height: uint64(latestBlock.Header.Height())})
	rpc.node.Store.SetHeight(uint64(latestBlock.Header.Height()))
	require.NoError(err)

	err = node.Start()
	require.NoError(err)

	resp, err := rpc.Status(context.Background())
	assert.NoError(err)

	assert.Equal(int64(1), resp.SyncInfo.EarliestBlockHeight)
	assert.Equal(int64(2), resp.SyncInfo.LatestBlockHeight)

	assert.Equal(validators[1].Address, resp.ValidatorInfo.Address)
	assert.Equal(validators[1].PubKey, resp.ValidatorInfo.PubKey)
	assert.Equal(validators[1].VotingPower, resp.ValidatorInfo.VotingPower)

	// specific validation
	assert.Equal(tconfig.DefaultBaseConfig().Moniker, resp.NodeInfo.Moniker)
	state, err := rpc.node.Store.LoadState()
	assert.NoError(err)
	defaultProtocolVersion := p2p.NewProtocolVersion(
		version.P2PProtocol,
		state.Version.Consensus.Block,
		state.Version.Consensus.App,
	)
	assert.Equal(defaultProtocolVersion, resp.NodeInfo.ProtocolVersion)

	assert.NotNil(resp.NodeInfo.Other.TxIndex)
	cases := []struct {
		expected bool
		other    p2p.DefaultNodeInfoOther
	}{

		{false, p2p.DefaultNodeInfoOther{}},
		{false, p2p.DefaultNodeInfoOther{TxIndex: "aa"}},
		{false, p2p.DefaultNodeInfoOther{TxIndex: "off"}},
		{true, p2p.DefaultNodeInfoOther{TxIndex: "on"}},
	}
	for _, tc := range cases {
		res := resp.NodeInfo.Other.TxIndex == tc.other.TxIndex
		assert.Equal(tc.expected, res, tc)
	}
}

func TestFutureGenesisTime(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	require := require.New(t)

	var beginBlockTime time.Time
	wg := sync.WaitGroup{}
	wg.Add(1)
	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	mockApp.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{}).Run(func(_ mock.Arguments) {
		beginBlockTime = time.Now()
		wg.Done()
	})
	mockApp.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	mockApp.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	mockApp.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	genesisTime := time.Now().Local().Add(time.Second * time.Duration(1))
	node, err := newFullNode(context.Background(), config.NodeConfig{
		DALayer:    "mock",
		Aggregator: true,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 200 * time.Millisecond,
		}},
		key, signingKey,
		abcicli.NewLocalClient(nil, mockApp),
		&tmtypes.GenesisDoc{
			ChainID:       "test",
			InitialHeight: 1,
			GenesisTime:   genesisTime,
		},
		log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	err = node.Start()
	require.NoError(err)

	wg.Wait()

	assert.True(beginBlockTime.After(genesisTime))
}
