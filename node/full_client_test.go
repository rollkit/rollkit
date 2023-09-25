package node

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	tconfig "github.com/cometbft/cometbft/config"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/encoding"
	"github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/conv"
	abciconv "github.com/rollkit/rollkit/conv/abci"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/mocks"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

var mockTxProcessingTime = 10 * time.Millisecond

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	return getRandomBlockWithProposer(height, nTxs, types.GetRandomBytes(20))
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
				Signatures:      make([][]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.SignedHeader.AppHash = types.GetRandomBytes(32)

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = types.GetRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = types.GetRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	cmprotoLC, err := cmtypes.CommitFromProto(&cmproto.Commit{})
	if err != nil {
		return nil
	}
	lastCommitHash := make(types.Hash, 32)
	copy(lastCommitHash, cmprotoLC.Hash().Bytes())
	block.SignedHeader.LastCommitHash = lastCommitHash

	block.SignedHeader.Validators = types.GetRandomValidatorSet()

	return block
}

func getBlockMeta(rpc *FullClient, n int64) *cmtypes.BlockMeta {
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
	ctx := context.Background()
	node, err := newFullNode(ctx, config.NodeConfig{DALayer: "mock"}, key, signingKey, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
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
		require.NoError(t, rpc.node.BlockIndexer.Index(cmtypes.EventDataNewBlockHeader{
			Header: cmtypes.Header{Height: h},
			ResultBeginBlock: abci.ResponseBeginBlock{
				Events: []abci.Event{
					{
						Type: "begin_event",
						Attributes: []abci.EventAttribute{
							{
								Key:   "proposer",
								Value: "FCAA001",
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
								Key:   "foo",
								Value: fmt.Sprintf("%d", h),
								Index: true,
							},
						},
					},
				},
			},
		}))
	}

}

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

	genDoc := &cmtypes.GenesisDoc{
		ChainID:       "test",
		InitialHeight: int64(1),
		AppHash:       []byte("test hash"),
		Validators: []cmtypes.GenesisValidator{
			{Address: bytes.HexBytes{}, Name: "test", Power: 1, PubKey: ed25519.GenPrivKey().PubKey()},
		},
	}

	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	privKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	n, _ := newFullNode(context.Background(), config.NodeConfig{DALayer: "mock"}, privKey, signingKey, proxy.NewLocalClientCreator(mockApp), genDoc, log.TestingLogger())

	rpc := NewFullClient(n)

	var expectedID uint = 2
	gc, err := rpc.GenesisChunked(context.Background(), expectedID)
	assert.Error(err)
	assert.Nil(gc)

	err = rpc.node.Start()
	require.NoError(t, err)
	defer func() {
		assert.NoError(rpc.node.Stop())
	}()
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
	defer func() {
		assert.NoError(rpc.node.Stop())
	}()
	res, err := rpc.BroadcastTxAsync(context.Background(), expectedTx)
	assert.NoError(err)
	assert.NotNil(res)
	assert.Empty(res.Code)
	assert.Empty(res.Data)
	assert.Empty(res.Log)
	assert.Empty(res.Codespace)
	assert.NotEmpty(res.Hash)
	mockApp.AssertExpectations(t)
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
	defer func() {
		assert.NoError(rpc.node.Stop())
	}()
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
	defer func() {
		require.NoError(rpc.node.Stop())
	}()
	go func() {
		time.Sleep(mockTxProcessingTime)
		err := rpc.node.EventBus().PublishEventTx(cmtypes.EventDataTx{TxResult: abci.TxResult{
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
	defer func() {
		require.NoError(rpc.node.Stop())
	}()
	block := getRandomBlock(1, 10)
	err = rpc.node.Store.SaveBlock(block, &types.Commit{})
	rpc.node.Store.SetHeight(block.Height())
	require.NoError(err)

	blockResp, err := rpc.Block(context.Background(), nil)
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)
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
	defer func() {
		require.NoError(rpc.node.Stop())
	}()
	for _, b := range blocks {
		err = rpc.node.Store.SaveBlock(b, &types.Commit{})
		rpc.node.Store.SetHeight(b.Height())
		require.NoError(err)
	}
	t.Run("Fetch all commits", func(t *testing.T) {
		for _, b := range blocks {
			h := int64(b.Height())
			commit, err := rpc.Commit(context.Background(), &h)
			require.NoError(err)
			require.NotNil(commit)
			assert.Equal(h, commit.Height)
		}
	})

	t.Run("Fetch commit for nil height", func(t *testing.T) {
		commit, err := rpc.Commit(context.Background(), nil)
		require.NoError(err)
		require.NotNil(commit)
		assert.Equal(int64(blocks[3].Height()), commit.Height)
	})
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
		err := rpc.node.Store.SaveBlock(block, &types.Commit{})
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
	defer func() {
		require.NoError(rpc.node.Stop())
	}()
	block := getRandomBlock(1, 10)
	err = rpc.node.Store.SaveBlock(block, &types.Commit{})
	require.NoError(err)
	abciBlock, err := abciconv.ToABCIBlock(block)
	require.NoError(err)

	height := int64(block.Height())
	retrievedBlock, err := rpc.Block(context.Background(), &height)
	require.NoError(err)
	require.NotNil(retrievedBlock)
	assert.Equal(abciBlock, retrievedBlock.Block)
	assert.Equal(abciBlock.Hash(), retrievedBlock.Block.Hash())

	blockHash := block.Hash()
	blockResp, err := rpc.BlockByHash(context.Background(), blockHash[:])
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)
}

func TestTx(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	node, err := newFullNode(context.Background(), config.NodeConfig{
		DALayer:    "mock",
		Aggregator: true,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		}},
		key, signingKey, proxy.NewLocalClientCreator(mockApp),
		&cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators},
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

	err = rpc.node.Start()
	require.NoError(err)
	defer func() {
		require.NoError(rpc.node.Stop())
	}()
	tx1 := cmtypes.Tx("tx1")
	res, err := rpc.BroadcastTxSync(context.Background(), tx1)
	assert.NoError(err)
	assert.NotNil(res)

	time.Sleep(2 * time.Second)

	resTx, errTx := rpc.Tx(context.Background(), res.Hash, true)
	assert.NoError(errTx)
	assert.NotNil(resTx)
	assert.EqualValues(tx1, resTx.Tx)
	assert.EqualValues(res.Hash, resTx.Hash)

	tx2 := cmtypes.Tx("tx2")
	assert.Panics(func() {
		resTx, errTx := rpc.Tx(context.Background(), tx2.Hash(), true)
		assert.Nil(resTx)
		assert.Error(errTx)
	})
}

func TestUnconfirmedTxs(t *testing.T) {
	tx1 := cmtypes.Tx("tx1")
	tx2 := cmtypes.Tx("another tx")

	cases := []struct {
		name               string
		txs                []cmtypes.Tx
		expectedCount      int
		expectedTotal      int
		expectedTotalBytes int
	}{
		{"no txs", nil, 0, 0, 0},
		{"one tx", []cmtypes.Tx{tx1}, 1, 1, len(tx1)},
		{"two txs", []cmtypes.Tx{tx1, tx2}, 2, 2, len(tx1) + len(tx2)},
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
			defer func() {
				require.NoError(rpc.node.Stop())
			}()

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
	defer func() {
		require.NoError(rpc.node.Stop())
	}()

	tx1 := cmtypes.Tx("tx1")
	tx2 := cmtypes.Tx("another tx")

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
		err := rpc.node.Store.SaveBlock(block, &types.Commit{})
		rpc.node.Store.SetHeight(block.Height())
		require.NoError(err)
	}

	tests := []struct {
		desc string
		min  int64
		max  int64
		exp  []*cmtypes.BlockMeta
		err  bool
	}{
		{
			desc: "min = 1 and max = 5",
			min:  1,
			max:  5,
			exp:  []*cmtypes.BlockMeta{getBlockMeta(rpc, 1), getBlockMeta(rpc, 5)},
			err:  false,
		}, {
			desc: "min height is 0",
			min:  0,
			max:  10,
			exp:  []*cmtypes.BlockMeta{getBlockMeta(rpc, 1), getBlockMeta(rpc, 10)},
			err:  false,
		}, {
			desc: "max height is out of range",
			min:  0,
			max:  15,
			exp:  []*cmtypes.BlockMeta{getBlockMeta(rpc, 1), getBlockMeta(rpc, 10)},
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

func createGenesisValidators(t *testing.T, numNodes int, appCreator func(require *require.Assertions, vKeyToRemove cmcrypto.PrivKey, wg *sync.WaitGroup) *mocks.Application, wg *sync.WaitGroup) *FullClient {
	t.Helper()
	require := require.New(t)
	vKeys := make([]cmcrypto.PrivKey, numNodes)
	apps := make([]*mocks.Application, numNodes)
	nodes := make([]*FullNode, numNodes)

	genesisValidators := make([]cmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		genesisValidators[i] = cmtypes.GenesisValidator{Address: vKeys[i].PubKey().Address(), PubKey: vKeys[i].PubKey(), Power: int64(i + 100), Name: fmt.Sprintf("gen #%d", i)}
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
	t.Cleanup(func() {
		require.NoError(dalc.Stop())
	})

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
			&cmtypes.GenesisDoc{ChainID: "test", Validators: genesisValidators},
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
		node := nodes[i]
		err := nodes[i].Start()
		require.NoError(err)

		t.Cleanup(func() {
			require.NoError(node.Stop())
		})
	}
	return rpc
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

func createApp(require *require.Assertions, vKeyToRemove cmcrypto.PrivKey, wg *sync.WaitGroup) *mocks.Application {
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})

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

// Tests moving from two validators to one validator and then back to two validators
func TestValidatorSetHandling(t *testing.T) {
	assert := assert.New(t)

	var wg sync.WaitGroup

	numNodes := 2
	rpc := createGenesisValidators(t, numNodes, createApp, &wg)
	wg.Wait()

	// test first blocks
	for h := int64(1); h <= 3; h++ {
		checkValSet(rpc, assert, h, numNodes)
	}

	// 3rd EndBlock removes the first validator from the list
	for h := int64(4); h <= 5; h++ {
		checkValSet(rpc, assert, h, numNodes-1)
	}

	// 5th EndBlock adds validator back
	for h := int64(6); h <= 9; h++ {
		checkValSet(rpc, assert, h, numNodes)
	}

	// check for "latest block"
	checkValSetLatest(rpc, assert, int64(9), numNodes)
}

// Tests moving from a centralized validator to empty validator set
func TestValidatorSetHandlingBased(t *testing.T) {
	assert := assert.New(t)
	var wg sync.WaitGroup
	numNodes := 1
	rpc := createGenesisValidators(t, numNodes, createApp, &wg)

	wg.Wait()

	time.Sleep(100 * time.Millisecond)

	// test first blocks
	for h := int64(1); h <= 3; h++ {
		checkValSet(rpc, assert, h, numNodes)
	}

	// 3rd EndBlock removes the first validator and makes the rollup based
	for h := int64(4); h <= 9; h++ {
		checkValSet(rpc, assert, h, numNodes-1)
	}

	// check for "latest block"
	checkValSetLatest(rpc, assert, 9, numNodes-1)
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

	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// make node1 an aggregator, so that node2 can start gracefully
	node1, err := newFullNode(ctx, config.NodeConfig{
		Aggregator: true,
		DALayer:    "mock",
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9001",
		},
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 1 * time.Second,
		},
	}, key1, signingKey1, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	node2, err := newFullNode(ctx, config.NodeConfig{
		DALayer: "mock",
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9002",
			Seeds:         "/ip4/127.0.0.1/tcp/9001/p2p/" + id1.Pretty(),
		},
	}, key2, signingKey2, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	err = node1.Start()
	require.NoError(err)
	time.Sleep(1 * time.Second)

	defer func() {
		require.NoError(node1.Stop())
	}()
	err = node2.Start()
	require.NoError(err)
	defer func() {
		require.NoError(node2.Stop())
	}()

	time.Sleep(4 * time.Second)
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer timeoutCancel()

	local := NewFullClient(node1)
	require.NotNil(local)

	// broadcast the bad Tx, this should not be propogated or added to the local mempool
	resp, err := local.BroadcastTxSync(timeoutCtx, []byte("bad"))
	assert.NoError(err)
	assert.NotNil(resp)
	// broadcast the good Tx, this should be propogated and added to the local mempool
	resp, err = local.BroadcastTxSync(timeoutCtx, []byte("good"))
	assert.NoError(err)
	assert.NotNil(resp)
	// broadcast the good Tx again in the same block, this should not be propogated and
	// added to the local mempool
	resp, err = local.BroadcastTxSync(timeoutCtx, []byte("good"))
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

	vKeys := make([]cmcrypto.PrivKey, 2)
	validators := make([]*cmtypes.Validator, len(vKeys))
	genesisValidators := make([]cmtypes.GenesisValidator, len(vKeys))
	for i := 0; i < len(vKeys); i++ {
		vKeys[i] = ed25519.GenPrivKey()
		validators[i] = &cmtypes.Validator{
			Address:          vKeys[i].PubKey().Address(),
			PubKey:           vKeys[i].PubKey(),
			VotingPower:      int64(i + 100),
			ProposerPriority: int64(i),
		}
		genesisValidators[i] = cmtypes.GenesisValidator{
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
		proxy.NewLocalClientCreator(app),
		&cmtypes.GenesisDoc{
			ChainID:    "test",
			Validators: genesisValidators,
		},
		log.TestingLogger(),
	)
	require.NoError(err)
	require.NotNil(node)

	validatorSet := cmtypes.NewValidatorSet(validators)
	err = node.Store.SaveValidators(1, validatorSet)
	require.NoError(err)
	err = node.Store.SaveValidators(2, validatorSet)
	require.NoError(err)
	err = node.Store.UpdateState(types.State{LastValidators: validatorSet, NextValidators: validatorSet, Validators: validatorSet})
	assert.NoError(err)

	rpc := NewFullClient(node)
	assert.NotNil(rpc)

	earliestBlock := getRandomBlockWithProposer(1, 1, validators[0].Address.Bytes())
	err = rpc.node.Store.SaveBlock(earliestBlock, &types.Commit{})
	rpc.node.Store.SetHeight(uint64(earliestBlock.Height()))
	require.NoError(err)

	latestBlock := getRandomBlockWithProposer(2, 1, validators[1].Address.Bytes())
	err = rpc.node.Store.SaveBlock(latestBlock, &types.Commit{})
	rpc.node.Store.SetHeight(uint64(latestBlock.Height()))
	require.NoError(err)

	err = node.Start()
	require.NoError(err)
	defer func() {
		require.NoError(node.Stop())
	}()

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
	// check that NodeInfo DefaultNodeID matches the ID derived from p2p key
	rawKey, err := key.GetPublic().Raw()
	assert.NoError(err)
	assert.Equal(p2p.ID(hex.EncodeToString(cmcrypto.AddressHash(rawKey))), resp.NodeInfo.DefaultNodeID)
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
	genesisValidators, signingKey := getGenesisValidatorSetWithSigner(1)
	genesisTime := time.Now().Local().Add(time.Second * time.Duration(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := newFullNode(ctx, config.NodeConfig{
		DALayer:    "mock",
		Aggregator: true,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 200 * time.Millisecond,
		}},
		key, signingKey,
		proxy.NewLocalClientCreator(mockApp),
		&cmtypes.GenesisDoc{
			ChainID:       "test",
			InitialHeight: 1,
			GenesisTime:   genesisTime,
			Validators:    genesisValidators,
		},
		log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	err = node.Start()
	require.NoError(err)

	defer func() {
		require.NoError(node.Stop())
	}()
	wg.Wait()

	assert.True(beginBlockTime.After(genesisTime))
}
