package state

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/celestiaorg/go-cnc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/tendermint/tendermint/abci/types"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/pubsub/query"
	"github.com/tendermint/tendermint/proxy"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/mempool"
	mempoolv1 "github.com/rollkit/rollkit/mempool/v1"
	"github.com/rollkit/rollkit/mocks"
	"github.com/rollkit/rollkit/types"
)

func doTestCreateBlock(t *testing.T, fraudProofsEnabled bool) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(err)
	require.NotNil(client)

	nsID := cnc.MustNewV0([]byte{0, 0, 1, 2, 3, 4, 5, 6, 7, 8})

	mpool := mempoolv1.NewTxMempool(logger, cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client), 0)
	executor := NewBlockExecutor([]byte("test address"), nsID, "test", mpool, proxy.NewAppConnConsensus(client), fraudProofsEnabled, nil, logger)

	state := types.State{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000
	vKey := ed25519.GenPrivKey()
	validators := []*tmtypes.Validator{
		{
			Address:          vKey.PubKey().Address(),
			PubKey:           vKey.PubKey(),
			VotingPower:      int64(100),
			ProposerPriority: int64(1),
		},
	}
	state.Validators = tmtypes.NewValidatorSet(validators)

	// empty block
	block := executor.CreateBlock(1, &types.Commit{}, []byte{}, state)
	require.NotNil(block)
	assert.Empty(block.Data.Txs)
	assert.Equal(int64(1), block.SignedHeader.Header.Height())

	// one small Tx
	err = mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block = executor.CreateBlock(2, &types.Commit{}, []byte{}, state)
	require.NotNil(block)
	assert.Equal(int64(2), block.SignedHeader.Header.Height())
	assert.Len(block.Data.Txs, 1)

	// now there are 3 Txs, and only two can fit into single block
	err = mpool.CheckTx([]byte{4, 5, 6, 7}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	err = mpool.CheckTx(make([]byte, 100), func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block = executor.CreateBlock(3, &types.Commit{}, []byte{}, state)
	require.NotNil(block)
	assert.Len(block.Data.Txs, 2)
}

func TestCreateBlockWithFraudProofsDisabled(t *testing.T) {
	doTestCreateBlock(t, false)
}

func TestCreateBlockWithFraudProofsEnabled(t *testing.T) {
	doTestCreateBlock(t, true)
}

func doTestApplyBlock(t *testing.T, fraudProofsEnabled bool) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("GenerateFraudProof", mock.Anything).Return(abci.ResponseGenerateFraudProof{})
	var mockAppHash []byte
	_, err := rand.Read(mockAppHash[:])
	require.NoError(err)
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{
		AppHash: mockAppHash[:],
	})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{
		Data: mockAppHash[:],
	})

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(err)
	require.NotNil(client)

	nsID := cnc.MustNewV0([]byte{0, 0, 1, 2, 3, 4, 5, 6, 7, 8})
	chainID := "test"

	mpool := mempoolv1.NewTxMempool(logger, cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client), 0)
	eventBus := tmtypes.NewEventBus()
	require.NoError(eventBus.Start())
	executor := NewBlockExecutor([]byte("test address"), nsID, chainID, mpool, proxy.NewAppConnConsensus(client), fraudProofsEnabled, eventBus, logger)

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

	vKey := ed25519.GenPrivKey()
	validators := []*tmtypes.Validator{
		{
			Address:          vKey.PubKey().Address(),
			PubKey:           vKey.PubKey(),
			VotingPower:      int64(100),
			ProposerPriority: int64(1),
		},
	}
	state := types.State{
		NextValidators: tmtypes.NewValidatorSet(validators),
		Validators:     tmtypes.NewValidatorSet(validators),
		LastValidators: tmtypes.NewValidatorSet(validators),
	}
	state.InitialHeight = 1
	state.LastBlockHeight = 0
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	_ = mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block := executor.CreateBlock(1, &types.Commit{Signatures: []types.Signature{types.Signature([]byte{1, 1, 1})}}, []byte{}, state)
	require.NotNil(block)
	assert.Equal(int64(1), block.SignedHeader.Header.Height())
	assert.Len(block.Data.Txs, 1)

	// Update the signature on the block to current from last
	headerBytes, _ := block.SignedHeader.Header.MarshalBinary()
	sig, _ := vKey.Sign(headerBytes)
	block.SignedHeader.Commit = types.Commit{
		Signatures: []types.Signature{sig},
	}
	block.SignedHeader.Validators = tmtypes.NewValidatorSet(validators)

	newState, resp, err := executor.ApplyBlock(context.Background(), state, block)
	require.NoError(err)
	require.NotNil(newState)
	require.NotNil(resp)
	assert.Equal(int64(1), newState.LastBlockHeight)
	appHash, _, err := executor.Commit(context.Background(), newState, block, resp)
	require.NoError(err)
	assert.Equal(mockAppHash, appHash)

	require.NoError(mpool.CheckTx([]byte{0, 1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx([]byte{5, 6, 7, 8, 9}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx([]byte{1, 2, 3, 4, 5}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(make([]byte, 90), func(r *abci.Response) {}, mempool.TxInfo{}))
	block = executor.CreateBlock(2, &types.Commit{Signatures: []types.Signature{types.Signature([]byte{1, 1, 1})}}, []byte{}, newState)
	require.NotNil(block)
	assert.Equal(int64(2), block.SignedHeader.Header.Height())
	assert.Len(block.Data.Txs, 3)

	headerBytes, _ = block.SignedHeader.Header.MarshalBinary()
	sig, _ = vKey.Sign(headerBytes)
	block.SignedHeader.Commit = types.Commit{
		Signatures: []types.Signature{sig},
	}
	block.SignedHeader.Validators = tmtypes.NewValidatorSet(validators)

	newState, resp, err = executor.ApplyBlock(context.Background(), newState, block)
	require.NoError(err)
	require.NotNil(newState)
	require.NotNil(resp)
	assert.Equal(int64(2), newState.LastBlockHeight)
	_, _, err = executor.Commit(context.Background(), newState, block, resp)
	require.NoError(err)

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

func TestApplyBlockWithFraudProofsDisabled(t *testing.T) {
	doTestApplyBlock(t, false)
}

func TestApplyBlockWithFraudProofsEnabled(t *testing.T) {
	doTestApplyBlock(t, true)
}
