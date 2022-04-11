package state

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abciclient "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/pubsub/query"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/mempool"
	mempoolv1 "github.com/celestiaorg/optimint/mempool/v1"
	"github.com/celestiaorg/optimint/mocks"
	"github.com/celestiaorg/optimint/types"
)

func TestCreateBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})

	client := abciclient.NewLocalClient(nil, app)
	require.NotNil(client)

	nsID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}

	appClient := abciclient.NewLocalClient(nil, app)
	mpool := mempoolv1.NewTxMempool(logger, cfg.DefaultMempoolConfig(), appClient.(types.AppConnMempool), 1)
	executor := NewBlockExecutor([]byte("test address"), nsID, "test", mpool, appClient, nil, logger)

	state := State{}
	state.ConsensusParams.Block = new(tmproto.BlockParams)
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000
	state.Validators = tmtypes.NewValidatorSet(nil)

	// empty block
	block := executor.CreateBlock(1, &types.Commit{}, [32]byte{}, state)
	require.NotNil(block)
	assert.Empty(block.Data.Txs)
	assert.Equal(uint64(1), block.Header.Height)

	ctx := context.Background()
	// one small Tx
	err := mpool.CheckTx(ctx, []byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block = executor.CreateBlock(2, &types.Commit{}, [32]byte{}, state)
	require.NotNil(block)
	assert.Equal(uint64(2), block.Header.Height)
	assert.Len(block.Data.Txs, 1)

	// now there are 3 Txs, and only two can fit into single block
	err = mpool.CheckTx(ctx, []byte{4, 5, 6, 7}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	err = mpool.CheckTx(ctx, make([]byte, 100), func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block = executor.CreateBlock(3, &types.Commit{}, [32]byte{}, state)
	require.NotNil(block)
	assert.Len(block.Data.Txs, 2)
}

func TestApplyBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	var mockAppHash [32]byte
	_, err := rand.Read(mockAppHash[:])
	require.NoError(err)
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{
		Data: mockAppHash[:],
	})

	appClient := abciclient.NewLocalClient(nil, app)
	require.NotNil(appClient)

	nsID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	chainID := "test"

	mpool := mempoolv1.NewTxMempool(logger, cfg.DefaultMempoolConfig(), appClient.(types.AppConnMempool), 1)
	eventBus := tmtypes.NewEventBus()
	require.NoError(eventBus.Start())
	executor := NewBlockExecutor([]byte("test address"), nsID, chainID, mpool, appClient, eventBus, logger)

	txQuery, err := query.New("tm.event='Tx'")
	require.NoError(err)
	txSub, err := eventBus.Subscribe(context.Background(), "test", txQuery, 1000)
	require.NoError(err)
	require.NotNil(txSub)

	headerQuery, err := query.New("tm.event='NewBlockHeader'")
	require.NoError(err)
	headerSub, err := eventBus.Subscribe(context.Background(), "test", headerQuery, 100)
	require.NoError(err)
	require.NotNil(headerSub)

	state := State{
		NextValidators: tmtypes.NewValidatorSet(nil),
		Validators:     tmtypes.NewValidatorSet(nil),
		LastValidators: tmtypes.NewValidatorSet(nil),
	}
	state.InitialHeight = 1
	state.LastBlockHeight = 0
	state.ConsensusParams.Block = new(tmproto.BlockParams)
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	ctx := context.Background()

	_ = mpool.CheckTx(ctx, []byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block := executor.CreateBlock(1, &types.Commit{}, [32]byte{}, state)
	require.NotNil(block)
	assert.Equal(uint64(1), block.Header.Height)
	assert.Len(block.Data.Txs, 1)

	newState, resp, _, err := executor.ApplyBlock(context.Background(), state, block)
	require.NoError(err)
	require.NotNil(newState)
	require.NotNil(resp)
	assert.Equal(int64(1), newState.LastBlockHeight)
	assert.Equal(mockAppHash, newState.AppHash)

	require.NoError(mpool.CheckTx(ctx, []byte{0, 1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(ctx, []byte{5, 6, 7, 8, 9}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(ctx, []byte{1, 2, 3, 4, 5}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(ctx, make([]byte, 90), func(r *abci.Response) {}, mempool.TxInfo{}))
	block = executor.CreateBlock(2, &types.Commit{}, [32]byte{}, newState)
	require.NotNil(block)
	assert.Equal(uint64(2), block.Header.Height)
	assert.Len(block.Data.Txs, 3)

	newState, resp, _, err = executor.ApplyBlock(context.Background(), newState, block)
	require.NoError(err)
	require.NotNil(newState)
	require.NotNil(resp)
	assert.Equal(int64(2), newState.LastBlockHeight)

	// wait for at least 4 Tx events, for up to 3 second.
	// 3 seconds is a fail-scenario only
	timer := time.NewTimer(3 * time.Second)
	txs := make(map[int64]int)
	cnt := 0
	for cnt != 4 {
		select {
		case evt := <-txSub.Out():
			cnt++
			data, ok := evt.Data().(tmtypes.EventDataTx)
			assert.True(ok)
			assert.NotEmpty(data.Tx)
			txs[data.Height]++
		case <-timer.C:
			t.FailNow()
		}
	}
	assert.Zero(len(txSub.Out())) // expected exactly 4 Txs - channel should be empty
	assert.EqualValues(1, txs[1])
	assert.EqualValues(3, txs[2])

	require.EqualValues(2, len(headerSub.Out()))
	for h := 1; h <= 2; h++ {
		evt := <-headerSub.Out()
		data, ok := evt.Data().(tmtypes.EventDataNewBlockHeader)
		assert.True(ok)
		if data.Header.Height == 2 {
			assert.EqualValues(3, data.NumTxs)
		}
	}
}
