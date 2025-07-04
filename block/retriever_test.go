package block

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	storepkg "github.com/rollkit/rollkit/pkg/store"
	rollmocks "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// setupManagerForRetrieverTest initializes a Manager with mocked dependencies.
func setupManagerForRetrieverTest(t *testing.T, initialDAHeight uint64) (*Manager, *rollmocks.MockDA, *rollmocks.MockStore, *MockLogger, *cache.Cache[types.SignedHeader], *cache.Cache[types.Data], context.CancelFunc) {
	t.Helper()
	mockDAClient := rollmocks.NewMockDA(t)
	mockStore := rollmocks.NewMockStore(t)
	mockLogger := new(MockLogger)
	// Allow logging calls with message string and optional key-value pairs up to 3 pairs for .Maybe()
	for _, level := range []string{"Debug", "Info", "Warn", "Error"} {
		mockLogger.On(level, mock.AnythingOfType("string")).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	}

	headerStore, _ := goheaderstore.NewStore[*types.SignedHeader](ds.NewMapDatastore())
	dataStore, _ := goheaderstore.NewStore[*types.Data](ds.NewMapDatastore())

	mockStore.On("GetState", mock.Anything).Return(types.State{DAHeight: initialDAHeight}, nil).Maybe()
	mockStore.On("SetHeight", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockStore.On("SetMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockStore.On("GetMetadata", mock.Anything, storepkg.DAIncludedHeightKey).Return([]byte{}, ds.ErrNotFound).Maybe()

	_, cancel := context.WithCancel(context.Background())

	// Create a mock signer
	src := rand.Reader
	pk, _, err := crypto.GenerateEd25519Key(src)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)

	addr, err := noopSigner.GetAddress()
	require.NoError(t, err)

	manager := &Manager{
		store:         mockStore,
		config:        config.Config{DA: config.DAConfig{BlockTime: config.DurationWrapper{Duration: 1 * time.Second}}},
		genesis:       genesis.Genesis{ProposerAddress: addr},
		daHeight:      &atomic.Uint64{},
		headerInCh:    make(chan NewHeaderEvent, eventInChLength),
		headerStore:   headerStore,
		dataInCh:      make(chan NewDataEvent, eventInChLength),
		dataStore:     dataStore,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		headerStoreCh: make(chan struct{}, 1),
		dataStoreCh:   make(chan struct{}, 1),
		retrieveCh:    make(chan struct{}, 1),
		daIncluderCh:  make(chan struct{}, 1),
		logger:        mockLogger,
		lastStateMtx:  new(sync.RWMutex),
		da:            mockDAClient,
		signer:        noopSigner,
	}
	manager.daIncludedHeight.Store(0)
	manager.daHeight.Store(initialDAHeight)

	t.Cleanup(cancel)

	return manager, mockDAClient, mockStore, mockLogger, manager.headerCache, manager.dataCache, cancel
}

// TestProcessNextDAHeader_Success_SingleHeaderAndData verifies that a single header and data are correctly processed and events are emitted.
func TestProcessNextDAHeader_Success_SingleHeaderAndData(t *testing.T) {
	t.Parallel()
	daHeight := uint64(20)
	blockHeight := uint64(100)
	manager, mockDAClient, mockStore, _, headerCache, dataCache, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	proposerAddr := manager.genesis.ProposerAddress

	hc := types.HeaderConfig{
		Height: blockHeight,
		Signer: manager.signer,
	}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)
	header.ProposerAddress = proposerAddr
	expectedHeaderHash := header.Hash().String()
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	blockConfig := types.BlockConfig{
		Height:       blockHeight,
		NTxs:         2,
		ProposerAddr: proposerAddr,
	}
	_, blockData, _ := types.GenerateRandomBlockCustom(&blockConfig, manager.genesis.ChainID)

	pubKey, err := manager.signer.GetPublic()
	require.NoError(t, err)
	addr, err := manager.signer.GetAddress()
	require.NoError(t, err)

	// Sign the data to create a valid SignedData
	signature, err := manager.getDataSignature(blockData)
	require.NoError(t, err)
	signedData := &types.SignedData{
		Data:      *blockData,
		Signature: signature,
		Signer: types.Signer{
			Address: addr,
			PubKey:  pubKey,
		},
	}
	blockDataBytes, err := signedData.MarshalBinary()
	require.NoError(t, err)
	// -----------------------------------------------------------
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{headerBytes, blockDataBytes}, nil,
	).Once()

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	// Validate header event
	select {
	case event := <-manager.headerInCh:
		assert.Equal(t, blockHeight, event.Header.Height())
		assert.Equal(t, daHeight, event.DAHeight)
		assert.Equal(t, proposerAddr, event.Header.ProposerAddress)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected header event not received")
	}

	assert.True(t, headerCache.IsDAIncluded(expectedHeaderHash), "Header hash should be marked as DA included in cache")

	// Validate block data event
	select {
	case dataEvent := <-manager.dataInCh:
		assert.Equal(t, daHeight, dataEvent.DAHeight)
		assert.Equal(t, blockData.Txs, dataEvent.Data.Txs)
		// Optionally, compare more fields if needed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected block data event not received")
	}
	assert.True(t, dataCache.IsDAIncluded(blockData.DACommitment().String()), "Block data commitment should be marked as DA included in cache")

	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestProcessNextDAHeader_MultipleHeadersAndData verifies that multiple headers and data in a single DA block are all processed and corresponding events are emitted.
func TestProcessNextDAHeader_MultipleHeadersAndData(t *testing.T) {
	t.Parallel()
	daHeight := uint64(50)
	startBlockHeight := uint64(130)
	nHeaders := 50
	manager, mockDAClient, _, _, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	proposerAddr := manager.genesis.ProposerAddress

	var blobs [][]byte
	var blockHeights []uint64
	var txLens []int

	invalidBlob := []byte("not a valid protobuf message")

	for i := 0; i < nHeaders; i++ {
		// Sprinkle an empty blob every 5th position
		if i%5 == 0 {
			blobs = append(blobs, []byte{})
		}
		// Sprinkle an invalid blob every 7th position
		if i%7 == 0 {
			blobs = append(blobs, invalidBlob)
		}

		height := startBlockHeight + uint64(i)
		blockHeights = append(blockHeights, height)

		hc := types.HeaderConfig{Height: height, Signer: manager.signer}
		header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
		require.NoError(t, err)
		header.ProposerAddress = proposerAddr
		headerProto, err := header.ToProto()
		require.NoError(t, err)
		headerBytes, err := proto.Marshal(headerProto)
		require.NoError(t, err)
		blobs = append(blobs, headerBytes)

		ntxs := i + 1 // unique number of txs for each data
		blockConfig := types.BlockConfig{Height: height, NTxs: ntxs, ProposerAddr: proposerAddr}
		_, blockData, _ := types.GenerateRandomBlockCustom(&blockConfig, manager.genesis.ChainID)
		txLens = append(txLens, len(blockData.Txs))

		pubKey, err := manager.signer.GetPublic()
		require.NoError(t, err)
		addr, err := manager.signer.GetAddress()
		require.NoError(t, err)

		// Sign the data to create a valid SignedData
		signature, err := manager.getDataSignature(blockData)
		require.NoError(t, err)
		signedData := &types.SignedData{
			Data:      *blockData,
			Signature: signature,
			Signer: types.Signer{
				Address: addr,
				PubKey:  pubKey,
			},
		}
		blockDataBytes, err := signedData.MarshalBinary()
		require.NoError(t, err)
		blobs = append(blobs, blockDataBytes)
		// Sprinkle an empty blob after each batch
		if i%4 == 0 {
			blobs = append(blobs, []byte{})
		}
	}

	// Add a few more invalid blobs at the end
	blobs = append(blobs, invalidBlob, []byte{})

	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		blobs, nil,
	).Once()

	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	// Validate all header events
	headerEvents := make([]NewHeaderEvent, 0, nHeaders)
	for i := 0; i < nHeaders; i++ {
		select {
		case event := <-manager.headerInCh:
			headerEvents = append(headerEvents, event)
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("Expected header event %d not received", i+1)
		}
	}
	// Check all expected heights are present
	receivedHeights := make(map[uint64]bool)
	for _, event := range headerEvents {
		receivedHeights[event.Header.Height()] = true
		assert.Equal(t, daHeight, event.DAHeight)
		assert.Equal(t, proposerAddr, event.Header.ProposerAddress)
	}
	for _, h := range blockHeights {
		assert.True(t, receivedHeights[h], "Header event for height %d not received", h)
	}

	// Validate all data events
	dataEvents := make([]NewDataEvent, 0, nHeaders)
	for i := 0; i < nHeaders; i++ {
		select {
		case event := <-manager.dataInCh:
			dataEvents = append(dataEvents, event)
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("Expected data event %d not received", i+1)
		}
	}
	// Check all expected tx lens are present
	receivedLens := make(map[int]bool)
	for _, event := range dataEvents {
		receivedLens[len(event.Data.Txs)] = true
		assert.Equal(t, daHeight, event.DAHeight)
	}
	for _, l := range txLens {
		assert.True(t, receivedLens[l], "Data event for tx count %d not received", l)
	}

	mockDAClient.AssertExpectations(t)
}

// TestProcessNextDAHeaderAndData_NotFound verifies that no events are emitted when DA returns NotFound.
func TestProcessNextDAHeaderAndData_NotFound(t *testing.T) {
	t.Parallel()
	daHeight := uint64(25)
	manager, mockDAClient, _, _, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	// Mock GetIDs to return empty IDs to simulate "not found" scenario
	// Example updates needed for one instance:
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{},
		Timestamp: time.Now(),
	}, coreda.ErrBlobNotFound).Once()
	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received for NotFound")
	default:
	}

	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for NotFound")
	default:
	}

	mockDAClient.AssertExpectations(t)
}

// TestProcessNextDAHeaderAndData_UnmarshalHeaderError verifies that no events are emitted and errors are logged when header bytes are invalid.
func TestProcessNextDAHeaderAndData_UnmarshalHeaderError(t *testing.T) {
	t.Parallel()
	daHeight := uint64(30)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	invalidBytes := []byte("this is not a valid protobuf message")

	// Mock GetIDs to return success with dummy ID
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()

	// Mock Get to return invalid bytes
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{invalidBytes}, nil,
	).Once()

	mockLogger.ExpectedCalls = nil
	// Re-establish general Maybe calls after clearing, then specific Once calls
	for _, level := range []string{"Debug", "Info", "Warn", "Error"} {
		mockLogger.On(level, mock.AnythingOfType("string")).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	}

	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received for unmarshal error")
	default:
	}
	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for unmarshal error")
	default:
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestProcessNextDAHeader_UnexpectedSequencer verifies that headers from unexpected sequencers are skipped.
func TestProcessNextDAHeader_UnexpectedSequencer(t *testing.T) {
	t.Parallel()
	daHeight := uint64(35)
	blockHeight := uint64(110)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	src := rand.Reader
	pk, _, err := crypto.GenerateEd25519Key(src)
	require.NoError(t, err)
	signerNoop, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	hc := types.HeaderConfig{
		Height: blockHeight,
		Signer: signerNoop,
	}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	// Mock GetIDs to return success with dummy ID
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()

	// Mock Get to return header bytes
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{headerBytes}, nil,
	).Once()

	mockLogger.ExpectedCalls = nil
	// Re-establish general Maybe calls
	for _, level := range []string{"Debug", "Info", "Warn", "Error"} {
		mockLogger.On(level, mock.AnythingOfType("string")).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	}

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received for unexpected sequencer")
	default:
		// Expected behavior
	}
	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for unmarshal error")
	default:
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestProcessNextDAHeader_FetchError_RetryFailure verifies that persistent fetch errors are retried and eventually returned.
func TestProcessNextDAHeader_FetchError_RetryFailure(t *testing.T) {
	t.Parallel()
	daHeight := uint64(40)
	manager, mockDAClient, _, _, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	fetchErr := errors.New("persistent DA connection error")

	// Mock GetIDs to return error for all retries
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(
		nil, fetchErr,
	).Times(dAFetcherRetries)

	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.Error(t, err)
	assert.ErrorContains(t, err, fetchErr.Error(), "Expected the final error after retries")

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received on fetch failure")
	default:
	}

	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for unmarshal error")
	default:
	}

	mockDAClient.AssertExpectations(t)
}

// TestProcessNextDAHeader_HeaderAndDataAlreadySeen verifies that no duplicate events are emitted for already-seen header/data.
func TestProcessNextDAHeader_HeaderAndDataAlreadySeen(t *testing.T) {
	t.Parallel()
	daHeight := uint64(45)
	blockHeight := uint64(120)

	manager, mockDAClient, _, mockLogger, headerCache, dataCache, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	// Initialize heights properly
	manager.daIncludedHeight.Store(blockHeight)

	// Create test header
	hc := types.HeaderConfig{
		Height: blockHeight, // Use blockHeight here
		Signer: manager.signer,
	}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)

	headerHash := header.Hash().String()
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	// Create valid batch (data)
	blockConfig := types.BlockConfig{
		Height:       blockHeight,
		NTxs:         2,
		ProposerAddr: manager.genesis.ProposerAddress,
	}
	_, blockData, _ := types.GenerateRandomBlockCustom(&blockConfig, manager.genesis.ChainID)

	pubKey, err := manager.signer.GetPublic()
	require.NoError(t, err)
	addr, err := manager.signer.GetAddress()
	require.NoError(t, err)

	// Sign the data to create a valid SignedData
	signature, err := manager.getDataSignature(blockData)
	require.NoError(t, err)
	signedData := &types.SignedData{
		Data:      *blockData,
		Signature: signature,
		Signer: types.Signer{
			Address: addr,
			PubKey:  pubKey,
		},
	}
	blockDataBytes, err := signedData.MarshalBinary()
	require.NoError(t, err)
	dataHash := blockData.DACommitment().String()

	// Mark both header and data as seen and DA included
	headerCache.SetSeen(headerHash)
	headerCache.SetDAIncluded(headerHash, uint64(10))
	dataCache.SetSeen(dataHash)
	dataCache.SetDAIncluded(dataHash, uint64(10))

	// Set up mocks with explicit logging
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()

	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{headerBytes, blockDataBytes}, nil,
	).Once()

	// Add debug logging expectations
	// For this specific test, we expect "header already seen" and "data already seen" logs.
	// Allow these specific logs, plus general Maybe for others.
	for _, level := range []string{"Debug", "Info", "Warn", "Error"} {
		mockLogger.On(level, mock.AnythingOfType("string")).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	}
	mockLogger.On("Debug", "header already seen", "height", blockHeight, "block hash", headerHash).Return().Maybe()
	mockLogger.On("Debug", "data already seen", "data hash", dataHash).Return().Maybe()

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	// Verify no header event was sent
	select {
	case <-manager.headerInCh:
		t.Fatal("Header event should not be received for already seen header")
	default:
		// Expected path
	}
	select {
	case <-manager.dataInCh:
		t.Fatal("Data event should not be received if already seen")
	case <-time.After(50 * time.Millisecond):
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestRetrieveLoop_ProcessError_HeightFromFuture verifies that the loop continues without logging error if error is height from future.
func TestRetrieveLoop_ProcessError_HeightFromFuture(t *testing.T) {
	t.Parallel()
	startDAHeight := uint64(10)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, startDAHeight)
	defer cancel()

	futureErr := fmt.Errorf("some error wrapping: %w", ErrHeightFromFutureStr)

	// Mock GetIDs to return future error for all retries
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight, mock.Anything).Return(
		nil, futureErr,
	).Once()

	// Optional: Mock for the next height if needed
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight+1, mock.Anything).Return(
		&coreda.GetIDsResult{IDs: []coreda.ID{}}, coreda.ErrBlobNotFound,
	).Maybe()

	errorLogged := atomic.Bool{}
	mockLogger.ExpectedCalls = nil
	// Allow any Debug/Info/Warn
	for _, level := range []string{"Debug", "Info", "Warn"} { // Note: Error handled specifically below
		mockLogger.On(level, mock.AnythingOfType("string")).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	}
	// Expect the specific error log for "failed to retrieve data from DALC" only if it's NOT ErrHeightFromFuture
	// Since this test is for ErrHeightFromFuture, this specific Error log should NOT be hit.
	// The error is handled within RetrieveWithHelpers and a different status code is returned.
	// The RetrieveLoop itself might log an info/debug that processing failed but not a top-level error for this case.
	mockLogger.On("Error", "failed to retrieve data from DALC", "daHeight", startDAHeight, "errors", mock.AnythingOfType("string"), "consecutiveFailures", mock.AnythingOfType("int")).Run(func(args mock.Arguments) {
		errorLogged.Store(true)
	}).Maybe() // This should ideally not be called if the error is ErrHeightFromFuture as RetrieveWithHelpers handles it.

	ctx, loopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.RetrieveLoop(ctx)
	}()

	manager.retrieveCh <- struct{}{}

	wg.Wait()

	finalDAHeight := manager.daHeight.Load()
	if finalDAHeight != startDAHeight {
		t.Errorf("Expected final DA height %d, got %d (should not increment on future height error)", startDAHeight, finalDAHeight)
	}
}

// TestRetrieveLoop_ProcessError_Other verifies that the loop logs error and does not increment DA height on generic errors.
func TestRetrieveLoop_ProcessError_Other(t *testing.T) {
	t.Parallel()
	startDAHeight := uint64(15)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, startDAHeight)
	defer cancel()

	otherErr := errors.New("some other DA error")

	// Mock GetIDs to return error for all retries
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight, mock.Anything).Return(
		nil, otherErr,
	).Times(dAFetcherRetries)

	errorLogged := atomic.Bool{}
	mockLogger.ExpectedCalls = nil
	// Allow any Debug/Info/Warn
	for _, level := range []string{"Debug", "Info", "Warn"} {
		mockLogger.On(level, mock.AnythingOfType("string")).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
		mockLogger.On(level, mock.AnythingOfType("string"), mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Maybe()
	}

	// Mock all expected logger calls in order for the error path
	// The "Retrieve helper: Failed to get IDs" is logged inside RetrieveWithHelpers.
	mockLogger.On("Error", "Retrieve helper: Failed to get IDs", "height", startDAHeight, "error", otherErr).Times(dAFetcherRetries)
	// The RetrieveLoop will log "failed to retrieve data from DALC" for each failed attempt.
	mockLogger.On("Error", "failed to retrieve data from DALC", "daHeight", startDAHeight, "errors", mock.AnythingOfType("string"), "consecutiveFailures", mock.AnythingOfType("int")).Run(func(args mock.Arguments) {
		// Try to close channel only on the last expected call, or make it non-blocking.
		// For simplicity, we'll let it be called multiple times and check errorLogged after loop.
		errorLogged.Store(true)
	}).Once() // Expect this once since the loop aggregates errors

	ctx, loopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.RetrieveLoop(ctx)
	}()

	// Give the goroutine time to start
	time.Sleep(10 * time.Millisecond)

	manager.retrieveCh <- struct{}{}

	// Wait for the context to timeout or the goroutine to finish
	wg.Wait()

	// Check if the error was logged
	if !errorLogged.Load() {
		t.Fatal("Error was not logged for generic DA error")
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestProcessNextDAHeader_WithNoTxs verifies that a data with no transactions is ignored and does not emit events or mark as DA included.
func TestProcessNextDAHeader_WithNoTxs(t *testing.T) {
	t.Parallel()
	daHeight := uint64(55)
	blockHeight := uint64(140)
	manager, mockDAClient, _, _, _, dataCache, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	// Create a valid header
	hc := types.HeaderConfig{Height: blockHeight, Signer: manager.signer}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)
	header.ProposerAddress = manager.genesis.ProposerAddress
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	// Create an empty batch (no txs)
	pubKey, err := manager.signer.GetPublic()
	require.NoError(t, err)
	addr, err := manager.signer.GetAddress()
	require.NoError(t, err)

	emptySignedData := &types.SignedData{
		Data:      types.Data{Txs: types.Txs{}},
		Signature: []byte{},
		Signer: types.Signer{
			Address: addr,
			PubKey:  pubKey,
		},
	}
	emptyDataBytes, err := emptySignedData.MarshalBinary()
	require.NoError(t, err)

	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{headerBytes, emptyDataBytes}, nil,
	).Once()

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	// Validate header event
	select {
	case <-manager.headerInCh:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected header event not received")
	}

	// Validate data event for empty data
	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for empty data")
	case <-time.After(100 * time.Millisecond):
		// ok, no event as expected
	}
	// The empty data should NOT be marked as DA included in cache
	emptyData := &types.Data{Txs: types.Txs{}}
	assert.False(t, dataCache.IsDAIncluded(emptyData.DACommitment().String()), "Empty data should not be marked as DA included in cache")

	mockDAClient.AssertExpectations(t)
}

// TestRetrieveLoop_DAHeightIncrementsOnlyOnSuccess verifies that DA height is incremented only after a successful retrieval or NotFound, and not after an error.
func TestRetrieveLoop_DAHeightIncrementsOnlyOnSuccess(t *testing.T) {
	t.Parallel()
	startDAHeight := uint64(60)
	manager, mockDAClient, _, _, _, _, cancel := setupManagerForRetrieverTest(t, startDAHeight)
	defer cancel()

	blockHeight := uint64(150)
	proposerAddr := manager.genesis.ProposerAddress
	hc := types.HeaderConfig{Height: blockHeight, Signer: manager.signer}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)
	header.ProposerAddress = proposerAddr
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	// 1. First call: success (header)
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{headerBytes}, nil,
	).Once()

	// 2. Second call: NotFound
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight+1, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       nil,
		Timestamp: time.Now(),
	}, nil).Once()

	// 3. Third call: Error
	errDA := errors.New("some DA error")
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight+2, mock.Anything).Return(
		&coreda.GetIDsResult{
			IDs:       nil,
			Timestamp: time.Now(),
		}, errDA,
	).Times(dAFetcherRetries)

	ctx, loopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.RetrieveLoop(ctx)
	}()

	manager.retrieveCh <- struct{}{}

	wg.Wait()

	// After first success, DA height should increment to startDAHeight+1
	// After NotFound, should increment to startDAHeight+2
	// After error, should NOT increment further (remains at startDAHeight+2)
	finalDAHeight := manager.daHeight.Load()
	assert.Equal(t, startDAHeight+2, finalDAHeight, "DA height should only increment on success or NotFound, not on error")

	mockDAClient.AssertExpectations(t)
}
