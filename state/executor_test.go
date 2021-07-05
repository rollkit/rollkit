package state

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	cfg "github.com/lazyledger/lazyledger-core/config"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/mocks"
	"github.com/lazyledger/optimint/types"
)

func TestCreateBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(err)
	require.NotNil(client)

	nsID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	mpool := mempool.NewCListMempool(cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client), 0)
	executor := NewBlockExecutor(key, nsID, mpool, proxy.NewAppConnConsensus(client), logger)

	state := State{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	// empty block
	block := executor.CreateBlock(1, &types.Commit{}, state)
	require.NotNil(block)
	assert.Empty(block.Data.Txs)
	assert.Equal(uint64(1), block.Header.Height)

	// one small Tx
	err = mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block = executor.CreateBlock(2, &types.Commit{}, state)
	require.NotNil(block)
	assert.Equal(uint64(2), block.Header.Height)
	assert.Len(block.Data.Txs, 1)

	// now there are 3 Txs, and only two can fit into single block
	err = mpool.CheckTx([]byte{4, 5, 6, 7}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	err = mpool.CheckTx(make([]byte, 100), func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block = executor.CreateBlock(3, &types.Commit{}, state)
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

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(err)
	require.NotNil(client)

	nsID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	mpool := mempool.NewCListMempool(cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client), 0)
	executor := NewBlockExecutor(key, nsID, mpool, proxy.NewAppConnConsensus(client), logger)

	state := State{}
	state.InitialHeight = 1
	state.LastBlockHeight = 0
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	_ = mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	require.NoError(err)
	block := executor.CreateBlock(1, &types.Commit{}, state)
	require.NotNil(block)
	assert.Equal(uint64(1), block.Header.Height)
	assert.Len(block.Data.Txs, 1)

	newState, _, err := executor.ApplyBlock(context.TODO(), state, block)
	require.NoError(err)
	require.NotNil(newState)
	assert.Equal(int64(1), newState.LastBlockHeight)
	assert.Equal(mockAppHash, newState.AppHash)

	require.NoError(mpool.CheckTx([]byte{0, 1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx([]byte{5, 6, 7, 8, 9}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx([]byte{1, 2, 3, 4, 5}, func(r *abci.Response) {}, mempool.TxInfo{}))
	require.NoError(mpool.CheckTx(make([]byte, 90), func(r *abci.Response) {}, mempool.TxInfo{}))
	block = executor.CreateBlock(2, &types.Commit{}, newState)
	require.NotNil(block)
	assert.Equal(uint64(2), block.Header.Height)
	assert.Len(block.Data.Txs, 3)

	newState, _, err = executor.ApplyBlock(context.TODO(), newState, block)
	require.NoError(err)
	require.NotNil(newState)
	assert.Equal(int64(2), newState.LastBlockHeight)
}
