package block

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	return &Manager{
		dalc:          da.NewDAClient(backend, gasPrice, gasMultiplier, nil, nil, logger),
		headerCache:   NewHeaderCache(),
		logger:        logger,
		gasPrice:      gasPrice,
		gasMultiplier: gasMultiplier,
	}
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
	genesis := &RollkitGenesis{
		ChainID:         chainID,
		InitialHeight:   1,
		ProposerAddress: genesisDoc.Validators[0].Address.Bytes(),
	}
	sampleState := types.State{
		ChainID:         chainID,
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
	d := dataCache.getData(header.Height())
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
	genesis := &RollkitGenesis{
		ChainID:         "TestInitialStateUnexpectedHigherGenesis",
		InitialHeight:   2,
		ProposerAddress: genesisDoc.Validators[0].Address.Bytes(),
	}
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
	_, err = getInitialState(context.TODO(), genesis, store, coreexecutor.NewDummyExecutor(), logger)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")
}

func TestSignVerifySignature(t *testing.T) {
	require := require.New(t)
	m := getManager(t, coreda.NewDummyDA(100_000, 0, 0), -1, -1)
	payload := []byte("test")
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	cases := []struct {
		name    string
		privKey crypto.PrivKey
		pubKey  crypto.PubKey
	}{
		{"ed25519", privKey, pubKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m.proposerKey = c.privKey
			signature, err := m.proposerKey.Sign(payload)
			require.NoError(err)
			ok, err := c.pubKey.Verify(payload, signature)
			require.NoError(err)
			require.True(ok)
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
		// {"fixed_gas_price_with_multiplier", 1.0, 1.2, []float64{
		// 	1.0, 1.2, 1.2 * 1.2,
		// }, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDA := &damocks.DA{}
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
				Return([][]byte{}, da.ErrTxTimedOut).Once()
			mockDA.
				On("Submit", mock.Anything, blobs, tc.expectedGasPrices[1], []byte(nil), []byte(nil)).
				Return([][]byte{}, da.ErrTxTimedOut).Once()
			mockDA.
				On("Submit", mock.Anything, blobs, tc.expectedGasPrices[2], []byte(nil), []byte(nil)).
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
	store.On("Height").Return(uint64(3))

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
	store.On("Height").Return(uint64(3))

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
		//{
		//	name: "Signing key does not match genesis proposer public key",
		//	args: func() args {
		//		genesisData, _ := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "Test_isProposer")
		//		s, err := types.NewFromGenesisDoc(genesisData)
		//		require.NoError(err)

		//		randomPrivKey := ed25519.GenPrivKey()
		//		signingKey, err := types.PrivKeyToSigningKey(randomPrivKey)
		//		require.NoError(err)
		//		return args{
		//			s,
		//			signingKey,
		//		}
		//	}(),
		//	isProposer: false,
		//	err:        nil,
		//},
		//{
		//	name: "No validators found in genesis",
		//	args: func() args {
		//		genesisData, privKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, "Test_isProposer")
		//		genesisData.Validators = nil
		//		s, err := types.NewFromGenesisDoc(genesisData)
		//		require.NoError(err)

		//		signingKey, err := types.PrivKeyToSigningKey(privKey)
		//		require.NoError(err)
		//		return args{
		//			s,
		//			signingKey,
		//		}
		//	}(),
		//	isProposer: false,
		//	err:        ErrNoValidatorsInState,
		//},
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

func TestManager_getRemainingSleep(t *testing.T) {
	tests := []struct {
		name          string
		manager       *Manager
		start         time.Time
		expectedSleep time.Duration
	}{
		{
			name: "Normal aggregation, elapsed < interval",
			manager: &Manager{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime: config.DurationWrapper{
							Duration: 10 * time.Second,
						},
						LazyBlockTime: config.DurationWrapper{
							Duration: 20 * time.Second,
						},
						LazyAggregator: false,
					},
				},
				buildingBlock: false,
			},
			start:         time.Now().Add(-5 * time.Second),
			expectedSleep: 5 * time.Second,
		},
		{
			name: "Normal aggregation, elapsed > interval",
			manager: &Manager{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime: config.DurationWrapper{
							Duration: 10 * time.Second,
						},
						LazyBlockTime: config.DurationWrapper{
							Duration: 20 * time.Second,
						},
						LazyAggregator: false,
					},
				},
				buildingBlock: false,
			},
			start:         time.Now().Add(-15 * time.Second),
			expectedSleep: 0 * time.Second,
		},
		{
			name: "Lazy aggregation, not building block",
			manager: &Manager{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime: config.DurationWrapper{
							Duration: 10 * time.Second,
						},
						LazyBlockTime: config.DurationWrapper{
							Duration: 20 * time.Second,
						},
						LazyAggregator: true,
					},
				},
				buildingBlock: false,
			},
			start:         time.Now().Add(-5 * time.Second),
			expectedSleep: 15 * time.Second,
		},
		{
			name: "Lazy aggregation, building block, elapsed < interval",
			manager: &Manager{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime: config.DurationWrapper{
							Duration: 10 * time.Second,
						},
						LazyBlockTime: config.DurationWrapper{
							Duration: 20 * time.Second,
						},
						LazyAggregator: true,
					},
				},
				buildingBlock: true,
			},
			start:         time.Now().Add(-5 * time.Second),
			expectedSleep: 5 * time.Second,
		},
		{
			name: "Lazy aggregation, building block, elapsed > interval",
			manager: &Manager{
				config: config.Config{
					Node: config.NodeConfig{
						BlockTime: config.DurationWrapper{
							Duration: 10 * time.Second,
						},
						LazyBlockTime: config.DurationWrapper{
							Duration: 20 * time.Second,
						},
						LazyAggregator: true,
					},
				},
				buildingBlock: true,
			},
			start:         time.Now().Add(-15 * time.Second),
			expectedSleep: 1 * time.Second, // 10% of BlockTime
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualSleep := tt.manager.getRemainingSleep(tt.start)
			// Allow for a small difference, e.g., 5 millisecond
			assert.True(t, WithinDuration(t, tt.expectedSleep, actualSleep, 5*time.Millisecond))
		})
	}
}

// TestAggregationLoop tests the AggregationLoop function
func TestAggregationLoop(t *testing.T) {
	mockStore := new(mocks.Store)
	mockLogger := log.NewTestLogger(t)

	m := &Manager{
		store:  mockStore,
		logger: mockLogger,
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
		bq: NewBatchQueue(),
	}

	mockStore.On("Height").Return(uint64(0))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go m.AggregationLoop(ctx)

	// Wait for the function to complete or timeout
	<-ctx.Done()

	mockStore.AssertExpectations(t)
}

// TestLazyAggregationLoop tests the lazyAggregationLoop function
func TestLazyAggregationLoop(t *testing.T) {
	mockLogger := log.NewTestLogger(t)

	m := &Manager{
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

	blockTimer := time.NewTimer(m.config.Node.BlockTime.Duration)
	defer blockTimer.Stop()

	go m.lazyAggregationLoop(ctx, blockTimer)
	m.bq.notifyCh <- struct{}{}

	// Wait for the function to complete or timeout
	<-ctx.Done()
}

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

func TestGetTxsFromBatch(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		batchQueue    []BatchData
		wantBatchData *BatchData
		wantErr       error
		assertions    func(t *testing.T, gotBatchData *BatchData)
	}{
		{
			name:          "no batch available",
			batchQueue:    nil,
			wantBatchData: nil,
			wantErr:       ErrNoBatch,
			assertions:    nil,
		},
		{
			name: "empty batch",
			batchQueue: []BatchData{
				{Batch: &coresequencer.Batch{Transactions: nil}, Time: now},
			},
			wantBatchData: &BatchData{Batch: &coresequencer.Batch{Transactions: nil}, Time: now},
			wantErr:       nil,
			assertions: func(t *testing.T, gotBatchData *BatchData) {
				assert.Empty(t, gotBatchData.Batch.Transactions, "Transactions should be empty when batch has no transactions")
				assert.NotNil(t, gotBatchData.Time, "Timestamp should not be nil for empty batch")
			},
		},
		{
			name: "valid batch with transactions",
			batchQueue: []BatchData{
				{Batch: &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}, Time: now},
			},
			wantBatchData: &BatchData{Batch: &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}, Time: now},
			wantErr:       nil,
			assertions: func(t *testing.T, gotBatchData *BatchData) {
				assert.Len(t, gotBatchData.Batch.Transactions, 2, "Expected 2 transactions")
				assert.NotNil(t, gotBatchData.Time, "Timestamp should not be nil for valid batch")
				assert.Equal(t, [][]byte{[]byte("tx1"), []byte("tx2")}, gotBatchData.Batch.Transactions, "Transactions do not match")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create manager with the test batch queue
			m := &Manager{
				bq: &BatchQueue{queue: tt.batchQueue},
			}

			// Call the method under test
			gotBatchData, err := m.getTxsFromBatch()

			// Check error
			if tt.wantErr != nil {
				assert.Equal(t, tt.wantErr, err)
			} else {
				require.NoError(t, err)
			}

			// Run additional assertions if provided
			if tt.assertions != nil {
				tt.assertions(t, gotBatchData)
			}
		})
	}
}
