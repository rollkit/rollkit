package block_test

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/block"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop"
	blockmocks "github.com/rollkit/rollkit/test/mocks"
	testmocks "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// Test constants for DA submission tests
const (
	daTestChainID = "test-chain-da"
)

// setupManagerForDASubmissionTest initializes a Manager instance suitable for DA submission tests.
// It configures the manager as a proposer and uses mock dependencies.
// Crucially, it allows pre-populating the store with blocks pending submission.
func setupManagerForDASubmissionTest(t *testing.T, initialLastSubmittedHeight uint64, blocksToPreload []*types.SignedHeader) (*block.Manager, *blockmocks.Client, *testmocks.Store, config.Config) {
	require := require.New(t)

	// Dependencies
	logger := log.NewTestLogger(t)
	mockStore := testmocks.NewStore(t)
	// Executor/Sequencer mocks needed for manager init, but won't be used heavily in these tests
	mockExecutor := blockmocks.NewExecutor(t)
	mockSequencer := blockmocks.NewSequencer(t)
	mockDAClient := blockmocks.NewClient(t)
	dummyKV := dsync.MutexWrap(ds.NewMapDatastore())
	headerStore, err := goheaderstore.NewStore[*types.SignedHeader](dummyKV, goheaderstore.WithStorePrefix("header"))
	require.NoError(err)
	dataStore, err := goheaderstore.NewStore[*types.Data](dummyKV, goheaderstore.WithStorePrefix("data"))
	require.NoError(err)

	// Signer (required for proposer manager init)
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(daTestChainID)
	proposerKey, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	// Config
	nodeConfig := config.NodeConfig{
		BlockTime:      config.DurationWrapper{Duration: 1 * time.Second}, // Not critical for DA tests
		LazyAggregator: false,
	}
	daConfig := config.DAConfig{
		BlockTime:  config.DurationWrapper{Duration: 100 * time.Millisecond}, // Faster DA time for tests
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
	mockExecutor.On("InitChain", context.Background(), gen.GenesisDAStartHeight, gen.InitialHeight, gen.ChainID).Return([]byte("initial state root"), uint64(1000), nil).Maybe()
	mockStore.On("GetState", context.Background()).Return(types.State{LastBlockHeight: 0, InitialHeight: 1}, nil).Maybe() // Assume some initial state exists
	mockStore.On("SetHeight", context.Background(), uint64(0)).Return(nil).Maybe()
	// Mock GetMetadata for LastSubmittedHeightKey to set the initial pending state
	lastSubmittedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(lastSubmittedBytes, initialLastSubmittedHeight)
	mockStore.On("GetMetadata", context.Background(), block.LastSubmittedHeightKey).Return(lastSubmittedBytes, nil).Once()
	mockStore.On("GetMetadata", context.Background(), block.LastBatchDataKey).Return(nil, ds.ErrNotFound).Maybe()
	mockStore.On("GetMetadata", context.Background(), block.DAIncludedHeightKey).Return(nil, ds.ErrNotFound).Maybe()

	// Preload blocks into the mock store if provided
	var maxPreloadedHeight uint64
	for _, header := range blocksToPreload {
		height := header.Height()
		// Mock GetBlockData for when PendingHeaders tries to fetch them
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, &types.Data{}, nil).Maybe()
		if height > maxPreloadedHeight {
			maxPreloadedHeight = height
		}
	}
	// Mock the current height of the store
	mockStore.On("Height", mock.Anything).Return(maxPreloadedHeight, nil).Maybe()

	// Create Manager
	manager, err := block.NewManager(
		context.Background(),
		proposerKey, // Need proposer key for manager init
		conf,
		gen,
		mockStore,
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

	// Verify initial pending state
	require.Equal(initialLastSubmittedHeight, manager.PendingHeaders().GetLastSubmittedHeight(), "Initial last submitted height mismatch")

	return manager, mockDAClient, mockStore, conf
}

// TestManager_DASubmission_Success tests submitting a single pending header successfully.
func TestManager_DASubmission_Success(t *testing.T) {
	require := require.New(t)

	// Create a block to preload
	header1 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 1}}}
	blocksToPreload := []*types.SignedHeader{header1}

	// Setup manager with the preloaded block and initial submitted height 0
	manager, mockDAClient, mockStore, conf := setupManagerForDASubmissionTest(t, 0, blocksToPreload)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Timeout for the test
	defer cancel()

	// --- Mock Interactions ---
	// Mock Store.Height called by PendingHeaders.getPendingHeaders
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Once()
	// Mock Store.GetBlockData called by PendingHeaders.getPendingHeaders
	mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, &types.Data{}, nil).Once()
	// Mock DAClient.MaxBlobSize called by submitHeadersToDA
	mockDAClient.On("MaxBlobSize", mock.Anything).Return(uint64(10000), nil).Once()
	// Mock DAClient.Submit for successful submission
	expectedSubmitResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			Message:        "Submitted successfully",
			Height:         100, // Example DA height
			SubmittedCount: 1,   // Submitted the one pending block
		},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 1 }) // Expecting 1 header
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(expectedSubmitResult).Once()
	// Mock Store.SetMetadata for updating LastSubmittedHeightKey
	expectedSubmittedHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedSubmittedHeightBytes, 1)
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, expectedSubmittedHeightBytes).Return(nil).Once()

	// --- Run Header Submission Loop ---
	go manager.HeaderSubmissionLoop(ctx)

	// Wait for the submission attempt
	time.Sleep(conf.DA.BlockTime.Duration + 100*time.Millisecond) // Wait > DA block time + buffer

	// --- Assertions ---
	require.Equal(uint64(1), manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated to 1")

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_DASubmission_MultiplePending tests submitting multiple pending headers.
func TestManager_DASubmission_MultiplePending(t *testing.T) {
	require := require.New(t)

	// Create blocks to preload
	header1 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 1}}}
	header2 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 2}}}
	header3 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 3}}}
	blocksToPreload := []*types.SignedHeader{header1, header2, header3}

	// Setup manager with preloaded blocks and initial submitted height 0
	manager, mockDAClient, mockStore, conf := setupManagerForDASubmissionTest(t, 0, blocksToPreload)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Timeout for the test
	defer cancel()

	// --- Mock Interactions ---
	// Mock Store.Height called by PendingHeaders.getPendingHeaders
	mockStore.On("Height", mock.Anything).Return(uint64(3), nil).Once()
	// Mock Store.GetBlockData called by PendingHeaders.getPendingHeaders for each block
	mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, &types.Data{}, nil).Once()
	mockStore.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, &types.Data{}, nil).Once()
	mockStore.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, &types.Data{}, nil).Once()
	// Mock DAClient.MaxBlobSize called by submitHeadersToDA
	mockDAClient.On("MaxBlobSize", mock.Anything).Return(uint64(10000), nil).Once()
	// Mock DAClient.Submit for successful submission of all 3 blocks
	expectedSubmitResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			Message:        "Submitted successfully",
			Height:         101, // Example DA height
			SubmittedCount: 3,   // Submitted all 3 pending blocks
		},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 3 }) // Expecting 3 headers
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(expectedSubmitResult).Once()
	// Mock Store.SetMetadata for updating LastSubmittedHeightKey to 3
	expectedSubmittedHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedSubmittedHeightBytes, 3)
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, expectedSubmittedHeightBytes).Return(nil).Once()

	// --- Run Header Submission Loop ---
	go manager.HeaderSubmissionLoop(ctx)

	// Wait for the submission attempt
	time.Sleep(conf.DA.BlockTime.Duration + 100*time.Millisecond) // Wait > DA block time + buffer

	// --- Assertions ---
	require.Equal(uint64(3), manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated to 3")

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_DASubmission_PartialSubmit tests submitting some but not all pending headers.
func TestManager_DASubmission_PartialSubmit(t *testing.T) {
	require := require.New(t)

	// Create blocks to preload
	header1 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 1}}}
	header2 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 2}}}
	header3 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: 3}}}
	blocksToPreload := []*types.SignedHeader{header1, header2, header3}

	// Setup manager with preloaded blocks and initial submitted height 0
	manager, mockDAClient, mockStore, conf := setupManagerForDASubmissionTest(t, 0, blocksToPreload)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Timeout for the test
	defer cancel()

	// --- Mock Interactions ---
	// Mock Store.Height called by PendingHeaders.getPendingHeaders
	mockStore.On("Height", mock.Anything).Return(uint64(3), nil).Once()
	// Mock Store.GetBlockData called by PendingHeaders.getPendingHeaders for each block
	mockStore.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, &types.Data{}, nil).Once()
	mockStore.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, &types.Data{}, nil).Once()
	mockStore.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, &types.Data{}, nil).Once()
	// Mock DAClient.MaxBlobSize called by submitHeadersToDA
	mockDAClient.On("MaxBlobSize", mock.Anything).Return(uint64(10000), nil).Once()
	// Mock DAClient.Submit for successful partial submission (only block 1)
	expectedSubmitResult := coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			Message:        "Submitted partially",
			Height:         102, // Example DA height
			SubmittedCount: 1,   // Only submitted the first pending block
		},
	}
	submitMatcher := mock.MatchedBy(func(data [][]byte) bool { return len(data) == 3 }) // Still attempts to submit all 3
	mockDAClient.On("Submit", mock.Anything, submitMatcher, uint64(10000), float64(-1)).Return(expectedSubmitResult).Once()
	// Mock Store.SetMetadata for updating LastSubmittedHeightKey to 1
	expectedSubmittedHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedSubmittedHeightBytes, 1) // Should be updated to height 1
	mockStore.On("SetMetadata", mock.Anything, block.LastSubmittedHeightKey, expectedSubmittedHeightBytes).Return(nil).Once()

	// --- Run Header Submission Loop ---
	go manager.HeaderSubmissionLoop(ctx)

	// Wait for the submission attempt
	time.Sleep(conf.DA.BlockTime.Duration + 100*time.Millisecond) // Wait > DA block time + buffer

	// --- Assertions ---
	require.Equal(uint64(1), manager.PendingHeaders().GetLastSubmittedHeight(), "Last submitted height should be updated to 1 after partial submit")

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}
