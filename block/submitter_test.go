package block

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreda "github.com/evstack/ev-node/core/da"
	"github.com/evstack/ev-node/pkg/cache"
	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/signer/noop"
	"github.com/evstack/ev-node/pkg/store"
	"github.com/evstack/ev-node/test/mocks"
	"github.com/evstack/ev-node/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
)

const numItemsToSubmit = 3

// newTestManagerWithDA creates a Manager instance with a mocked DA layer for testing.
func newTestManagerWithDA(t *testing.T, da *mocks.MockDA) (m *Manager) {
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL")
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
		metrics:        NopMetrics(),
	}
}

// --- Generic success test for data and headers submission ---
type submitToDASuccessCase[T any] struct {
	name        string
	fillPending func(ctx context.Context, t *testing.T, m *Manager)
	getToSubmit func(m *Manager, ctx context.Context) ([]T, error)
	submitToDA  func(m *Manager, ctx context.Context, items []T) error
	mockDASetup func(da *mocks.MockDA)
}

func runSubmitToDASuccessCase[T any](t *testing.T, tc submitToDASuccessCase[T]) {
	da := &mocks.MockDA{}
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
		mockDASetup: func(da *mocks.MockDA) {
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
		mockDASetup: func(da *mocks.MockDA) {
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
	errorMsg    string
	daError     error
	mockDASetup func(da *mocks.MockDA, gasPriceHistory *[]float64, daError error)
}

func runSubmitToDAFailureCase[T any](t *testing.T, tc submitToDAFailureCase[T]) {
	da := &mocks.MockDA{}
	m := newTestManagerWithDA(t, da)

	ctx := t.Context()
	tc.fillPending(ctx, t, m)

	var gasPriceHistory []float64
	tc.mockDASetup(da, &gasPriceHistory, tc.daError)

	items, err := tc.getToSubmit(m, ctx)
	require.NoError(t, err)
	require.NotEmpty(t, items)

	err = tc.submitToDA(m, ctx, items)
	assert.Error(t, err, "expected error")
	assert.Contains(t, err.Error(), tc.errorMsg)

	// Validate that gas price increased according to gas multiplier
	previousGasPrice := m.gasPrice
	assert.Equal(t, gasPriceHistory[0], m.gasPrice) // verify that the first call is done with the right price
	for _, gasPrice := range gasPriceHistory[1:] {
		assert.Equal(t, gasPrice, previousGasPrice*m.gasMultiplier)
		previousGasPrice = gasPrice
	}
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
				errorMsg: "failed to submit all data(s) to DA layer",
				daError:  tc.daError,
				mockDASetup: func(da *mocks.MockDA, gasPriceHistory *[]float64, daError error) {
					da.ExpectedCalls = nil
					da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) { *gasPriceHistory = append(*gasPriceHistory, args.Get(2).(float64)) }).
						Return(nil, daError)
				},
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
				errorMsg: "failed to submit all header(s) to DA layer",
				daError:  tc.daError,
				mockDASetup: func(da *mocks.MockDA, gasPriceHistory *[]float64, daError error) {
					da.ExpectedCalls = nil
					da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
						Run(func(args mock.Arguments) { *gasPriceHistory = append(*gasPriceHistory, args.Get(2).(float64)) }).
						Return(nil, daError)
				},
			})
		})
	}
}

// --- Generic retry partial failures test for data and headers ---
type retryPartialFailuresCase[T any] struct {
	name               string
	metaKey            string
	fillPending        func(ctx context.Context, t *testing.T, m *Manager)
	submitToDA         func(m *Manager, ctx context.Context, items []T) error
	getLastSubmitted   func(m *Manager) uint64
	getPendingToSubmit func(m *Manager, ctx context.Context) ([]T, error)
	setupStoreAndDA    func(m *Manager, mockStore *mocks.MockStore, da *mocks.MockDA)
}

func runRetryPartialFailuresCase[T any](t *testing.T, tc retryPartialFailuresCase[T]) {
	m := newTestManagerWithDA(t, nil)
	mockStore := mocks.NewMockStore(t)
	m.store = mockStore
	m.logger = logging.Logger("test")

	_ = logging.SetLogLevel("test", "debug")
	da := &mocks.MockDA{}
	m.da = da
	m.gasPrice = 1.0
	m.gasMultiplier = 2.0
	tc.setupStoreAndDA(m, mockStore, da)
	ctx := t.Context()
	tc.fillPending(ctx, t, m)

	// Prepare items to submit
	items, err := tc.getPendingToSubmit(m, ctx)
	require.NoError(t, err)
	require.Len(t, items, 3)

	// Set up DA mock: three calls, each time only one item is accepted
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil).Times(3)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			t.Logf("DA Submit call 1: args=%#v", args)
		}).Once().Return([]coreda.ID{getDummyID(1, []byte("commitment2"))}, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			t.Logf("DA Submit call 2: args=%#v", args)
		}).Once().Return([]coreda.ID{getDummyID(1, []byte("commitment3"))}, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			t.Logf("DA Submit call 3: args=%#v", args)
		}).Once().Return([]coreda.ID{getDummyID(1, []byte("commitment4"))}, nil)

	err = tc.submitToDA(m, ctx, items)
	assert.NoError(t, err)

	// After all succeed, lastSubmitted should be 3
	assert.Equal(t, uint64(3), tc.getLastSubmitted(m))
}

func TestSubmitToDA_RetryPartialFailures_Generic(t *testing.T) {
	casesData := retryPartialFailuresCase[*types.SignedData]{
		name:    "Data",
		metaKey: "last-submitted-data-height",
		fillPending: func(ctx context.Context, t *testing.T, m *Manager) {
			fillPendingData(ctx, t, m.pendingData, "Test", numItemsToSubmit)
		},
		submitToDA: func(m *Manager, ctx context.Context, items []*types.SignedData) error {
			return m.submitDataToDA(ctx, items)
		},
		getLastSubmitted: func(m *Manager) uint64 {
			return m.pendingData.getLastSubmittedDataHeight()
		},
		getPendingToSubmit: func(m *Manager, ctx context.Context) ([]*types.SignedData, error) {
			return m.createSignedDataToSubmit(ctx)
		},
		setupStoreAndDA: func(m *Manager, mockStore *mocks.MockStore, da *mocks.MockDA) {
			lastSubmittedBytes := make([]byte, 8)
			lastHeight := uint64(0)
			binary.LittleEndian.PutUint64(lastSubmittedBytes, lastHeight)
			mockStore.On("GetMetadata", mock.Anything, "last-submitted-data-height").Return(lastSubmittedBytes, nil).Maybe()
			mockStore.On("SetMetadata", mock.Anything, "last-submitted-data-height", mock.Anything).Return(nil).Maybe()
			mockStore.On("Height", mock.Anything).Return(uint64(4), nil).Maybe()
			for h := uint64(2); h <= 4; h++ {
				mockStore.On("GetBlockData", mock.Anything, h).Return(nil, &types.Data{
					Txs:      types.Txs{types.Tx(fmt.Sprintf("tx%d", h))},
					Metadata: &types.Metadata{Height: h},
				}, nil).Maybe()
			}
		},
	}

	casesHeader := retryPartialFailuresCase[*types.SignedHeader]{
		name:    "Header",
		metaKey: "last-submitted-header-height",
		fillPending: func(ctx context.Context, t *testing.T, m *Manager) {
			fillPendingHeaders(ctx, t, m.pendingHeaders, "Test", numItemsToSubmit)
		},
		submitToDA: func(m *Manager, ctx context.Context, items []*types.SignedHeader) error {
			return m.submitHeadersToDA(ctx, items)
		},
		getLastSubmitted: func(m *Manager) uint64 {
			return m.pendingHeaders.getLastSubmittedHeaderHeight()
		},
		getPendingToSubmit: func(m *Manager, ctx context.Context) ([]*types.SignedHeader, error) {
			return m.pendingHeaders.getPendingHeaders(ctx)
		},
		setupStoreAndDA: func(m *Manager, mockStore *mocks.MockStore, da *mocks.MockDA) {
			lastSubmittedBytes := make([]byte, 8)
			lastHeight := uint64(0)
			binary.LittleEndian.PutUint64(lastSubmittedBytes, lastHeight)
			mockStore.On("GetMetadata", mock.Anything, "last-submitted-header-height").Return(lastSubmittedBytes, nil).Maybe()
			mockStore.On("SetMetadata", mock.Anything, "last-submitted-header-height", mock.Anything).Return(nil).Maybe()
			mockStore.On("Height", mock.Anything).Return(uint64(4), nil).Maybe()
			for h := uint64(2); h <= 4; h++ {
				header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: h}}}
				mockStore.On("GetBlockData", mock.Anything, h).Return(header, nil, nil).Maybe()
			}
		},
	}

	t.Run(casesData.name, func(t *testing.T) {
		runRetryPartialFailuresCase(t, casesData)
	})

	t.Run(casesHeader.name, func(t *testing.T) {
		runRetryPartialFailuresCase(t, casesHeader)
	})
}

// TestCreateSignedDataToSubmit tests createSignedDataToSubmit for normal, empty, and error cases.
func TestCreateSignedDataToSubmit(t *testing.T) {
	// Normal case: pending data exists and is signed correctly
	t.Run("normal case", func(t *testing.T) {
		m := newTestManagerWithDA(t, nil)
		fillPendingData(t.Context(), t, m.pendingData, "Test Creating Signed Data", numItemsToSubmit)
		pubKey, err := m.signer.GetPublic()
		require.NoError(t, err)
		proposerAddr, err := m.signer.GetAddress()
		require.NoError(t, err)
		signedDataList, err := m.createSignedDataToSubmit(t.Context())
		require.NoError(t, err)
		require.Len(t, signedDataList, numItemsToSubmit)
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
		mockStore := mocks.NewMockStore(t)
		logger := logging.Logger("test")
		_ = logging.SetLogLevel("test", "FATAL")
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
	logger := logging.Logger("test")

	_ = logging.SetLogLevel("test", "debug")
	pendingHeaders, err := NewPendingHeaders(store.New(kv), logger)
	require.NoError(t, err)
	return pendingHeaders
}

func newPendingData(t *testing.T) *PendingData {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "debug")
	pendingData, err := NewPendingData(store.New(kv), logger)
	require.NoError(t, err)
	return pendingData
}

// TestSubmitHeadersToDA_WithMetricsRecorder verifies that submitHeadersToDA calls RecordMetrics
// when the sequencer implements the MetricsRecorder interface.
func TestSubmitHeadersToDA_WithMetricsRecorder(t *testing.T) {
	da := &mocks.MockDA{}
	m := newTestManagerWithDA(t, da)

	// Set up mock sequencer with metrics
	mockSequencer := new(MockSequencerWithMetrics)
	m.sequencer = mockSequencer

	// Fill the pending headers with test data
	ctx := context.Background()
	fillPendingHeaders(ctx, t, m.pendingHeaders, "Test Submitting Headers", numItemsToSubmit)

	// Get the headers to submit
	headers, err := m.pendingHeaders.getPendingHeaders(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, headers)

	// Simulate DA layer successfully accepting the header submission
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	// Expect RecordMetrics to be called with the correct parameters
	mockSequencer.On("RecordMetrics",
		float64(1.0),                  // gasPrice (from newTestManagerWithDA)
		uint64(0),                     // blobSize (mocked as 0)
		coreda.StatusSuccess,          // statusCode
		mock.AnythingOfType("uint64"), // numPendingBlocks (varies based on test data)
		mock.AnythingOfType("uint64"), // lastSubmittedHeight
	).Maybe()

	// Call submitHeadersToDA and expect no error
	err = m.submitHeadersToDA(ctx, headers)
	assert.NoError(t, err)

	// Verify that RecordMetrics was called at least once
	mockSequencer.AssertExpectations(t)
}

// TestSubmitDataToDA_WithMetricsRecorder verifies that submitDataToDA calls RecordMetrics
// when the sequencer implements the MetricsRecorder interface.
func TestSubmitDataToDA_WithMetricsRecorder(t *testing.T) {
	da := &mocks.MockDA{}
	m := newTestManagerWithDA(t, da)

	// Set up mock sequencer with metrics
	mockSequencer := new(MockSequencerWithMetrics)
	m.sequencer = mockSequencer

	// Fill pending data for testing
	ctx := context.Background()
	fillPendingData(ctx, t, m.pendingData, "Test Submitting Data", numItemsToSubmit)

	// Get the data to submit
	signedDataList, err := m.createSignedDataToSubmit(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, signedDataList)

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
	).Maybe()

	err = m.submitDataToDA(ctx, signedDataList)
	assert.NoError(t, err)

	// Verify that RecordMetrics was called
	mockSequencer.AssertExpectations(t)
}

func getDummyID(height uint64, commitment []byte) coreda.ID {
	id := make([]byte, len(commitment)+8)
	binary.LittleEndian.PutUint64(id, height)
	copy(id[8:], commitment)
	return id
}
