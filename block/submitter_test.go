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

// TestSubmitDataToDA_Success verifies that submitDataToDA succeeds when the DA layer accepts the data.
func TestSubmitDataToDA_Success(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	// Simulate DA success
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	pubKey, err := m.signer.GetPublic()
	require.NoError(t, err)
	addr, err := m.signer.GetAddress()
	require.NoError(t, err)

	transactions := [][]byte{[]byte("tx1"), []byte("tx2")}

	signedData := types.SignedData{
		Data: types.Data{
			Txs: make(types.Txs, len(transactions)),
			Metadata: &types.Metadata{
				Height: 1,
			},
		},
		Signer: types.Signer{
			Address: addr,
			PubKey:  pubKey,
		},
	}

	for i, tx := range transactions {
		signedData.Txs[i] = types.Tx(tx)
	}

	signature, err := m.getDataSignature(&signedData.Data)
	require.NoError(t, err)
	signedData.Signature = signature

	err = m.submitDataToDA(t.Context(), []*types.SignedData{&signedData})
	assert.NoError(t, err)
}

// TestSubmitDataToDA_Failure verifies that submitDataToDA returns an error for various DA failures.
func TestSubmitDataToDA_Failure(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	// Table-driven test for different DA error scenarios
	testCases := []struct {
		name    string
		daError error
	}{
		{"AlreadyInMempool", coreda.ErrTxAlreadyInMempool},
		{"TimedOut", coreda.ErrTxTimedOut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock expectations for each error scenario
			var gasPriceHistory []float64
			da.ExpectedCalls = nil
			da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) { gasPriceHistory = append(gasPriceHistory, args.Get(2).(float64)) }). //save the gas price to verify it later
				Return(nil, tc.daError)

			pubKey, err := m.signer.GetPublic()
			require.NoError(t, err)
			addr, err := m.signer.GetAddress()
			require.NoError(t, err)

			transactions := [][]byte{[]byte("tx1"), []byte("tx2")}

			signedData := types.SignedData{
				Data: types.Data{
					Txs: make(types.Txs, len(transactions)),
					Metadata: &types.Metadata{
						Height: 1,
					},
				},
				Signer: types.Signer{
					Address: addr,
					PubKey:  pubKey,
				},
			}

			signedData.Txs = make(types.Txs, len(transactions))
			for i, tx := range transactions {
				signedData.Txs[i] = types.Tx(tx)
			}

			signature, err := m.getDataSignature(&signedData.Data)
			require.NoError(t, err)
			signedData.Signature = signature

			// Expect an error from submitDataToDA
			err = m.submitDataToDA(t.Context(), []*types.SignedData{&signedData})
			assert.Error(t, err, "expected error")

			// Validate that gas price increased according to gas multiplier
			previousGasPrice := m.gasPrice
			assert.Equal(t, gasPriceHistory[0], m.gasPrice) // verify that the first call is done with the right price
			for _, gasPrice := range gasPriceHistory[1:] {
				assert.Equal(t, gasPrice, previousGasPrice*m.gasMultiplier)
				previousGasPrice = gasPrice
			}
		})
	}
}

func TestSubmitHeadersToDA_Success(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	// Fill the pending headers with mock block data
	fillPendingHeaders(t.Context(), t, m.pendingHeaders, "Test Submitting Headers", 3)

	// Simulate DA layer successfully accepting the header submission
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	headers, err := m.pendingHeaders.getPendingHeaders(t.Context())
	require.NoError(t, err)

	// Call submitHeadersToDA and expect no error
	err = m.submitHeadersToDA(context.Background(), headers)
	assert.NoError(t, err)
}

// TestSubmitHeadersToDA_Failure verifies that submitHeadersToDA returns an error for various DA failures.
func TestSubmitHeadersToDA_Failure(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	// Table-driven test for different DA error scenarios
	testCases := []struct {
		name    string
		daError error
	}{
		{"AlreadyInMempool", coreda.ErrTxAlreadyInMempool},
		{"TimedOut", coreda.ErrTxTimedOut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Fill the pending headers with mock block data for each subtest
			fillPendingHeaders(t.Context(), t, m.pendingHeaders, "Test Submitting Headers", 3)
			// Reset mock expectations for each error scenario
			da.ExpectedCalls = nil
			// Simulate DA layer returning a specific error
			var gasPriceHistory []float64
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) { gasPriceHistory = append(gasPriceHistory, args.Get(2).(float64)) }). //save the gas price to verify it later
				Return(nil, tc.daError)

			// Call submitHeadersToDA and expect an error
			headers, err := m.pendingHeaders.getPendingHeaders(context.Background())
			require.NoError(t, err)
			err = m.submitHeadersToDA(context.Background(), headers)
			assert.Error(t, err, "expected error for DA error: %v", tc.daError)
			assert.Contains(t, err.Error(), "failed to submit all items to DA layer")

			// Validate that gas price increased according to gas multiplier
			previousGasPrice := m.gasPrice
			assert.Equal(t, gasPriceHistory[0], m.gasPrice) // verify that the first call is done with the right price
			for _, gasPrice := range gasPriceHistory[1:] {
				assert.Equal(t, gasPrice, previousGasPrice*m.gasMultiplier)
				previousGasPrice = gasPrice
			}
		})
	}
}

// TestCreateSignedDataToSubmit tests createSignedDataToSubmit for normal, empty, and error cases.
func TestCreateSignedDataToSubmit(t *testing.T) {
	// Normal case: pending data exists and is signed correctly
	t.Run("normal case", func(t *testing.T) {
		m := newTestManagerWithDA(t, nil)
		fillPendingData(t.Context(), t, m.pendingData, "Test Creating Signed Data", 2)
		pubKey, err := m.signer.GetPublic()
		require.NoError(t, err)
		proposerAddr, err := m.signer.GetAddress()
		require.NoError(t, err)
		signedDataList, err := m.createSignedDataToSubmit(t.Context())
		require.NoError(t, err)
		require.Len(t, signedDataList, 2)
		assert.Equal(t, types.Tx("tx1"), signedDataList[0].Txs[0])
		assert.Equal(t, types.Tx("tx2"), signedDataList[0].Txs[1])
		assert.Equal(t, pubKey, signedDataList[0].Signer.PubKey)
		assert.Equal(t, proposerAddr, signedDataList[0].Signer.Address)
		assert.NotEmpty(t, signedDataList[0].Signature)
		assert.Equal(t, types.Tx("tx3"), signedDataList[1].Txs[0])
		assert.Equal(t, types.Tx("tx4"), signedDataList[1].Txs[1])
		assert.Equal(t, pubKey, signedDataList[1].Signer.PubKey)
		assert.Equal(t, proposerAddr, signedDataList[1].Signer.Address)
		assert.NotEmpty(t, signedDataList[1].Signature)
	})

	// Empty pending data: should return no error and an empty list
	t.Run("empty pending data", func(t *testing.T) {
		m := newTestManagerWithDA(t, nil)
		signedDataList, err := m.createSignedDataToSubmit(t.Context())
		assert.NoError(t, err, "expected no error when pending data is empty")
		assert.Empty(t, signedDataList, "expected signedDataList to be empty when no pending data")
	})

	// getPendingData returns error: should return error and error message should match
	t.Run("getPendingData returns error", func(t *testing.T) {
		m := newTestManagerWithDA(t, nil)
		mockStore := mocks.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(nil, ds.ErrNotFound).Once()
		mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Once()
		mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(nil, nil, fmt.Errorf("mock error")).Once()
		pendingData, err := NewPendingData(mockStore, logger)
		require.NoError(t, err)
		m.pendingData = pendingData
		signedDataList, err := m.createSignedDataToSubmit(t.Context())
		assert.Error(t, err, "expected error when getPendingData fails")
		assert.Contains(t, err.Error(), "mock error", "error message should contain 'mock error'")
		assert.Nil(t, signedDataList, "signedDataList should be nil on error")
	})

	// signer returns error: should return error and error message should match
	t.Run("signer returns error", func(t *testing.T) {
		m := newTestManagerWithDA(t, nil)
		m.signer = nil
		signedDataList, err := m.createSignedDataToSubmit(t.Context())
		assert.Error(t, err, "expected error when signer is nil")
		assert.Contains(t, err.Error(), "signer is nil; cannot sign data", "error message should mention nil signer")
		assert.Nil(t, signedDataList, "signedDataList should be nil on error")
	})
}

// TestSubmitDataToDA_RetryPartialFailures tests that when submitting multiple blobs, if some fail, they are retried and lastSubmittedDataHeight is only updated after all succeed.
func TestSubmitDataToDA_RetryPartialFailures(t *testing.T) {
	m := newTestManagerWithDA(t, nil)
	// Set up mocks and manager with lastSubmittedDataHeight = 1
	mockStore := mocks.NewStore(t)
	lastSubmittedBytes := make([]byte, 8)
	// Set lastSubmittedDataHeight to 1
	lastHeight := uint64(1)
	binary.LittleEndian.PutUint64(lastSubmittedBytes, lastHeight)
	mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(lastSubmittedBytes, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, "last-submitted-data-height", mock.Anything).Return(nil).Maybe()
	// Store height is 3, so heights 2 and 3 are pending
	mockStore.On("Height", mock.Anything).Return(uint64(3), nil).Maybe()
	// Provide Data for heights 2 and 3
	mockStore.On("GetBlockData", mock.Anything, uint64(2)).Return(nil, &types.Data{Txs: types.Txs{types.Tx("tx2")}, Metadata: &types.Metadata{Height: 2}}, nil).Maybe()
	mockStore.On("GetBlockData", mock.Anything, uint64(3)).Return(nil, &types.Data{Txs: types.Txs{types.Tx("tx3")}, Metadata: &types.Metadata{Height: 3}}, nil).Maybe()

	pendingData, err := NewPendingData(mockStore, m.logger)
	require.NoError(t, err)
	m.pendingData = pendingData

	// Prepare two SignedData for heights 2 and 3
	ctx := t.Context()
	signedDataList, err := m.createSignedDataToSubmit(ctx)
	require.NoError(t, err)
	require.Len(t, signedDataList, 2)

	// Set up DA mock: first call only one succeeds, second call succeeds for the rest
	da := &mocks.DA{}
	m.da = da
	m.gasPrice = 1.0
	m.gasMultiplier = 2.0

	// First attempt: only the first blob is accepted
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil).Twice()
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Once().Return([]coreda.ID{[]byte("id2")}, nil) // Only one submitted
	// Second attempt: the remaining blob is accepted
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Once().Return([]coreda.ID{[]byte("id3")}, nil)

	err = m.submitDataToDA(ctx, signedDataList)
	assert.NoError(t, err)

	// After all succeed, lastSubmittedDataHeight should be 3
	assert.Equal(t, uint64(3), m.pendingData.getLastSubmittedDataHeight())
}

// fillPendingHeaders populates the given PendingHeaders with a sequence of mock SignedHeader objects for testing.
// It generates headers with consecutive heights and stores them in the underlying store so that PendingHeaders logic can retrieve them.
//
// Parameters:
//
//	ctx: context for store operations
//	t: the testing.T instance
//	pendingHeaders: the PendingHeaders instance to fill
//	chainID: the chain ID to use for generated headers
//	startHeight: the starting height for headers (default 1 if 0)
//	count: the number of headers to generate (default 3 if 0)
func fillPendingHeaders(ctx context.Context, t *testing.T, pendingHeaders *PendingHeaders, chainID string, numBlocks uint64) {
	t.Helper()

	store := pendingHeaders.base.store
	for i := uint64(0); i < numBlocks; i++ {
		height := i + 1
		header, data := types.GetRandomBlock(height, 0, chainID)
		sig := &header.Signature
		err := store.SaveBlockData(ctx, header, data, sig)
		require.NoError(t, err, "failed to save block data for header at height %d", height)
		err = store.SetHeight(ctx, height)
		require.NoError(t, err, "failed to set store height for header at height %d", height)
	}
}

func fillPendingData(ctx context.Context, t *testing.T, pendingData *PendingData, chainID string, numBlocks uint64) {
	t.Helper()
	store := pendingData.base.store
	txNum := 1
	for i := uint64(0); i < numBlocks; i++ {
		height := i + 1
		header, data := types.GetRandomBlock(height, 2, chainID)
		data.Txs = make(types.Txs, len(data.Txs))
		for i := 0; i < len(data.Txs); i++ {
			data.Txs[i] = types.Tx(fmt.Sprintf("tx%d", txNum))
			txNum++
		}
		sig := &header.Signature
		err := store.SaveBlockData(ctx, header, data, sig)
		require.NoError(t, err, "failed to save block data for data at height %d", height)
		err = store.SetHeight(ctx, height)
		require.NoError(t, err, "failed to set store height for data at height %d", height)
	}
}

func newPendingHeaders(t *testing.T) *PendingHeaders {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	pendingHeaders, err := NewPendingHeaders(store.New(kv), log.NewTestLogger(t))
	require.NoError(t, err)
	return pendingHeaders
}

func newPendingData(t *testing.T) *PendingData {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	pendingData, err := NewPendingData(store.New(kv), log.NewTestLogger(t))
	require.NoError(t, err)
	return pendingData
}
