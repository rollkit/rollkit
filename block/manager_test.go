package block

import (
	"context"
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

	"github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/cache"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/pkg/store"
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

// Returns a minimalistic block manager using a mock DA Client
func getManager(t *testing.T, dac da.Client, gasPrice float64, gasMultiplier float64) *Manager {
	logger := log.NewTestLogger(t)
	m := &Manager{
		dalc:          dac,
		headerCache:   cache.NewCache[types.SignedHeader](),
		logger:        logger,
		gasPrice:      gasPrice,
		gasMultiplier: gasMultiplier,
		lastStateMtx:  &sync.RWMutex{},
		metrics:       NopMetrics(),
	}

	m.publishBlock = m.publishBlockInternal

	return m
}

func TestInitialStateClean(t *testing.T) {
	require := require.New(t)
	ctx := context.TODO()

	// Create genesis document
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateClean")
	logger := log.NewTestLogger(t)
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(es)
	mockExecutor := mocks.NewExecutor(t)

	// Set expectation for InitChain call within getInitialState
	mockExecutor.On("InitChain", ctx, genesisData.GenesisDAStartHeight, genesisData.InitialHeight, genesisData.ChainID).
		Return([]byte("mockAppHash"), uint64(1000), nil).Once()

	s, err := getInitialState(ctx, genesisData, nil, emptyStore, mockExecutor, logger)
	require.NoError(err)
	initialHeight := genesisData.InitialHeight
	require.Equal(initialHeight-1, s.LastBlockHeight)
	require.Equal(initialHeight, s.InitialHeight)

	// Assert mock expectations
	mockExecutor.AssertExpectations(t)
}

func TestInitialStateStored(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create genesis document
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateStored")
	sampleState := types.State{
		ChainID:         "TestInitialStateStored",
		InitialHeight:   1,
		LastBlockHeight: 100,
	}

	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	logger := log.NewTestLogger(t)
	mockExecutor := mocks.NewExecutor(t)

	// getInitialState should not call InitChain if state exists
	s, err := getInitialState(ctx, genesisData, nil, store, mockExecutor, logger)
	require.NoError(err)
	require.Equal(s.LastBlockHeight, uint64(100))
	require.Equal(s.InitialHeight, uint64(1))

	// Assert mock expectations (InitChain should not have been called)
	mockExecutor.AssertExpectations(t)
}

func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	logger := log.NewTestLogger(t)
	ctx := context.Background()

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
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	mockExecutor := mocks.NewExecutor(t)

	_, err = getInitialState(ctx, genesis, nil, store, mockExecutor, logger)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")

	// Assert mock expectations (InitChain should not have been called)
	mockExecutor.AssertExpectations(t)
}

func TestSignVerifySignature(t *testing.T) {
	require := require.New(t)
	mockDAC := mocks.NewClient(t)       // Use mock DA Client
	m := getManager(t, mockDAC, -1, -1) // Pass mock DA Client
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
			m.signer = c.signer
			signature, err := m.signer.Sign(payload)
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

// Test_submitBlocksToDA_BlockMarshalErrorCase1: A itself has a marshalling error. So A, B and C never get submitted.
func Test_submitBlocksToDA_BlockMarshalErrorCase1(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase1"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	mockDA := mocks.NewClient(t)       // Use mock DA Client
	m := getManager(t, mockDA, -1, -1) // Pass mock DA Client

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

	mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(100000), nil).Once()

	m.store = store

	var err error
	m.pendingHeaders, err = NewPendingHeaders(store, m.logger)
	require.NoError(err)

	err = m.submitHeadersToDA(ctx)
	assert.ErrorContains(err, "failed to transform header to proto")
	blocks, err := m.pendingHeaders.getPendingHeaders(ctx)
	assert.NoError(err)
	assert.Equal(3, len(blocks))
	mockDA.AssertExpectations(t)
}

func Test_submitBlocksToDA_BlockMarshalErrorCase2(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase2"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	mockDA := mocks.NewClient(t)       // Use mock DA Client
	m := getManager(t, mockDA, -1, -1) // Pass mock DA Client

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewStore(t)
	invalidateBlockHeader(header3) // C fails marshal
	store.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height", mock.Anything).Return(uint64(3), nil)

	m.store = store

	// Expect MaxBlobSize call (likely happens before marshalling loop)
	mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(100000), nil).Once()

	var err error
	m.pendingHeaders, err = NewPendingHeaders(store, m.logger)
	require.NoError(err)
	err = m.submitHeadersToDA(ctx)
	assert.ErrorContains(err, "failed to transform header to proto") // Error from C is expected
	blocks, err := m.pendingHeaders.getPendingHeaders(ctx)
	assert.NoError(err)
	// Expect all blocks to remain pending because the batch submission was halted
	assert.Equal(3, len(blocks))

	mockDA.AssertExpectations(t)
	store.AssertExpectations(t)
}

// invalidateBlockHeader results in a block header that produces a marshalling error
func invalidateBlockHeader(header *types.SignedHeader) {
	header.Signer.PubKey = &crypto.Ed25519PublicKey{}
}

func Test_isProposer(t *testing.T) {
	require := require.New(t)

	type args struct {
		state         crypto.PubKey
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
				_, privKey, _ := types.GetGenesisWithPrivkey("Test_isProposer")
				signer, err := noopsigner.NewNoopSigner(privKey)
				require.NoError(err)
				return args{
					privKey.GetPublic(),
					signer,
				}
			}(),
			isProposer: true,
			err:        nil,
		},
		{
			name: "Signing key does not match genesis proposer public key",
			args: func() args {
				_, privKey, _ := types.GetGenesisWithPrivkey("Test_isProposer_Mismatch")
				// Generate a different private key
				otherPrivKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
				require.NoError(err)
				signer, err := noopsigner.NewNoopSigner(otherPrivKey)
				require.NoError(err)
				return args{
					privKey.GetPublic(),
					signer,
				}
			}(),
			isProposer: false,
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

// TestBytesToBatchData tests conversion between bytes and batch data.
func TestBytesToBatchData(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	// empty input returns empty slice
	out, err := bytesToBatchData(nil)
	require.NoError(err)
	require.Empty(out)
	out, err = bytesToBatchData([]byte{})
	require.NoError(err)
	require.Empty(out)

	// valid multi-entry data
	orig := [][]byte{[]byte("foo"), []byte("bar"), {}}
	b := convertBatchDataToBytes(orig)
	out, err = bytesToBatchData(b)
	require.NoError(err)
	require.Equal(orig, out)

	// corrupted length prefix (declared length greater than available bytes)
	bad := []byte{0, 0, 0, 5, 'x', 'y'}
	_, err = bytesToBatchData(bad)
	assert.Error(err)
	assert.Contains(err.Error(), "corrupted data")
}
