package block_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/mock" // Already present, ensure it stays
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/block"
	coreda "github.com/rollkit/rollkit/core/da" // Add coreda import
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop"
	blockmocks "github.com/rollkit/rollkit/test/mocks"
	testmocks "github.com/rollkit/rollkit/test/mocks" // Import the general test mocks for Store
	"github.com/rollkit/rollkit/types"
)

// Test constants
const (
	testChainID = "test-chain-agg"
)

// setupManagerForAggregationTest initializes a Manager instance suitable for aggregation feature tests.
// It uses mock dependencies and a basic configuration. Returns the config used.
func setupManagerForAggregationTest(t *testing.T) (*block.Manager, *blockmocks.Executor, *blockmocks.Sequencer, *blockmocks.Client, *testmocks.Store, config.Config) {
	require := require.New(t)

	// Dependencies
	logger := log.NewTestLogger(t)
	// Use mock store instead of real store
	mockStore := testmocks.NewStore(t)
	mockExecutor := blockmocks.NewExecutor(t)
	mockSequencer := blockmocks.NewSequencer(t)
	mockDAClient := blockmocks.NewClient(t)
	// Header/Data stores still needed for Manager init, but can use a dummy kvstore as they won't be heavily used in these tests
	dummyKV := dsync.MutexWrap(ds.NewMapDatastore())
	headerStore, err := goheaderstore.NewStore[*types.SignedHeader](dummyKV, goheaderstore.WithStorePrefix("header"))
	require.NoError(err)
	dataStore, err := goheaderstore.NewStore[*types.Data](dummyKV, goheaderstore.WithStorePrefix("data"))
	require.NoError(err)

	// Signer (required for proposer)
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(testChainID)
	proposerKey, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	// Config
	nodeConfig := config.NodeConfig{
		BlockTime:        config.DurationWrapper{Duration: 1 * time.Second}, // Use a reasonable block time for tests
		LazyAggregator:   false,                                             // Ensure normal mode
		MaxPendingBlocks: 0,                                                 // No limit for basic tests
	}
	daConfig := config.DAConfig{
		BlockTime:  config.DurationWrapper{Duration: 15 * time.Second}, // Example DA block time
		MempoolTTL: 25,
	}
	conf := config.Config{Node: nodeConfig, DA: daConfig}

	// Genesis
	gen := genesis.NewGenesis(
		genesisData.ChainID,
		genesisData.InitialHeight,
		genesisData.GenesisDAStartHeight,
		genesisData.ProposerAddress,
	)

	// Mock initial manager setup calls
	initialAppHash := []byte("initial state root")
	mockExecutor.On("InitChain", context.Background(), gen.GenesisDAStartHeight, gen.InitialHeight, gen.ChainID).Return(initialAppHash, uint64(1000), nil).Maybe() // Use Maybe for flexibility
	// Mock GetState to simulate starting from genesis (no existing state)
	mockStore.On("GetState", context.Background()).Return(types.State{}, ds.ErrNotFound).Once()
	// Mocks for saving genesis block data (called inside getInitialState if state not found)
	mockStore.On("SaveBlockData", context.Background(), mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	// Mock setting height after genesis init
	mockStore.On("SetHeight", context.Background(), uint64(0)).Return(nil).Once() // Initial height is 1, last block height is 0
	// Mock metadata reads during NewManager
	mockStore.On("GetMetadata", context.Background(), block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Maybe()
	mockStore.On("GetMetadata", context.Background(), block.LastBatchDataKey).Return(nil, ds.ErrNotFound).Maybe() // Ensure this is mocked
	mockStore.On("GetMetadata", context.Background(), block.DAIncludedHeightKey).Return(nil, ds.ErrNotFound).Maybe()
	// Mock MaxBlobSize needed by NewManager
	mockDAClient.On("MaxBlobSize", context.Background()).Return(uint64(10000), nil).Maybe()

	// Create Manager
	manager, err := block.NewManager(
		context.Background(),
		proposerKey,
		conf,
		gen,
		mockStore, // Use mock store here
		mockExecutor,
		mockSequencer,
		mockDAClient,
		logger,
		headerStore,
		dataStore,
		block.NopMetrics(),
		-1, // Default gas price
		-1, // Default gas multiplier
	)
	require.NoError(err)
	require.NotNil(manager)
	require.True(manager.IsProposer(), "Manager should be a proposer for aggregation tests")

	return manager, mockExecutor, mockSequencer, mockDAClient, mockStore, conf // Return mock store and config
}

// TestManager_Aggregation_BasicBlockProduction tests the basic flow of producing a single block
// in the normal aggregation mode.
func TestManager_Aggregation_BasicBlockProduction(t *testing.T) {
	require := require.New(t)
	// Receive mockStore and config from setup
	manager, mockExecutor, mockSequencer, _, mockStore, _ := setupManagerForAggregationTest(t) // Ignore config for now

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Timeout for the test
	defer cancel()

	// Test data
	tx1 := []byte("tx1")
	txs := [][]byte{tx1}
	nextHeight := manager.GetLastState().InitialHeight // Should be 1
	expectedNewStateRoot := []byte("new state root")
	batchData := [][]byte{[]byte("batch data 1")}
	batchTime := time.Now()

	// Mock interactions for producing block 1
	// 1. Previous block state (Genesis) - Mock store interactions
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}} // Dummy genesis header
	genesisData := &types.Data{}
	genesisSignature := &types.Signature{}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, genesisData, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(genesisSignature, nil).Once()

	// 2. Executor gets transactions
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()

	// 3. Sequencer submits transactions
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == testChainID && len(req.Batch.Transactions) == 1 && bytes.Equal(req.Batch.Transactions[0], tx1)
	})
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	// 4. Sequencer returns the next batch
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		// TODO(cline): Add check for LastBatchData if needed later
		return string(req.RollupId) == testChainID
	})
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: txs},
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, batchReqMatcher).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once() // Expect saving batch data

	// 5. Executor executes transactions
	executeReqMatcher := mock.MatchedBy(func(txList [][]byte) bool {
		return len(txList) == 1 && bytes.Equal(txList[0], tx1)
	})
	mockExecutor.On("ExecuteTxs", mock.Anything, executeReqMatcher, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()

	// 6. Store saves the new block data (header, data, signature)
	// We capture the arguments to verify them later as the signature is generated internally
	var capturedHeader *types.SignedHeader
	var capturedData *types.Data
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			capturedHeader = args.Get(1).(*types.SignedHeader)
			capturedData = args.Get(2).(*types.Data)
		}).Return(nil).Once()

	// 7. Executor finalizes the block
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()

	// 8. Store sets the new height
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()

	// 9. Store updates the state
	updateStateMatcher := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == nextHeight &&
			bytes.Equal(newState.AppHash, expectedNewStateRoot) &&
			newState.LastBlockTime == batchTime
		// TODO(cline): Add DAHeight check if needed
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher).Return(nil).Once()

	// Run aggregation loop in background
	go manager.AggregationLoop(ctx)

	// Assertions: Wait for the block to be produced and sent to channels
	select {
	case header := <-manager.HeaderCh:
		require.NotNil(header)
		require.Equal(nextHeight, header.Height())
		require.Equal(genesisHeader.Hash(), header.LastHeaderHash)
		require.Equal(len(txs), len(capturedData.Txs)) // Use captured data for tx check
		require.Equal(tx1, capturedData.Txs[0])
		require.Equal(expectedNewStateRoot, header.AppHash)        // AppHash before commit
		require.Equal(uint64(batchTime.UnixNano()), header.Time()) // Compare uint64 directly

		// Verify captured header matches sent header (basic checks)
		require.Equal(header.Height(), capturedHeader.Height())
		require.Equal(header.ProposerAddress, capturedHeader.ProposerAddress)

	case <-ctx.Done():
		require.Fail("timed out waiting for header on HeaderCh")
	}

	select {
	case data := <-manager.DataCh:
		require.NotNil(data)
		require.Equal(nextHeight, data.Metadata.Height)
		require.Equal(len(txs), len(data.Txs))
		require.Equal(tx1, data.Txs[0])
		require.Equal(genesisHeader.Hash(), data.Metadata.LastDataHash) // DataHash of genesis block
		require.Equal(uint64(batchTime.UnixNano()), data.Metadata.Time) // Compare uint64 directly

		// Verify captured data matches sent data
		require.Equal(data.Txs, capturedData.Txs)

	case <-ctx.Done():
		require.Fail("timed out waiting for data on DataCh")
	}

	// Final check of mock expectations after loop stops
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Aggregation_NoBatch tests block production skipping when the sequencer returns ErrNoBatch.
func TestManager_Aggregation_NoBatch(t *testing.T) {
	require := require.New(t)
	// Receive mockStore and config from setup
	manager, mockExecutor, mockSequencer, _, mockStore, conf := setupManagerForAggregationTest(t) // Use config here

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Timeout for the test
	defer cancel()

	// Test data
	tx1 := []byte("tx1")
	txs := [][]byte{tx1}

	// Mock interactions
	// 1. Previous block state (Genesis) - Mock store interactions
	genesisState := manager.GetLastState()
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(&types.SignedHeader{}, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()

	// 2. Executor gets transactions
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()

	// 3. Sequencer submits transactions
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == testChainID && len(req.Batch.Transactions) == 1 && bytes.Equal(req.Batch.Transactions[0], tx1)
	})
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	// 4. Sequencer returns ErrNoBatch
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.RollupId) == testChainID
	})
	mockSequencer.On("GetNextBatch", mock.Anything, batchReqMatcher).Return(nil, block.ErrNoBatch).Once()

	// Run aggregation loop in background
	go manager.AggregationLoop(ctx)

	// Assertions: Wait a bit to ensure publishBlock was called and exited due to ErrNoBatch
	time.Sleep(conf.Node.BlockTime.Duration + 50*time.Millisecond) // Wait slightly longer than one block time

	// Ensure no block was produced by checking channels are empty
	select {
	case header := <-manager.HeaderCh:
		require.Fail("HeaderCh should be empty, but received header", "height", header.Height())
	default:
		// Expected: Channel is empty
	}
	select {
	case data := <-manager.DataCh:
		require.Fail("DataCh should be empty, but received data", "height", data.Metadata.Height)
	default:
		// Expected: Channel is empty
	}

	// Final check of mock expectations
	// Crucially, SaveBlockData, SetHeight, UpdateState, ExecuteTxs, SetFinal should NOT have been called
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockStore.AssertNotCalled(t, "SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "SetHeight", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "UpdateState", mock.Anything, mock.Anything)
}

// TestManager_Aggregation_DASubmission_Success tests successful submission of a block header to DA.
func TestManager_Aggregation_DASubmission_Success(t *testing.T) {
	require := require.New(t)
	// Setup with mocks, get config
	manager, mockExecutor, mockSequencer, mockDAClient, mockStore, conf := setupManagerForAggregationTest(t)

	aggCtx, aggCancel := context.WithTimeout(context.Background(), 5*time.Second) // Context for the test
	defer aggCancel()

	// --- Setup Mocks for Block Production ---
	tx1 := []byte("tx1-da-success")
	txs := [][]byte{tx1}
	nextHeight := manager.GetLastState().InitialHeight // Should be 1
	expectedNewStateRoot := []byte("new state root da success")
	batchData := [][]byte{[]byte("batch data da success")}
	batchTime := time.Now()
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, mock.Anything).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: txs},
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()
	mockExecutor.On("ExecuteTxs", mock.Anything, mock.Anything, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()
	// Capture the produced header
	var producedHeader *types.SignedHeader
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			producedHeader = args.Get(1).(*types.SignedHeader)
		}).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()

	// --- Run Aggregation Loop to Produce Block ---
	go manager.AggregationLoop(aggCtx)

	// Wait for the block to be produced
	select {
	case <-manager.HeaderCh:
		// Block produced, continue
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block production")
	}
	// Consume the data as well
	select {
	case <-manager.DataCh:
		// Block produced, continue
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block data")
	}

	// Stop the aggregation loop after one block
	aggCancel() // Cancel the context for AggregationLoop

	// Retrieve the produced header from the mock (needed for GetBlockData mock below)
	// This relies on the capturedHeader variable being set by the SaveBlockData mock run function
	require.NotNil(producedHeader, "Produced header should have been captured by SaveBlockData mock")

	// --- Mock DA Submission ---
	daCtx := context.Background() // New context for DA submission part
	// Need to mock GetBlockData again as submitHeadersToDA reads pending headers
	mockStore.On("Height", daCtx).Return(nextHeight, nil).Once() // Manager checks current height
	// Convert [][]byte to types.Txs for the mock return
	var dataTxs types.Txs
	for _, txBytes := range txs {
		dataTxs = append(dataTxs, types.Tx(txBytes))
	}
	mockStore.On("GetBlockData", daCtx, nextHeight).Return(producedHeader, &types.Data{Txs: dataTxs}, nil).Once()
	mockStore.On("GetMetadata", daCtx, block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once() // Assume first submission

	mockDAClient.On("MaxBlobSize", daCtx).Return(uint64(10000), nil).Once()

	// Mock successful DA submission
	expectedSubmitResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			Message:        "Submitted successfully",
			Height:         100, // Example DA height
			SubmittedCount: 1,
		},
	}
	// Match based on the number of blobs (1 header)
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool {
		return len(data) == 1
	})
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(expectedSubmitResult).Once()

	// Expect metadata update for last submitted height
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, mock.Anything).Return(nil).Once()

	// --- Call submitHeadersToDA ---
	// This method is private, so we trigger it via the public loop or need a test helper/reflection.
	// For simplicity, let's assume a helper or direct call is possible for testing.
	// If HeaderSubmissionLoop was public or we had a test helper:
	// err = manager.SubmitHeadersToDA(ctx) // Hypothetical direct call
	// require.NoError(err)

	// --- Run Header Submission Loop ---
	submissionCtx, submissionCancel := context.WithCancel(daCtx)
	go manager.HeaderSubmissionLoop(submissionCtx)
	// Give the loop time to run and submit
	time.Sleep(conf.DA.BlockTime.Duration + 100*time.Millisecond) // Wait slightly longer than DA block time
	submissionCancel()                                            // Stop the loop

	// --- Assertions ---
	// Verify PendingHeaders state (should reflect the submitted height)
	require.Equal(nextHeight, manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated")

	// Verify all mocks were called as expected
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Aggregation_DASubmission_Retry tests DA submission retry logic on temporary failures.
func TestManager_Aggregation_DASubmission_Retry(t *testing.T) {
	require := require.New(t)
	// Setup with mocks, get config
	manager, mockExecutor, mockSequencer, mockDAClient, mockStore, conf := setupManagerForAggregationTest(t)

	aggCtx, aggCancel := context.WithTimeout(context.Background(), 5*time.Second) // Context for block production
	defer aggCancel()

	// --- Setup Mocks for Block Production (Height 1) ---
	tx1 := []byte("tx1-da-retry")
	txs := [][]byte{tx1}
	nextHeight := manager.GetLastState().InitialHeight // Should be 1
	expectedNewStateRoot := []byte("new state root da retry")
	batchData := [][]byte{[]byte("batch data da retry")}
	batchTime := time.Now()
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, mock.Anything).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: txs},
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()
	mockExecutor.On("ExecuteTxs", mock.Anything, mock.Anything, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()
	var producedHeader *types.SignedHeader
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			producedHeader = args.Get(1).(*types.SignedHeader)
		}).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()

	// --- Run Aggregation Loop to Produce Block ---
	go manager.AggregationLoop(aggCtx)
	select {
	case <-manager.HeaderCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block production")
	}
	select {
	case <-manager.DataCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block data")
	}
	aggCancel() // Stop aggregation loop
	require.NotNil(producedHeader, "Produced header should have been captured")

	// --- Mock DA Submission (Retry Scenario) ---
	daCtx := context.Background()
	mockStore.On("Height", daCtx).Return(nextHeight, nil).Once()
	var dataTxs types.Txs
	for _, txBytes := range txs {
		dataTxs = append(dataTxs, types.Tx(txBytes))
	}
	mockStore.On("GetBlockData", daCtx, nextHeight).Return(producedHeader, &types.Data{Txs: dataTxs}, nil).Once()
	mockStore.On("GetMetadata", daCtx, block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once()
	mockDAClient.On("MaxBlobSize", daCtx).Return(uint64(10000), nil).Times(2) // Expect MaxBlobSize check on each attempt

	// Mock DA submission: Fail first, succeed second
	failResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:    coreda.StatusNotIncludedInBlock, // Simulate temporary failure
			Message: "Failed to include in block",
		},
	}
	successResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			Message:        "Submitted successfully on retry",
			Height:         101, // Example DA height
			SubmittedCount: 1,
		},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 1 })
	// Fail first time
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(failResult).Once()
	// Succeed second time
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(successResult).Once()

	// Expect metadata update only after successful submission
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, mock.Anything).Return(nil).Once()

	// --- Run Header Submission Loop ---
	submissionCtx, submissionCancel := context.WithCancel(daCtx)
	go manager.HeaderSubmissionLoop(submissionCtx)
	// Wait long enough for two attempts (2 * DA block time + buffer)
	waitTime := conf.DA.BlockTime.Duration*2 + 100*time.Millisecond
	time.Sleep(waitTime)
	submissionCancel() // Stop the loop

	// --- Assertions ---
	require.Equal(nextHeight, manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated after successful retry")

	// Verify all mocks were called as expected
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Aggregation_EmptyBlockProduction tests producing a block when the batch has no transactions.
func TestManager_Aggregation_EmptyBlockProduction(t *testing.T) {
	require := require.New(t)
	// Receive mockStore and config from setup
	manager, mockExecutor, mockSequencer, _, mockStore, _ := setupManagerForAggregationTest(t) // Ignore config

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Timeout for the test
	defer cancel()

	// Test data
	nextHeight := manager.GetLastState().InitialHeight       // Should be 1
	expectedNewStateRoot := []byte("empty block state root") // Executor might still change state
	batchData := [][]byte{[]byte("empty batch data")}
	batchTime := time.Now()

	// Mock interactions for producing block 1 (empty)
	// 1. Previous block state (Genesis)
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()

	// 2. Executor gets transactions (returns empty)
	mockExecutor.On("GetTxs", mock.Anything).Return([][]byte{}, nil).Once()

	// 3. Sequencer submits transactions (empty batch)
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == testChainID && len(req.Batch.Transactions) == 0
	})
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	// 4. Sequencer returns the next batch (empty)
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.RollupId) == testChainID
	})
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: [][]byte{}}, // Empty transactions
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, batchReqMatcher).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()

	// 5. Executor executes transactions (empty list)
	mockExecutor.On("ExecuteTxs", mock.Anything, [][]byte{}, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()

	// 6. Store saves the new block data (header, data, signature)
	var capturedData *types.Data // Only need to capture data to check Txs
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			// capturedHeader = args.Get(1).(*types.SignedHeader) // Not needed for this test
			capturedData = args.Get(2).(*types.Data)
		}).Return(nil).Once()

	// 7. Executor finalizes the block
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()

	// 8. Store sets the new height
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()

	// 9. Store updates the state
	updateStateMatcher := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == nextHeight &&
			bytes.Equal(newState.AppHash, expectedNewStateRoot) && // Expecting the new root from ExecuteTxs
			newState.LastBlockTime == batchTime
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher).Return(nil).Once()

	// Run aggregation loop in background
	go manager.AggregationLoop(ctx)

	// Assertions: Wait for the block to be produced
	select {
	case header := <-manager.HeaderCh:
		require.NotNil(header)
		require.Equal(nextHeight, header.Height())
		require.Equal(genesisHeader.Hash(), header.LastHeaderHash)
		require.Equal(expectedNewStateRoot, header.AppHash)
		require.Equal(uint64(batchTime.UnixNano()), header.Time())
		// Check captured data has no Txs
		require.NotNil(capturedData)
		require.Empty(capturedData.Txs, "Captured block data should have no transactions")

	case <-ctx.Done():
		require.Fail("timed out waiting for header on HeaderCh")
	}

	select {
	case data := <-manager.DataCh:
		require.NotNil(data)
		require.Equal(nextHeight, data.Metadata.Height)
		require.Empty(data.Txs, "Block data sent to channel should have no transactions")
		require.Equal(genesisHeader.Hash(), data.Metadata.LastDataHash)
		require.Equal(uint64(batchTime.UnixNano()), data.Metadata.Time)

	case <-ctx.Done():
		require.Fail("timed out waiting for data on DataCh")
	}

	// Cancel context to stop the loop gracefully
	cancel()

	// Final check of mock expectations
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Aggregation_DASubmission_TooBig tests DA submission retry logic when StatusTooBig is returned.
func TestManager_Aggregation_DASubmission_TooBig(t *testing.T) {
	require := require.New(t)
	// Setup with mocks, get config
	manager, mockExecutor, mockSequencer, mockDAClient, mockStore, conf := setupManagerForAggregationTest(t)

	aggCtx, aggCancel := context.WithTimeout(context.Background(), 5*time.Second) // Context for block production
	defer aggCancel()

	// --- Setup Mocks for Block Production (Height 1) ---
	tx1 := []byte("tx1-da-too-big")
	txs := [][]byte{tx1}
	nextHeight := manager.GetLastState().InitialHeight // Should be 1
	expectedNewStateRoot := []byte("new state root da too big")
	batchData := [][]byte{[]byte("batch data da too big")}
	batchTime := time.Now()
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, mock.Anything).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: txs},
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()
	mockExecutor.On("ExecuteTxs", mock.Anything, mock.Anything, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()
	var producedHeader *types.SignedHeader
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			producedHeader = args.Get(1).(*types.SignedHeader)
		}).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()

	// --- Run Aggregation Loop to Produce Block ---
	go manager.AggregationLoop(aggCtx)
	select {
	case <-manager.HeaderCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block production")
	}
	select {
	case <-manager.DataCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block data")
	}
	aggCancel() // Stop aggregation loop
	require.NotNil(producedHeader, "Produced header should have been captured")

	// --- Mock DA Submission (TooBig Scenario) ---
	daCtx := context.Background()
	mockStore.On("Height", daCtx).Return(nextHeight, nil).Once()
	var dataTxs types.Txs
	for _, txBytes := range txs {
		dataTxs = append(dataTxs, types.Tx(txBytes))
	}
	mockStore.On("GetBlockData", daCtx, nextHeight).Return(producedHeader, &types.Data{Txs: dataTxs}, nil).Once()
	mockStore.On("GetMetadata", daCtx, block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once()

	initialMaxBlobSize := uint64(10000)
	reducedMaxBlobSize := initialMaxBlobSize / 4 // Current logic divides by 4
	mockDAClient.On("MaxBlobSize", daCtx).Return(initialMaxBlobSize, nil).Times(2)

	// Mock DA submission: Fail first with TooBig, succeed second with reduced size
	tooBigResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:    coreda.StatusTooBig,
			Message: "Blob too big",
		},
	}
	successResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			Message:        "Submitted successfully with reduced size",
			Height:         102, // Example DA height
			SubmittedCount: 1,
		},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 1 })
	// Fail first time with initial size
	mockDAClient.On("Submit", mock.Anything, submitMatcher, initialMaxBlobSize, float64(-1)).Return(tooBigResult).Once()
	// Succeed second time with reduced size
	mockDAClient.On("Submit", mock.Anything, submitMatcher, reducedMaxBlobSize, float64(-1)).Return(successResult).Once()

	// Expect metadata update only after successful submission
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, mock.Anything).Return(nil).Once()

	// --- Run Header Submission Loop ---
	submissionCtx, submissionCancel := context.WithCancel(daCtx)
	go manager.HeaderSubmissionLoop(submissionCtx)
	// Wait long enough for two attempts
	waitTime := conf.DA.BlockTime.Duration*2 + 100*time.Millisecond
	time.Sleep(waitTime)
	submissionCancel() // Stop the loop

	// --- Assertions ---
	require.Equal(nextHeight, manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated after successful retry")

	// Verify all mocks were called as expected
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Aggregation_DASubmission_Error tests DA submission handling when a generic StatusError is returned.
func TestManager_Aggregation_DASubmission_Error(t *testing.T) {
	require := require.New(t)
	// Setup with mocks, get config
	manager, mockExecutor, mockSequencer, mockDAClient, mockStore, conf := setupManagerForAggregationTest(t)

	aggCtx, aggCancel := context.WithTimeout(context.Background(), 5*time.Second) // Context for block production
	defer aggCancel()

	// --- Setup Mocks for Block Production (Height 1) ---
	tx1 := []byte("tx1-da-error")
	txs := [][]byte{tx1}
	nextHeight := manager.GetLastState().InitialHeight // Should be 1
	expectedNewStateRoot := []byte("new state root da error")
	batchData := [][]byte{[]byte("batch data da error")}
	batchTime := time.Now()
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, mock.Anything).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: txs},
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()
	mockExecutor.On("ExecuteTxs", mock.Anything, mock.Anything, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()
	var producedHeader *types.SignedHeader
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			producedHeader = args.Get(1).(*types.SignedHeader)
		}).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()

	// --- Run Aggregation Loop to Produce Block ---
	go manager.AggregationLoop(aggCtx)
	select {
	case <-manager.HeaderCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block production")
	}
	select {
	case <-manager.DataCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block data")
	}
	aggCancel() // Stop aggregation loop
	require.NotNil(producedHeader, "Produced header should have been captured")

	// --- Mock DA Submission (Error Scenario) ---
	daCtx := context.Background()
	mockStore.On("Height", daCtx).Return(nextHeight, nil).Once()
	var dataTxs types.Txs
	for _, txBytes := range txs {
		dataTxs = append(dataTxs, types.Tx(txBytes))
	}
	mockStore.On("GetBlockData", daCtx, nextHeight).Return(producedHeader, &types.Data{Txs: dataTxs}, nil).Once()
	mockStore.On("GetMetadata", daCtx, block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once()
	mockDAClient.On("MaxBlobSize", daCtx).Return(uint64(10000), nil).Once() // Only called once before error

	// Mock DA submission: Fail with StatusError
	errorResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:    coreda.StatusError,
			Message: "Generic DA layer error",
		},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 1 })
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(errorResult).Once()

	// --- Run Header Submission Loop ---
	submissionCtx, submissionCancel := context.WithCancel(daCtx)
	go manager.HeaderSubmissionLoop(submissionCtx)
	// Wait long enough for one attempt
	waitTime := conf.DA.BlockTime.Duration + 100*time.Millisecond
	time.Sleep(waitTime)
	submissionCancel() // Stop the loop

	// --- Assertions ---
	// Last submitted height should NOT be updated on StatusError
	require.Equal(uint64(0), manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should NOT be updated on StatusError")

	// Verify all mocks were called as expected
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t) // SetMetadata for LastSubmittedHeightKey should NOT have been called
}

// TestManager_Aggregation_DASubmission_GasPriceAdjustment tests DA submission retry logic with gas price adjustments.
func TestManager_Aggregation_DASubmission_GasPriceAdjustment(t *testing.T) {
	require := require.New(t)

	// --- Custom Setup with Gas Price ---
	// Base setup
	logger := log.NewTestLogger(t)
	mockStore := testmocks.NewStore(t)
	mockExecutor := blockmocks.NewExecutor(t)
	mockSequencer := blockmocks.NewSequencer(t)
	mockDAClient := blockmocks.NewClient(t)
	dummyKV := dsync.MutexWrap(ds.NewMapDatastore())
	headerStore, err := goheaderstore.NewStore[*types.SignedHeader](dummyKV, goheaderstore.WithStorePrefix("header"))
	require.NoError(err)
	dataStore, err := goheaderstore.NewStore[*types.Data](dummyKV, goheaderstore.WithStorePrefix("data"))
	require.NoError(err)
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(testChainID)
	proposerKey, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	// Config with gas price and multiplier
	initialGasPrice := 1.0
	gasMultiplier := 1.2
	nodeConfig := config.NodeConfig{
		BlockTime:        config.DurationWrapper{Duration: 1 * time.Second},
		LazyAggregator:   false,
		MaxPendingBlocks: 0,
	}
	daConfig := config.DAConfig{
		BlockTime:  config.DurationWrapper{Duration: 15 * time.Second},
		MempoolTTL: 25,
	}
	// Use specific gas price and multiplier
	conf := config.Config{Node: nodeConfig, DA: daConfig}
	gen := genesis.NewGenesis(genesisData.ChainID, genesisData.InitialHeight, genesisData.GenesisDAStartHeight, genesisData.ProposerAddress)
	mockExecutor.On("InitChain", context.Background(), gen.GenesisDAStartHeight, gen.InitialHeight, gen.ChainID).Return([]byte("initial state root"), uint64(1000), nil).Once()
	mockExecutor.On("SetFinal", context.Background(), uint64(0)).Return(nil).Maybe()

	// Create Manager with specific gas settings
	manager, err := block.NewManager(
		context.Background(), proposerKey, conf, gen, mockStore, mockExecutor, mockSequencer, mockDAClient, logger,
		headerStore, dataStore, block.NopMetrics(), initialGasPrice, gasMultiplier, // Pass gas price and multiplier
	)
	require.NoError(err)
	require.NotNil(manager)
	// --- End Custom Setup ---

	aggCtx, aggCancel := context.WithTimeout(context.Background(), 5*time.Second) // Context for block production
	defer aggCancel()

	// --- Setup Mocks for Block Production (Height 1) ---
	tx1 := []byte("tx1-da-gas")
	txs := [][]byte{tx1}
	nextHeight := manager.GetLastState().InitialHeight // Should be 1
	expectedNewStateRoot := []byte("new state root da gas")
	batchData := [][]byte{[]byte("batch data da gas")}
	batchTime := time.Now()
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()
	mockExecutor.On("GetTxs", mock.Anything).Return(txs, nil).Once()
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, mock.Anything).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
	batchResponse := &coresequencer.GetNextBatchResponse{
		Batch:     &coresequencer.Batch{Transactions: txs},
		Timestamp: batchTime,
		BatchData: batchData,
	}
	mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(batchResponse, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()
	mockExecutor.On("ExecuteTxs", mock.Anything, mock.Anything, nextHeight, batchTime, genesisState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()
	var producedHeader *types.SignedHeader
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).
		Run(func(args mock.Arguments) {
			producedHeader = args.Get(1).(*types.SignedHeader)
		}).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()

	// --- Run Aggregation Loop to Produce Block ---
	go manager.AggregationLoop(aggCtx)
	select {
	case <-manager.HeaderCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block production")
	}
	select {
	case <-manager.DataCh:
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block data")
	}
	aggCancel() // Stop aggregation loop
	require.NotNil(producedHeader, "Produced header should have been captured")

	// --- Mock DA Submission (Gas Adjustment Scenario) ---
	daCtx := context.Background()
	mockStore.On("Height", daCtx).Return(nextHeight, nil).Once()
	var dataTxs types.Txs
	for _, txBytes := range txs {
		dataTxs = append(dataTxs, types.Tx(txBytes))
	}
	mockStore.On("GetBlockData", daCtx, nextHeight).Return(producedHeader, &types.Data{Txs: dataTxs}, nil).Once()
	mockStore.On("GetMetadata", daCtx, block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once()
	mockDAClient.On("MaxBlobSize", daCtx).Return(uint64(10000), nil).Times(2)

	// Mock DA submission: Fail first, succeed second with adjusted gas price
	failResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{Code: coreda.StatusNotIncludedInBlock},
	}
	successResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{Code: coreda.StatusSuccess, SubmittedCount: 1},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 1 })
	expectedAdjustedGasPrice := initialGasPrice * gasMultiplier
	// Fail first time with initial gas price
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), initialGasPrice).Return(failResult).Once()
	// Succeed second time with adjusted gas price
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), expectedAdjustedGasPrice).Return(successResult).Once()

	// Expect metadata update after successful submission
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, mock.Anything).Return(nil).Once()

	// --- Run Header Submission Loop ---
	submissionCtx, submissionCancel := context.WithCancel(daCtx)
	go manager.HeaderSubmissionLoop(submissionCtx)
	// Wait long enough for two attempts
	waitTime := conf.DA.BlockTime.Duration*2 + 100*time.Millisecond
	time.Sleep(waitTime)
	submissionCancel() // Stop the loop

	// --- Assertions ---
	require.Equal(nextHeight, manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated after successful retry")

	// Verify all mocks were called as expected
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Aggregation_MaxPendingBlocks tests that block production stops when MaxPendingBlocks limit is reached.
func TestManager_Aggregation_MaxPendingBlocks(t *testing.T) {
	require := require.New(t)

	// --- Custom Setup with MaxPendingBlocks = 1 ---
	logger := log.NewTestLogger(t)
	mockStore := testmocks.NewStore(t)
	mockExecutor := blockmocks.NewExecutor(t)
	mockSequencer := blockmocks.NewSequencer(t)
	mockDAClient := blockmocks.NewClient(t)
	dummyKV := dsync.MutexWrap(ds.NewMapDatastore())
	headerStore, err := goheaderstore.NewStore[*types.SignedHeader](dummyKV, goheaderstore.WithStorePrefix("header"))
	require.NoError(err)
	dataStore, err := goheaderstore.NewStore[*types.Data](dummyKV, goheaderstore.WithStorePrefix("data"))
	require.NoError(err)
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(testChainID)
	proposerKey, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	// Config with MaxPendingBlocks = 1
	nodeConfig := config.NodeConfig{
		BlockTime:        config.DurationWrapper{Duration: 1 * time.Second},
		LazyAggregator:   false,
		MaxPendingBlocks: 1, // Set limit to 1
	}
	daConfig := config.DAConfig{
		BlockTime:  config.DurationWrapper{Duration: 15 * time.Second},
		MempoolTTL: 25,
	}
	conf := config.Config{Node: nodeConfig, DA: daConfig}
	gen := genesis.NewGenesis(genesisData.ChainID, genesisData.InitialHeight, genesisData.GenesisDAStartHeight, genesisData.ProposerAddress)
	mockExecutor.On("InitChain", context.Background(), gen.GenesisDAStartHeight, gen.InitialHeight, gen.ChainID).Return([]byte("initial state root"), uint64(1000), nil).Once()
	mockExecutor.On("SetFinal", context.Background(), uint64(0)).Return(nil).Maybe()

	manager, err := block.NewManager(
		context.Background(), proposerKey, conf, gen, mockStore, mockExecutor, mockSequencer, mockDAClient, logger,
		headerStore, dataStore, block.NopMetrics(), -1, -1,
	)
	require.NoError(err)
	require.NotNil(manager)
	// --- End Custom Setup ---

	aggCtx, aggCancel := context.WithTimeout(context.Background(), 5*time.Second) // Context for the test
	defer aggCancel()

	// --- Produce Block 1 (Successful) ---
	tx1 := []byte("tx1-max-pending")
	txs1 := [][]byte{tx1}
	height1 := manager.GetLastState().InitialHeight // Should be 1
	stateRoot1 := []byte("state root 1")
	batchData1 := [][]byte{[]byte("batch data 1")}
	batchTime1 := time.Now()
	genesisState := manager.GetLastState()
	genesisHeader := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 0}}}
	mockStore.On("GetBlockData", mock.Anything, genesisState.LastBlockHeight).Return(genesisHeader, &types.Data{}, nil).Once()
	mockStore.On("GetSignature", mock.Anything, genesisState.LastBlockHeight).Return(&types.Signature{}, nil).Once()
	mockExecutor.On("GetTxs", mock.Anything).Return(txs1, nil).Once()
	mockSequencer.On("SubmitRollupBatchTxs", mock.Anything, mock.Anything).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()
	batchResponse1 := &coresequencer.GetNextBatchResponse{Batch: &coresequencer.Batch{Transactions: txs1}, Timestamp: batchTime1, BatchData: batchData1}
	mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(batchResponse1, nil).Once()
	mockStore.On("SetMetadata", mock.Anything, block.LastBatchDataKey, mock.Anything).Return(nil).Once()
	mockExecutor.On("ExecuteTxs", mock.Anything, mock.Anything, height1, batchTime1, genesisState.AppHash).Return(stateRoot1, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, height1).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, height1).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()
	// Mock GetMetadata for pending headers check (assuming first block is pending)
	mockStore.On("GetMetadata", mock.Anything, block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once()

	// Run loop once to produce block 1
	go manager.AggregationLoop(aggCtx)
	select {
	case <-manager.HeaderCh: // Consume header
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block 1 production")
	}
	select {
	case <-manager.DataCh: // Consume data
	case <-aggCtx.Done():
		require.Fail("timed out waiting for block 1 data")
	}
	// Don't cancel yet, let the loop try to produce block 2

	// --- Attempt to Produce Block 2 (Should Fail due to Limit) ---
	// Mock store height for the check inside publishBlock
	mockStore.On("Height", mock.Anything).Return(height1, nil).Once() // Return height after block 1

	// Wait for the next block production attempt
	time.Sleep(conf.Node.BlockTime.Duration + 100*time.Millisecond)

	// --- Assertions ---
	// Verify the manager state reflects block 1 was produced
	require.Equal(height1, manager.GetLastState().LastBlockHeight)
	// We can't directly check numPendingHeaders as it's unexported.
	// The check below ensures the second block wasn't produced.

	// Cancel the loop
	aggCancel()

	// Verify mocks: Crucially, GetTxs, GetNextBatch etc. should NOT be called for the second block attempt
	mockExecutor.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
	mockStore.AssertExpectations(t)
	// Ensure SaveBlockData was only called once (for block 1)
	mockStore.AssertNumberOfCalls(t, "SaveBlockData", 1)
}
