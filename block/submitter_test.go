package block

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

const numItemsToSubmit = 3

// newTestManagerWithDA creates a Manager instance with a mocked DA layer for testing.
func newTestManagerWithDA(t *testing.T, da *mocks.DA) (m *Manager) {
	logger := log.NewNopLogger()
	nodeConf := config.DefaultConfig

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	testSigner, err := noop.NewNoopSigner(privKey)
	require.NoError(t, err)

	proposerAddr, err := testSigner.GetAddress()
	require.NoError(t, err)
	gen := genesis.NewGenesis(
		"testchain",
		1,
		time.Now(),
		proposerAddr,
	)

	return &Manager{
		da:             da,
		logger:         logger,
		config:         nodeConf,
		gasPrice:       1.0,
		gasMultiplier:  2.0,
		headerCache:    cache.NewCache[types.SignedHeader](),
		dataCache:      cache.NewCache[types.Data](),
		signer:         testSigner,
		genesis:        gen,
		pendingData:    newPendingData(t),
		pendingHeaders: newPendingHeaders(t),
	}
}

// --- Generic success test for data and headers submission ---
type submitToDASuccessCase[T any] struct {
	name        string
	fillPending func(ctx context.Context, t *testing.T, m *Manager)
	getToSubmit func(m *Manager, ctx context.Context) ([]T, error)
	submitToDA  func(m *Manager, ctx context.Context, items []T) error
	mockDASetup func(da *mocks.DA)
}

func runSubmitToDASuccessCase[T any](t *testing.T, tc submitToDASuccessCase[T]) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	ctx := t.Context()
	tc.fillPending(ctx, t, m)
	tc.mockDASetup(da)

	items, err := tc.getToSubmit(m, ctx)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	err = tc.submitToDA(m, ctx, items)
	assert.NoError(t, err)
}

func TestSubmitDataToDA_Success(t *testing.T) {
	runSubmitToDASuccessCase(t, submitToDASuccessCase[*types.SignedData]{
		name: "Data",
		fillPending: func(ctx context.Context, t *testing.T, m *Manager) {
			fillPendingData(ctx, t, m.pendingData, "Test Submitting Data", numItemsToSubmit)
		},
		getToSubmit: func(m *Manager, ctx context.Context) ([]*types.SignedData, error) {
			return m.createSignedDataToSubmit(ctx)
		},
		submitToDA: func(m *Manager, ctx context.Context, items []*types.SignedData) error {
			return m.submitDataToDA(ctx, items)
		},
		mockDASetup: func(da *mocks.DA) {
			da.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return([]coreda.ID{getDummyID(1, []byte("commitment"))}, nil)
		},
	})
}

func TestSubmitHeadersToDA_Success(t *testing.T) {
	runSubmitToDASuccessCase(t, submitToDASuccessCase[*types.SignedHeader]{
		name: "Headers",
		fillPending: func(ctx context.Context, t *testing.T, m *Manager) {
			fillPendingHeaders(ctx, t, m.pendingHeaders, "Test Submitting Headers", numItemsToSubmit)
		},
		getToSubmit: func(m *Manager, ctx context.Context) ([]*types.SignedHeader, error) {
			return m.pendingHeaders.getPendingHeaders(ctx)
		},
		submitToDA: func(m *Manager, ctx context.Context, items []*types.SignedHeader) error {
			return m.submitHeadersToDA(ctx, items)
		},
		mockDASetup: func(da *mocks.DA) {
			da.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return([]coreda.ID{getDummyID(1, []byte("commitment"))}, nil)
		},
	})
}

// --- Generic failure test for data and headers submission ---
type submitToDAFailureCase[T any] struct {
	name        string
	fillPending func(ctx context.Context, t *testing.T, m *Manager)
	getToSubmit func(m *Manager, ctx context.Context) ([]T, error)
	submitToDA  func(m *Manager, ctx context.Context, items []T) error
	daError     error
}

func runSubmitToDAFailureCase[T any](t *testing.T, tc submitToDAFailureCase[T]) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	ctx := t.Context()
	tc.fillPending(ctx, t, m)

	// Set up the DA mock to return the expected error
	da.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, tc.daError).Times(maxSubmitAttempts)

	items, err := tc.getToSubmit(m, ctx)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	err = tc.submitToDA(m, ctx, items)
	assert.Error(t, err)
}

func TestSubmitDataToDA_Failure(t *testing.T) {
	testCases := []struct {
		name    string
		daError error
	}{
		{"AlreadyInMempool", coreda.ErrTxAlreadyInMempool},
		{"TimedOut", coreda.ErrTxTimedOut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runSubmitToDAFailureCase(t, submitToDAFailureCase[*types.SignedData]{
				name: tc.name,
				fillPending: func(ctx context.Context, t *testing.T, m *Manager) {
					fillPendingData(ctx, t, m.pendingData, "Test Submitting Data", numItemsToSubmit)
				},
				getToSubmit: func(m *Manager, ctx context.Context) ([]*types.SignedData, error) {
					return m.createSignedDataToSubmit(ctx)
				},
				submitToDA: func(m *Manager, ctx context.Context, items []*types.SignedData) error {
					return m.submitDataToDA(ctx, items)
				},
				daError: tc.daError,
			})
		})
	}
}

func TestSubmitHeadersToDA_Failure(t *testing.T) {
	testCases := []struct {
		name    string
		daError error
	}{
		{"AlreadyInMempool", coreda.ErrTxAlreadyInMempool},
		{"TimedOut", coreda.ErrTxTimedOut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runSubmitToDAFailureCase(t, submitToDAFailureCase[*types.SignedHeader]{
				name: tc.name,
				fillPending: func(ctx context.Context, t *testing.T, m *Manager) {
					fillPendingHeaders(ctx, t, m.pendingHeaders, "Test Submitting Headers", numItemsToSubmit)
				},
				getToSubmit: func(m *Manager, ctx context.Context) ([]*types.SignedHeader, error) {
					return m.pendingHeaders.getPendingHeaders(ctx)
				},
				submitToDA: func(m *Manager, ctx context.Context, items []*types.SignedHeader) error {
					return m.submitHeadersToDA(ctx, items)
				},
				daError: tc.daError,
			})
		})
	}
}

// TestCreateSignedDataToSubmit tests createSignedDataToSubmit for normal, empty, and error cases.
func TestCreateSignedDataToSubmit(t *testing.T) {
	// Setup: create a Manager with a valid signer and genesis proposer address
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	testSigner, err := noop.NewNoopSigner(privKey)
	require.NoError(t, err)
	proposerAddr, err := testSigner.GetAddress()
	require.NoError(t, err)

	gen := genesis.NewGenesis("testchain", 1, time.Now(), proposerAddr)

	t.Run("Normal Case", func(t *testing.T) {
		m := &Manager{
			signer:      testSigner,
			genesis:     gen,
			pendingData: newPendingData(t),
		}

		// Create a batch with transactions
		batch := types.Data{
			Height: 1,
			Txs:    []types.Tx{[]byte("tx1"), []byte("tx2")},
		}
		lastSubmitted := uint64(0)
		err := m.pendingData.storePendingData(context.Background(), &batch, lastSubmitted)
		require.NoError(t, err)

		signedData, err := m.createSignedDataToSubmit(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, signedData)
		assert.Len(t, signedData, 1)
		assert.Equal(t, pubKey, signedData[0].Signer.PubKey)
		assert.Equal(t, proposerAddr, signedData[0].Signer.Address)
		assert.NotNil(t, signedData[0].Signature)
	})

	t.Run("Empty Batch", func(t *testing.T) {
		m := &Manager{
			signer:      testSigner,
			genesis:     gen,
			pendingData: newPendingData(t),
		}

		// Create an empty batch
		batch := types.Data{
			Height: 2,
			Txs:    []types.Tx{},
		}
		lastSubmitted := uint64(1)
		err := m.pendingData.storePendingData(context.Background(), &batch, lastSubmitted)
		require.NoError(t, err)

		signedData, err := m.createSignedDataToSubmit(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, signedData)
	})

	t.Run("Nil signer", func(t *testing.T) {
		m := &Manager{
			signer:      nil,
			genesis:     gen,
			pendingData: newPendingData(t),
		}

		// Create a batch with transactions
		batch := types.Data{
			Height: 1,
			Txs:    []types.Tx{[]byte("tx1")},
		}
		lastSubmitted := uint64(0)
		err := m.pendingData.storePendingData(context.Background(), &batch, lastSubmitted)
		require.NoError(t, err)

		signedData, err := m.createSignedDataToSubmit(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signer is nil")
		assert.Nil(t, signedData)
	})
}

// Helper functions for testing

func newPendingData(t *testing.T) *pendingBlocks {
	db := store.New(ds.NewMapDatastore())
	return &pendingBlocks{
		daIncludedTxs: db.NewKVStore("pendingData"),
		logger:        log.NewNopLogger(),
	}
}

func newPendingHeaders(t *testing.T) *pendingBlocks {
	db := store.New(ds.NewMapDatastore())
	return &pendingBlocks{
		daIncludedTxs: db.NewKVStore("pendingHeaders"),
		logger:        log.NewNopLogger(),
	}
}

func newPendingBlocks(t *testing.T) *pendingBlocks {
	db := store.New(ds.NewMapDatastore())
	return &pendingBlocks{
		daIncludedTxs: db.NewKVStore("pendingBlocks"),
		logger:        log.NewNopLogger(),
	}
}

func fillPendingData(ctx context.Context, t *testing.T, pending *pendingBlocks, msgPrefix string, num int) {
	for i := 0; i < num; i++ {
		data := &types.Data{
			Height: uint64(i + 1),
			Txs:    []types.Tx{[]byte(fmt.Sprintf("%s %d", msgPrefix, i))},
		}
		lastSubmitted := uint64(0)
		if i > 0 {
			lastSubmitted = uint64(i)
		}
		err := pending.storePendingData(ctx, data, lastSubmitted)
		require.NoError(t, err)
	}
}

func fillPendingHeaders(ctx context.Context, t *testing.T, pending *pendingBlocks, msgPrefix string, num int) {
	for i := 0; i < num; i++ {
		header := createTestHeader(t, uint64(i+1), msgPrefix)
		lastSubmitted := uint64(0)
		if i > 0 {
			lastSubmitted = uint64(i)
		}
		err := pending.storePendingBlock(ctx, header, lastSubmitted)
		require.NoError(t, err)
	}
}

func fillWithBlockData(ctx context.Context, t *testing.T, pending *pendingBlocks, msgPrefix string) {
	fillPendingHeaders(ctx, t, pending, msgPrefix, numItemsToSubmit)
}

func createTestHeader(t *testing.T, height uint64, msgPrefix string) *types.SignedHeader {
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	signer, err := noop.NewNoopSigner(privKey)
	require.NoError(t, err)
	
	header := &types.Header{
		Height: func() types.Height { return types.Height{RollappHeight: height} }(),
		DataHash: []byte(fmt.Sprintf("%s data hash %d", msgPrefix, height)),
	}
	
	sig, err := signer.Sign(header.Bytes())
	require.NoError(t, err)
	
	return &types.SignedHeader{
		Header:    header,
		Signature: sig,
	}
}

func getDummyID(num uint64, data []byte) coreda.ID {
	id := make([]byte, 8+len(data))
	binary.BigEndian.PutUint64(id[:8], num)
	copy(id[8:], data)
	return id
}