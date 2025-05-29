package block

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	da "github.com/rollkit/rollkit/core/da"
	executor "github.com/rollkit/rollkit/core/execution"
	sequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer"
	noopsigner "github.com/rollkit/rollkit/pkg/signer/noop"
	storepkg "github.com/rollkit/rollkit/pkg/store"
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
func getManager(t *testing.T, da da.DA, gasPrice float64, gasMultiplier float64) (*Manager, *mocks.Store) {
	t.Helper()
	ctx := context.Background()
	// create a minimalistic block manager
	cfg := config.Config{
		DA: config.DAConfig{
			BlockTime: config.DurationWrapper{Duration: 100 * time.Millisecond},
		},
		Node: config.NodeConfig{
			BlockTime: config.DurationWrapper{Duration: 100 * time.Millisecond},
		},
	}
	metrics := NopMetrics()
	genesis, gPrivKey, _ := types.GetGenesisWithPrivkey("getManager")
	mockStore := mocks.NewStore(t)
	mockStore.On("GetState", mock.Anything).Return(types.State{}, ds.ErrNotFound)

	// Add mocks for SaveBlockData (called during getInitialState when no state exists)
	mockStore.On("SaveBlockData", mock.Anything, mock.AnythingOfType("*types.SignedHeader"), mock.AnythingOfType("*types.Data"), mock.AnythingOfType("*types.Signature")).Return(nil)

	// Add mock for SetHeight (called in NewManager)
	mockStore.On("SetHeight", mock.Anything, mock.AnythingOfType("uint64")).Return(nil)

	// Add mock for GetMetadata calls in NewManager - correct order: (ctx, key)
	mockStore.On("GetMetadata", mock.Anything, LastBatchDataKey).Return(nil, ds.ErrNotFound)
	mockStore.On("GetMetadata", mock.Anything, DAIncludedHeightKey).Return(nil, ds.ErrNotFound)
	// Add mock for LastSubmittedHeightKey (called by NewPendingHeaders)
	mockStore.On("GetMetadata", mock.Anything, LastSubmittedHeightKey).Return(nil, ds.ErrNotFound)

	// Create an in-memory datastore for header/data stores
	memDs := dssync.MutexWrap(ds.NewMapDatastore())
	headerStore, err := goheaderstore.NewStore[*types.SignedHeader](memDs, goheaderstore.WithStorePrefix("header"))
	require.NoError(t, err)
	dataStore, err := goheaderstore.NewStore[*types.Data](memDs, goheaderstore.WithStorePrefix("data"))
	require.NoError(t, err)

	privKeySigner, _ := noopsigner.NewNoopSigner(gPrivKey)
	logger := log.NewNopLogger()
	m, err := NewManager(
		ctx,
		privKeySigner,
		cfg,
		genesis,
		mockStore, // This is the mock store for general state, not for headers/data
		executor.NewDummyExecutor(),
		sequencer.NewDummySequencer(),
		da,
		logger,
		headerStore, // Real header store
		dataStore,   // Real data store
		nil,         // headerBroadcaster
		nil,         // dataBroadcaster
		metrics,
		gasPrice,
		gasMultiplier,
		func(proposerAddress []byte, pubKey crypto.PubKey) (types.Hash, error) {
			return make(types.Hash, 32), nil
		},
		func(header *types.Header) (types.Hash, error) {
			return make(types.Hash, 32), nil
		},
		func(signature *types.Signature, header *types.Header, proposerAddress []byte) (types.Hash, error) {
			return make(types.Hash, 32), nil
		},
		createDefaultSignaturePayloadProvider(), // SignaturePayloadProvider
	)
	require.NoError(t, err)
	return m, mockStore
}

// TestInitialStateClean verifies that getInitialState initializes state correctly when no state is stored.
func TestInitialStateClean(t *testing.T) {
	require := require.New(t)
	ctx := t.Context()

	// Create genesis document
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateClean")
	logger := log.NewTestLogger(t)
	es, _ := storepkg.NewDefaultInMemoryKVStore()
	emptyStore := storepkg.New(es)
	mockExecutor := mocks.NewExecutor(t)

	// Set expectation for InitChain call within getInitialState
	mockExecutor.On("InitChain", ctx, genesisData.GenesisDAStartTime, genesisData.InitialHeight, genesisData.ChainID).
		Return([]byte("mockAppHash"), uint64(1000), nil).Once()

	s, err := getInitialState(ctx, genesisData, nil, emptyStore, mockExecutor, logger)
	require.NoError(err)
	initialHeight := genesisData.InitialHeight
	require.Equal(initialHeight-1, s.LastBlockHeight)
	require.Equal(initialHeight, s.InitialHeight)

	// Assert mock expectations
	mockExecutor.AssertExpectations(t)
}

// TestInitialStateStored verifies that getInitialState loads existing state from the store and does not call InitChain.
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

	es, _ := storepkg.NewDefaultInMemoryKVStore()
	store := storepkg.New(es)
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

// TestInitialStateUnexpectedHigherGenesis verifies that getInitialState returns an error if the genesis initial height is higher than the stored state's last block height.
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
		genesisData.GenesisDAStartTime,
		genesisData.ProposerAddress,
	)
	sampleState := types.State{
		ChainID:         "TestInitialStateUnexpectedHigherGenesis",
		InitialHeight:   1,
		LastBlockHeight: 0,
	}
	es, _ := storepkg.NewDefaultInMemoryKVStore()
	store := storepkg.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	mockExecutor := mocks.NewExecutor(t)

	_, err = getInitialState(ctx, genesis, nil, store, mockExecutor, logger)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")

	// Assert mock expectations (InitChain should not have been called)
	mockExecutor.AssertExpectations(t)
}

// TestSignVerifySignature verifies that signatures can be created and verified using the configured signer.
func TestSignVerifySignature(t *testing.T) {
	require := require.New(t)
	mockDAC := mocks.NewDA(t)
	m, _ := getManager(t, mockDAC, -1, -1)
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
	mockDAC := mocks.NewDA(t)

	// Create a minimalistic block manager
	m, mockStore := getManager(t, mockDAC, -1, -1)
	height := uint64(1)
	header, data := types.GetRandomBlock(height, 5, "TestIsDAIncluded")
	mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil).Times(3)
	mockStore.On("Height", mock.Anything).Return(uint64(100), nil).Maybe()
	ctx := context.Background()
	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(ctx, height))

	// Get the header hash using the same headerHasher that IsDAIncluded uses
	headerHash, err := m.headerHasher(&header.Header)
	require.NoError(err)

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.headerCache.SetDAIncluded(headerHash.String())
	require.False(m.IsDAIncluded(ctx, height))

	// Set the data as DAIncluded and verify IsDAIncluded returns true
	m.dataCache.SetDAIncluded(data.DACommitment().String())
	require.True(m.IsDAIncluded(ctx, height))
}

// Test_submitBlocksToDA_BlockMarshalErrorCase1 verifies that a marshalling error in the first block prevents all blocks from being submitted.
func Test_submitBlocksToDA_BlockMarshalErrorCase1(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase1"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	mockDA := mocks.NewDA(t)
	m, _ := getManager(t, mockDA, -1, -1)

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
	mockDA.AssertExpectations(t)
}

// Test_submitBlocksToDA_BlockMarshalErrorCase2 verifies that a marshalling error in a later block prevents all blocks from being submitted.
func Test_submitBlocksToDA_BlockMarshalErrorCase2(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase2"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	mockDA := mocks.NewDA(t)
	m, _ := getManager(t, mockDA, -1, -1)

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
	// Expect all blocks to remain pending because the batch submission was halted
	assert.Equal(3, len(blocks))

	mockDA.AssertExpectations(t)
	store.AssertExpectations(t)
}

// invalidateBlockHeader results in a block header that produces a marshalling error
func invalidateBlockHeader(header *types.SignedHeader) {
	header.Signer.PubKey = &crypto.Ed25519PublicKey{}
}

// Test_isProposer verifies the isProposer utility for matching the signing key to the genesis proposer public key.
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

// TestBytesToBatchData verifies conversion between bytes and batch data, including error handling for corrupted data.
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

// createDefaultSignaturePayloadProvider creates a basic signature payload provider for tests
// that matches the behavior used in GetSignature function in utils.go
func createDefaultSignaturePayloadProvider() types.SignaturePayloadProvider {
	return func(header *types.Header, data *types.Data) ([]byte, error) {
		if header == nil {
			return nil, errors.New("header cannot be nil")
		}

		// Use the same approach as GetSignature: just return the marshaled header bytes
		// This matches the behavior in utils.go GetSignature function
		return header.MarshalBinary()
	}
}
