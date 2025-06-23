package block

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/libp2p/go-libp2p/core/crypto"
	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
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

	return &Manager{
		da:            da,
		logger:        logger,
		config:        nodeConf,
		gasPrice:      1.0,
		gasMultiplier: 2.0,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		signer:        testSigner,
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

	err = m.submitDataToDA(context.Background(), &signedData)
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
			err = m.submitDataToDA(context.Background(), &signedData)
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

// TestSubmitHeadersToDA_WithMetricsRecorder verifies that submitHeadersToDA calls RecordMetrics
// when the sequencer implements the MetricsRecorder interface.
func TestSubmitHeadersToDA_WithMetricsRecorder(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	// Set up mock sequencer with metrics
	mockSequencer := new(MockSequencerWithMetrics)
	m.sequencer = mockSequencer

	// Prepare a mock PendingHeaders with test data
	m.pendingHeaders = newPendingBlocks(t)

	// Fill the pending headers with mock block data
	fillWithBlockData(context.Background(), t, m.pendingHeaders, "Test Submitting Headers")

	// Simulate DA layer successfully accepting the header submission
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	// Expect RecordMetrics to be called with the correct parameters
	// Allow multiple calls since fillWithBlockData might create multiple headers
	mockSequencer.On("RecordMetrics",
		float64(1.0),                  // gasPrice (from newTestManagerWithDA)
		uint64(0),                     // blobSize (mocked as 0)
		coreda.StatusSuccess,          // statusCode
		mock.AnythingOfType("uint64"), // numPendingBlocks (varies based on test data)
		mock.AnythingOfType("uint64"), // lastSubmittedHeight
	).Maybe()

	// Call submitHeadersToDA and expect no error
	err := m.submitHeadersToDA(context.Background())
	assert.NoError(t, err)

	// Verify that RecordMetrics was called at least once
	mockSequencer.AssertExpectations(t)
}

// TestSubmitDataToDA_WithMetricsRecorder verifies that submitDataToDA calls RecordMetrics
// when the sequencer implements the MetricsRecorder interface.
func TestSubmitDataToDA_WithMetricsRecorder(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	// Set up mock sequencer with metrics
	mockSequencer := new(MockSequencerWithMetrics)
	m.sequencer = mockSequencer

	// Initialize pendingHeaders to avoid nil pointer dereference
	m.pendingHeaders = newPendingBlocks(t)

	// Simulate DA success
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	// Expect RecordMetrics to be called with the correct parameters
	mockSequencer.On("RecordMetrics",
		float64(1.0),                  // gasPrice (from newTestManagerWithDA)
		uint64(0),                     // blobSize (mocked as 0)
		coreda.StatusSuccess,          // statusCode
		mock.AnythingOfType("uint64"), // numPendingBlocks (varies based on test data)
		mock.AnythingOfType("uint64"), // daIncludedHeight
	).Once()

	pubKey, err := m.signer.GetPublic()
	require.NoError(t, err)
	addr, err := m.signer.GetAddress()
	require.NoError(t, err)

	transactions := [][]byte{[]byte("tx1"), []byte("tx2")}

	signedData := types.SignedData{
		Data: types.Data{
			Txs: make(types.Txs, len(transactions)),
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

	err = m.submitDataToDA(context.Background(), &signedData)
	assert.NoError(t, err)

	// Verify that RecordMetrics was called
	mockSequencer.AssertExpectations(t)
}

// TestCreateSignedDataFromBatch tests createSignedDataFromBatch for normal, empty, and error cases.
func TestCreateSignedDataFromBatch(t *testing.T) {
	// Setup: create a Manager with a valid signer and genesis proposer address
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
	m := &Manager{
		signer:  testSigner,
		genesis: gen,
	}

	t.Run("normal batch", func(t *testing.T) {
		batch := &coresequencer.Batch{
			Transactions: [][]byte{[]byte("tx1"), []byte("tx2")},
		}
		signedData, err := m.createSignedDataFromBatch(batch)
		require.NoError(t, err)
		assert.Equal(t, 2, len(signedData.Txs))
		assert.Equal(t, types.Tx("tx1"), signedData.Txs[0])
		assert.Equal(t, types.Tx("tx2"), signedData.Txs[1])
		assert.Equal(t, pubKey, signedData.Signer.PubKey)
		assert.Equal(t, proposerAddr, signedData.Signer.Address)
		assert.NotEmpty(t, signedData.Signature)
	})

	t.Run("empty batch", func(t *testing.T) {
		batch := &coresequencer.Batch{
			Transactions: [][]byte{},
		}
		signedData, err := m.createSignedDataFromBatch(batch)
		assert.Error(t, err)
		assert.Nil(t, signedData)
	})

	t.Run("signer returns error", func(t *testing.T) {
		badManager := &Manager{
			signer:  nil, // nil signer will cause getDataSignature to fail
			genesis: m.genesis,
		}
		batch := &coresequencer.Batch{
			Transactions: [][]byte{[]byte("tx1")},
		}
		_, err := badManager.createSignedDataFromBatch(batch)
		assert.Error(t, err)
	})
}
