package block

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	goheaderstore "github.com/celestiaorg/go-header/store"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// setupManagerForPublishBlockTest creates a Manager instance with mocks for testing publishBlockInternal.
func setupManagerForPublishBlockTest(
	t *testing.T,
	initialHeight uint64,
	lastSubmittedHeight uint64,
	logBuffer *bytes.Buffer,
) (*Manager, *mocks.Store, *mocks.Executor, *mocks.Sequencer, signer.Signer, context.CancelFunc) {
	require := require.New(t)

	mockStore := mocks.NewStore(t)
	mockExec := mocks.NewExecutor(t)
	mockSeq := mocks.NewSequencer(t)

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	testSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)
	proposerAddr, err := testSigner.GetAddress()
	require.NoError(err)

	cfg := config.DefaultConfig
	cfg.Node.BlockTime.Duration = 1 * time.Second
	genesis := genesispkg.NewGenesis("testchain", initialHeight, time.Now(), proposerAddr)

	_, cancel := context.WithCancel(context.Background())
	logger := log.NewLogger(
		logBuffer,
		log.ColorOption(false),
	)

	lastSubmittedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(lastSubmittedBytes, lastSubmittedHeight)
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(lastSubmittedBytes, nil).Maybe()

	var headerStore *goheaderstore.Store[*types.SignedHeader]
	var dataStore *goheaderstore.Store[*types.Data]
	// Manager initialization (simplified, add fields as needed by tests)
	manager := &Manager{
		store:     mockStore,
		exec:      mockExec,
		sequencer: mockSeq,
		signer:    testSigner,
		config:    cfg,
		genesis:   genesis,
		logger:    logger,
		headerBroadcaster: broadcasterFn[*types.SignedHeader](func(ctx context.Context, payload *types.SignedHeader) error {
			return nil
		}),
		dataBroadcaster: broadcasterFn[*types.Data](func(ctx context.Context, payload *types.Data) error {
			return nil
		}),
		headerStore:    headerStore,
		daHeight:       &atomic.Uint64{},
		dataStore:      dataStore,
		headerCache:    cache.NewCache[types.SignedHeader](),
		dataCache:      cache.NewCache[types.Data](),
		lastStateMtx:   &sync.RWMutex{},
		metrics:        NopMetrics(),
		pendingHeaders: nil,
	}
	manager.publishBlock = manager.publishBlockInternal

	pendingHeaders, err := NewPendingHeaders(mockStore, logger)
	require.NoError(err, "Failed to create PendingHeaders")
	manager.pendingHeaders = pendingHeaders

	manager.lastState = types.State{
		ChainID:         genesis.ChainID,
		InitialHeight:   genesis.InitialHeight,
		LastBlockHeight: initialHeight - 1,
		LastBlockTime:   genesis.GenesisDAStartTime,
		AppHash:         []byte("initialAppHash"),
	}
	if initialHeight == 0 {
		manager.lastState.LastBlockHeight = 0
	}

	return manager, mockStore, mockExec, mockSeq, testSigner, cancel
}

// TestPublishBlockInternal_MaxPendingHeadersReached verifies that publishBlockInternal returns an error if the maximum number of pending headers is reached.
func TestPublishBlockInternal_MaxPendingHeadersReached(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	currentHeight := uint64(10)
	lastSubmitted := uint64(5)
	maxPending := uint64(5)
	logBuffer := new(bytes.Buffer)

	manager, mockStore, mockExec, mockSeq, _, cancel := setupManagerForPublishBlockTest(t, currentHeight+1, lastSubmitted, logBuffer)
	defer cancel()

	manager.config.Node.MaxPendingHeaders = maxPending
	ctx := context.Background()

	mockStore.On("Height", ctx).Return(currentHeight, nil)

	err := manager.publishBlock(ctx)

	require.Nil(err, "publishBlockInternal should not return an error (otherwise the chain would halt)")
	require.Contains(logBuffer.String(), "pending blocks [5] reached limit [5]", "log message mismatch")

	mockStore.AssertExpectations(t)
	mockExec.AssertNotCalled(t, "GetTxs", mock.Anything)
	mockSeq.AssertNotCalled(t, "GetNextBatch", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "GetSignature", mock.Anything, mock.Anything)
}

// Test_publishBlock_NoBatch verifies that publishBlock returns nil when no batch is available from the sequencer.
func Test_publishBlock_NoBatch(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	ctx := context.Background()

	// Setup manager with mocks
	mockStore := mocks.NewStore(t)
	mockSeq := mocks.NewSequencer(t)
	mockExec := mocks.NewExecutor(t)
	logger := log.NewTestLogger(t)
	chainID := "Test_publishBlock_NoBatch"
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(chainID)
	noopSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	m := &Manager{
		store:     mockStore,
		sequencer: mockSeq,
		exec:      mockExec,
		logger:    logger,
		signer:    noopSigner,
		genesis:   genesisData,
		config: config.Config{
			Node: config.NodeConfig{
				MaxPendingHeaders: 0,
			},
		},
		pendingHeaders: &PendingHeaders{
			store:  mockStore,
			logger: logger,
		},
		lastStateMtx: &sync.RWMutex{},
		metrics:      NopMetrics(),
	}

	m.publishBlock = m.publishBlockInternal

	bz := make([]byte, 8)
	binary.LittleEndian.PutUint64(bz, 0)
	mockStore.On("GetMetadata", ctx, LastSubmittedHeightKey).Return(bz, nil)
	err = m.pendingHeaders.init()
	require.NoError(err)

	// Mock store calls for height and previous block/commit
	currentHeight := uint64(1)
	mockStore.On("Height", ctx).Return(currentHeight, nil)
	mockSignature := types.Signature([]byte{1, 2, 3})
	mockStore.On("GetSignature", ctx, currentHeight).Return(&mockSignature, nil)
	lastHeader, lastData := types.GetRandomBlock(currentHeight, 0, chainID)
	mockStore.On("GetBlockData", ctx, currentHeight).Return(lastHeader, lastData, nil)
	mockStore.On("GetBlockData", ctx, currentHeight+1).Return(nil, nil, errors.New("not found"))

	// No longer testing GetTxs and SubmitBatchTxs since they're handled by reaper.go

	// *** Crucial Mock: Sequencer returns ErrNoBatch ***
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.Id) == chainID
	})
	mockSeq.On("GetNextBatch", ctx, batchReqMatcher).Return(nil, ErrNoBatch).Once()

	// Call publishBlock
	err = m.publishBlock(ctx)

	// Assertions
	require.NoError(err, "publishBlock should return nil error when no batch is available")

	// Verify mocks: Ensure methods after the check were NOT called
	mockStore.AssertNotCalled(t, "SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockExec.AssertNotCalled(t, "ExecuteTxs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockExec.AssertNotCalled(t, "SetFinal", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "SetHeight", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "UpdateState", mock.Anything, mock.Anything)

	mockSeq.AssertExpectations(t)
	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)
}

// Test_publishBlock_EmptyBatch verifies that publishBlock returns nil and does not publish a block when the batch is empty.
func Test_publishBlock_EmptyBatch(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	ctx := context.Background()

	// Setup manager with mocks
	mockStore := mocks.NewStore(t)
	mockSeq := mocks.NewSequencer(t)
	mockExec := mocks.NewExecutor(t)
	logger := log.NewTestLogger(t)
	chainID := "Test_publishBlock_EmptyBatch"
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(chainID)
	noopSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	daH := atomic.Uint64{}
	daH.Store(0)

	m := &Manager{
		store:     mockStore,
		sequencer: mockSeq,
		exec:      mockExec,
		logger:    logger,
		signer:    noopSigner,
		genesis:   genesisData,
		config: config.Config{
			Node: config.NodeConfig{
				MaxPendingHeaders: 0,
			},
		},
		pendingHeaders: &PendingHeaders{
			store:  mockStore,
			logger: logger,
		},
		lastStateMtx: &sync.RWMutex{},
		metrics:      NopMetrics(),
		lastState: types.State{
			ChainID:         chainID,
			InitialHeight:   1,
			LastBlockHeight: 1,
			LastBlockTime:   time.Now(),
			AppHash:         []byte("initialAppHash"),
		},
		headerCache: cache.NewCache[types.SignedHeader](),
		dataCache:   cache.NewCache[types.Data](),
		headerBroadcaster: broadcasterFn[*types.SignedHeader](func(ctx context.Context, payload *types.SignedHeader) error {
			return nil
		}),
		dataBroadcaster: broadcasterFn[*types.Data](func(ctx context.Context, payload *types.Data) error {
			return nil
		}),
		daHeight: &daH,
	}

	m.publishBlock = m.publishBlockInternal

	bz := make([]byte, 8)
	binary.LittleEndian.PutUint64(bz, 0)
	mockStore.On("GetMetadata", ctx, LastSubmittedHeightKey).Return(bz, nil)
	err = m.pendingHeaders.init()
	require.NoError(err)

	// Mock store calls
	currentHeight := uint64(1)
	mockStore.On("Height", ctx).Return(currentHeight, nil)
	mockSignature := types.Signature([]byte{1, 2, 3})
	mockStore.On("GetSignature", ctx, currentHeight).Return(&mockSignature, nil)
	lastHeader, lastData := types.GetRandomBlock(currentHeight, 0, chainID)
	mockStore.On("GetBlockData", ctx, currentHeight).Return(lastHeader, lastData, nil)
	mockStore.On("GetBlockData", ctx, currentHeight+1).Return(nil, nil, errors.New("not found"))

	// No longer testing GetTxs and SubmitBatchTxs since they're handled by reaper.go

	// *** Crucial Mock: Sequencer returns an empty batch ***
	emptyBatchResponse := &coresequencer.GetNextBatchResponse{
		Batch: &coresequencer.Batch{
			Transactions: [][]byte{},
		},
		Timestamp: time.Now(),
		BatchData: [][]byte{[]byte("some_batch_data")},
	}
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.Id) == chainID
	})
	mockSeq.On("GetNextBatch", ctx, batchReqMatcher).Return(emptyBatchResponse, nil).Once()

	// Mock SetMetadata for LastBatchDataKey (required for empty batch handling)
	mockStore.On("SetMetadata", ctx, "l", mock.AnythingOfType("[]uint8")).Return(nil).Once()

	// With our new implementation, we should expect SaveBlockData to be called for empty blocks
	mockStore.On("SaveBlockData", ctx, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()

	// We should also expect ExecuteTxs to be called with an empty transaction list
	newAppHash := []byte("newAppHash")
	mockExec.On("ExecuteTxs", mock.Anything, mock.Anything, currentHeight+1, mock.AnythingOfType("time.Time"), m.lastState.AppHash).Return(newAppHash, uint64(100), nil).Once()

	// SetHeight should be called
	mockStore.On("SetHeight", ctx, currentHeight+1).Return(nil).Once()

	// UpdateState should be called
	mockStore.On("UpdateState", ctx, mock.AnythingOfType("types.State")).Return(nil).Once()

	// SaveBlockData should be called again after validation
	mockStore.On("SaveBlockData", ctx, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()

	// Call publishBlock
	err = m.publishBlock(ctx)

	// Assertions
	require.NoError(err, "publishBlock should return nil error when the batch is empty")

	mockSeq.AssertExpectations(t)
	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)
}

// Test_publishBlock_Success verifies the happy path where a block with transactions is successfully created, applied, and published.
func Test_publishBlock_Success(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	initialHeight := uint64(5)
	newHeight := initialHeight + 1
	chainID := "testchain"

	manager, mockStore, mockExec, mockSeq, _, _ := setupManagerForPublishBlockTest(t, initialHeight, 0, new(bytes.Buffer))
	manager.lastState.LastBlockHeight = initialHeight

	mockStore.On("Height", t.Context()).Return(initialHeight, nil).Once()
	mockSignature := types.Signature([]byte{1, 2, 3})
	mockStore.On("GetSignature", t.Context(), initialHeight).Return(&mockSignature, nil).Once()
	lastHeader, lastData := types.GetRandomBlock(initialHeight, 5, chainID)
	lastHeader.ProposerAddress = manager.genesis.ProposerAddress
	mockStore.On("GetBlockData", t.Context(), initialHeight).Return(lastHeader, lastData, nil).Once()
	mockStore.On("GetBlockData", t.Context(), newHeight).Return(nil, nil, errors.New("not found")).Once()
	mockStore.On("SaveBlockData", t.Context(), mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	mockStore.On("SaveBlockData", t.Context(), mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	mockStore.On("SetHeight", t.Context(), newHeight).Return(nil).Once()
	mockStore.On("UpdateState", t.Context(), mock.AnythingOfType("types.State")).Return(nil).Once()
	mockStore.On("SetMetadata", t.Context(), LastBatchDataKey, mock.AnythingOfType("[]uint8")).Return(nil).Once()

	headerCh := make(chan *types.SignedHeader, 1)
	manager.headerBroadcaster = broadcasterFn[*types.SignedHeader](func(ctx context.Context, payload *types.SignedHeader) error {
		select {
		case headerCh <- payload:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	dataCh := make(chan *types.Data, 1)
	manager.dataBroadcaster = broadcasterFn[*types.Data](func(ctx context.Context, payload *types.Data) error {
		select {
		case dataCh <- payload:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	// --- Mock Executor ---
	sampleTxs := [][]byte{[]byte("tx1"), []byte("tx2")}
	// No longer mocking GetTxs since it's handled by reaper.go
	newAppHash := []byte("newAppHash")
	mockExec.On("ExecuteTxs", mock.Anything, mock.Anything, newHeight, mock.AnythingOfType("time.Time"), manager.lastState.AppHash).Return(newAppHash, uint64(100), nil).Once()

	// No longer mocking SubmitBatchTxs since it's handled by reaper.go
	batchTimestamp := lastHeader.Time().Add(1 * time.Second)
	batchDataBytes := [][]byte{[]byte("batch_data_1")}
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch: &coresequencer.Batch{
			Transactions: sampleTxs,
		},
		Timestamp: batchTimestamp,
		BatchData: batchDataBytes,
	}
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.Id) == chainID
	})
	mockSeq.On("GetNextBatch", t.Context(), batchReqMatcher).Return(batchResponse, nil).Once()
	err := manager.publishBlock(t.Context())
	require.NoError(err, "publishBlock should succeed")
	select {
	case publishedHeader := <-headerCh:
		assert.Equal(t, newHeight, publishedHeader.Height(), "Published header height mismatch")
		assert.Equal(t, manager.genesis.ProposerAddress, publishedHeader.ProposerAddress, "Published header proposer mismatch")
		assert.Equal(t, batchTimestamp.UnixNano(), publishedHeader.Time().UnixNano(), "Published header time mismatch")

	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for header on HeaderCh")
	}

	select {
	case publishedData := <-dataCh:
		assert.Equal(t, len(sampleTxs), len(publishedData.Txs), "Published data tx count mismatch")
		var txs [][]byte
		for _, tx := range publishedData.Txs {
			txs = append(txs, tx)
		}
		assert.Equal(t, sampleTxs, txs, "Published data txs mismatch")
		assert.NotNil(t, publishedData.Metadata, "Published data metadata should not be nil")
		if publishedData.Metadata != nil {
			assert.Equal(t, newHeight, publishedData.Metadata.Height, "Published data metadata height mismatch")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timed out waiting for data on DataCh")
	}

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)
	mockSeq.AssertExpectations(t)

	finalState := manager.GetLastState()
	assert.Equal(t, newHeight, finalState.LastBlockHeight, "Final state height mismatch")
	assert.Equal(t, newAppHash, finalState.AppHash, "Final state AppHash mismatch")
	assert.Equal(t, batchTimestamp, finalState.LastBlockTime, "Final state time mismatch")
}
