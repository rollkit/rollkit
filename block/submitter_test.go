package block

import (
	"context"
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

	mockStore := mocks.NewStore(t)
	mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(nil, ds.ErrNotFound).Maybe()
	mockStore.On("SetMetadata", mock.Anything, "last-submitted-data-height", mock.Anything).Return(nil).Maybe()
	pendingData, err := NewPendingData(mockStore, logger)
	require.NoError(t, err)

	return &Manager{
		da:            da,
		logger:        logger,
		config:        nodeConf,
		gasPrice:      1.0,
		gasMultiplier: 2.0,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		signer:        testSigner,
		pendingData:   pendingData,
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
	// Prepare a mock PendingHeaders with test data
	m.pendingHeaders = newPendingBlocks(t)

	// Fill the pending headers with mock block data
	fillWithBlockData(context.Background(), t, m.pendingHeaders, "Test Submitting Headers")

	// Simulate DA layer successfully accepting the header submission
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	// Call submitHeadersToDA and expect no error
	err := m.submitHeadersToDA(context.Background())
	assert.NoError(t, err)
}

// TestSubmitHeadersToDA_Failure verifies that submitHeadersToDA returns an error for various DA failures.
func TestSubmitHeadersToDA_Failure(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)
	// Prepare a mock PendingHeaders with test data
	m.pendingHeaders = newPendingBlocks(t)

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
			fillWithBlockData(context.Background(), t, m.pendingHeaders, "Test Submitting Headers")
			// Reset mock expectations for each error scenario
			da.ExpectedCalls = nil
			// Simulate DA layer returning a specific error
			var gasPriceHistory []float64
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) { gasPriceHistory = append(gasPriceHistory, args.Get(2).(float64)) }). //save the gas price to verify it later
				Return(nil, tc.daError)

			// Call submitHeadersToDA and expect an error
			err := m.submitHeadersToDA(context.Background())
			assert.Error(t, err, "expected error for DA error: %v", tc.daError)
			assert.Contains(t, err.Error(), "failed to submit all headers to DA layer")

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
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
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

	t.Run("normal case", func(t *testing.T) {
		mockStore := mocks.NewStore(t)
		logger := log.NewNopLogger()
		// Set up Height and GetBlockData to return two Data items
		mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(nil, ds.ErrNotFound).Once()
		mockStore.On("Height", mock.Anything).Return(uint64(2), nil).Once()
		mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(nil, &types.Data{Txs: types.Txs{types.Tx("tx1"), types.Tx("tx2")}, Metadata: &types.Metadata{Height: 1}}, nil).Once()
		mockStore.On("GetBlockData", mock.Anything, uint64(2)).Return(nil, &types.Data{Txs: types.Txs{types.Tx("tx3")}, Metadata: &types.Metadata{Height: 2}}, nil).Once()
		pendingData, err := NewPendingData(mockStore, logger)
		require.NoError(t, err)
		m := &Manager{
			signer:      testSigner,
			genesis:     gen,
			pendingData: pendingData,
		}
		signedDataList, err := m.createSignedDataToSubmit(context.Background())
		require.NoError(t, err)
		require.Len(t, signedDataList, 2)
		assert.Equal(t, types.Tx("tx1"), signedDataList[0].Txs[0])
		assert.Equal(t, types.Tx("tx2"), signedDataList[0].Txs[1])
		assert.Equal(t, pubKey, signedDataList[0].Signer.PubKey)
		assert.Equal(t, proposerAddr, signedDataList[0].Signer.Address)
		assert.NotEmpty(t, signedDataList[0].Signature)
		assert.Equal(t, types.Tx("tx3"), signedDataList[1].Txs[0])
		assert.Equal(t, pubKey, signedDataList[1].Signer.PubKey)
		assert.Equal(t, proposerAddr, signedDataList[1].Signer.Address)
		assert.NotEmpty(t, signedDataList[1].Signature)
	})

	t.Run("empty pending data", func(t *testing.T) {
		mockStore := mocks.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(nil, ds.ErrNotFound).Once()
		mockStore.On("Height", mock.Anything).Return(uint64(0), nil).Once()
		pendingData, err := NewPendingData(mockStore, logger)
		require.NoError(t, err)
		m := &Manager{
			signer:      testSigner,
			genesis:     gen,
			pendingData: pendingData,
		}
		signedDataList, err := m.createSignedDataToSubmit(context.Background())
		assert.NoError(t, err)
		assert.Empty(t, signedDataList)
	})

	t.Run("getPendingData returns error", func(t *testing.T) {
		mockStore := mocks.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(nil, ds.ErrNotFound).Once()
		mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Once()
		mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(nil, nil, fmt.Errorf("mock error")).Once()
		pendingData, err := NewPendingData(mockStore, logger)
		require.NoError(t, err)
		m := &Manager{
			signer:      testSigner,
			genesis:     gen,
			pendingData: pendingData,
		}
		_, err = m.createSignedDataToSubmit(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mock error")
	})

	t.Run("signer returns error", func(t *testing.T) {
		mockStore := mocks.NewStore(t)
		logger := log.NewNopLogger()
		mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(nil, ds.ErrNotFound).Once()
		mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Once()
		mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(nil, &types.Data{Txs: types.Txs{types.Tx("tx1")}, Metadata: &types.Metadata{Height: 1}}, nil).Once()
		pendingData, err := NewPendingData(mockStore, logger)
		require.NoError(t, err)
		m := &Manager{
			signer:      nil, // nil signer will cause getDataSignature to fail
			genesis:     gen,
			pendingData: pendingData,
		}
		_, err = m.createSignedDataToSubmit(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signer is nil; cannot sign data")
	})
}
