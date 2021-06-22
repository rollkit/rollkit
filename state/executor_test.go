package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	cfg "github.com/lazyledger/lazyledger-core/config"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"

	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/mocks"
	"github.com/lazyledger/optimint/types"
)

func TestCreateProposalBlock(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	logger := log.TestingLogger()

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})

	client, err := proxy.NewLocalClientCreator(app).NewABCIClient()
	require.NoError(err)
	require.NotNil(client)

	mpool := mempool.NewCListMempool(cfg.DefaultMempoolConfig(), proxy.NewAppConnMempool(client), 0)
	executor := NewBlockExecutor(mpool, proxy.NewAppConnConsensus(client), logger)

	state := &State{}
	state.ConsensusParams.Block.MaxBytes = 100
	state.ConsensusParams.Block.MaxGas = 100000

	// empty block
	block := executor.CreateProposalBlock(1, &types.Commit{}, state)
	assert.NotNil(block)
	assert.Empty(block.Data.Txs)
	assert.Equal(uint64(1), block.Header.Height)

	// one small Tx
	mpool.CheckTx([]byte{1, 2, 3, 4}, func(r *abci.Response) {}, mempool.TxInfo{})
	block = executor.CreateProposalBlock(2, &types.Commit{}, state)
	assert.NotNil(block)
	assert.Equal(uint64(2), block.Header.Height)
	assert.Len(block.Data.Txs, 1)

	// now there are 3 Txs, and only two can fit into single block
	mpool.CheckTx([]byte{4, 5, 6, 7}, func(r *abci.Response) {}, mempool.TxInfo{})
	mpool.CheckTx(make([]byte, 100), func(r *abci.Response) {}, mempool.TxInfo{})
	block = executor.CreateProposalBlock(3, &types.Commit{}, state)
	assert.NotNil(block)
	assert.Len(block.Data.Txs, 2)
}
