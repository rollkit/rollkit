package block

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
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
func setupManagerForPublishBlockTest(t *testing.T, isProposer bool, initialHeight uint64, lastSubmittedHeight uint64) (
	*Manager,
	*mocks.Store,
	*mocks.Executor,
	*mocks.Sequencer,
	signer.Signer,
	chan *types.SignedHeader,
	chan *types.Data,
	context.CancelFunc,
) {
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

	headerCh := make(chan *types.SignedHeader, 1)
	dataCh := make(chan *types.Data, 1)
	_, cancel := context.WithCancel(context.Background())
	logger := log.NewTestLogger(t)

	lastSubmittedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(lastSubmittedBytes, lastSubmittedHeight)
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(lastSubmittedBytes, nil).Maybe()

	var headerStore *goheaderstore.Store[*types.SignedHeader]
	var dataStore *goheaderstore.Store[*types.Data]

	// Manager initialization (simplified, add fields as needed by tests)
	manager := &Manager{
		store:          mockStore,
		exec:           mockExec,
		sequencer:      mockSeq,
		signer:         testSigner,
		config:         cfg,
		genesis:        genesis,
		logger:         logger,
		HeaderCh:       headerCh,
		DataCh:         dataCh,
		headerStore:    headerStore,
		dataStore:      dataStore,
		headerCache:    cache.NewCache[types.SignedHeader](),
		dataCache:      cache.NewCache[types.Data](),
		lastStateMtx:   &sync.RWMutex{},
		metrics:        NopMetrics(),
		isProposer:     isProposer,
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
		LastBlockTime:   genesis.GenesisDAStartHeight,
		AppHash:         []byte("initialAppHash"),
	}
	if initialHeight == 0 {
		manager.lastState.LastBlockHeight = 0
	}

	return manager, mockStore, mockExec, mockSeq, testSigner, headerCh, dataCh, cancel
}

// TestPublishBlockInternal_NotProposer verifies that publishBlockInternal
// returns ErrNotProposer if the manager is not configured as a proposer.
func TestPublishBlockInternal_NotProposer(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	manager, mockStore, mockExec, mockSeq, _, _, _, cancel := setupManagerForPublishBlockTest(t, false, 1, 0)
	defer cancel()

	ctx := context.Background()

	err := manager.publishBlock(ctx)

	require.Error(err, "publishBlockInternal should return an error")
	assert.ErrorIs(err, ErrNotProposer, "error should be ErrNotProposer")

	mockStore.AssertNotCalled(t, "Height", mock.Anything)
	mockExec.AssertNotCalled(t, "GetTxs", mock.Anything)
	mockSeq.AssertNotCalled(t, "GetNextBatch", mock.Anything, mock.Anything)
}

// TestPublishBlockInternal_MaxPendingBlocksReached verifies that publishBlockInternal
// returns an error if the maximum number of pending blocks is reached.
func TestPublishBlockInternal_MaxPendingBlocksReached(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	currentHeight := uint64(10)
	lastSubmitted := uint64(5)
	maxPending := uint64(5)

	manager, mockStore, mockExec, mockSeq, _, _, _, cancel := setupManagerForPublishBlockTest(t, true, currentHeight+1, lastSubmitted)
	defer cancel()

	manager.config.Node.MaxPendingBlocks = maxPending
	ctx := context.Background()

	mockStore.On("Height", ctx).Return(currentHeight, nil)

	err := manager.publishBlock(ctx)

	require.Error(err, "publishBlockInternal should return an error")
	assert.Contains(err.Error(), "pending blocks [5] reached limit [5]", "error message mismatch")

	mockStore.AssertExpectations(t)
	mockExec.AssertNotCalled(t, "GetTxs", mock.Anything)
	mockSeq.AssertNotCalled(t, "GetNextBatch", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "GetSignature", mock.Anything, mock.Anything)
}

func Test_publishBlock_ManagerNotProposer(t *testing.T) {
	require := require.New(t)
	mockDA := mocks.NewClient(t)       // Use mock DA Client
	m := getManager(t, mockDA, -1, -1) // Pass mock DA Client
	m.isProposer = false
	err := m.publishBlock(context.Background())
	require.ErrorIs(err, ErrNotProposer)
}

func Test_publishBlock_NoBatch(t *testing.T) {
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
		store:      mockStore,
		sequencer:  mockSeq,
		exec:       mockExec,
		logger:     logger,
		isProposer: true,
		signer:     noopSigner,
		genesis:    genesisData,
		config: config.Config{
			Node: config.NodeConfig{
				MaxPendingBlocks: 0,
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

	// Mock GetTxs on the executor
	mockExec.On("GetTxs", ctx).Return([][]byte{}, nil).Once()

	// Mock sequencer SubmitRollupBatchTxs (should still be called even if GetTxs is empty)
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID && len(req.Batch.Transactions) == 0
	})
	mockSeq.On("SubmitRollupBatchTxs", ctx, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	// *** Crucial Mock: Sequencer returns ErrNoBatch ***
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.RollupId) == chainID
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

func Test_publishBlock_EmptyBatch(t *testing.T) {
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

	m := &Manager{
		store:      mockStore,
		sequencer:  mockSeq,
		exec:       mockExec,
		logger:     logger,
		isProposer: true,
		signer:     noopSigner,
		genesis:    genesisData,
		config: config.Config{
			Node: config.NodeConfig{
				MaxPendingBlocks: 0,
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

	// Mock store calls
	currentHeight := uint64(1)
	mockStore.On("Height", ctx).Return(currentHeight, nil)
	mockSignature := types.Signature([]byte{1, 2, 3})
	mockStore.On("GetSignature", ctx, currentHeight).Return(&mockSignature, nil)
	lastHeader, lastData := types.GetRandomBlock(currentHeight, 0, chainID)
	mockStore.On("GetBlockData", ctx, currentHeight).Return(lastHeader, lastData, nil)
	mockStore.On("GetBlockData", ctx, currentHeight+1).Return(nil, nil, errors.New("not found"))

	// Mock GetTxs on the executor
	mockExec.On("GetTxs", ctx).Return([][]byte{}, nil).Once()

	// Mock sequencer SubmitRollupBatchTxs
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID && len(req.Batch.Transactions) == 0
	})
	mockSeq.On("SubmitRollupBatchTxs", ctx, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	// *** Crucial Mock: Sequencer returns an empty batch ***
	emptyBatchResponse := &coresequencer.GetNextBatchResponse{
		Batch: &coresequencer.Batch{
			Transactions: [][]byte{},
		},
		Timestamp: time.Now(),
		BatchData: [][]byte{[]byte("some_batch_data")},
	}
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.RollupId) == chainID
	})
	mockSeq.On("GetNextBatch", ctx, batchReqMatcher).Return(emptyBatchResponse, nil).Once()
	mockStore.On("SetMetadata", ctx, LastBatchDataKey, mock.Anything).Return(nil).Once()

	// Call publishBlock
	err = m.publishBlock(ctx)

	// Assertions
	require.NoError(err, "publishBlock should return nil error when the batch is empty")

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

// Test_publishBlock_Success tests the happy path where a block with transactions
// is successfully created, applied, and published.
func Test_publishBlock_Success(t *testing.T) {
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	initialHeight := uint64(5)
	newHeight := initialHeight + 1
	chainID := "testchain"

	manager, mockStore, mockExec, mockSeq, _, headerCh, dataCh, _ := setupManagerForPublishBlockTest(t, true, initialHeight, 0)
	manager.lastState.LastBlockHeight = initialHeight

	mockStore.On("Height", ctx).Return(initialHeight, nil).Once()
	mockSignature := types.Signature([]byte{1, 2, 3})
	mockStore.On("GetSignature", ctx, initialHeight).Return(&mockSignature, nil).Once()
	lastHeader, lastData := types.GetRandomBlock(initialHeight, 5, chainID)
	lastHeader.ProposerAddress = manager.genesis.ProposerAddress
	mockStore.On("GetBlockData", ctx, initialHeight).Return(lastHeader, lastData, nil).Once()
	mockStore.On("GetBlockData", ctx, newHeight).Return(nil, nil, errors.New("not found")).Once()
	mockStore.On("SaveBlockData", ctx, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	mockStore.On("SaveBlockData", ctx, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	mockStore.On("SetHeight", ctx, newHeight).Return(nil).Once()
	mockStore.On("UpdateState", ctx, mock.AnythingOfType("types.State")).Return(nil).Once()
	mockStore.On("SetMetadata", ctx, LastBatchDataKey, mock.AnythingOfType("[]uint8")).Return(nil).Once()

	// --- Mock Executor ---
	sampleTxs := [][]byte{[]byte("tx1"), []byte("tx2")}
	mockExec.On("GetTxs", ctx).Return(sampleTxs, nil).Once()
	newAppHash := []byte("newAppHash")
	mockExec.On("ExecuteTxs", ctx, mock.Anything, newHeight, mock.AnythingOfType("time.Time"), manager.lastState.AppHash).Return(newAppHash, uint64(100), nil).Once()
	mockExec.On("SetFinal", ctx, newHeight).Return(nil).Once()

	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID && len(req.Batch.Transactions) == len(sampleTxs)
	})
	mockSeq.On("SubmitRollupBatchTxs", ctx, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
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
		return string(req.RollupId) == chainID
	})
	mockSeq.On("GetNextBatch", ctx, batchReqMatcher).Return(batchResponse, nil).Once()
	err := manager.publishBlock(ctx)
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
