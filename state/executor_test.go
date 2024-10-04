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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	abci "github.com/cometbft/cometbft/abci/types"
	cfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"

	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// MockSequencerAddress is a sample address used by the mock sequencer
const MockSequencerAddress = "localhost:50051"

func prepareProposalResponse(_ context.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
	return &abci.ResponsePrepareProposal{
		Txs: req.Txs,
	}, nil
}

func doTestCreateBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse)
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	fmt.Println("App On CheckTx")
	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	fmt.Println("Created New Local Client")
	require.NoError(err)
	require.NotNil(client)

	fmt.Println("Made NID")
	mpool := mempool.NewCListMempool(cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client, proxy.NopMetrics()), 0)
	fmt.Println("Made a NewTxMempool")
	executor := NewBlockExecutor([]byte("test address"), "test", mpool, nil, proxy.NewAppConnConsensus(client, proxy.NopMetrics()), nil, 100, logger, NopMetrics())
	fmt.Println("Made a New Block Executor")

	state := types.State{}

	state.ConsensusParams.Block = &cmproto.BlockParams{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	vKey := ed25519.GenPrivKey()
	validators := []*cmtypes.Validator{
		{
			Address:          vKey.PubKey().Address(),
			PubKey:           vKey.PubKey(),
			VotingPower:      int64(100),
			ProposerPriority: int64(1),
		},
	}
	state.Validators = cmtypes.NewValidatorSet(validators)

	// empty block
	header, data, err := executor.CreateBlock(1, &types.Signature{}, abci.ExtendedCommitInfo{}, []byte{}, state, cmtypes.Txs{}, time.Now())
	require.NoError(err)
	require.NotNil(header)
	assert.Empty(data.Txs)
	assert.Equal(uint64(1), header.Height())

	// one small Tx
	tx := []byte{1, 2, 3, 4}
	err = mpool.CheckTx(tx, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{})
	require.NoError(err)
	header, data, err = executor.CreateBlock(2, &types.Signature{}, abci.ExtendedCommitInfo{}, []byte{}, state, cmtypes.Txs{tx}, time.Now())
	require.NoError(err)
	require.NotNil(header)
	assert.Equal(uint64(2), header.Height())
	assert.Len(data.Txs, 1)

	// now there are 2 Txs, and max bytes is 100, so create block should fail
	tx1 := []byte{4, 5, 6, 7}
	tx2 := make([]byte, 100)
	err = mpool.CheckTx(tx1, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{})
	require.NoError(err)
	err = mpool.CheckTx(tx2, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{})
	require.NoError(err)
	header, data, err = executor.CreateBlock(3, &types.Signature{}, abci.ExtendedCommitInfo{}, []byte{}, state, cmtypes.Txs{tx1, tx2}, time.Now())
	require.Error(err)
	require.Nil(header)
	require.Nil(data)

	// limit max bytes and create block should fail
	tx = make([]byte, 10)
	mpool.Flush()
	executor.maxBytes = 10
	err = mpool.CheckTx(tx, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{})
	require.NoError(err)
	header, data, err = executor.CreateBlock(4, &types.Signature{}, abci.ExtendedCommitInfo{}, []byte{}, state, cmtypes.Txs{tx}, time.Now())
	require.Error(err)
	require.Nil(header)
	require.Nil(data)
}

func TestCreateBlockWithFraudProofsDisabled(t *testing.T) {
	doTestCreateBlock(t)
}

func doTestApplyBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	var mockAppHash []byte
	_, err := rand.Read(mockAppHash[:])
	require.NoError(err)

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse)
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(
		func(_ context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
			txResults := make([]*abci.ExecTxResult, len(req.Txs))
			for idx := range req.Txs {
				txResults[idx] = &abci.ExecTxResult{
					Code: abci.CodeTypeOK,
				}
			}

			return &abci.ResponseFinalizeBlock{
				TxResults: txResults,
				AppHash:   mockAppHash,
			}, nil
		},
	)

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(err)
	require.NotNil(client)

	mpool := mempool.NewCListMempool(cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client, proxy.NopMetrics()), 0)
	eventBus := cmtypes.NewEventBus()
	require.NoError(eventBus.Start())

	seqClient := seqGRPC.NewClient()
	require.NoError(seqClient.Start(
		MockSequencerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	))
	mpoolReaper := mempool.NewCListMempoolReaper(mpool, []byte("test"), seqClient, logger)
	ctx := context.Background()
	require.NoError(mpoolReaper.StartReaper(ctx))
	txQuery, err := query.New("tm.event='Tx'")
	require.NoError(err)
	txSub, err := eventBus.Subscribe(ctx, "test", txQuery, 1000)
	require.NoError(err)
	require.NotNil(txSub)

	headerQuery, err := query.New("tm.event='NewBlockHeader'")
	require.NoError(err)
	headerSub, err := eventBus.Subscribe(ctx, "test", headerQuery, 100)
	require.NoError(err)
	require.NotNil(headerSub)

	vKey := ed25519.GenPrivKey()
	validators := []*cmtypes.Validator{
		{
			Address:          vKey.PubKey().Address(),
			PubKey:           vKey.PubKey(),
			VotingPower:      int64(100),
			ProposerPriority: int64(1),
		},
	}
	state := types.State{
		NextValidators: cmtypes.NewValidatorSet(validators),
		Validators:     cmtypes.NewValidatorSet(validators),
		LastValidators: cmtypes.NewValidatorSet(validators),
	}
	state.InitialHeight = 1
	state.LastBlockHeight = 0
	state.ConsensusParams.Block = &cmproto.BlockParams{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000
	chainID := "test"
	executor := NewBlockExecutor(vKey.PubKey().Address().Bytes(), chainID, mpool, mpoolReaper, proxy.NewAppConnConsensus(client, proxy.NopMetrics()), eventBus, 100, logger, NopMetrics())

	tx := []byte{1, 2, 3, 4}
	err = mpool.CheckTx(tx, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{})
	require.NoError(err)
	signature := types.Signature([]byte{1, 1, 1})
	header, data, err := executor.CreateBlock(1, &signature, abci.ExtendedCommitInfo{}, []byte{}, state, cmtypes.Txs{tx}, time.Now())
	require.NoError(err)
	require.NotNil(header)
	assert.Equal(uint64(1), header.Height())
	assert.Len(data.Txs, 1)
	dataHash := data.Hash()
	header.DataHash = dataHash

	// Update the signature on the block to current from last
	voteBytes := header.Header.MakeCometBFTVote()
	signature, _ = vKey.Sign(voteBytes)
	header.Signature = signature
	header.Validators = cmtypes.NewValidatorSet(validators)

	newState, resp, err := executor.ApplyBlock(context.Background(), state, header, data)
	require.NoError(err)
	require.NotNil(newState)
	require.NotNil(resp)
	assert.Equal(uint64(1), newState.LastBlockHeight)
	appHash, _, err := executor.Commit(context.Background(), newState, header, data, resp)
	require.NoError(err)
	assert.Equal(mockAppHash, appHash)

	tx1 := []byte{0, 1, 2, 3, 4}
	tx2 := []byte{5, 6, 7, 8, 9}
	tx3 := []byte{1, 2, 3, 4, 5}
	require.NoError(mpool.CheckTx(tx1, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(tx2, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(tx3, func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{}))
	signature = types.Signature([]byte{1, 1, 1})
	header, data, err = executor.CreateBlock(2, &signature, abci.ExtendedCommitInfo{}, []byte{}, newState, cmtypes.Txs{tx1, tx2, tx3}, time.Now())
	require.NoError(err)
	require.NotNil(header)
	assert.Equal(uint64(2), header.Height())
	assert.Len(data.Txs, 3)
	dataHash = data.Hash()
	header.DataHash = dataHash

	voteBytes = header.Header.MakeCometBFTVote()
	signature, _ = vKey.Sign(voteBytes)
	header.Signature = signature
	header.Validators = cmtypes.NewValidatorSet(validators)

	newState, resp, err = executor.ApplyBlock(context.Background(), newState, header, data)
	require.NoError(err)
	require.NotNil(newState)
	require.NotNil(resp)
	assert.Equal(uint64(2), newState.LastBlockHeight)
	_, _, err = executor.Commit(context.Background(), newState, header, data, resp)
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
			data, ok := evt.Data().(cmtypes.EventDataTx)
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
		_, ok := evt.Data().(cmtypes.EventDataNewBlockHeader)
		assert.True(ok)
	}
}

func TestApplyBlockWithFraudProofsDisabled(t *testing.T) {
	doTestApplyBlock(t)
}

func TestUpdateStateConsensusParams(t *testing.T) {
	logger := log.TestingLogger()
	app := &mocks.Application{}
	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(t, err)
	require.NotNil(t, client)

	chainID := "test"

	mpool := mempool.NewCListMempool(cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client, proxy.NopMetrics()), 0)
	seqClient := seqGRPC.NewClient()
	require.NoError(t, seqClient.Start(
		MockSequencerAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	))
	mpoolReaper := mempool.NewCListMempoolReaper(mpool, []byte("test"), seqClient, logger)
	require.NoError(t, mpoolReaper.StartReaper(context.Background()))
	eventBus := cmtypes.NewEventBus()
	require.NoError(t, eventBus.Start())
	executor := NewBlockExecutor([]byte("test address"), chainID, mpool, mpoolReaper, proxy.NewAppConnConsensus(client, proxy.NopMetrics()), eventBus, 100, logger, NopMetrics())

	state := types.State{
		ConsensusParams: cmproto.ConsensusParams{
			Block: &cmproto.BlockParams{
				MaxBytes: 100,
				MaxGas:   100000,
			},
			Validator: &cmproto.ValidatorParams{
				PubKeyTypes: []string{cmtypes.ABCIPubKeyTypeEd25519},
			},
			Version: &cmproto.VersionParams{
				App: 1,
			},
			Abci: &cmproto.ABCIParams{},
		},
		Validators:     cmtypes.NewValidatorSet([]*cmtypes.Validator{{Address: []byte("test"), PubKey: nil, VotingPower: 100, ProposerPriority: 1}}),
		NextValidators: cmtypes.NewValidatorSet([]*cmtypes.Validator{{Address: []byte("test"), PubKey: nil, VotingPower: 100, ProposerPriority: 1}}),
	}

	header, data := types.GetRandomBlock(1234, 2)

	txResults := make([]*abci.ExecTxResult, len(data.Txs))
	for idx := range data.Txs {
		txResults[idx] = &abci.ExecTxResult{
			Code: abci.CodeTypeOK,
		}
	}

	resp := &abci.ResponseFinalizeBlock{
		ConsensusParamUpdates: &cmproto.ConsensusParams{
			Block: &cmproto.BlockParams{
				MaxBytes: 200,
				MaxGas:   200000,
			},
			Validator: &cmproto.ValidatorParams{
				PubKeyTypes: []string{cmtypes.ABCIPubKeyTypeEd25519},
			},
			Version: &cmproto.VersionParams{
				App: 2,
			},
		},

		TxResults: txResults,
	}
	validatorUpdates, err := cmtypes.PB2TM.ValidatorUpdates(resp.ValidatorUpdates)
	assert.NoError(t, err)

	updatedState, err := executor.updateState(state, header, data, resp, validatorUpdates)
	require.NoError(t, err)

	assert.Equal(t, uint64(1235), updatedState.LastHeightConsensusParamsChanged)
	assert.Equal(t, int64(200), updatedState.ConsensusParams.Block.MaxBytes)
	assert.Equal(t, int64(200000), updatedState.ConsensusParams.Block.MaxGas)
	assert.Equal(t, uint64(2), updatedState.ConsensusParams.Version.App)
}
