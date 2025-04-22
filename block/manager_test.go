package block

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// mockSequencer is a local mock implementation for coresequencer.Sequencer
type mockSequencer struct {
	mock.Mock
}

func (m *mockSequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*coresequencer.GetNextBatchResponse), args.Error(1)
}

func (m *mockSequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*coresequencer.SubmitRollupBatchTxsResponse), args.Error(1)
}

func (m *mockSequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*coresequencer.VerifyBatchResponse), args.Error(1)
}

// WithinDuration asserts that the two durations are within the specified tolerance of each other.
func WithinDuration(t *testing.T, expected, actual, tolerance time.Duration) bool {
	diff := expected - actual
	if diff < 0 {
		diff = -diff
	}
	if diff <= tolerance {
		return true
	}
	return assert.Fail(t, fmt.Sprintf("Not within duration.\nExpected: %v\nActual: %v\nTolerance: %v", expected, actual, tolerance))
}

// Returns a minimalistic block manager
func getManager(t *testing.T, backend coreda.DA, gasPrice float64, gasMultiplier float64) *Manager {
	logger := log.NewTestLogger(t)
	return &Manager{
		dalc:          coreda.NewDummyClient(backend, []byte("test")),
		headerCache:   cache.NewCache[types.SignedHeader](),
		logger:        logger,
		gasPrice:      gasPrice,
		gasMultiplier: gasMultiplier,
		lastStateMtx:  &sync.RWMutex{},
		metrics:       NopMetrics(),
	}
}

func TestInitialStateClean(t *testing.T) {
	require := require.New(t)

	// Create genesis document
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateClean")
	logger := log.NewTestLogger(t)
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(es)
	s, err := getInitialState(context.TODO(), genesisData, nil, emptyStore, coreexecutor.NewDummyExecutor(), logger)
	require.NoError(err)
	initialHeight := genesisData.InitialHeight
	require.Equal(initialHeight-1, s.LastBlockHeight)
	require.Equal(initialHeight, s.InitialHeight)
}

func TestInitialStateStored(t *testing.T) {
	require := require.New(t)

	// Create genesis document
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateStored")
	sampleState := types.State{
		ChainID:         "TestInitialStateStored",
		InitialHeight:   1,
		LastBlockHeight: 100,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	logger := log.NewTestLogger(t)
	s, err := getInitialState(context.TODO(), genesisData, nil, store, coreexecutor.NewDummyExecutor(), logger)
	require.NoError(err)
	require.Equal(s.LastBlockHeight, uint64(100))
	require.Equal(s.InitialHeight, uint64(1))
}

func TestHandleEmptyDataHash(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Mock store and data cache
	store := mocks.NewStore(t)
	dataCache := cache.NewCache[types.Data]()

	// Setup the manager with the mock and data cache
	m := &Manager{
		store:     store,
		dataCache: dataCache,
	}

	// Define the test data
	headerHeight := 2
	header := &types.Header{
		DataHash: dataHashForEmptyTxs,
		BaseHeader: types.BaseHeader{
			Height: 2,
			Time:   uint64(time.Now().UnixNano()),
		},
	}

	// Mock data for the previous block
	lastData := &types.Data{}
	lastDataHash := lastData.Hash()

	// header.DataHash equals dataHashForEmptyTxs and no error occurs
	store.On("GetBlockData", ctx, uint64(headerHeight-1)).Return(nil, lastData, nil)

	// Execute the method under test
	m.handleEmptyDataHash(ctx, header)

	// Assertions
	store.AssertExpectations(t)

	// make sure that the store has the correct data
	d := dataCache.GetItem(header.Height())
	require.NotNil(d)
	require.Equal(d.LastDataHash, lastDataHash)
	require.Equal(d.Metadata.ChainID, header.ChainID())
	require.Equal(d.Metadata.Height, header.Height())
	require.Equal(d.Metadata.Time, header.BaseHeader.Time)
}

func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	logger := log.NewTestLogger(t)

	// Create genesis document with initial height 2
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateUnexpectedHigherGenesis")
	// Create a new genesis with height 2
	genesis := genesispkg.NewGenesis(
		genesisData.ChainID,
		uint64(2), // Set initial height to 2
		genesisData.GenesisDAStartHeight,
		genesisData.ProposerAddress,
	)
	sampleState := types.State{
		ChainID:         "TestInitialStateUnexpectedHigherGenesis",
		InitialHeight:   1,
		LastBlockHeight: 0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	_, err = getInitialState(context.TODO(), genesis, nil, store, coreexecutor.NewDummyExecutor(), logger)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")
}

func TestSignVerifySignature(t *testing.T) {
	require := require.New(t)
	m := getManager(t, coreda.NewDummyDA(100_000, 0, 0), -1, -1)
	payload := []byte("test")
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	noopSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)
	cases := []struct {
		name   string
		signer signer.Signer
	}{
		{"ed25519", noopSigner},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m.proposerKey = c.signer
			signature, err := m.proposerKey.Sign(payload)
			require.NoError(err)
			pubKey, err := c.signer.GetPublic()
			require.NoError(err)
			ok, err := pubKey.Verify(payload, signature)
			require.NoError(err)
			require.True(ok)
		})
	}
}

func TestIsDAIncluded(t *testing.T) {
	require := require.New(t)

	// Create a minimalistic block manager
	m := &Manager{
		headerCache: cache.NewCache[types.SignedHeader](),
	}
	hash := types.Hash([]byte("hash"))

	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(hash))

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.headerCache.SetDAIncluded(hash.String())
	require.True(m.IsDAIncluded(hash))
}

func TestSubmitBlocksToMockDA(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name              string
		gasPrice          float64
		gasMultiplier     float64
		expectedGasPrices []float64
		isErrExpected     bool
	}{
		{"defaults", -1, -1, []float64{
			-1, -1, -1,
		}, false},
		{"fixed_gas_price", 1.0, -1, []float64{
			1.0, 1.0, 1.0,
		}, false},
		{"default_gas_price_with_multiplier", -1, 1.2, []float64{
			-1, -1, -1,
		}, false},
		// {"fixed_gas_price_with_multiplier", 1.0, 1.2, []float64{
		// 	1.0, 1.2, 1.2 * 1.2,
		// }, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDA := &mocks.DA{}
			m := getManager(t, mockDA, tc.gasPrice, tc.gasMultiplier)
			m.config.DA.BlockTime.Duration = time.Millisecond
			m.config.DA.MempoolTTL = 1
			kvStore, err := store.NewDefaultInMemoryKVStore()
			require.NoError(t, err)
			m.store = store.New(kvStore)

			var blobs [][]byte
			header, data := types.GetRandomBlock(1, 5, "TestSubmitBlocksToMockDA")
			blob, err := header.MarshalBinary()
			require.NoError(t, err)

			err = m.store.SaveBlockData(ctx, header, data, &types.Signature{})
			require.NoError(t, err)
			err = m.store.SetHeight(ctx, 1)
			require.NoError(t, err)

			blobs = append(blobs, blob)
			// Set up the mock to
			// * throw timeout waiting for tx to be included exactly twice
			// * wait for tx to drop from mempool exactly DABlockTime * DAMempoolTTL seconds
			// * retry with a higher gas price
			// * successfully submit
			mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(12345), nil)
			mockDA.
				On("SubmitWithOptions", mock.Anything, blobs, tc.expectedGasPrices[0], []byte(nil), []byte(nil)).
				Return([][]byte{}, coreda.ErrTxTimedOut).Once()
			mockDA.
				On("SubmitWithOptions", mock.Anything, blobs, tc.expectedGasPrices[1], []byte(nil), []byte(nil)).
				Return([][]byte{}, coreda.ErrTxTimedOut).Once()
			mockDA.
				On("SubmitWithOptions", mock.Anything, blobs, tc.expectedGasPrices[2], []byte(nil), []byte(nil)).
				Return([][]byte{bytes.Repeat([]byte{0x00}, 8)}, nil)

			m.pendingHeaders, err = NewPendingHeaders(m.store, m.logger)
			require.NoError(t, err)
			err = m.submitHeadersToDA(ctx)
			require.NoError(t, err)
			mockDA.AssertExpectations(t)
		})
	}
}

// Test_submitBlocksToDA_BlockMarshalErrorCase1: A itself has a marshalling error. So A, B and C never get submitted.
func Test_submitBlocksToDA_BlockMarshalErrorCase1(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase1"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	m := getManager(t, coreda.NewDummyDA(100_000, 0, 0), -1, -1)

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewStore(t)
	invalidateBlockHeader(header1)
	store.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height", mock.Anything).Return(uint64(3), nil)

	m.store = store

	var err error
	m.pendingHeaders, err = NewPendingHeaders(store, m.logger)
	require.NoError(err)

	err = m.submitHeadersToDA(ctx)
	assert.ErrorContains(err, "failed to transform header to proto")
	blocks, err := m.pendingHeaders.getPendingHeaders(ctx)
	assert.NoError(err)
	assert.Equal(3, len(blocks))
}

// Test_submitBlocksToDA_BlockMarshalErrorCase2: A and B are fair blocks, but C has a marshalling error
// - Block A and B get submitted to DA layer not block C
func Test_submitBlocksToDA_BlockMarshalErrorCase2(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase2"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	m := getManager(t, coreda.NewDummyDA(100_000, 0, 0), -1, -1)

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewStore(t)
	invalidateBlockHeader(header3)
	store.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height", mock.Anything).Return(uint64(3), nil)

	m.store = store

	var err error
	m.pendingHeaders, err = NewPendingHeaders(store, m.logger)
	require.NoError(err)
	err = m.submitHeadersToDA(ctx)
	assert.ErrorContains(err, "failed to transform header to proto")
	blocks, err := m.pendingHeaders.getPendingHeaders(ctx)
	assert.NoError(err)
	assert.Equal(3, len(blocks)) // we stop submitting all headers when there is a marshalling error
}

// invalidateBlockHeader results in a block header that produces a marshalling error
func invalidateBlockHeader(header *types.SignedHeader) {
	header.Signer.PubKey = &crypto.Ed25519PublicKey{}
}

func Test_isProposer(t *testing.T) {
	require := require.New(t)

	type args struct {
		state         types.State
		signerPrivKey signer.Signer
	}
	tests := []struct {
		name       string
		args       args
		isProposer bool
		err        error
	}{
		{
			name: "Signing key matches genesis proposer public key",
			args: func() args {
				genesisData, privKey, _ := types.GetGenesisWithPrivkey("Test_isProposer")
				s, err := types.NewFromGenesisDoc(genesisData)
				require.NoError(err)
				signer, err := noopsigner.NewNoopSigner(privKey)
				require.NoError(err)
				return args{
					s,
					signer,
				}
			}(),
			isProposer: true,
			err:        nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isProposer, err := isProposer(tt.args.signerPrivKey, tt.args.state)
			if !errors.Is(err, tt.err) {
				t.Errorf("isProposer() error = %v, expected err %v", err, tt.err)
				return
			}
			if isProposer != tt.isProposer {
				t.Errorf("isProposer() = %v, expected %v", isProposer, tt.isProposer)
			}
		})
	}
}

func Test_publishBlock_ManagerNotProposer(t *testing.T) {
	require := require.New(t)
	m := getManager(t, coreda.NewDummyDA(100_000, 0, 0), -1, -1)
	m.isProposer = false
	err := m.publishBlock(context.Background())
	require.ErrorIs(err, ErrNotProposer)
}

func Test_publishBlock_NoBatch(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Setup manager with mocks
	mockStore := mocks.NewStore(t)
	mockSeq := new(mockSequencer)
	mockExecutor := coreexecutor.NewDummyExecutor()
	logger := log.NewTestLogger(t)
	chainID := "Test_publishBlock_NoBatch"
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(chainID)
	noopSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	m := &Manager{
		store:       mockStore,
		sequencer:   mockSeq,
		exec:        mockExecutor,
		logger:      logger,
		isProposer:  true,
		proposerKey: noopSigner,
		genesis:     genesisData,
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
	// Use GetRandomBlock which returns header and data
	lastHeader, lastData := types.GetRandomBlock(currentHeight, 0, chainID)
	mockStore.On("GetBlockData", ctx, currentHeight).Return(lastHeader, lastData, nil)
	// Mock GetBlockData for newHeight to indicate no pending block exists
	mockStore.On("GetBlockData", ctx, currentHeight+1).Return(nil, nil, errors.New("not found"))

	// No need to mock GetTxs on the dummy executor if its default behavior is acceptable

	// Mock sequencer SubmitRollupBatchTxs (should still be called)
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID // Ensure correct rollup ID
	})
	mockSeq.On("SubmitRollupBatchTxs", ctx, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil)

	// *** Crucial Mock: Sequencer returns ErrNoBatch ***
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.RollupId) == chainID // Ensure correct rollup ID
	})
	mockSeq.On("GetNextBatch", ctx, batchReqMatcher).Return(nil, ErrNoBatch)

	// Call publishBlock
	err = m.publishBlock(ctx)

	// Assertions
	require.NoError(err, "publishBlock should return nil error when no batch is available")

	// Verify mocks: Ensure methods after the check were NOT called
	mockStore.AssertNotCalled(t, "SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	// We can't directly assert non-calls on m.createBlock or m.applyBlock easily without more complex setup,
	// but returning nil error and not calling SaveBlockData implies they weren't reached.
	mockSeq.AssertExpectations(t) // Ensure GetNextBatch was called
	mockStore.AssertExpectations(t)
}

func Test_publishBlock_EmptyBatch(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Setup manager with mocks
	mockStore := mocks.NewStore(t)
	mockSeq := new(mockSequencer) // Use the local mock struct
	mockExecutor := coreexecutor.NewDummyExecutor()
	logger := log.NewTestLogger(t)
	chainID := "Test_publishBlock_EmptyBatch"
	genesisData, privKey, _ := types.GetGenesisWithPrivkey(chainID)
	noopSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)

	m := &Manager{
		store:       mockStore,
		sequencer:   mockSeq, // Assign the local mock
		exec:        mockExecutor,
		logger:      logger,
		isProposer:  true,
		proposerKey: noopSigner,
		genesis:     genesisData,
		config: config.Config{
			Node: config.NodeConfig{
				MaxPendingBlocks: 0, // No limit
			},
		},
		pendingHeaders: &PendingHeaders{ // Basic pending headers mock
			store:  mockStore,
			logger: logger,
		},
		lastStateMtx: &sync.RWMutex{},
		metrics:      NopMetrics(),
	}

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
	// Use GetRandomBlock
	lastHeader, lastData := types.GetRandomBlock(currentHeight, 0, chainID)
	mockStore.On("GetBlockData", ctx, currentHeight).Return(lastHeader, lastData, nil)
	mockStore.On("GetBlockData", ctx, currentHeight+1).Return(nil, nil, errors.New("not found")) // No pending block

	// No need to mock GetTxs on the dummy executor

	// Mock sequencer SubmitRollupBatchTxs
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID
	})
	mockSeq.On("SubmitRollupBatchTxs", ctx, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil)

	// *** Crucial Mock: Sequencer returns an empty batch ***
	emptyBatchResponse := &coresequencer.GetNextBatchResponse{
		Batch: &coresequencer.Batch{
			Transactions: [][]byte{}, // Empty transactions
		},
		Timestamp: time.Now(),
		BatchData: [][]byte{[]byte("some_batch_data")}, // Sequencer might return metadata even if empty
	}
	batchReqMatcher := mock.MatchedBy(func(req coresequencer.GetNextBatchRequest) bool {
		return string(req.RollupId) == chainID
	})
	mockSeq.On("GetNextBatch", ctx, batchReqMatcher).Return(emptyBatchResponse, nil)
	// Mock store SetMetadata for the last batch data
	mockStore.On("SetMetadata", ctx, LastBatchDataKey, mock.Anything).Return(nil)

	// Call publishBlock
	err = m.publishBlock(ctx)

	// Assertions
	require.NoError(err, "publishBlock should return nil error when the batch is empty")

	// Verify mocks: Ensure methods after the check were NOT called
	mockStore.AssertNotCalled(t, "SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockSeq.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestAggregationLoop tests the AggregationLoop function
func TestAggregationLoop(t *testing.T) {
	mockStore := new(mocks.Store)
	mockLogger := log.NewTestLogger(t)

	m := &Manager{
		store:  mockStore,
		logger: mockLogger,
		genesis: genesispkg.NewGenesis(
			"myChain",
			1,
			time.Now(),
			[]byte{}, // Empty extra data
		),
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime:      config.DurationWrapper{Duration: time.Second},
				LazyAggregator: false,
			},
		},
	}

	mockStore.On("Height", mock.Anything).Return(uint64(0), nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go m.AggregationLoop(ctx)

	// Wait for the function to complete or timeout
	<-ctx.Done()

	mockStore.AssertExpectations(t)
}

// TestLazyAggregationLoop tests the lazyAggregationLoop function
// TODO: uncomment the test when the lazy aggregation is properly fixed
// func TestLazyAggregationLoop(t *testing.T) {
// 	t.Parallel()

// 	mockStore := mocks.NewStore(t)
// 	//mockLogger := log.NewTestLogger(t)

// 	// Set block time to 100ms and lazy block time to 1s
// 	blockTime := 100 * time.Millisecond
// 	lazyBlockTime := 1 * time.Second

// 	m := getManager(t, coreda.NewDummyDA(100_000, 0, 0), -1, -1)
// 	m.exec = coreexecutor.NewDummyExecutor()
// 	m.isProposer = true
// 	m.store = mockStore
// 	m.metrics = NopMetrics()
// 	m.HeaderCh = make(chan *types.SignedHeader, 10)
// 	m.DataCh = make(chan *types.Data, 10)
// 	m.config.Node.LazyAggregator = true
// 	m.config.Node.BlockTime.Duration = blockTime
// 	m.config.Node.LazyBlockTime.Duration = lazyBlockTime

// 	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
// 	require.NoError(t, err)
// 	noopSigner, err := noopsigner.NewNoopSigner(privKey)
// 	require.NoError(t, err)
// 	m.proposerKey = noopSigner
// 	mockSignature := types.Signature([]byte{1, 2, 3})

// 	// Mock store expectations
// 	mockStore.On("Height", mock.Anything).Return(uint64(0), nil)
// 	mockStore.On("SetHeight", mock.Anything, mock.Anything).Return(nil)
// 	mockStore.On("UpdateState", mock.Anything, mock.Anything, mock.Anything).Return(nil)
// 	mockStore.On("GetBlockData", mock.Anything, mock.Anything).Return(&types.SignedHeader{}, &types.Data{}, nil)
// 	mockStore.On("GetSignature", mock.Anything, mock.Anything).Return(&mockSignature, nil)

// 	// Create a context with a timeout longer than the test duration
// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 	defer cancel()

// 	// Create a channel to track block production
// 	blockProduced := make(chan struct{}, 10)
// 	mockStore.On("SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Run(func(args mock.Arguments) {
// 		blockProduced <- struct{}{}
// 	})

// 	blockTimer := time.NewTimer(blockTime)
// 	defer blockTimer.Stop()

// 	// Start the lazy aggregation loop
// 	start := time.Now()
// 	go m.lazyAggregationLoop(ctx, blockTimer)

// 	// Wait for the first block to be produced (should happen after blockTime)
// 	select {
// 	case <-blockProduced:
// 		// First block produced as expected
// 	case <-time.After(2 * blockTime):
// 		require.Fail(t, "First block not produced within expected time")
// 	}

// 	// Wait for the second block (should happen after lazyBlockTime since no transactions)
// 	select {
// 	case <-blockProduced:
// 		// Second block produced as expected
// 	case <-time.After(lazyBlockTime + 100*time.Millisecond):
// 		require.Fail(t, "Second block not produced within expected time")
// 	}

// 	// Wait for the third block (should happen after lazyBlockTime since no transactions)
// 	select {
// 	case <-blockProduced:
// 		// Second block produced as expected
// 	case <-time.After(lazyBlockTime + 100*time.Millisecond):
// 		require.Fail(t, "Second block not produced within expected time")
// 	}
// 	end := time.Now()

// 	// Ensure that the duration between block productions adheres to the defined lazyBlockTime.
// 	// This check prevents blocks from being produced too fast, maintaining consistent timing.
// 	expectedDuration := 2*lazyBlockTime + blockTime
// 	expectedEnd := start.Add(expectedDuration)
// 	require.WithinDuration(t, expectedEnd, end, 3*blockTime)

// 	// Cancel the context to stop the loop
// 	cancel()

// 	// Verify mock expectations
// 	mockStore.AssertExpectations(t)
// }

// TestNormalAggregationLoop tests the normalAggregationLoop function
func TestNormalAggregationLoop(t *testing.T) {
	mockLogger := log.NewTestLogger(t)

	m := &Manager{
		logger: mockLogger,
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime:      config.DurationWrapper{Duration: 1 * time.Second},
				LazyAggregator: false,
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	blockTimer := time.NewTimer(m.config.Node.BlockTime.Duration)
	defer blockTimer.Stop()

	go m.normalAggregationLoop(ctx, blockTimer)

	// Wait for the function to complete or timeout
	<-ctx.Done()
}
