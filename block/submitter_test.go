package block

import (
	"context"
	"testing"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func newTestManagerWithDA(t *testing.T, da *mocks.DA) (m *Manager, mockStore *mocks.Store) {
	logger := log.NewNopLogger()
	nodeConf := config.DefaultConfig
	mockStore = mocks.NewStore(t)

	// Mock initial metadata reads during manager creation if necessary
	mockStore.On("GetMetadata", mock.Anything, DAIncludedHeightKey).Return(nil, ds.ErrNotFound).Maybe()
	mockStore.On("GetMetadata", mock.Anything, LastBatchDataKey).Return(nil, ds.ErrNotFound).Maybe()
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Maybe()

	return &Manager{
		store:         mockStore,
		da:            da,
		logger:        logger,
		config:        nodeConf,
		gasPrice:      1.0,
		gasMultiplier: 2.0,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
	}, mockStore
}

func TestSubmitBatchToDA_Success(t *testing.T) {
	da := &mocks.DA{}
	m, _ := newTestManagerWithDA(t, da)

	batch := coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}

	// Simulate DA success
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	err := m.submitBatchToDA(context.Background(), batch)
	assert.NoError(t, err)
}

func TestSubmitBatchToDA_AlreadyInMempool(t *testing.T) {
	da := &mocks.DA{}
	m, _ := newTestManagerWithDA(t, da)

	batch := coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}

	// Simulate DA success
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, coreda.ErrTxAlreadyInMempool)

	err := m.submitBatchToDA(context.Background(), batch)
	assert.Error(t, err)
	assert.EqualError(t, err, "failed to submit all transactions to DA layer, submitted 0 txs (2 left) after 30 attempts")
}

/*
func TestSubmitHeadersToDA_Success(t *testing.T) {
	da := &mocks.DA{}
	m, mockStore := newTestManagerWithDA(t, da)

	// Prepare a mock header
	pendingHeaders, _ := NewPendingHeaders(m.store, m.logger)
	m.pendingHeaders = pendingHeaders

	// Simulate DA success
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
	mockStore.On("Height", mock.Anything).Return(uint64(10))

	// Call submitHeadersToDA
	err := m.submitHeadersToDA(context.Background())
	assert.NoError(t, err)
}
*/
