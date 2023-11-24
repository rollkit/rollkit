package state

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/mempool"
	mempoolv1 "github.com/rollkit/rollkit/mempool/v1"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

const (
	CheckTx    = "CheckTx"
	BeginBlock = "BeginBlock"
	DeliverTx  = "DeliverTx"
	EndBlock   = "EndBlock"
	Commit     = "Commit"
)

func doTestCreateBlock(t *testing.T) {
	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})

	fmt.Println("App On CheckTx")
	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	fmt.Println("Created New Local Client")
	require.NoError(t, err)
	require.NotNil(t, client)

	nsID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	fmt.Println("Made NID")
	mpool := mempoolv1.NewTxMempool(logger, cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client, proxy.NopMetrics()), 0)
	fmt.Println("Made a NewTxMempool")
	executor := NewBlockExecutor([]byte("test address"), nsID, "test", mpool, proxy.NewAppConnConsensus(client, proxy.NopMetrics()), nil, logger)
	fmt.Println("Made a New Block Executor")

	state := types.State{}

	state.ConsensusParams.Block = &cmproto.BlockParams{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	// empty block
	block := executor.CreateBlock(1, &types.Commit{}, []byte{}, state)
	require.NotNil(t, block)
	assert.Empty(t, block.Data.Txs)
	assert.Equal(t, uint64(1), block.Height())

	// one small Tx
	err = mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(t, err)
	block = executor.CreateBlock(2, &types.Commit{}, []byte{}, state)
	require.NotNil(t, block)
	assert.Equal(t, uint64(2), block.Height())
	assert.Len(t, block.Data.Txs, 1)

	// now there are 3 Txs, and only two can fit into single block
	err = mpool.CheckTx([]byte{4, 5, 6, 7}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(t, err)
	err = mpool.CheckTx(make([]byte, 100), func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(t, err)
	block = executor.CreateBlock(3, &types.Commit{}, []byte{}, state)
	require.NotNil(t, block)
	assert.Len(t, block.Data.Txs, 2)
}

func TestCreateBlockWithFraudProofsDisabled(t *testing.T) {
	doTestCreateBlock(t)
}

func doTestApplyBlock(t *testing.T) {
	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	app.On(BeginBlock, mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On(DeliverTx, mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On(EndBlock, mock.Anything).Return(abci.ResponseEndBlock{})
	var mockAppHash []byte
	_, err := rand.Read(mockAppHash[:])
	require.NoError(t, err)
	app.On(Commit, mock.Anything).Return(abci.ResponseCommit{
		Data: mockAppHash[:],
	})

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(t, err)
	require.NotNil(t, client)

	nsID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	chainID := "test"

	mpool := mempoolv1.NewTxMempool(logger, cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client, proxy.NopMetrics()), 0)
	eventBus := cmtypes.NewEventBus()
	require.NoError(t, eventBus.Start())
	executor := NewBlockExecutor([]byte("test address"), nsID, chainID, mpool, proxy.NewAppConnConsensus(client, proxy.NopMetrics()), eventBus, logger)

	txQuery, err := query.New("tm.event='Tx'")
	require.NoError(t, err)
	txSub, err := eventBus.Subscribe(context.Background(), "test", txQuery, 1000)
	require.NoError(t, err)
	require.NotNil(t, txSub)

	headerQuery, err := query.New("tm.event='NewBlockHeader'")
	require.NoError(t, err)
	headerSub, err := eventBus.Subscribe(context.Background(), "test", headerQuery, 100)
	require.NoError(t, err)
	require.NotNil(t, headerSub)

	vKey := ed25519.GenPrivKey()
	validators := []*cmtypes.Validator{
		{
			Address:          vKey.PubKey().Address(),
			PubKey:           vKey.PubKey(),
			VotingPower:      int64(100),
			ProposerPriority: int64(1),
		},
	}
	state := types.State{}
	state.InitialHeight = 1
	state.LastBlockHeight = 0
	state.ConsensusParams.Block = &cmproto.BlockParams{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	_ = mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(t, err)
	block := executor.CreateBlock(1, &types.Commit{Signatures: []types.Signature{types.Signature([]byte{1, 1, 1})}}, []byte{}, state)
	require.NotNil(t, block)
	assert.Equal(t, uint64(1), block.Height())
	assert.Len(t, block.Data.Txs, 1)
	dataHash, err := block.Data.Hash()
	assert.NoError(t, err)
	block.SignedHeader.DataHash = dataHash

	// Update the signature on the block to current from last
	headerBytes, _ := block.SignedHeader.Header.MarshalBinary()
	sig, _ := vKey.Sign(headerBytes)
	block.SignedHeader.Commit = types.Commit{
		Signatures: []types.Signature{sig},
	}
	block.SignedHeader.Validators = cmtypes.NewValidatorSet(validators)

	newState, resp, err := executor.ApplyBlock(context.Background(), state, block)
	require.NoError(t, err)
	require.NotNil(t, newState)
	require.NotNil(t, resp)
	assert.Equal(t, uint64(1), newState.LastBlockHeight)
	appHash, _, err := executor.Commit(context.Background(), newState, block, resp)
	require.NoError(t, err)
	assert.Equal(t, mockAppHash, appHash)

	require.NoError(t, mpool.CheckTx([]byte{0, 1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(t, mpool.CheckTx([]byte{5, 6, 7, 8, 9}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(t, mpool.CheckTx([]byte{1, 2, 3, 4, 5}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(t, mpool.CheckTx(make([]byte, 90), func(r *abci.Response) {}, mempool.TxInfo{}))
	block = executor.CreateBlock(2, &types.Commit{Signatures: []types.Signature{types.Signature([]byte{1, 1, 1})}}, []byte{}, newState)
	require.NotNil(t, block)
	assert.Equal(t, uint64(2), block.Height())
	assert.Len(t, block.Data.Txs, 3)
	dataHash, err = block.Data.Hash()
	assert.NoError(t, err)
	block.SignedHeader.DataHash = dataHash

	headerBytes, _ = block.SignedHeader.Header.MarshalBinary()
	sig, _ = vKey.Sign(headerBytes)
	block.SignedHeader.Commit = types.Commit{
		Signatures: []types.Signature{sig},
	}
	block.SignedHeader.Validators = cmtypes.NewValidatorSet(validators)

	newState, resp, err = executor.ApplyBlock(context.Background(), newState, block)
	require.NoError(t, err)
	require.NotNil(t, newState)
	require.NotNil(t, resp)
	assert.Equal(t, uint64(2), newState.LastBlockHeight)
	_, _, err = executor.Commit(context.Background(), newState, block, resp)
	require.NoError(t, err)

	// wait for at least 4 Tx events, for up to 3 second.
	// 3 seconds is a fail-scenario only
	timer := time.NewTimer(3 * time.Second)
	txs := make(map[int64]int)
	cnt := 0
	for cnt != 4 {
		select {
		case evt := <-txSub.Out():
			cnt++
			data, ok := evt.Data().(cmtypes.EventDataTx)
			assert.True(t, ok)
			assert.NotEmpty(t, data.Tx)
			txs[data.Height]++
		case <-timer.C:
			t.FailNow()
		}
	}
	assert.Zero(t, len(txSub.Out())) // expected exactly 4 Txs - channel should be empty
	assert.EqualValues(t, 1, txs[1])
	assert.EqualValues(t, 3, txs[2])

	require.EqualValues(t, 2, len(headerSub.Out()))
	for h := 1; h <= 2; h++ {
		evt := <-headerSub.Out()
		data, ok := evt.Data().(cmtypes.EventDataNewBlockHeader)
		assert.True(t, ok)
		if data.Header.Height == 2 {
			assert.EqualValues(t, 3, data.NumTxs)
		}
	}
}

func TestApplyBlockWithFraudProofsDisabled(t *testing.T) {
	doTestApplyBlock(t)
}
