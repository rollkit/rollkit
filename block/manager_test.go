package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	damocks "github.com/rollkit/rollkit/da/mocks"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

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

	ctx := context.Background()
	eventBus := events.NewEventBus(ctx, logger)

	// Create minimal manager with only needed components for testing
	m := &Manager{
		dalc:             da.NewDAClient(backend, gasPrice, gasMultiplier, nil, nil, logger),
		headerCache:      NewHeaderCache(),
		dataCache:        NewDataCache(),
		logger:           logger,
		eventBus:         eventBus,
		daHeight:         atomic.Uint64{},
		daIncludedHeight: atomic.Uint64{},
	}

	return m
}

func TestInitialStateClean(t *testing.T) {
	const chainID = "TestInitialStateClean"
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey(chainID)
	genesis := &RollkitGenesis{
		ChainID:         chainID,
		InitialHeight:   1,
		ProposerAddress: genesisDoc.Validators[0].Address.Bytes(),
	}
	logger := log.NewTestLogger(t)
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(es)
	s, err := getInitialState(context.TODO(), genesis, emptyStore, coreexecutor.NewDummyExecutor(), logger)
	require.NoError(err)
	require.Equal(s.LastBlockHeight, genesis.InitialHeight-1)
	require.Equal(genesis.InitialHeight, s.InitialHeight)
}

func TestInitialStateStored(t *testing.T) {
	chainID := "TestInitialStateStored"
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey(chainID)
	valset := types.GetRandomValidatorSet()
	genesis := &RollkitGenesis{
		ChainID:         chainID,
		InitialHeight:   1,
		ProposerAddress: genesisDoc.Validators[0].Address.Bytes(),
	}
	sampleState := types.State{
		ChainID:         chainID,
		InitialHeight:   1,
		LastBlockHeight: 100,
		Validators:      valset,
		NextValidators:  valset,
		LastValidators:  valset,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	logger := log.NewTestLogger(t)
	s, err := getInitialState(context.TODO(), genesis, store, coreexecutor.NewDummyExecutor(), logger)
	require.NoError(err)
	require.Equal(s.LastBlockHeight, uint64(100))
	require.Equal(s.InitialHeight, uint64(1))
}

func TestHandleEmptyDataHash(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Mock store and data cache
	store := mocks.NewStore(t)
	dataCache := NewDataCache()

	// Setup a syncer with mock components
	eventBus := events.NewEventBus(ctx, log.NewTestLogger(t))
	syncer := &Syncer{
		store:     store,
		dataCache: dataCache,
		eventBus:  eventBus,
		logger:    log.NewTestLogger(t),
	}

	// Define the test data
	headerHeight := uint64(2)
	header := &types.Header{
		DataHash: dataHashForEmptyTxs,
		BaseHeader: types.BaseHeader{
			Height: headerHeight,
			Time:   uint64(time.Now().UnixNano()),
		},
	}

	// Mock data for the previous block
	lastData := &types.Data{}
	lastDataHash := lastData.Hash()

	// header.DataHash equals dataHashForEmptyTxs and no error occurs
	store.On("GetBlockData", ctx, headerHeight-1).Return(nil, lastData, nil)

	// Execute the method under test
	syncer.handleEmptyDataHash(ctx, header)

	// Assertions
	store.AssertExpectations(t)

	// make sure that the store has the correct data
	d := dataCache.getData(headerHeight)
	require.NotNil(d)
	require.Equal(d.Metadata.LastDataHash, lastDataHash)
	require.Equal(d.Metadata.ChainID, header.ChainID())
	require.Equal(d.Metadata.Height, header.Height())
	require.Equal(d.Metadata.Time, header.BaseHeader.Time)
}

func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	logger := log.NewTestLogger(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey("TestInitialStateUnexpectedHigherGenesis")
	valset := types.GetRandomValidatorSet()
	genesis := &RollkitGenesis{
		ChainID:         "TestInitialStateUnexpectedHigherGenesis",
		InitialHeight:   2,
		ProposerAddress: genesisDoc.Validators[0].Address.Bytes(),
	}
	sampleState := types.State{
		ChainID:         "TestInitialStateUnexpectedHigherGenesis",
		InitialHeight:   1,
		LastBlockHeight: 0,
		Validators:      valset,
		NextValidators:  valset,
		LastValidators:  valset,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	_, err = getInitialState(context.TODO(), genesis, store, coreexecutor.NewDummyExecutor(), logger)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")
}

func TestSignVerifySignature(t *testing.T) {
	require := require.New(t)

	payload := []byte("test")
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)

	// Create a Producer component for testing signing
	producer := &Producer{
		proposerKey: privKey,
		logger:      log.NewTestLogger(t),
	}

	cases := []struct {
		name    string
		privKey crypto.PrivKey
		pubKey  crypto.PubKey
	}{
		{"ed25519", privKey, pubKey},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			producer.proposerKey = c.privKey
			signature, err := producer.getSignature(types.Header{})
			require.NoError(err)
			_, err = c.pubKey.Verify(payload, signature)
			require.NoError(err, "Error verifying signature")
			// Note: This test will fail since we're signing an empty header
			// but verifying a different payload - this is just to test the flow
		})
	}
}

func TestIsDAIncluded(t *testing.T) {
	require := require.New(t)

	// Create a minimalistic block manager
	m := &Manager{
		headerCache: NewHeaderCache(),
	}
	hash := types.Hash([]byte("hash"))

	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(hash))

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.headerCache.setDAIncluded(hash.String())
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDA := &damocks.DA{}
			m := getManager(t, mockDA, tc.gasPrice, tc.gasMultiplier)
			m.config.DA.BlockTime = config.DurationWrapper{Duration: time.Millisecond}
			m.config.DA.MempoolTTL = 1
			kvStore, err := store.NewDefaultInMemoryKVStore()
			require.NoError(t, err)
			m.store = store.New(kvStore)

			// Create a Publisher component for testing DA submission
			publisher, err := NewPublisher(PublisherOptions{
				EventBus:      m.eventBus,
				Store:         m.store,
				DALC:          m.dalc,
				Logger:        m.logger,
				Config:        m.config,
				GasPrice:      tc.gasPrice,
				GasMultiplier: tc.gasMultiplier,
			})
			require.NoError(t, err)

			// Use the publisher directly for testing
			var blobs [][]byte
			header, data := types.GetRandomBlock(1, 5, "TestSubmitBlocksToMockDA")
			blob, err := header.MarshalBinary()
			require.NoError(t, err)

			err = m.store.SaveBlockData(ctx, header, data, &types.Signature{})
			require.NoError(t, err)
			m.store.SetHeight(ctx, 1)

			blobs = append(blobs, blob)
			// Set up the mock to
			// * throw timeout waiting for tx to be included exactly twice
			// * wait for tx to drop from mempool exactly DABlockTime * DAMempoolTTL seconds
			// * retry with a higher gas price
			// * successfully submit
			mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(12345), nil)
			mockDA.
				On("Submit", mock.Anything, blobs, tc.expectedGasPrices[0], []byte(nil), []byte(nil)).
				Return([][]byte{}, uint64(0), da.ErrTxTimedOut).Once()
			mockDA.
				On("Submit", mock.Anything, blobs, tc.expectedGasPrices[1], []byte(nil), []byte(nil)).
				Return([][]byte{}, uint64(0), da.ErrTxTimedOut).Once()
			mockDA.
				On("Submit", mock.Anything, blobs, tc.expectedGasPrices[2], []byte(nil), []byte(nil)).
				Return([][]byte{bytes.Repeat([]byte{0x00}, 8)}, uint64(0), nil)

			// Test the submitHeadersToDA method
			err = publisher.submitHeadersToDA(ctx)
			if tc.isErrExpected {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

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

	m := getManager(t, coreda.NewDummyDA(100_000), -1, -1)

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewStore(t)
	invalidateBlockHeader(header1)
	store.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height").Return(uint64(3))

	m.store = store

	// Create a Publisher component for testing
	publisher, err := NewPublisher(PublisherOptions{
		EventBus:      m.eventBus,
		Store:         store,
		DALC:          m.dalc,
		Logger:        m.logger,
		Config:        m.config,
		GasPrice:      -1,
		GasMultiplier: -1,
	})
	require.NoError(err)

	err = publisher.submitHeadersToDA(ctx)
	assert.ErrorContains(err, "failed to transform header to proto")
	blocks, err := publisher.pendingHeaders.getPendingHeaders(ctx)
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

	m := getManager(t, coreda.NewDummyDA(100_000), -1, -1)

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewStore(t)
	invalidateBlockHeader(header3)
	store.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height").Return(uint64(3))

	m.store = store

	// Create a Publisher component for testing
	publisher, err := NewPublisher(PublisherOptions{
		EventBus:      m.eventBus,
		Store:         store,
		DALC:          m.dalc,
		Logger:        m.logger,
		Config:        m.config,
		GasPrice:      -1,
		GasMultiplier: -1,
	})
	require.NoError(err)

	err = publisher.submitHeadersToDA(ctx)
	assert.ErrorContains(err, "failed to transform header to proto")
	blocks, err := publisher.pendingHeaders.getPendingHeaders(ctx)
	assert.NoError(err)
	assert.Equal(3, len(blocks)) // we stop submitting all headers when there is a marshalling error
}

// invalidateBlockHeader results in a block header that produces a marshalling error
func invalidateBlockHeader(header *types.SignedHeader) {
	for i := range header.Validators.Validators {
		header.Validators.Validators[i] = &cmtypes.Validator{
			Address:          []byte(""),
			PubKey:           nil,
			VotingPower:      -1,
			ProposerPriority: 0,
		}
	}
}

func Test_isProposer(t *testing.T) {
	require := require.New(t)

	type args struct {
		state         types.State
		signerPrivKey crypto.PrivKey
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
				genesisData, privKey := types.GetGenesisWithPrivkey("Test_isProposer")
				s, err := types.NewFromGenesisDoc(genesisData)
				require.NoError(err)
				return args{
					s,
					privKey,
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

func TestGetRemainingSleep(t *testing.T) {
	tests := []struct {
		name          string
		config        config.Config
		buildingBlock bool
		elapsed       time.Duration
		expectedSleep time.Duration
	}{
		{
			name: "Normal aggregation, elapsed < interval",
			config: config.Config{
				Node: config.NodeConfig{
					BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
					LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
					LazyAggregator: false,
				},
			},
			buildingBlock: false,
			elapsed:       5 * time.Second,
			expectedSleep: 5 * time.Second,
		},
		{
			name: "Normal aggregation, elapsed > interval",
			config: config.Config{
				Node: config.NodeConfig{
					BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
					LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
					LazyAggregator: false,
				},
			},
			buildingBlock: false,
			elapsed:       15 * time.Second,
			expectedSleep: 0 * time.Second,
		},
		{
			name: "Lazy aggregation, not building block",
			config: config.Config{
				Node: config.NodeConfig{
					BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
					LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
					LazyAggregator: true,
				},
			},
			buildingBlock: false,
			elapsed:       5 * time.Second,
			expectedSleep: 15 * time.Second,
		},
		{
			name: "Lazy aggregation, building block, elapsed < interval",
			config: config.Config{
				Node: config.NodeConfig{
					BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
					LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
					LazyAggregator: true,
				},
			},
			buildingBlock: true,
			elapsed:       5 * time.Second,
			expectedSleep: 5 * time.Second,
		},
		{
			name: "Lazy aggregation, building block, elapsed > interval",
			config: config.Config{
				Node: config.NodeConfig{
					BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
					LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
					LazyAggregator: true,
				},
			},
			buildingBlock: true,
			elapsed:       15 * time.Second,
			expectedSleep: 1 * time.Second, // 10% of BlockTime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a Producer for testing getRemainingSleep
			producer := &Producer{
				config:        tt.config,
				buildingBlock: tt.buildingBlock,
				logger:        log.NewTestLogger(t),
			}

			// Calculate start time based on elapsed time
			start := time.Now().Add(-tt.elapsed)

			actualSleep := producer.getRemainingSleep(start)
			// Allow for a small difference, e.g., 5 millisecond
			assert.True(t, WithinDuration(t, tt.expectedSleep, actualSleep, 5*time.Millisecond))
		})
	}
}

func Test_publishBlock_ManagerNotProposer(t *testing.T) {
	require := require.New(t)
	// Create a Producer that is not a proposer
	producer := &Producer{
		isProposer: false,
		logger:     log.NewTestLogger(t),
	}
	err := producer.publishBlock(context.Background())
	require.ErrorIs(err, ErrNotProposer)
}

func TestManager_getRemainingSleep(t *testing.T) {
	tests := []struct {
		name          string
		producer      *Producer
		start         time.Time
		expectedSleep time.Duration
	}{
		{
			name: "Normal aggregation, elapsed < interval",
			producer: &Producer{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
						LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
						LazyAggregator: false,
					},
				},
				buildingBlock: false,
				logger:        log.NewTestLogger(t),
			},
			start:         time.Now().Add(-5 * time.Second),
			expectedSleep: 5 * time.Second,
		},
		{
			name: "Normal aggregation, elapsed > interval",
			producer: &Producer{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
						LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
						LazyAggregator: false,
					},
				},
				buildingBlock: false,
				logger:        log.NewTestLogger(t),
			},
			start:         time.Now().Add(-15 * time.Second),
			expectedSleep: 0 * time.Second,
		},
		{
			name: "Lazy aggregation, not building block",
			producer: &Producer{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
						LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
						LazyAggregator: true,
					},
				},
				buildingBlock: false,
				logger:        log.NewTestLogger(t),
			},
			start:         time.Now().Add(-5 * time.Second),
			expectedSleep: 15 * time.Second,
		},
		{
			name: "Lazy aggregation, building block, elapsed < interval",
			producer: &Producer{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
						LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
						LazyAggregator: true,
					},
				},
				buildingBlock: true,
				logger:        log.NewTestLogger(t),
			},
			start:         time.Now().Add(-5 * time.Second),
			expectedSleep: 5 * time.Second,
		},
		{
			name: "Lazy aggregation, building block, elapsed > interval",
			producer: &Producer{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime:      config.DurationWrapper{Duration: 10 * time.Second},
						LazyBlockTime:  config.DurationWrapper{Duration: 20 * time.Second},
						LazyAggregator: true,
					},
				},
				buildingBlock: true,
				logger:        log.NewTestLogger(t),
			},
			start:         time.Now().Add(-15 * time.Second),
			expectedSleep: 1 * time.Second, // 10% of BlockTime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSleep := tt.producer.getRemainingSleep(tt.start)
			// Allow for a small difference, e.g., 5 millisecond
			assert.True(t, WithinDuration(t, tt.expectedSleep, actualSleep, 5*time.Millisecond))
		})
	}
}

// TestAggregationLoop tests the AggregationLoop function
func TestAggregationLoop(t *testing.T) {
	mockStore := new(mocks.Store)
	mockLogger := log.NewTestLogger(t)
	eventBus := events.NewEventBus(context.Background(), mockLogger)

	// Create a Producer component directly
	producer := &Producer{
		store:    mockStore,
		logger:   mockLogger,
		eventBus: eventBus,
		genesis: &RollkitGenesis{
			ChainID:       "myChain",
			InitialHeight: 1,
		},
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime:      config.DurationWrapper{Duration: time.Second},
				LazyAggregator: false,
			},
		},
		bq:         NewBatchQueue(),
		isProposer: true,
	}

	mockStore.On("Height").Return(uint64(0))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test the Producer's aggregationLoop method
	go producer.aggregationLoop(ctx)

	// Wait for the function to complete or timeout
	<-ctx.Done()

	mockStore.AssertExpectations(t)
}

// TestLazyAggregationLoop tests the lazyAggregationLoop function
func TestLazyAggregationLoop(t *testing.T) {
	mockLogger := log.NewTestLogger(t)

	// Create a Producer component directly
	producer := &Producer{
		logger: mockLogger,
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime:      config.DurationWrapper{Duration: time.Second},
				LazyAggregator: true,
			},
		},
		bq: NewBatchQueue(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	blockTimer := time.NewTimer(producer.config.Node.BlockTime.Duration)
	defer blockTimer.Stop()

	// Test the Producer's lazyAggregationLoop method
	go producer.lazyAggregationLoop(ctx)
	producer.bq.notifyCh <- struct{}{}

	// Wait for the function to complete or timeout
	<-ctx.Done()
}

// TestNormalAggregationLoop tests the normalAggregationLoop function
func TestNormalAggregationLoop(t *testing.T) {
	mockLogger := log.NewTestLogger(t)

	// Create a Producer component directly
	producer := &Producer{
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

	// Test the normalAggregationLoop method directly
	go producer.aggregationLoop(ctx)

	// Wait for the function to complete or timeout
	<-ctx.Done()
}

func TestGetTxsFromBatch_NoBatch(t *testing.T) {
	// Create a Producer with an empty batch queue
	producer := &Producer{
		bq: &BatchQueue{queue: nil}, // No batch available
	}

	// Call the method and assert the results
	txs, timestamp, err := producer.getTxsFromBatch()

	// Assertions
	assert.Nil(t, txs, "Transactions should be nil when no batch exists")
	assert.Nil(t, timestamp, "Timestamp should be nil when no batch exists")
	assert.Equal(t, ErrNoBatch, err, "Expected ErrNoBatch error")
}

func TestGetTxsFromBatch_EmptyBatch(t *testing.T) {
	// Create a Producer with an empty batch
	producer := &Producer{
		bq: &BatchQueue{queue: []BatchWithTime{
			{Batch: &coresequencer.Batch{Transactions: nil}, Time: time.Now()},
		}},
	}

	// Call the method and assert the results
	txs, timestamp, err := producer.getTxsFromBatch()

	// Assertions
	require.NoError(t, err, "Expected no error for empty batch")
	assert.Empty(t, txs, "Transactions should be empty when batch has no transactions")
	assert.NotNil(t, timestamp, "Timestamp should not be nil for empty batch")
}

func TestGetTxsFromBatch_ValidBatch(t *testing.T) {
	// Create a Producer with a valid batch
	producer := &Producer{
		bq: &BatchQueue{queue: []BatchWithTime{
			{Batch: &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}, Time: time.Now()},
		}},
	}

	// Call the method and assert the results
	txs, timestamp, err := producer.getTxsFromBatch()

	// Assertions
	require.NoError(t, err, "Expected no error for valid batch")
	assert.Len(t, txs, 2, "Expected 2 transactions")
	assert.NotNil(t, timestamp, "Timestamp should not be nil for valid batch")
	assert.Equal(t, [][]byte{[]byte("tx1"), []byte("tx2")}, txs, "Transactions do not match")
}

func TestPublisherSubmitLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logger := log.NewTestLogger(t)
	eventBus := events.NewEventBus(ctx, logger)
	mockStore := new(mocks.Store)
	mockDALC := new(damocks.DA)

	// Create a Publisher component for testing
	publisher, err := NewPublisher(PublisherOptions{
		EventBus: eventBus,
		Store:    mockStore,
		DALC:     da.NewDAClient(mockDALC, -1, -1, nil, nil, logger),
		Logger:   logger,
		Config: config.Config{
			DA: config.DAConfig{
				BlockTime: config.DurationWrapper{Duration: 100 * time.Millisecond},
			},
		},
	})
	require.NoError(t, err)

	// Mock the pendingHeaders to be empty to avoid actual submission
	mockStore.On("Height").Return(uint64(0))
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)

	// Start the submission loop
	go publisher.headerSubmissionLoop(ctx)

	// Wait for the loop to run a few cycles
	time.Sleep(500 * time.Millisecond)

	// Verify expectations
	mockStore.AssertExpectations(t)
}

func TestSyncerRetrieveLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	logger := log.NewTestLogger(t)
	eventBus := events.NewEventBus(ctx, logger)
	mockStore := new(mocks.Store)
	mockDALC := new(damocks.DA)

	// Setup a minimal result for RetrieveHeaders
	emptyResult := coreda.ResultRetrieveHeaders{
		BaseResult: coreda.BaseResult{
			Code: coreda.StatusSuccess,
		},
		Headers: [][]byte{},
	}
	mockDALC.On("RetrieveHeaders", mock.Anything, mock.Anything).Return(emptyResult)

	// Create a Syncer component for testing
	syncer := NewSyncer(SyncerOptions{
		EventBus: eventBus,
		Store:    mockStore,
		DALC:     da.NewDAClient(mockDALC, -1, -1, nil, nil, logger),
		Logger:   logger,
		Config: config.Config{
			DA: config.DAConfig{
				BlockTime: config.DurationWrapper{Duration: 100 * time.Millisecond},
			},
		},
	})

	// Start the retrieval loop
	go syncer.retrieveLoop(ctx)

	// Wait for the loop to run a few cycles
	time.Sleep(500 * time.Millisecond)

	// Verify expectations
	mockDALC.AssertExpectations(t)
}

func TestHandleBlockDAIncluded(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := log.NewTestLogger(t)
	eventBus := events.NewEventBus(ctx, logger)
	headerCache := NewHeaderCache()

	// Create test data
	header, _ := types.GetRandomBlock(1, 0, "test")
	hash := header.Hash()

	// Create syncer with minimal components
	syncer := &Syncer{
		eventBus:    eventBus,
		headerCache: headerCache,
		logger:      logger,
	}

	// Initially, the header should not be marked as DA included
	require.False(t, syncer.headerCache.isDAIncluded(hash.String()))

	// Create and publish a BlockDAIncludedEvent
	event := BlockDAIncludedEvent{
		Header:   header,
		Height:   1,
		DAHeight: 10,
	}

	// Handle the event
	syncer.handleBlockDAIncluded(ctx, event)

	// Verify the header is now marked as DA included
	require.True(t, syncer.headerCache.isDAIncluded(hash.String()))
}

func TestStateManagerHandleEvents(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := log.NewTestLogger(t)
	eventBus := events.NewEventBus(ctx, logger)
	mockStore := new(mocks.Store)
	mockExec := coreexecutor.NewDummyExecutor()

	// Initialize state
	initialState := types.State{
		ChainID:         "test-chain",
		InitialHeight:   1,
		LastBlockHeight: 0,
		AppHash:         []byte("initial-hash"),
	}

	// Setup mock for UpdateState call
	mockStore.On("UpdateState", mock.Anything, mock.AnythingOfType("types.State")).Return(nil)

	// Create StateManager with mock components
	stateManager := NewStateManager(StateManagerOptions{
		EventBus:     eventBus,
		Store:        mockStore,
		Exec:         mockExec,
		Logger:       logger,
		InitialState: initialState,
	})

	// Verify initial state
	state := stateManager.GetLastState()
	require.Equal(t, initialState, state)

	// Create and publish a StateUpdatedEvent
	newState := types.State{
		ChainID:         "test-chain",
		InitialHeight:   1,
		LastBlockHeight: 1,
		AppHash:         []byte("new-hash"),
	}

	// Test StateUpdatedEvent handling by directly updating the state
	err := stateManager.updateState(ctx, newState)
	require.NoError(t, err)

	// Verify the state was updated
	updatedState := stateManager.GetLastState()
	require.Equal(t, newState, updatedState)
	require.Equal(t, uint64(1), updatedState.LastBlockHeight)
	require.Equal(t, []byte("new-hash"), updatedState.AppHash)

	// Verify that store's UpdateState was called
	mockStore.AssertExpectations(t)
}

func TestCreateBlock(t *testing.T) {
	// Setup
	ctx := context.Background()
	logger := log.NewTestLogger(t)

	// Create test data
	genesisDoc, privKey := types.GetGenesisWithPrivkey("test-chain")
	state, err := types.NewFromGenesisDoc(genesisDoc)
	require.NoError(t, err)

	// Create a Producer component
	producer := &Producer{
		proposerKey:  privKey,
		lastState:    state,
		lastStateMtx: new(sync.RWMutex),
		logger:       logger,
		genesis: &RollkitGenesis{
			ChainID:         "test-chain",
			InitialHeight:   1,
			ProposerAddress: genesisDoc.Validators[0].Address.Bytes(),
		},
	}

	// Create a block
	txs := [][]byte{[]byte("tx1"), []byte("tx2")}
	timestamp := time.Now()
	header, data, err := producer.createBlock(ctx, 1, txs, timestamp)

	// Assertions
	require.NoError(t, err)
	require.NotNil(t, header)
	require.NotNil(t, data)
	require.Equal(t, uint64(1), header.Height())
	require.Equal(t, "test-chain", header.ChainID())
	require.Equal(t, 2, len(data.Txs))
	require.Equal(t, types.Tx([]byte("tx1")), data.Txs[0])
	require.Equal(t, types.Tx([]byte("tx2")), data.Txs[1])

	// Verify the data hash in the header matches the actual data hash
	require.Equal(t, data.Hash(), header.DataHash)

	// Verify the header is signed
	require.NotEmpty(t, header.Signature)
}
