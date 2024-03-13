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
	cmconfig "github.com/cometbft/cometbft/config"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/libs/bytes"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/cometbft/cometbft/light"

	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
	abciconv "github.com/rollkit/rollkit/types/abci"

	cmtmath "github.com/cometbft/cometbft/libs/math"
)

var expectedInfo = &abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

func getBlockMeta(rpc *FullClient, n int64) *cmtypes.BlockMeta {
	b, err := rpc.node.Store.GetBlock(context.Background(), uint64(n))
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
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	ctx := context.Background()
	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	node, err := newFullNode(
		ctx,
		config.NodeConfig{
			DAAddress:   MockDAAddress,
			DANamespace: MockDANamespace,
		},
		key,
		signingKey,
		proxy.NewLocalClientCreator(app),
		genesisDoc,
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
		log.TestingLogger(),
	)
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
		require.NoError(t, rpc.node.BlockIndexer.Index(cmtypes.EventDataNewBlockEvents{
			Height: h,
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
	mockApp.On("Info", mock.Anything, mock.Anything).Return(expectedInfo, nil)

	info, err := rpc.ABCIInfo(context.Background())
	assert.NoError(err)
	assert.Equal(*expectedInfo, info.Response)
}

func TestCheckTx(t *testing.T) {
	assert := assert.New(t)

	expectedTx := []byte("tx data")

	mockApp, rpc := getRPC(t)
	mockApp.On("CheckTx", context.Background(), &abci.RequestCheckTx{Tx: expectedTx}).Once().Return(&abci.ResponseCheckTx{}, nil)

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
	mockApp.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	privKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, _ := newFullNode(ctx, config.NodeConfig{DAAddress: MockDAAddress, DANamespace: MockDANamespace}, privKey, signingKey, proxy.NewLocalClientCreator(mockApp), genDoc, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), test.NewFileLogger(t))

	rpc := NewFullClient(n)

	var expectedID uint = 2
	gc, err := rpc.GenesisChunked(context.Background(), expectedID)
	assert.Error(err)
	assert.Nil(gc)

	startNodeWithCleanup(t, rpc.node)
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
	mockApp.On("CheckTx", mock.Anything, &abci.RequestCheckTx{Tx: expectedTx}).Return(&abci.ResponseCheckTx{}, nil)

	startNodeWithCleanup(t, rpc.node)
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

	startNodeWithCleanup(t, rpc.node)
	mockApp.On("CheckTx", mock.Anything, &abci.RequestCheckTx{Tx: expectedTx}).Return(&expectedResponse, nil)

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

// func TestBroadcastTxCommit(t *testing.T) {
// 	assert := assert.New(t)
// 	require := require.New(t)

// 	expectedTx := []byte("tx data")
// 	expectedCheckResp := abci.ResponseCheckTx{
// 		Code:      abci.CodeTypeOK,
// 		Data:      []byte("data"),
// 		Log:       "log",
// 		Info:      "info",
// 		GasWanted: 0,
// 		GasUsed:   0,
// 		Events:    nil,
// 		Codespace: "space",
// 	}

// 	expectedDeliverResp := abci.ExecTxResult{
// 		Code:      0,
// 		Data:      []byte("foo"),
// 		Log:       "bar",
// 		Info:      "baz",
// 		GasWanted: 100,
// 		GasUsed:   10,
// 		Events:    nil,
// 		Codespace: "space",
// 	}

// 	mockApp, rpc := getRPC(t)
// 	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
// 	mockApp.On("CheckTx", mock.Anything, &abci.RequestCheckTx{Tx: expectedTx}).Return(&expectedCheckResp, nil)

// 	// in order to broadcast, the node must be started
// 	err := rpc.node.Start()
// 	require.NoError(err)
// 	defer func() {
// 		require.NoError(rpc.node.Stop())
// 	}()

// 	go func() {
// 		time.Sleep(mockTxProcessingTime)
// 		err := rpc.node.EventBus().PublishEventTx(cmtypes.EventDataTx{TxResult: abci.TxResult{
// 			Height: 1,
// 			Index:  0,
// 			Tx:     expectedTx,
// 			Result: expectedDeliverResp,
// 		}})
// 		require.NoError(err)
// 	}()

// 	res, err := rpc.BroadcastTxCommit(context.Background(), expectedTx)
// 	assert.NoError(err)
// 	require.NotNil(res)
// 	assert.Equal(expectedCheckResp, res.CheckTx)
// 	assert.Equal(expectedDeliverResp, res.TxResult)
// 	mockApp.AssertExpectations(t)
// }

func TestGetBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	startNodeWithCleanup(t, rpc.node)
	ctx := context.Background()
	block := types.GetRandomBlock(1, 10)
	err := rpc.node.Store.SaveBlock(ctx, block, &types.Commit{})
	rpc.node.Store.SetHeight(ctx, block.Height())
	require.NoError(err)

	blockResp, err := rpc.Block(ctx, nil)
	require.NoError(err)
	require.NotNil(blockResp)

	assert.NotNil(blockResp.Block)
}

func TestGetCommit(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	mockApp, rpc := getRPC(t)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	blocks := []*types.Block{types.GetRandomBlock(1, 5), types.GetRandomBlock(2, 6), types.GetRandomBlock(3, 8), types.GetRandomBlock(4, 10)}

	startNodeWithCleanup(t, rpc.node)
	ctx := context.Background()
	for _, b := range blocks {
		err := rpc.node.Store.SaveBlock(ctx, b, &types.Commit{})
		rpc.node.Store.SetHeight(ctx, b.Height())
		require.NoError(err)
	}
	t.Run("Fetch all commits", func(t *testing.T) {
		for _, b := range blocks {
			h := int64(b.Height())
			commit, err := rpc.Commit(ctx, &h)
			require.NoError(err)
			require.NotNil(commit)
			assert.Equal(h, commit.Height)
		}
	})

	t.Run("Fetch commit for nil height", func(t *testing.T) {
		commit, err := rpc.Commit(ctx, nil)
		require.NoError(err)
		require.NotNil(commit)
		assert.Equal(int64(blocks[3].Height()), commit.Height)
	})
}

func TestCometBFTLightClientCompability(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	mockApp, rpc := getRPC(t)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	// creat 3 consecutive signed blocks
	config := types.BlockConfig{
		Height: 1,
		NTxs:   1,
	}
	block1, privKey := types.GenerateRandomBlockCustom(&config)
	block2 := types.GetRandomNextBlock(block1, privKey, []byte{}, 2)
	block3 := types.GetRandomNextBlock(block2, privKey, []byte{}, 3)

	blocks := []*types.Block{block1, block2, block3}

	startNodeWithCleanup(t, rpc.node)
	ctx := context.Background()

	// save the 3 blocks
	for _, b := range blocks {
		err := rpc.node.Store.SaveBlock(ctx, b, &b.SignedHeader.Commit) // #nosec G601
		rpc.node.Store.SetHeight(ctx, b.Height())
		require.NoError(err)
	}

	// Check if the block header provided by rpc.Commit() can be verified using tendermint light client
	t.Run("checking rollkit ABCI header verifiability", func(t *testing.T) {
		var (
			trustingPeriod = 3 * time.Hour
			trustLevel     = cmtmath.Fraction{Numerator: 2, Denominator: 1}
			maxClockDrift  = 10 * time.Second
		)

		// trusted header to verify against
		var trustedHeader cmtypes.SignedHeader
		setTrustedHeader := false
		// valset of single sequencer is constant
		fixedValSet := block1.SignedHeader.Validators

		// for each block (except block 1), verify it's ABCI header with previous block's ABCI header as trusted header
		for _, b := range blocks {
			h := int64(b.Height())
			commit, err := rpc.Commit(context.Background(), &h)
			require.NoError(err)
			require.NotNil(commit)
			require.NotNil(commit.SignedHeader)
			assert.Equal(h, commit.Height)

			// if there's no trusted header to verify against, set trusted header and skip verifying
			if !setTrustedHeader {
				trustedHeader = commit.SignedHeader
				setTrustedHeader = true
				continue
			}

			// verify the ABCI header
			err = light.Verify(&trustedHeader, fixedValSet, &commit.SignedHeader, fixedValSet, trustingPeriod, b.Time(), maxClockDrift, trustLevel)
			require.NoError(err, "failed to pass light.Verify()")

			trustedHeader = commit.SignedHeader
		}
	})
}

func TestBlockSearch(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	mockApp, rpc := getRPC(t)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	ctx := context.Background()
	heights := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, h := range heights {
		block := types.GetRandomBlock(uint64(h), 5)
		err := rpc.node.Store.SaveBlock(ctx, block, &types.Commit{})
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
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	startNodeWithCleanup(t, rpc.node)
	ctx := context.Background()
	block := types.GetRandomBlock(1, 10)
	err := rpc.node.Store.SaveBlock(ctx, block, &types.Commit{})
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

func finalizeBlockResponse(_ context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
	txResults := make([]*abci.ExecTxResult, len(req.Txs))
	for idx := range req.Txs {
		txResults[idx] = &abci.ExecTxResult{
			Code: abci.CodeTypeOK,
		}
	}

	return &abci.ResponseFinalizeBlock{
		TxResults: txResults,
	}, nil
}

func TestTx(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	mockApp.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	mockApp.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := newFullNode(ctx, config.NodeConfig{
		DAAddress:   MockDAAddress,
		DANamespace: MockDANamespace,
		Aggregator:  true,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 1 * time.Second, // blocks must be at least 1 sec apart for adjacent headers to get verified correctly
		}},
		key, signingKey, proxy.NewLocalClientCreator(mockApp),
		genesisDoc,
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
		test.NewFileLogger(t))
	require.NoError(err)
	require.NotNil(node)

	rpc := NewFullClient(node)
	require.NotNil(rpc)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)

	startNodeWithCleanup(t, rpc.node)
	tx1 := cmtypes.Tx("tx1")
	res, err := rpc.BroadcastTxSync(ctx, tx1)
	assert.NoError(err)
	assert.NotNil(res)

	time.Sleep(2 * time.Second)

	resTx, errTx := rpc.Tx(ctx, res.Hash, true)
	assert.NoError(errTx)
	assert.NotNil(resTx)
	assert.EqualValues(tx1, resTx.Tx)
	assert.EqualValues(res.Hash, resTx.Hash)

	tx2 := cmtypes.Tx("tx2")
	resTx, errTx = rpc.Tx(ctx, tx2.Hash(), true)
	assert.Nil(resTx)
	assert.Error(errTx)
	assert.Equal(fmt.Errorf("tx (%X) not found", tx2.Hash()), errTx)
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

			mockApp, rpc := getRPC(t)
			mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
			mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)

			startNodeWithCleanup(t, rpc.node)

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

	mockApp, rpc := getRPC(t)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)

	startNodeWithCleanup(t, rpc.node)

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
	assert.NotContains(txRes.Txs, tx2[0])
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
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	ctx := context.Background()

	heights := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, h := range heights {
		block := types.GetRandomBlock(uint64(h), 5)
		err := rpc.node.Store.SaveBlock(ctx, block, &types.Commit{})
		rpc.node.Store.SetHeight(ctx, block.Height())
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
				assert.Equal(result.BlockMetas[0].BlockID.Hash, test.exp[1].BlockID.Hash)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].BlockID.Hash, test.exp[0].BlockID.Hash)
				assert.Equal(result.BlockMetas[0].Header.Version.Block, test.exp[1].Header.Version.Block)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header.Version.Block, test.exp[0].Header.Version.Block)
				assert.Equal(result.BlockMetas[0].Header, test.exp[1].Header)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header, test.exp[0].Header)
				assert.Equal(result.BlockMetas[0].Header.DataHash, test.exp[1].Header.DataHash)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header.DataHash, test.exp[0].Header.DataHash)
				assert.Equal(result.BlockMetas[0].Header.LastCommitHash, test.exp[1].Header.LastCommitHash)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header.LastCommitHash, test.exp[0].Header.LastCommitHash)
				assert.Equal(result.BlockMetas[0].Header.EvidenceHash, test.exp[1].Header.EvidenceHash)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header.AppHash, test.exp[0].Header.AppHash)
				assert.Equal(result.BlockMetas[0].Header.AppHash, test.exp[1].Header.AppHash)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header.ConsensusHash, test.exp[0].Header.ConsensusHash)
				assert.Equal(result.BlockMetas[0].Header.ConsensusHash, test.exp[1].Header.ConsensusHash)
				assert.Equal(result.BlockMetas[len(result.BlockMetas)-1].Header.ValidatorsHash, test.exp[0].Header.ValidatorsHash)
				assert.Equal(result.BlockMetas[0].Header.NextValidatorsHash, test.exp[1].Header.NextValidatorsHash)
			}

		})
	}
}

func TestMempool2Nodes(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey1, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse)
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("CheckTx", mock.Anything, &abci.RequestCheckTx{Tx: []byte("bad")}).Return(&abci.ResponseCheckTx{Code: 1}, nil)
	app.On("CheckTx", mock.Anything, &abci.RequestCheckTx{Tx: []byte("good")}).Return(&abci.ResponseCheckTx{Code: 0}, nil)
	app.On("CheckTx", mock.Anything, &abci.RequestCheckTx{Tx: []byte("good"), Type: abci.CheckTxType_Recheck}).Return(&abci.ResponseCheckTx{Code: 0}, nil).Maybe()
	key1, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	key2, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	signingKey2, _, _ := crypto.GenerateEd25519Key(crand.Reader)

	id1, err := peer.IDFromPrivateKey(key1)
	require.NoError(err)

	// app.Commit(context.Background(), )
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// make node1 an aggregator, so that node2 can start gracefully
	node1, err := newFullNode(ctx, config.NodeConfig{
		DAAddress:   MockDAAddress,
		DANamespace: MockDANamespace,
		Aggregator:  true,
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9001",
		},
		BlockManagerConfig: getBMConfig(),
	}, key1, signingKey1, proxy.NewLocalClientCreator(app), genesisDoc, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), log.TestingLogger())
	require.NoError(err)
	require.NotNil(node1)

	node2, err := newFullNode(ctx, config.NodeConfig{
		DAAddress:   MockDAAddress,
		DANamespace: MockDANamespace,
		P2P: config.P2PConfig{
			ListenAddress: "/ip4/127.0.0.1/tcp/9002",
			Seeds:         "/ip4/127.0.0.1/tcp/9001/p2p/" + id1.Loggable()["peerID"].(string),
		},
	}, key2, signingKey2, proxy.NewLocalClientCreator(app), genesisDoc, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), log.TestingLogger())
	require.NoError(err)
	require.NotNil(node2)

	startNodeWithCleanup(t, node1)
	require.NoError(waitForFirstBlock(node1, Store))

	startNodeWithCleanup(t, node2)
	require.NoError(waitForAtLeastNBlocks(node2, 1, Store))

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
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	pubKey := genesisDoc.Validators[0].PubKey

	// note that node is never started - we only need to ensure that node is properly initialized (newFullNode)
	node, err := newFullNode(
		context.Background(),
		config.NodeConfig{
			DAAddress:   MockDAAddress,
			DANamespace: MockDANamespace,
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
		genesisDoc,
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
		test.NewFileLogger(t),
	)
	require.NoError(err)
	require.NotNil(node)
	ctx := context.Background()

	err = node.Store.UpdateState(ctx, types.State{})
	assert.NoError(err)

	rpc := NewFullClient(node)
	assert.NotNil(rpc)

	config := types.BlockConfig{
		Height:       1,
		NTxs:         1,
		ProposerAddr: pubKey.Bytes(),
	}
	earliestBlock, _ := types.GenerateRandomBlockCustom(&config)
	err = rpc.node.Store.SaveBlock(ctx, earliestBlock, &types.Commit{})
	rpc.node.Store.SetHeight(ctx, earliestBlock.Height())
	require.NoError(err)

	config = types.BlockConfig{
		Height:       2,
		NTxs:         1,
		ProposerAddr: pubKey.Bytes(),
	}
	latestBlock, _ := types.GenerateRandomBlockCustom(&config)
	err = rpc.node.Store.SaveBlock(ctx, latestBlock, &types.Commit{})
	rpc.node.Store.SetHeight(ctx, latestBlock.Height())
	require.NoError(err)

	resp, err := rpc.Status(context.Background())
	assert.NoError(err)

	t.Run("SyncInfo", func(t *testing.T) {
		assert.EqualValues(earliestBlock.Height(), resp.SyncInfo.EarliestBlockHeight)
		assert.EqualValues(latestBlock.Height(), resp.SyncInfo.LatestBlockHeight)
		assert.Equal(
			hex.EncodeToString(earliestBlock.SignedHeader.AppHash),
			hex.EncodeToString(resp.SyncInfo.EarliestAppHash))
		assert.Equal(
			hex.EncodeToString(latestBlock.SignedHeader.AppHash),
			hex.EncodeToString(resp.SyncInfo.LatestAppHash))

		assert.Equal(hex.EncodeToString(earliestBlock.SignedHeader.DataHash), hex.EncodeToString(resp.SyncInfo.EarliestBlockHash))
		assert.Equal(hex.EncodeToString(latestBlock.SignedHeader.DataHash), hex.EncodeToString(resp.SyncInfo.LatestBlockHash))
		assert.Equal(false, resp.SyncInfo.CatchingUp)
	})
	t.Run("ValidatorInfo", func(t *testing.T) {
		assert.Equal(genesisDoc.Validators[0].Address, resp.ValidatorInfo.Address)
		assert.Equal(genesisDoc.Validators[0].PubKey, resp.ValidatorInfo.PubKey)
		assert.EqualValues(int64(1), resp.ValidatorInfo.VotingPower)
	})
	t.Run("NodeInfo", func(t *testing.T) {
		// Changed the RPC method to get this from the genesis.
		// specific validation
		state, err := rpc.node.Store.GetState(ctx)
		assert.NoError(err)

		defaultProtocolVersion := p2p.NewProtocolVersion(
			version.P2PProtocol,
			state.Version.Consensus.Block,
			state.Version.Consensus.App,
		)
		assert.Equal(defaultProtocolVersion, resp.NodeInfo.ProtocolVersion)

		// check that NodeInfo DefaultNodeID matches the ID derived from p2p key
		rawKey, err := key.GetPublic().Raw()
		assert.NoError(err)
		assert.Equal(p2p.ID(hex.EncodeToString(cmcrypto.AddressHash(rawKey))), resp.NodeInfo.DefaultNodeID)

		assert.Equal(rpc.node.nodeConfig.P2P.ListenAddress, resp.NodeInfo.ListenAddr)
		assert.Equal(rpc.node.genesis.ChainID, resp.NodeInfo.Network)
		// TODO: version match.
		assert.Equal(cmconfig.DefaultBaseConfig().Moniker, resp.NodeInfo.Moniker)

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
		assert.Equal(rpc.config.ListenAddress, resp.NodeInfo.Other.RPCAddress)
	})
}

func TestFutureGenesisTime(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var beginBlockTime time.Time
	wg := sync.WaitGroup{}
	wg.Add(1)
	mockApp := &mocks.Application{}
	mockApp.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	mockApp.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	mockApp.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse).Run(func(_ mock.Arguments) {
		beginBlockTime = time.Now()
		wg.Done()
	})
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	key, _, _ := crypto.GenerateEd25519Key(crand.Reader)
	genesisDoc, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(err)
	genesisTime := time.Now().Local().Add(time.Second * time.Duration(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := newFullNode(ctx, config.NodeConfig{
		DAAddress:   MockDAAddress,
		DANamespace: MockDANamespace,
		Aggregator:  true,
		BlockManagerConfig: config.BlockManagerConfig{
			BlockTime: 200 * time.Millisecond,
		}},
		key, signingKey,
		proxy.NewLocalClientCreator(mockApp),
		&cmtypes.GenesisDoc{
			ChainID:       "test",
			InitialHeight: 1,
			GenesisTime:   genesisTime,
			Validators:    genesisDoc.Validators,
		},
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
		test.NewFileLogger(t))
	require.NoError(err)
	require.NotNil(node)

	startNodeWithCleanup(t, node)
	wg.Wait()

	assert.True(beginBlockTime.After(genesisTime))
}

func TestHealth(t *testing.T) {
	assert := assert.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(abci.ResponseCheckTx{}, nil)
	mockApp.On("Commit", mock.Anything).Return(abci.ResponseCommit{}, nil)

	startNodeWithCleanup(t, rpc.node)

	resultHealth, err := rpc.Health(context.Background())
	assert.Nil(err)
	assert.Empty(resultHealth)
}

func TestNetInfo(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("FinalizeBlock", mock.Anything, mock.Anything).Return(finalizeBlockResponse)
	mockApp.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	mockApp.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)

	startNodeWithCleanup(t, rpc.node)

	netInfo, err := rpc.NetInfo(context.Background())
	require.NoError(err)
	assert.NotNil(netInfo)
	assert.True(netInfo.Listening)
	assert.Equal(0, len(netInfo.Peers))
}
