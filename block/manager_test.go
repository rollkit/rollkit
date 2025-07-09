package block

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/cache"
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
func getManager(t *testing.T, da da.DA, gasPrice float64, gasMultiplier float64) (*Manager, *mocks.MockStore) {
	logger := logging.Logger("test")
	mockStore := mocks.NewMockStore(t)
	
	// Create channel manager
	channelConfig := DefaultChannelManagerConfig()
	channelManager := NewChannelManager(logger, channelConfig)
	
	m := &Manager{
		da:                       da,
		headerCache:              cache.NewCache[types.SignedHeader](),
		dataCache:                cache.NewCache[types.Data](),
		logger:                   logger,
		gasPrice:                 gasPrice,
		gasMultiplier:            gasMultiplier,
		lastStateMtx:             &sync.RWMutex{},
		metrics:                  NopMetrics(),
		store:                    mockStore,
		channelManager:           channelManager,
		signaturePayloadProvider: defaultSignaturePayloadProvider,
	}

	m.publishBlock = m.publishBlockInternal

	return m, mockStore
}

// TestInitialStateClean verifies that getInitialState initializes state correctly when no state is stored.
func TestInitialStateClean(t *testing.T) {
	require := require.New(t)
	ctx := t.Context()

	// Create genesis document
	genesisData, _, _ := types.GetGenesisWithPrivkey("TestInitialStateClean")
	logger := logging.Logger("test")
	es, _ := storepkg.NewDefaultInMemoryKVStore()
	emptyStore := storepkg.New(es)
	mockExecutor := mocks.NewMockExecutor(t)

	// Set expectation for InitChain call within getInitialState
	mockExecutor.On("InitChain", ctx, genesisData.GenesisDAStartTime, genesisData.InitialHeight, genesisData.ChainID).
		Return([]byte("mockAppHash"), uint64(1000), nil).Once()

	s, err := getInitialState(ctx, genesisData, nil, emptyStore, mockExecutor, logger, nil /* uses default signature verification */)
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
	logger := logging.Logger("test")
	mockExecutor := mocks.NewMockExecutor(t)

	// getInitialState should not call InitChain if state exists
	s, err := getInitialState(ctx, genesisData, nil, store, mockExecutor, logger, nil /* uses default signature verification */)
	require.NoError(err)
	require.Equal(s.LastBlockHeight, uint64(100))
	require.Equal(s.InitialHeight, uint64(1))

	// Assert mock expectations (InitChain should not have been called)
	mockExecutor.AssertExpectations(t)
}

// TestInitialStateUnexpectedHigherGenesis verifies that getInitialState returns an error if the genesis initial height is higher than the stored state's last block height.
func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	logger := logging.Logger("test")
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
	mockExecutor := mocks.NewMockExecutor(t)

	_, err = getInitialState(ctx, genesis, nil, store, mockExecutor, logger, nil /* uses default signature verification */)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")

	// Assert mock expectations (InitChain should not have been called)
	mockExecutor.AssertExpectations(t)
}

// TestSignVerifySignature verifies that signatures can be created and verified using the configured signer.
func TestSignVerifySignature(t *testing.T) {
	require := require.New(t)
	mockDAC := mocks.NewMockDA(t)
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
	mockDAC := mocks.NewMockDA(t)

	// Create a minimalistic block manager
	m, mockStore := getManager(t, mockDAC, -1, -1)
	height := uint64(1)
	header, data := types.GetRandomBlock(height, 5, "TestIsDAIncluded")
	mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil).Times(3)
	mockStore.On("Height", mock.Anything).Return(uint64(100), nil).Maybe()
	ctx := context.Background()
	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(ctx, height))

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.headerCache.SetDAIncluded(header.Hash().String(), uint64(1))
	require.False(m.IsDAIncluded(ctx, height))

	// Set the data as DAIncluded and verify IsDAIncluded returns true
	m.dataCache.SetDAIncluded(data.DACommitment().String(), uint64(1))
	require.True(m.IsDAIncluded(ctx, height))
}

// Test_submitBlocksToDA_BlockMarshalErrorCase1 verifies that a marshalling error in the first block prevents all blocks from being submitted.
func Test_submitBlocksToDA_BlockMarshalErrorCase1(t *testing.T) {
	chainID := "Test_submitBlocksToDA_BlockMarshalErrorCase1"
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	mockDA := mocks.NewMockDA(t)
	m, _ := getManager(t, mockDA, -1, -1)

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewMockStore(t)
	invalidateBlockHeader(header1)
	store.On("GetMetadata", mock.Anything, storepkg.LastSubmittedHeaderHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height", mock.Anything).Return(uint64(3), nil)

	m.store = store

	var err error
	m.pendingHeaders, err = NewPendingHeaders(store, m.logger)
	require.NoError(err)

	headers, err := m.pendingHeaders.getPendingHeaders(ctx)
	require.NoError(err)
	err = m.submitHeadersToDA(ctx, headers)
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

	mockDA := mocks.NewMockDA(t)
	m, _ := getManager(t, mockDA, -1, -1)

	header1, data1 := types.GetRandomBlock(uint64(1), 5, chainID)
	header2, data2 := types.GetRandomBlock(uint64(2), 5, chainID)
	header3, data3 := types.GetRandomBlock(uint64(3), 5, chainID)

	store := mocks.NewMockStore(t)
	invalidateBlockHeader(header3)
	store.On("GetMetadata", mock.Anything, storepkg.LastSubmittedHeaderHeightKey).Return(nil, ds.ErrNotFound)
	store.On("GetBlockData", mock.Anything, uint64(1)).Return(header1, data1, nil)
	store.On("GetBlockData", mock.Anything, uint64(2)).Return(header2, data2, nil)
	store.On("GetBlockData", mock.Anything, uint64(3)).Return(header3, data3, nil)
	store.On("Height", mock.Anything).Return(uint64(3), nil)

	m.store = store

	var err error
	m.pendingHeaders, err = NewPendingHeaders(store, m.logger)
	require.NoError(err)

	headers, err := m.pendingHeaders.getPendingHeaders(ctx)
	require.NoError(err)
	err = m.submitHeadersToDA(ctx, headers)
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

// TestGetDataSignature_Success ensures a valid signature is returned when the signer is set.
func TestGetDataSignature_Success(t *testing.T) {
	require := require.New(t)
	mockDAC := mocks.NewMockDA(t)
	m, _ := getManager(t, mockDAC, -1, -1)

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	signer, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)
	m.signer = signer
	_, data := types.GetRandomBlock(1, 2, "TestGetDataSignature")
	sig, err := m.getDataSignature(data)
	require.NoError(err)
	require.NotEmpty(sig)
}

// TestGetDataSignature_NilSigner ensures the correct error is returned when the signer is nil.
func TestGetDataSignature_NilSigner(t *testing.T) {
	require := require.New(t)
	mockDAC := mocks.NewMockDA(t)
	m, _ := getManager(t, mockDAC, -1, -1)

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	signer, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)
	m.signer = signer
	_, data := types.GetRandomBlock(1, 2, "TestGetDataSignature")

	m.signer = nil
	_, err = m.getDataSignature(data)
	require.ErrorContains(err, "signer is nil; cannot sign data")
}

// TestIsValidSignedData covers valid, nil, wrong proposer, and invalid signature cases for isValidSignedData.
func TestIsValidSignedData(t *testing.T) {
	require := require.New(t)
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	testSigner, err := noopsigner.NewNoopSigner(privKey)
	require.NoError(err)
	proposerAddr, err := testSigner.GetAddress()
	require.NoError(err)
	gen := genesispkg.NewGenesis(
		"testchain",
		1,
		time.Now(),
		proposerAddr,
	)
	m := &Manager{
		signer:  testSigner,
		genesis: gen,
	}

	t.Run("valid signed data", func(t *testing.T) {
		batch := &types.Data{
			Txs: types.Txs{types.Tx("tx1"), types.Tx("tx2")},
		}
		sig, err := m.getDataSignature(batch)
		require.NoError(err)
		pubKey, err := m.signer.GetPublic()
		require.NoError(err)
		signedData := &types.SignedData{
			Data:      *batch,
			Signature: sig,
			Signer: types.Signer{
				PubKey:  pubKey,
				Address: proposerAddr,
			},
		}
		assert.True(t, m.isValidSignedData(signedData))
	})

	t.Run("nil signed data", func(t *testing.T) {
		assert.False(t, m.isValidSignedData(nil))
	})

	t.Run("nil Txs", func(t *testing.T) {
		signedData := &types.SignedData{
			Data: types.Data{},
			Signer: types.Signer{
				Address: proposerAddr,
			},
		}
		signedData.Txs = nil
		assert.False(t, m.isValidSignedData(signedData))
	})

	t.Run("wrong proposer address", func(t *testing.T) {
		batch := &types.Data{
			Txs: types.Txs{types.Tx("tx1")},
		}
		sig, err := m.getDataSignature(batch)
		require.NoError(err)
		pubKey, err := m.signer.GetPublic()
		require.NoError(err)
		wrongAddr := make([]byte, len(proposerAddr))
		copy(wrongAddr, proposerAddr)
		wrongAddr[0] ^= 0xFF // flip a bit
		signedData := &types.SignedData{
			Data:      *batch,
			Signature: sig,
			Signer: types.Signer{
				PubKey:  pubKey,
				Address: wrongAddr,
			},
		}
		assert.False(t, m.isValidSignedData(signedData))
	})

	t.Run("invalid signature", func(t *testing.T) {
		batch := &types.Data{
			Txs: types.Txs{types.Tx("tx1")},
		}
		sig, err := m.getDataSignature(batch)
		require.NoError(err)
		pubKey, err := m.signer.GetPublic()
		require.NoError(err)
		// Corrupt the signature
		badSig := make([]byte, len(sig))
		copy(badSig, sig)
		badSig[0] ^= 0xFF
		signedData := &types.SignedData{
			Data:      *batch,
			Signature: badSig,
			Signer: types.Signer{
				PubKey:  pubKey,
				Address: proposerAddr,
			},
		}
		assert.False(t, m.isValidSignedData(signedData))
	})
}

// TestManager_execValidate tests the execValidate method for various header/data/state conditions.
func TestManager_execValidate(t *testing.T) {
	require := require.New(t)
	genesis, _, _ := types.GetGenesisWithPrivkey("TestChain")
	m, _ := getManager(t, nil, -1, -1)

	// Helper to create a valid state/header/data triplet
	makeValid := func() (types.State, *types.SignedHeader, *types.Data, crypto.PrivKey) {
		state := types.State{
			Version:         types.Version{Block: 1, App: 1},
			ChainID:         genesis.ChainID,
			InitialHeight:   genesis.InitialHeight,
			LastBlockHeight: genesis.InitialHeight - 1,
			LastBlockTime:   time.Now().Add(-time.Minute),
			AppHash:         []byte("apphash"),
		}
		newHeight := state.LastBlockHeight + 1
		// Build header and data
		header, data, privKey := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 1}, state.ChainID, state.AppHash)
		require.NotNil(header)
		require.NotNil(data)
		require.NotNil(privKey)
		return state, header, data, privKey
	}

	t.Run("valid header and data", func(t *testing.T) {
		state, header, data, _ := makeValid()
		err := m.execValidate(state, header, data)
		require.NoError(err)
	})

	t.Run("invalid header (ValidateBasic fails)", func(t *testing.T) {
		state, header, data, _ := makeValid()
		header.ProposerAddress = []byte("bad") // breaks proposer address check
		err := m.execValidate(state, header, data)
		require.ErrorContains(err, "invalid header")
	})

	t.Run("header/data mismatch (types.Validate fails)", func(t *testing.T) {
		state, header, data, _ := makeValid()
		data.Metadata.ChainID = "otherchain" // breaks types.Validate
		err := m.execValidate(state, header, data)
		require.ErrorContains(err, "validation failed")
	})

	t.Run("chain ID mismatch", func(t *testing.T) {
		state, header, data, _ := makeValid()
		state.ChainID = "wrongchain"
		err := m.execValidate(state, header, data)
		require.ErrorContains(err, "chain ID mismatch")
	})

	t.Run("height mismatch", func(t *testing.T) {
		state, header, data, _ := makeValid()
		state.LastBlockHeight += 2
		err := m.execValidate(state, header, data)
		require.ErrorContains(err, "invalid height")
	})

	t.Run("non-monotonic block time at height 1 does not error", func(t *testing.T) {
		state, header, data, _ := makeValid()
		state.LastBlockTime = header.Time()
		err := m.execValidate(state, header, data)
		require.NoError(err)
	})

	t.Run("non-monotonic block time with height > 1", func(t *testing.T) {
		state, header, data, privKey := makeValid()
		state.LastBlockTime = time.Now().Add(time.Minute)
		state.LastBlockHeight = 1
		header.BaseHeader.Height = state.LastBlockHeight + 1
		data.Metadata.Height = state.LastBlockHeight + 1
		signer, err := noopsigner.NewNoopSigner(privKey)
		require.NoError(err)
		header.Signature, err = types.GetSignature(header.Header, signer)
		require.NoError(err)
		err = m.execValidate(state, header, data)
		require.ErrorContains(err, "block time must be strictly increasing")
	})

	t.Run("app hash mismatch", func(t *testing.T) {
		state, header, data, _ := makeValid()
		state.AppHash = []byte("different")
		err := m.execValidate(state, header, data)
		require.ErrorContains(err, "appHash mismatch in delayed execution mode")
	})
}

// TestGetterMethods tests simple getter methods for the Manager
func TestGetterMethods(t *testing.T) {
	t.Run("GetLastState", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		state := types.State{ChainID: "test", LastBlockHeight: 5}
		m.SetLastState(state)

		result := m.GetLastState()
		require.Equal(state, result)
	})

	t.Run("GetDAIncludedHeight", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		m.daIncludedHeight.Store(10)

		result := m.GetDAIncludedHeight()
		require.Equal(uint64(10), result)
	})

	t.Run("PendingHeaders", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		result := m.PendingHeaders()
		require.Nil(result)
		require.Equal(m.pendingHeaders, result)
	})

	t.Run("SeqClient", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		result := m.SeqClient()
		require.Equal(m.sequencer, result)
	})

	t.Run("GetExecutor", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		result := m.GetExecutor()
		require.Equal(m.exec, result)
	})
}

// TestCacheMethods tests cache-related functionality in the Manager
func TestCacheMethods(t *testing.T) {
	t.Run("HeaderCache", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		cache := m.HeaderCache()
		require.NotNil(cache)
		require.Equal(m.headerCache, cache)
	})

	t.Run("DataCache", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		cache := m.DataCache()
		require.NotNil(cache)
		require.Equal(m.dataCache, cache)
	})

	t.Run("IsBlockHashSeen", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		hash := "test-hash"

		// Initially not seen
		require.False(m.IsBlockHashSeen(hash))

		// Mark as seen
		m.headerCache.SetSeen(hash)
		require.True(m.IsBlockHashSeen(hash))
	})
}

// TestUtilityFunctions tests standalone utility functions in the Manager
func TestUtilityFunctions(t *testing.T) {
	t.Run("ConvertBatchDataToBytes_EdgeCases", func(t *testing.T) {
		require := require.New(t)

		// Single empty byte slice
		input := [][]byte{{}}
		result := convertBatchDataToBytes(input)
		require.NotEmpty(result)
		reconstructed, err := bytesToBatchData(result)
		require.NoError(err)
		require.Equal(input, reconstructed)

		// Single non-empty byte slice
		input = [][]byte{[]byte("test")}
		result = convertBatchDataToBytes(input)
		require.NotEmpty(result)
		reconstructed, err = bytesToBatchData(result)
		require.NoError(err)
		require.Equal(input, reconstructed)

		// Multiple mixed entries
		input = [][]byte{[]byte("first"), {}, []byte("third")}
		result = convertBatchDataToBytes(input)
		reconstructed, err = bytesToBatchData(result)
		require.NoError(err)
		require.Equal(input, reconstructed)
	})

	t.Run("ExponentialBackoff", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		m.config.DA.BlockTime.Duration = 10 * time.Second

		// Test initial backoff
		result := m.exponentialBackoff(0)
		require.Equal(initialBackoff, result)

		// Test doubling
		result = m.exponentialBackoff(100 * time.Millisecond)
		require.Equal(200*time.Millisecond, result)

		// Test max cap
		result = m.exponentialBackoff(20 * time.Second)
		require.Equal(m.config.DA.BlockTime.Duration, result)
	})

	t.Run("GetHeaderSignature_NilSigner", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		m.signer = nil

		header := types.Header{}
		_, err := m.getHeaderSignature(header)
		require.ErrorContains(err, "signer is nil; cannot sign header")
	})

	t.Run("IsUsingExpectedSingleSequencer", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		// Create genesis data for the test
		genesisData, privKey, _ := types.GetGenesisWithPrivkey("TestIsUsingExpectedSingleSequencer")
		m.genesis = genesisData

		// Create a signer
		testSigner, err := noopsigner.NewNoopSigner(privKey)
		require.NoError(err)

		// Create a properly signed header that will pass ValidateBasic
		header := &types.SignedHeader{
			Header: types.Header{
				ProposerAddress: genesisData.ProposerAddress,
				BaseHeader: types.BaseHeader{
					ChainID: genesisData.ChainID,
					Height:  1,
					Time:    uint64(genesisData.GenesisDAStartTime.UnixNano()),
				},
				DataHash: make([]byte, 32), // Add proper data hash
				AppHash:  make([]byte, 32), // Add proper app hash
			},
			Signer: types.Signer{
				PubKey:  privKey.GetPublic(),
				Address: genesisData.ProposerAddress,
			},
		}

		// Sign the header
		headerBytes, err := header.Header.MarshalBinary()
		require.NoError(err)
		signature, err := testSigner.Sign(headerBytes)
		require.NoError(err)
		header.Signature = signature

		// Should return true for valid header with correct proposer
		require.True(m.isUsingExpectedSingleSequencer(header))

		// Should return false for header with wrong proposer address
		header.ProposerAddress = []byte("wrong-proposer")
		require.False(m.isUsingExpectedSingleSequencer(header))
	})
}

// TestErrorHandling tests error paths in Manager methods
func TestErrorHandling(t *testing.T) {
	t.Run("GetStoreHeight_Error", func(t *testing.T) {
		require := require.New(t)
		m, mockStore := getManager(t, mocks.NewMockDA(t), -1, -1)
		expectedErr := errors.New("store error")
		mockStore.On("Height", mock.Anything).Return(uint64(0), expectedErr)

		_, err := m.GetStoreHeight(context.Background())
		require.True(errors.Is(err, expectedErr))
	})

	t.Run("IsDAIncluded_StoreError", func(t *testing.T) {
		require := require.New(t)
		m, mockStore := getManager(t, mocks.NewMockDA(t), -1, -1)
		height := uint64(1)
		expectedErr := errors.New("store height error")
		mockStore.On("Height", mock.Anything).Return(uint64(0), expectedErr)

		result, err := m.IsDAIncluded(context.Background(), height)
		require.False(result)
		require.True(errors.Is(err, expectedErr))
	})

	t.Run("IsDAIncluded_HeightTooHigh", func(t *testing.T) {
		m, mockStore := getManager(t, mocks.NewMockDA(t), -1, -1)
		height := uint64(10)
		mockStore.On("Height", mock.Anything).Return(uint64(5), nil)

		result, err := m.IsDAIncluded(context.Background(), height)
		assert.False(t, result)
		assert.NoError(t, err)
	})

	t.Run("IsDAIncluded_GetBlockDataError", func(t *testing.T) {
		require := require.New(t)
		m, mockStore := getManager(t, mocks.NewMockDA(t), -1, -1)
		height := uint64(5)
		expectedErr := errors.New("get block data error")
		mockStore.On("Height", mock.Anything).Return(uint64(10), nil)
		mockStore.On("GetBlockData", mock.Anything, height).Return(nil, nil, expectedErr)

		result, err := m.IsDAIncluded(context.Background(), height)
		require.False(result)
		require.True(errors.Is(err, expectedErr))
	})

	t.Run("RetrieveBatch_SequencerError", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		// Set up genesis for chain ID
		genesisData, _, _ := types.GetGenesisWithPrivkey("TestRetrieveBatch")
		m.genesis = genesisData

		// Create a mock sequencer that returns an error
		mockSequencer := mocks.NewMockSequencer(t)
		expectedErr := errors.New("sequencer error")
		mockSequencer.On("GetNextBatch", mock.Anything, mock.Anything).Return(nil, expectedErr)
		m.sequencer = mockSequencer

		_, err := m.retrieveBatch(context.Background())
		require.True(errors.Is(err, expectedErr))
	})
}

// TestStateManagement tests state-related functionality and thread safety
func TestStateManagement(t *testing.T) {
	t.Run("SetLastState_ThreadSafety", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		var wg sync.WaitGroup
		states := []types.State{
			{ChainID: "test1", LastBlockHeight: 1},
			{ChainID: "test2", LastBlockHeight: 2},
			{ChainID: "test3", LastBlockHeight: 3},
		}

		// Concurrent writes
		for _, state := range states {
			wg.Add(1)
			go func(s types.State) {
				defer wg.Done()
				m.SetLastState(s)
			}(state)
		}

		wg.Wait()

		// Should have one of the states
		result := m.GetLastState()
		require.Contains([]string{"test1", "test2", "test3"}, result.ChainID)
	})

	t.Run("GetLastBlockTime", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		testTime := time.Now()
		state := types.State{
			ChainID:       "test",
			LastBlockTime: testTime,
		}
		m.SetLastState(state)

		result := m.getLastBlockTime()
		require.Equal(testTime, result)
	})

	t.Run("GetLastBlockTime_ThreadSafety", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		testTime := time.Now()
		state := types.State{
			ChainID:       "test",
			LastBlockTime: testTime,
		}
		m.SetLastState(state)

		var wg sync.WaitGroup
		results := make([]time.Time, 10)

		// Concurrent reads
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index] = m.getLastBlockTime()
			}(i)
		}

		wg.Wait()

		// All reads should return the same time
		for _, result := range results {
			require.Equal(testTime, result)
		}
	})
}

// TestNotificationSystem tests the transaction notification system
func TestNotificationSystem(t *testing.T) {
	t.Run("NotifyNewTransactions", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		// Should be able to notify without blocking
		m.NotifyNewTransactions()

		// Should handle multiple notifications gracefully
		for i := 0; i < 10; i++ {
			m.NotifyNewTransactions()
		}

		// Channel should have at least one notification
		select {
		case <-m.TxNotifyCh().Ch():
			// Successfully received notification
		case <-time.After(100 * time.Millisecond):
			require.Fail("Expected notification but got none")
		}
	})

	t.Run("NotifyNewTransactions_NonBlocking", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		// Fill the channel to test non-blocking behavior
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		m.TxNotifyCh().Send(ctx)
		cancel()

		// This should not block even though channel is full
		start := time.Now()
		m.NotifyNewTransactions()
		duration := time.Since(start)

		// Should complete quickly (non-blocking)
		require.Less(duration, 10*time.Millisecond)
	})
}

// TestValidationMethods tests validation logic in the Manager
func TestValidationMethods(t *testing.T) {
	t.Run("GetStoreHeight_Success", func(t *testing.T) {
		require := require.New(t)
		m, mockStore := getManager(t, mocks.NewMockDA(t), -1, -1)
		expectedHeight := uint64(42)
		mockStore.On("Height", mock.Anything).Return(expectedHeight, nil)

		height, err := m.GetStoreHeight(context.Background())
		require.NoError(err)
		require.Equal(expectedHeight, height)
	})

	t.Run("IsBlockHashSeen_True", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		hash := "test-hash"

		// Mark as seen first
		m.headerCache.SetSeen(hash)

		result := m.IsBlockHashSeen(hash)
		require.True(result)
	})

	t.Run("IsBlockHashSeen_False", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)
		hash := "unseen-hash"

		result := m.IsBlockHashSeen(hash)
		require.False(result)
	})

	t.Run("ExponentialBackoff_WithConfig", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		// Set up a config with a specific block time
		m.config.DA.BlockTime.Duration = 5 * time.Second

		// Test that backoff is capped at config value
		result := m.exponentialBackoff(10 * time.Second)
		require.Equal(5*time.Second, result)

		// Test normal doubling
		result = m.exponentialBackoff(100 * time.Millisecond)
		require.Equal(200*time.Millisecond, result)
	})
}

// TestConfigurationDefaults tests default value handling and edge cases
func TestConfigurationDefaults(t *testing.T) {
	t.Run("ExponentialBackoff_EdgeCases", func(t *testing.T) {
		require := require.New(t)
		m, _ := getManager(t, mocks.NewMockDA(t), -1, -1)

		// Test with zero block time - should still double the backoff since there's no cap
		m.config.DA.BlockTime.Duration = 0
		result := m.exponentialBackoff(100 * time.Millisecond)
		require.Equal(0*time.Millisecond, result) // Capped at 0

		// Test with very small block time
		m.config.DA.BlockTime.Duration = 1 * time.Millisecond
		result = m.exponentialBackoff(100 * time.Millisecond)
		require.Equal(1*time.Millisecond, result) // Should cap at block time

		// Test normal doubling with larger block time
		m.config.DA.BlockTime.Duration = 1 * time.Second
		result = m.exponentialBackoff(100 * time.Millisecond)
		require.Equal(200*time.Millisecond, result) // Should double
	})

	t.Run("IsProposer_NilSigner", func(t *testing.T) {
		require := require.New(t)
		privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
		require.NoError(err)

		result, err := isProposer(nil, privKey.GetPublic())
		require.NoError(err)
		require.False(result)
	})

	t.Run("ConvertBatchDataToBytes_NilInput", func(t *testing.T) {
		require := require.New(t)

		result := convertBatchDataToBytes(nil)
		require.Empty(result)

		// Should be able to convert back
		reconstructed, err := bytesToBatchData(result)
		require.NoError(err)
		require.Empty(reconstructed)
	})

	t.Run("BytesToBatchData_CorruptedData_InsufficientLengthBytes", func(t *testing.T) {
		require := require.New(t)

		// Only 3 bytes when we need 4 for length prefix
		bad := []byte{0, 0, 0}
		_, err := bytesToBatchData(bad)
		require.Error(err)
		require.Contains(err.Error(), "corrupted data")
		require.Contains(err.Error(), "insufficient bytes for length prefix")
	})
}

// TestSetRollkitHeightToDAHeight tests the SetRollkitHeightToDAHeight method which maps
// rollkit block heights to their corresponding DA heights for both headers and data.
// This method is critical for tracking which DA heights contain specific rollkit blocks.
func TestSetRollkitHeightToDAHeight(t *testing.T) {
	t.Run("Success_WithTransactions", func(t *testing.T) {
		require := require.New(t)
		ctx := context.Background()
		m, mockStore := getManager(t, nil, 0, 0)

		// Create a block with transactions
		header, data := types.GetRandomBlock(5, 3, "testchain")
		height := uint64(5)

		// Mock store expectations
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

		// Set DA included heights in cache
		headerHeight := uint64(10)
		dataHeight := uint64(20)
		m.headerCache.SetDAIncluded(header.Hash().String(), headerHeight)
		m.dataCache.SetDAIncluded(data.DACommitment().String(), dataHeight)

		// Mock metadata storage
		headerKey := fmt.Sprintf("%s/%d/h", storepkg.RollkitHeightToDAHeightKey, height)
		dataKey := fmt.Sprintf("%s/%d/d", storepkg.RollkitHeightToDAHeightKey, height)
		mockStore.On("SetMetadata", mock.Anything, headerKey, mock.Anything).Return(nil)
		mockStore.On("SetMetadata", mock.Anything, dataKey, mock.Anything).Return(nil)

		// Call the method
		err := m.SetRollkitHeightToDAHeight(ctx, height)
		require.NoError(err)

		mockStore.AssertExpectations(t)
	})

	t.Run("Success_EmptyTransactions", func(t *testing.T) {
		require := require.New(t)
		ctx := context.Background()
		m, mockStore := getManager(t, nil, 0, 0)

		// Create a block with no transactions
		header, data := types.GetRandomBlock(5, 0, "testchain")
		height := uint64(5)

		// Mock store expectations
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

		// Only set header as DA included (data uses special empty hash)
		headerHeight := uint64(10)
		m.headerCache.SetDAIncluded(header.Hash().String(), headerHeight)
		// Note: we don't set data in cache for empty transactions

		// Mock metadata storage - both should use header height for empty transactions
		headerKey := fmt.Sprintf("%s/%d/h", storepkg.RollkitHeightToDAHeightKey, height)
		dataKey := fmt.Sprintf("%s/%d/d", storepkg.RollkitHeightToDAHeightKey, height)
		mockStore.On("SetMetadata", mock.Anything, headerKey, mock.Anything).Return(nil)
		mockStore.On("SetMetadata", mock.Anything, dataKey, mock.Anything).Return(nil)

		// Call the method
		err := m.SetRollkitHeightToDAHeight(ctx, height)
		require.NoError(err)

		mockStore.AssertExpectations(t)
	})

	t.Run("Error_HeaderNotInCache", func(t *testing.T) {
		require := require.New(t)
		ctx := context.Background()
		m, mockStore := getManager(t, nil, 0, 0)

		// Create a block
		header, data := types.GetRandomBlock(5, 3, "testchain")
		height := uint64(5)

		// Mock store expectations
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

		// Don't set header in cache, but set data
		m.dataCache.SetDAIncluded(data.DACommitment().String(), uint64(11))

		// Call the method - should fail
		err := m.SetRollkitHeightToDAHeight(ctx, height)
		require.Error(err)
		require.Contains(err.Error(), "header hash")
		require.Contains(err.Error(), "not found in cache")

		mockStore.AssertExpectations(t)
	})

	t.Run("Error_DataNotInCache_NonEmptyTxs", func(t *testing.T) {
		require := require.New(t)
		ctx := context.Background()
		m, mockStore := getManager(t, nil, 0, 0)

		// Create a block with transactions
		header, data := types.GetRandomBlock(5, 3, "testchain")
		height := uint64(5)

		// Mock store expectations
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

		// Set header but not data in cache
		m.headerCache.SetDAIncluded(header.Hash().String(), uint64(10))

		// Mock metadata storage for header (should succeed)
		headerKey := fmt.Sprintf("%s/%d/h", storepkg.RollkitHeightToDAHeightKey, height)
		mockStore.On("SetMetadata", mock.Anything, headerKey, mock.Anything).Return(nil)

		// Call the method - should fail on data lookup
		err := m.SetRollkitHeightToDAHeight(ctx, height)
		require.Error(err)
		require.Contains(err.Error(), "data hash")
		require.Contains(err.Error(), "not found in cache")

		mockStore.AssertExpectations(t)
	})

	t.Run("Error_BlockNotFound", func(t *testing.T) {
		require := require.New(t)
		ctx := context.Background()
		m, mockStore := getManager(t, nil, 0, 0)

		height := uint64(999) // Non-existent height

		// Mock store expectations
		mockStore.On("GetBlockData", mock.Anything, height).Return(nil, nil, errors.New("block not found"))

		// Call the method - should fail
		err := m.SetRollkitHeightToDAHeight(ctx, height)
		require.Error(err)

		mockStore.AssertExpectations(t)
	})
}
