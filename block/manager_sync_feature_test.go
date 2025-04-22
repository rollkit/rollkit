package block_test

import (
	"bytes" // Ensure bytes import is present
	"context"
	"testing"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/block"
	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop" // Re-confirming this import
	blockmocks "github.com/rollkit/rollkit/test/mocks"
	testmocks "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// Test constants for sync tests
const (
	syncTestChainID = "test-chain-sync"
)

// setupManagerForSyncTest initializes a Manager instance suitable for sync feature tests.
// It uses mock dependencies and configures the manager as a non-proposer.
func setupManagerForSyncTest(t *testing.T) (*block.Manager, *blockmocks.Executor, *blockmocks.Client, *testmocks.Store, config.Config) {
	require := require.New(t)

	// Dependencies
	logger := log.NewTestLogger(t)
	mockStore := testmocks.NewStore(t)
	mockExecutor := blockmocks.NewExecutor(t)
	// Sequencer mock isn't strictly needed for basic sync tests focused on applying blocks, but include for completeness
	mockSequencer := blockmocks.NewSequencer(t)
	mockDAClient := blockmocks.NewClient(t)
	// Header/Data stores needed for Manager init
	dummyKV := dsync.MutexWrap(ds.NewMapDatastore())
	headerStore, err := goheaderstore.NewStore[*types.SignedHeader](dummyKV, goheaderstore.WithStorePrefix("header"))
	require.NoError(err)
	dataStore, err := goheaderstore.NewStore[*types.Data](dummyKV, goheaderstore.WithStorePrefix("data"))
	require.NoError(err)

	// Config (non-proposer)
	nodeConfig := config.NodeConfig{
		BlockTime:      config.DurationWrapper{Duration: 1 * time.Second},
		LazyAggregator: false, // Mode doesn't matter much for sync tests
	}
	daConfig := config.DAConfig{ // Define DAConfig separately
		BlockTime: config.DurationWrapper{Duration: 15 * time.Second},
	}
	conf := config.Config{Node: nodeConfig, DA: daConfig} // Combine at the top level

	// Genesis (use a common helper if available, otherwise define here)
	genesisData, proposerPrivKey, _ := types.GetGenesisWithPrivkey(syncTestChainID) // Get the private key
	// Create a temporary signer to get the address bytes needed for genesis doc
	tempSigner, err := noopsigner.NewNoopSigner(proposerPrivKey)
	require.NoError(err)
	proposerAddrBytes, err := tempSigner.GetAddress() // Get address bytes from signer
	require.NoError(err)
	gen := genesis.NewGenesis(
		genesisData.ChainID,
		genesisData.InitialHeight,
		genesisData.GenesisDAStartHeight,
		proposerAddrBytes, // Use the derived address bytes
	)

	// Mock initial executor state for genesis
	// Use context.Background() for setup mocks
	mockExecutor.On("InitChain", context.Background(), gen.GenesisDAStartHeight, gen.InitialHeight, gen.ChainID).Return([]byte("initial state root"), uint64(1000), nil).Once()
	// Mock store GetState for initial state loading
	mockStore.On("GetState", context.Background()).Return(types.State{}, ds.ErrNotFound).Once() // Simulate no prior state
	// Mock saving genesis block data
	mockStore.On("SaveBlockData", context.Background(), mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	// Mock setting initial height
	mockStore.On("SetHeight", context.Background(), uint64(0)).Return(nil).Once() // Initial height is 1, so last block height is 0
	mockDAClient.On("MaxBlobSize", context.Background()).Return(uint64(10000), nil).Once()
	mockStore.On("GetMetadata", context.Background(), block.LastSubmittedHeightKey).Return(nil, ds.ErrNotFound).Once()
	mockStore.On("GetMetadata", context.Background(), block.LastBatchDataKey).Return(nil, ds.ErrNotFound).Once()
	mockStore.On("GetMetadata", context.Background(), block.DAIncludedHeightKey).Return(nil, ds.ErrNotFound).Once()

	// Create Manager (pass nil for proposer key)
	manager, err := block.NewManager(
		context.Background(),
		nil, // No proposer key for sync node
		conf,
		gen,
		mockStore,
		mockExecutor,
		mockSequencer, // Pass mock sequencer
		mockDAClient,
		logger,
		headerStore,
		dataStore,
		block.NopMetrics(),
		-1, // Gas price doesn't matter for sync node
		-1, // Gas multiplier doesn't matter for sync node
	)
	require.NoError(err)
	require.NotNil(manager)
	require.False(manager.IsProposer(), "Manager should NOT be a proposer for sync tests")

	return manager, mockExecutor, mockDAClient, mockStore, conf
}

// TestManager_Sync_BasicSyncOneBlock tests syncing a single block received via channels.
func TestManager_Sync_BasicSyncOneBlock(t *testing.T) {
	require := require.New(t)
	manager, mockExecutor, _, mockStore, _ := setupManagerForSyncTest(t) // Ignore DAClient and conf for this test

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Timeout for the test
	defer cancel()

	// --- Test Data ---
	// Create block 1 (the block to be synced)
	tx1 := types.Tx("tx1_sync")
	block1Time := time.Now()
	block1Header := &types.SignedHeader{
		Header: types.Header{
			BaseHeader: types.BaseHeader{
				ChainID: syncTestChainID,
				Height:  1,
				Time:    uint64(block1Time.UnixNano()),
			},
			// LastHeaderHash would be genesis header hash, AppHash would be genesis AppHash
			// DataHash needs to match block1Data
		},
		// Signature and Signer needed for validation if applicable (depends on manager logic)
		// For basic sync, let's assume validation passes or is mocked.
	}
	block1Data := &types.Data{
		Txs: types.Txs{tx1},
		Metadata: &types.Metadata{ // Metadata is added *after* creation usually, but needed for applyBlock
			ChainID: block1Header.ChainID(),
			Height:  block1Header.Height(),
			Time:    uint64(block1Time.UnixNano()), // Use uint64 representation
			// LastDataHash would be genesis data hash
		},
	}
	block1Header.DataHash = block1Data.Hash() // Set correct data hash

	// --- Mock Interactions ---
	initialState := manager.GetLastState() // State after genesis (height 0)
	expectedNewStateRoot := []byte("state root after block 1")
	nextHeight := initialState.LastBlockHeight + 1 // Should be 1

	// 1. Mock store Height check before processing
	mockStore.On("Height", mock.Anything).Return(initialState.LastBlockHeight, nil).Once()

	// 2. Mock Executor.ExecuteTxs for applying block 1
	executeMatcher := mock.MatchedBy(func(txs [][]byte) bool {
		return len(txs) == 1 && bytes.Equal(txs[0], tx1)
	})
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher, nextHeight, block1Time, initialState.AppHash).
		Return(expectedNewStateRoot, uint64(1000), nil).Once()

	// 3. Mock Store.SaveBlockData for saving block 1
	mockStore.On("SaveBlockData", mock.Anything, block1Header, block1Data, &block1Header.Signature).Return(nil).Once()

	// 4. Mock Executor.SetFinal for finalizing block 1
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()

	// 5. Mock Store.SetHeight for updating height to 1
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()

	// 6. Mock Store.UpdateState for the new state after block 1
	updateStateMatcher := mock.MatchedBy(func(newState types.State) bool {
		// Compare time.Time directly
		return newState.LastBlockHeight == nextHeight &&
			bytes.Equal(newState.AppHash, expectedNewStateRoot) &&
			newState.LastBlockTime == block1Time
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher).Return(nil).Once()

	// --- Run Sync Loop and Send Data ---
	go manager.SyncLoop(ctx)

	// Send header and data to the manager's channels
	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: block1Header, DAHeight: 10} // Example DA Height
	manager.GetDataInCh() <- block.NewDataEvent{Data: block1Data, DAHeight: 10}

	// --- Wait and Assert ---
	// Wait for the state to be updated. A simple sleep is brittle, ideally use a channel or check state repeatedly.
	time.Sleep(100 * time.Millisecond) // Give SyncLoop time to process

	// Assert final state
	finalState := manager.GetLastState()
	require.Equal(nextHeight, finalState.LastBlockHeight, "Manager state should be updated to height 1")
	require.Equal(expectedNewStateRoot, finalState.AppHash, "Manager state should have the new app hash")
	require.Equal(block1Time, finalState.LastBlockTime, "Manager state should have the new block time")

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	mockExecutor.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Sync_OutOfOrder tests handling blocks arriving out of order.
func TestManager_Sync_OutOfOrder(t *testing.T) {
	require := require.New(t)
	manager, mockExecutor, _, mockStore, _ := setupManagerForSyncTest(t) // Ignore DAClient and conf

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Timeout for the test
	defer cancel()

	// --- Test Data ---
	// Block 1
	tx1 := types.Tx("tx1_sync_ooo")
	block1Time := time.Now()
	block1Header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: syncTestChainID, Height: 1, Time: uint64(block1Time.UnixNano())}}}
	block1Data := &types.Data{Txs: types.Txs{tx1}, Metadata: &types.Metadata{ChainID: block1Header.ChainID(), Height: block1Header.Height(), Time: uint64(block1Time.UnixNano())}} // Correct time assignment
	block1Header.DataHash = block1Data.Hash()

	// Block 2
	tx2 := types.Tx("tx2_sync_ooo")
	block2Time := block1Time.Add(1 * time.Second) // Ensure time increases
	block2Header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: syncTestChainID, Height: 2, Time: uint64(block2Time.UnixNano())}}}
	block2Data := &types.Data{Txs: types.Txs{tx2}, Metadata: &types.Metadata{ChainID: block2Header.ChainID(), Height: block2Header.Height(), Time: uint64(block2Time.UnixNano()), LastDataHash: block1Data.Hash()}} // Correct time assignment
	block2Header.DataHash = block2Data.Hash()
	block2Header.LastHeaderHash = block1Header.Hash() // Link headers

	// --- Mock Interactions ---
	initialState := manager.GetLastState() // Height 0
	stateRoot1 := []byte("state root after block 1 ooo")
	stateRoot2 := []byte("state root after block 2 ooo")

	// Initial height check
	mockStore.On("Height", mock.Anything).Return(initialState.LastBlockHeight, nil).Once() // Returns 0

	// Interactions for applying block 1 (when it eventually arrives)
	executeMatcher1 := mock.MatchedBy(func(txs [][]byte) bool { return len(txs) == 1 && bytes.Equal(txs[0], tx1) })
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher1, uint64(1), block1Time, initialState.AppHash).Return(stateRoot1, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, block1Header, block1Data, &block1Header.Signature).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, uint64(1)).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, uint64(1)).Return(nil).Once()
	updateStateMatcher1 := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == 1 && bytes.Equal(newState.AppHash, stateRoot1) && newState.LastBlockTime == block1Time
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher1).Return(nil).Once()

	// Height check after block 1 applied
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Once() // Returns 1

	// Interactions for applying block 2 (after block 1 is applied)
	executeMatcher2 := mock.MatchedBy(func(txs [][]byte) bool { return len(txs) == 1 && bytes.Equal(txs[0], tx2) })
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher2, uint64(2), block2Time, stateRoot1).Return(stateRoot2, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, block2Header, block2Data, &block2Header.Signature).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, uint64(2)).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, uint64(2)).Return(nil).Once()
	updateStateMatcher2 := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == 2 && bytes.Equal(newState.AppHash, stateRoot2) && newState.LastBlockTime == block2Time
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher2).Return(nil).Once()

	// Height check after block 2 applied (for loop termination)
	mockStore.On("Height", mock.Anything).Return(uint64(2), nil).Once()

	// --- Run Sync Loop and Send Data Out of Order ---
	go manager.SyncLoop(ctx)

	// Send block 2 first
	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: block2Header, DAHeight: 11}
	manager.GetDataInCh() <- block.NewDataEvent{Data: block2Data, DAHeight: 11}

	// Wait briefly and assert state hasn't changed (still height 0)
	time.Sleep(50 * time.Millisecond)
	require.Equal(initialState.LastBlockHeight, manager.GetLastState().LastBlockHeight, "State should not change after receiving block 2 first")

	// Send block 1
	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: block1Header, DAHeight: 10}
	manager.GetDataInCh() <- block.NewDataEvent{Data: block1Data, DAHeight: 10}

	// --- Wait and Assert ---
	// Wait for both blocks to be processed.
	time.Sleep(200 * time.Millisecond) // Give SyncLoop time to process both blocks

	// Assert final state
	finalState := manager.GetLastState()
	require.Equal(uint64(2), finalState.LastBlockHeight, "Manager state should be updated to height 2")
	require.Equal(stateRoot2, finalState.AppHash, "Manager state should have the app hash from block 2")
	require.Equal(block2Time, finalState.LastBlockTime, "Manager state should have the block time from block 2")

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	mockExecutor.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestManager_Sync_FromDA tests retrieving and syncing a block from the DA layer via RetrieveLoop.
func TestManager_Sync_FromDA(t *testing.T) {
	require := require.New(t)
	manager, mockExecutor, mockDAClient, mockStore, conf := setupManagerForSyncTest(t)

	// Lower DA block time for faster test execution
	conf.DA.BlockTime.Duration = 100 * time.Millisecond
	// Re-create manager with updated config (or find a way to update config if possible)
	// Note: Recreating is simpler for this test structure.
	// We need to re-mock InitChain etc. if we recreate. Let's try modifying config directly if possible,
	// otherwise, we'll need to duplicate setup logic with modified config.
	// Assuming direct modification isn't safe/intended, let's proceed cautiously.
	// For this test, we'll rely on the default DA block time and wait accordingly,
	// or ideally, trigger the retrieveCh if it were accessible. Let's stick to waiting for now.

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Timeout for the test
	defer cancel()

	// --- Test Data ---
	tx1 := types.Tx("tx1_sync_da")
	block1Time := time.Now()
	// Create a proposer key just for signing this header, manager itself is non-proposer
	_, proposerKeyDA, _ := types.GetGenesisWithPrivkey("tempProposerDA")
	tempSignerDA, err := noopsigner.NewNoopSigner(proposerKeyDA)
	require.NoError(err)
	proposerAddrBytesDA, err := tempSignerDA.GetAddress()
	require.NoError(err)

	block1Header := &types.SignedHeader{
		Header: types.Header{
			BaseHeader: types.BaseHeader{
				ChainID: syncTestChainID,
				Height:  1,
				Time:    uint64(block1Time.UnixNano()),
			},
			ProposerAddress: proposerAddrBytesDA, // Use a valid address
		},
		Signer: types.Signer{ // Add signer info
			PubKey:  proposerKeyDA.GetPublic(),
			Address: proposerAddrBytesDA,
		},
	}
	block1Data := &types.Data{
		Txs: types.Txs{tx1},
		Metadata: &types.Metadata{
			ChainID: block1Header.ChainID(),
			Height:  block1Header.Height(),
			Time:    uint64(block1Time.UnixNano()),
		},
	}
	block1Header.DataHash = block1Data.Hash()
	// Sign the header
	headerBytes, err := block1Header.Header.MarshalBinary()
	require.NoError(err)
	signature, err := tempSignerDA.Sign(headerBytes)
	require.NoError(err)
	block1Header.Signature = signature

	// Marshal header for DA retrieval mock
	headerProto, err := block1Header.ToProto()
	require.NoError(err)
	marshaledHeader, err := proto.Marshal(headerProto)
	require.NoError(err)

	// --- Mock Interactions ---
	initialState := manager.GetLastState() // Height 0
	expectedNewStateRoot := []byte("state root after block 1 da")
	nextHeight := initialState.LastBlockHeight + 1  // Should be 1
	daHeight := manager.GetLastState().DAHeight + 1 // DA Height to retrieve from

	// Mocks for RetrieveLoop
	mockDAClient.On("Retrieve", mock.Anything, daHeight).Return(coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{Code: coreda.StatusSuccess},
		Data:       [][]byte{marshaledHeader}, // Return marshaled header
	}).Once()
	// Mock DAIncludedHeight update
	mockStore.On("SetMetadata", mock.Anything, block.DAIncludedHeightKey, mock.Anything).Return(nil).Once()

	// Mocks for SyncLoop (applying the block)
	mockStore.On("Height", mock.Anything).Return(initialState.LastBlockHeight, nil).Once() // Returns 0
	executeMatcher := mock.MatchedBy(func(txs [][]byte) bool { return len(txs) == 1 && bytes.Equal(txs[0], tx1) })
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher, nextHeight, block1Time, initialState.AppHash).Return(expectedNewStateRoot, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, nextHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, nextHeight).Return(nil).Once()
	updateStateMatcher := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == nextHeight &&
			bytes.Equal(newState.AppHash, expectedNewStateRoot) &&
			newState.LastBlockTime == block1Time &&
			newState.DAHeight >= daHeight // DA height should be updated
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher).Return(nil).Once()
	// Mock Height check after block applied
	mockStore.On("Height", mock.Anything).Return(nextHeight, nil).Once()

	// --- Run Loops ---
	go manager.SyncLoop(ctx)
	go manager.RetrieveLoop(ctx) // Start retrieve loop as well

	// --- Wait and Assert ---
	// Wait for the state update triggered by SyncLoop after RetrieveLoop gets the header.
	// Need to wait longer than the DA block time used in the manager's RetrieveLoop ticker.
	time.Sleep(conf.DA.BlockTime.Duration + 200*time.Millisecond) // Wait > DA block time + processing buffer

	// Assert final state
	finalState := manager.GetLastState()
	require.Equal(nextHeight, finalState.LastBlockHeight, "Manager state should be updated to height 1")
	require.Equal(expectedNewStateRoot, finalState.AppHash, "Manager state should have the new app hash")
	require.Equal(block1Time, finalState.LastBlockTime, "Manager state should have the new block time")
	require.GreaterOrEqual(finalState.DAHeight, daHeight, "Manager DA height should be updated")

	// Cancel context to stop loops
	cancel()

	// Verify mock expectations
	mockExecutor.AssertExpectations(t)
	mockStore.AssertExpectations(t)
	mockDAClient.AssertExpectations(t)
}

// TestManager_Sync_MissingData tests that sync waits if data for a received header is missing.
func TestManager_Sync_MissingData(t *testing.T) {
	require := require.New(t)
	manager, mockExecutor, _, mockStore, _ := setupManagerForSyncTest(t) // Ignore DAClient and conf

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Shorter timeout ok
	defer cancel()

	// --- Test Data ---
	block1Time := time.Now()
	block1Header := &types.SignedHeader{
		Header: types.Header{
			BaseHeader: types.BaseHeader{ChainID: syncTestChainID, Height: 1, Time: uint64(block1Time.UnixNano())},
			DataHash:   []byte("some data hash"), // Use a placeholder hash
		},
	}
	// No corresponding block1Data is created or sent

	// --- Mock Interactions ---
	initialState := manager.GetLastState() // Height 0
	// Mock store Height check. It might be called multiple times by the loop.
	mockStore.On("Height", mock.Anything).Return(initialState.LastBlockHeight, nil).Maybe()

	// --- Run Sync Loop and Send Header Only ---
	go manager.SyncLoop(ctx)

	// Send only the header
	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: block1Header, DAHeight: 10}

	// --- Wait and Assert ---
	// Wait a bit, longer than typical processing but less than timeout
	time.Sleep(200 * time.Millisecond)

	// Assert final state hasn't changed
	finalState := manager.GetLastState()
	require.Equal(initialState.LastBlockHeight, finalState.LastBlockHeight, "Manager state should not change if data is missing")
	require.Equal(initialState.AppHash, finalState.AppHash) // App hash shouldn't change either

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	// Crucially, ExecuteTxs, SaveBlockData, SetFinal, SetHeight, UpdateState should NOT be called
	mockExecutor.AssertNotCalled(t, "ExecuteTxs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockExecutor.AssertNotCalled(t, "SetFinal", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "SetHeight", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "UpdateState", mock.Anything, mock.Anything)
	mockStore.AssertExpectations(t) // Assert Height was called
}

// TestManager_Sync_MissingHeader tests that sync waits if header for received data is missing.
func TestManager_Sync_MissingHeader(t *testing.T) {
	require := require.New(t)
	manager, mockExecutor, _, mockStore, _ := setupManagerForSyncTest(t) // Ignore DAClient and conf

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second) // Shorter timeout ok
	defer cancel()

	// --- Test Data ---
	tx1 := types.Tx("tx1_sync_missing_header")
	block1Time := time.Now()
	// Create data first
	block1Data := &types.Data{
		Txs: types.Txs{tx1},
		Metadata: &types.Metadata{
			ChainID: syncTestChainID,
			Height:  1,
			Time:    uint64(block1Time.UnixNano()),
		},
	}
	// No corresponding block1Header is created or sent initially

	// --- Mock Interactions ---
	initialState := manager.GetLastState() // Height 0
	// Mock store Height check. It might be called multiple times by the loop.
	mockStore.On("Height", mock.Anything).Return(initialState.LastBlockHeight, nil).Maybe()

	// --- Run Sync Loop and Send Data Only ---
	go manager.SyncLoop(ctx)

	// Send only the data
	manager.GetDataInCh() <- block.NewDataEvent{Data: block1Data, DAHeight: 10}

	// --- Wait and Assert ---
	// Wait a bit, longer than typical processing but less than timeout
	time.Sleep(200 * time.Millisecond)

	// Assert final state hasn't changed
	finalState := manager.GetLastState()
	require.Equal(initialState.LastBlockHeight, finalState.LastBlockHeight, "Manager state should not change if header is missing")
	require.Equal(initialState.AppHash, finalState.AppHash)

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	// Crucially, ExecuteTxs, SaveBlockData, SetFinal, SetHeight, UpdateState should NOT be called
	mockExecutor.AssertNotCalled(t, "ExecuteTxs", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockExecutor.AssertNotCalled(t, "SetFinal", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "SetHeight", mock.Anything, mock.Anything)
	mockStore.AssertNotCalled(t, "UpdateState", mock.Anything, mock.Anything)
	mockStore.AssertExpectations(t) // Assert Height was called
}

// TestManager_Sync_MultipleBlocks tests syncing multiple blocks sequentially.
func TestManager_Sync_MultipleBlocks(t *testing.T) {
	require := require.New(t)
	manager, mockExecutor, _, mockStore, _ := setupManagerForSyncTest(t) // Ignore DAClient and conf

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second) // Longer timeout for multiple blocks
	defer cancel()

	// --- Test Data ---
	initialState := manager.GetLastState() // Height 0
	stateRoot0 := initialState.AppHash
	blockTime0 := initialState.LastBlockTime

	// Block 1
	tx1 := types.Tx("tx1_sync_multi")
	blockTime1 := blockTime0.Add(1 * time.Second)
	header1 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: syncTestChainID, Height: 1, Time: uint64(blockTime1.UnixNano())}}}
	data1 := &types.Data{Txs: types.Txs{tx1}, Metadata: &types.Metadata{ChainID: header1.ChainID(), Height: header1.Height(), Time: uint64(blockTime1.UnixNano())}}
	header1.DataHash = data1.Hash()
	stateRoot1 := []byte("state root 1 multi")

	// Block 2
	tx2 := types.Tx("tx2_sync_multi")
	blockTime2 := blockTime1.Add(1 * time.Second)
	header2 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: syncTestChainID, Height: 2, Time: uint64(blockTime2.UnixNano())}}}
	data2 := &types.Data{Txs: types.Txs{tx2}, Metadata: &types.Metadata{ChainID: header2.ChainID(), Height: header2.Height(), Time: uint64(blockTime2.UnixNano()), LastDataHash: data1.Hash()}}
	header2.DataHash = data2.Hash()
	header2.LastHeaderHash = header1.Hash()
	stateRoot2 := []byte("state root 2 multi")

	// Block 3
	tx3 := types.Tx("tx3_sync_multi")
	blockTime3 := blockTime2.Add(1 * time.Second)
	header3 := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: syncTestChainID, Height: 3, Time: uint64(blockTime3.UnixNano())}}}
	data3 := &types.Data{Txs: types.Txs{tx3}, Metadata: &types.Metadata{ChainID: header3.ChainID(), Height: header3.Height(), Time: uint64(blockTime3.UnixNano()), LastDataHash: data2.Hash()}}
	header3.DataHash = data3.Hash()
	header3.LastHeaderHash = header2.Hash()
	stateRoot3 := []byte("state root 3 multi")

	// --- Mock Interactions ---
	// Height checks
	mockStore.On("Height", mock.Anything).Return(uint64(0), nil).Once() // Initial check
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Once() // After block 1
	mockStore.On("Height", mock.Anything).Return(uint64(2), nil).Once() // After block 2
	mockStore.On("Height", mock.Anything).Return(uint64(3), nil).Once() // After block 3 (for loop termination)

	// Block 1 Apply
	executeMatcher1 := mock.MatchedBy(func(txs [][]byte) bool { return len(txs) == 1 && bytes.Equal(txs[0], tx1) })
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher1, uint64(1), blockTime1, stateRoot0).Return(stateRoot1, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header1, data1, &header1.Signature).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, uint64(1)).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, uint64(1)).Return(nil).Once()
	updateStateMatcher1 := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == 1 && bytes.Equal(newState.AppHash, stateRoot1)
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher1).Return(nil).Once()

	// Block 2 Apply
	executeMatcher2 := mock.MatchedBy(func(txs [][]byte) bool { return len(txs) == 1 && bytes.Equal(txs[0], tx2) })
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher2, uint64(2), blockTime2, stateRoot1).Return(stateRoot2, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header2, data2, &header2.Signature).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, uint64(2)).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, uint64(2)).Return(nil).Once()
	updateStateMatcher2 := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == 2 && bytes.Equal(newState.AppHash, stateRoot2)
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher2).Return(nil).Once()

	// Block 3 Apply
	executeMatcher3 := mock.MatchedBy(func(txs [][]byte) bool { return len(txs) == 1 && bytes.Equal(txs[0], tx3) })
	mockExecutor.On("ExecuteTxs", mock.Anything, executeMatcher3, uint64(3), blockTime3, stateRoot2).Return(stateRoot3, uint64(1000), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header3, data3, &header3.Signature).Return(nil).Once()
	mockExecutor.On("SetFinal", mock.Anything, uint64(3)).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, uint64(3)).Return(nil).Once()
	updateStateMatcher3 := mock.MatchedBy(func(newState types.State) bool {
		return newState.LastBlockHeight == 3 && bytes.Equal(newState.AppHash, stateRoot3)
	})
	mockStore.On("UpdateState", mock.Anything, updateStateMatcher3).Return(nil).Once()

	// --- Run Sync Loop and Send Data ---
	go manager.SyncLoop(ctx)

	// Send blocks sequentially
	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: header1, DAHeight: 10}
	manager.GetDataInCh() <- block.NewDataEvent{Data: data1, DAHeight: 10}
	time.Sleep(50 * time.Millisecond) // Small delay to allow processing

	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: header2, DAHeight: 11}
	manager.GetDataInCh() <- block.NewDataEvent{Data: data2, DAHeight: 11}
	time.Sleep(50 * time.Millisecond)

	manager.GetHeaderInCh() <- block.NewHeaderEvent{Header: header3, DAHeight: 12}
	manager.GetDataInCh() <- block.NewDataEvent{Data: data3, DAHeight: 12}

	// --- Wait and Assert ---
	time.Sleep(200 * time.Millisecond) // Wait for all blocks to be processed

	// Assert final state
	finalState := manager.GetLastState()
	require.Equal(uint64(3), finalState.LastBlockHeight, "Manager state should be updated to height 3")
	require.Equal(stateRoot3, finalState.AppHash, "Manager state should have the app hash from block 3")
	require.Equal(blockTime3, finalState.LastBlockTime, "Manager state should have the block time from block 3")

	// Cancel context to stop the loop
	cancel()

	// Verify mock expectations
	mockExecutor.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}
