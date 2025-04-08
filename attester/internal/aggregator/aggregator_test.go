package aggregator_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/attester/internal/aggregator"
)

// Helper function to generate ed25519 key pairs for testing
func generateTestKeys(t *testing.T, count int) (map[string]ed25519.PublicKey, map[string]ed25519.PrivateKey) {
	t.Helper()
	pubKeys := make(map[string]ed25519.PublicKey)
	privKeys := make(map[string]ed25519.PrivateKey)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("attester-%d", i)
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		pubKeys[id] = pub
		privKeys[id] = priv
	}
	return pubKeys, privKeys
}

func TestNewSignatureAggregator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)) // Use stdout for test visibility if needed
	pubKeys, _ := generateTestKeys(t, 3)

	testCases := []struct {
		name            string
		quorumThreshold int
		attesterKeys    map[string]ed25519.PublicKey
		expectError     bool
		errorContains   string
	}{
		{
			name:            "Valid",
			quorumThreshold: 2,
			attesterKeys:    pubKeys,
			expectError:     false,
		},
		{
			name:            "InvalidQuorumZero",
			quorumThreshold: 0,
			attesterKeys:    pubKeys,
			expectError:     true,
			errorContains:   "quorum threshold must be positive",
		},
		{
			name:            "InvalidQuorumNegative",
			quorumThreshold: -1,
			attesterKeys:    pubKeys,
			expectError:     true,
			errorContains:   "quorum threshold must be positive",
		},
		{
			name:            "EmptyKeys",
			quorumThreshold: 1,
			attesterKeys:    make(map[string]ed25519.PublicKey),
			expectError:     true,
			errorContains:   "attester keys map cannot be empty",
		},
		{
			name:            "NilKeys",
			quorumThreshold: 1,
			attesterKeys:    nil,
			expectError:     true,
			errorContains:   "attester keys map cannot be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			agg, err := aggregator.NewSignatureAggregator(logger, tc.quorumThreshold, tc.attesterKeys)

			if tc.expectError {
				require.Error(t, err)
				require.Nil(t, agg)
				if tc.errorContains != "" {
					require.ErrorContains(t, err, tc.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, agg)
			}
		})
	}
}

func TestAddSignature_Success_NoQuorum(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	pubKeys, privKeys := generateTestKeys(t, 3)
	quorum := 3
	agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
	require.NoError(t, err)

	blockHeight := uint64(10)
	blockHash := []byte("block_hash_10")
	dataToSign := []byte("some data for block 10")
	attesterID := "attester-0"
	privKey := privKeys[attesterID]

	// Pre-set the data the aggregator needs to verify against
	agg.SetBlockData(blockHash, dataToSign)

	// Sign the data
	signature := ed25519.Sign(privKey, dataToSign)

	// Add the signature
	quorumReached, err := agg.AddSignature(blockHeight, blockHash, attesterID, signature)

	// Assertions
	require.NoError(t, err)
	require.False(t, quorumReached, "Quorum should not be reached with only 1 signature")

	// Verify internal state (optional, depends if needed)
	// _, sigExists := agg.GetAggregatedSignatures(blockHeight)
	// require.False(t, sigExists, "GetAggregatedSignatures should return false before quorum")
}

func TestAddSignature_QuorumReached(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	numAttesters := 3
	quorum := 2
	pubKeys, privKeys := generateTestKeys(t, numAttesters)
	agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
	require.NoError(t, err)

	blockHeight := uint64(11)
	blockHash := []byte("block_hash_11")
	dataToSign := []byte("some data for block 11")

	// Pre-set the data for verification
	agg.SetBlockData(blockHash, dataToSign)

	// Add first signature (attester-0)
	sig0 := ed25519.Sign(privKeys["attester-0"], dataToSign)
	quorumReached0, err0 := agg.AddSignature(blockHeight, blockHash, "attester-0", sig0)
	require.NoError(t, err0)
	require.False(t, quorumReached0, "Quorum should not be reached after 1st signature")

	// Add second signature (attester-1) - Quorum should be met
	sig1 := ed25519.Sign(privKeys["attester-1"], dataToSign)
	quorumReached1, err1 := agg.AddSignature(blockHeight, blockHash, "attester-1", sig1)
	require.NoError(t, err1)
	require.True(t, quorumReached1, "Quorum should be reached after 2nd signature")

	// (Optional) Add third signature (attester-2) - Quorum should remain met
	sig2 := ed25519.Sign(privKeys["attester-2"], dataToSign)
	quorumReached2, err2 := agg.AddSignature(blockHeight, blockHash, "attester-2", sig2)
	require.NoError(t, err2)
	require.True(t, quorumReached2, "Quorum should still be reached after 3rd signature")
}

// TestAddSignature_Errors covers various error scenarios for AddSignature.
func TestAddSignature_Errors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	pubKeys, privKeys := generateTestKeys(t, 2) // Generate keys for known attestors
	quorum := 2

	// Common data used across tests
	blockHeight := uint64(13)
	blockHash := []byte("block_hash_13")
	dataToSign := []byte("correct data for block 13")
	knownAttesterID := "attester-0"
	knownPrivKey := privKeys[knownAttesterID]
	validSignature := ed25519.Sign(knownPrivKey, dataToSign)

	testCases := []struct {
		name            string
		setupAggregator func(t *testing.T) *aggregator.SignatureAggregator // Func to setup state for the test
		attesterID      string
		signature       []byte
		expectedErrMsg  string
	}{
		{
			name: "InvalidSignature",
			setupAggregator: func(t *testing.T) *aggregator.SignatureAggregator {
				agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
				require.NoError(t, err)
				agg.SetBlockData(blockHash, dataToSign) // Pre-set CORRECT data
				return agg
			},
			attesterID:     knownAttesterID,
			signature:      ed25519.Sign(knownPrivKey, []byte("wrong data")), // Sign WRONG data
			expectedErrMsg: "invalid signature",
		},
		{
			name: "UnknownAttesterID",
			setupAggregator: func(t *testing.T) *aggregator.SignatureAggregator {
				agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
				require.NoError(t, err)
				agg.SetBlockData(blockHash, dataToSign)
				return agg
			},
			attesterID:     "unknown-attester", // This ID is not in pubKeys
			signature:      validSignature,
			expectedErrMsg: "unknown attester ID",
		},
		{
			name: "BlockDataNotAvailable",
			setupAggregator: func(t *testing.T) *aggregator.SignatureAggregator {
				// Setup aggregator WITHOUT calling SetBlockData for blockHash
				agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
				require.NoError(t, err)
				return agg
			},
			attesterID:     knownAttesterID,
			signature:      validSignature,
			expectedErrMsg: "data not available",
		},
		{
			name: "InvalidPublicKeySizeInMap", // Edge case: if map somehow contains invalid key
			setupAggregator: func(t *testing.T) *aggregator.SignatureAggregator {
				invalidKeys := map[string]ed25519.PublicKey{
					"bad-key-attester": []byte{1, 2, 3}, // Invalid size
				}
				agg, err := aggregator.NewSignatureAggregator(logger, quorum, invalidKeys)
				require.NoError(t, err) // Constructor doesn't check key size
				// Need to set block data, although verification will fail before using it
				agg.SetBlockData(blockHash, dataToSign)
				return agg
			},
			attesterID:     "bad-key-attester",
			signature:      validSignature, // Signature itself doesn't matter here
			expectedErrMsg: "invalid public key size",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			agg := tc.setupAggregator(t)

			quorumReached, err := agg.AddSignature(blockHeight, blockHash, tc.attesterID, tc.signature)

			require.Error(t, err)
			require.ErrorContains(t, err, tc.expectedErrMsg)
			require.False(t, quorumReached)

			// Verify no signatures were actually stored for this height
			sigs, ok := agg.GetAggregatedSignatures(blockHeight)
			require.False(t, ok)
			require.Empty(t, sigs)
		})
	}
}

func TestAddSignature_DuplicateSignature(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	pubKeys, privKeys := generateTestKeys(t, 3)
	quorum := 2
	agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
	require.NoError(t, err)

	blockHeight := uint64(14)
	blockHash := []byte("block_hash_14")
	dataToSign := []byte("some data for block 14")
	attesterID := "attester-0"
	privKey := privKeys[attesterID]

	agg.SetBlockData(blockHash, dataToSign)
	signature := ed25519.Sign(privKey, dataToSign)

	// Add the signature for the first time
	quorumReached1, err1 := agg.AddSignature(blockHeight, blockHash, attesterID, signature)
	require.NoError(t, err1)
	require.False(t, quorumReached1, "Quorum should not be reached after 1st signature")

	// Try adding the exact same signature again
	quorumReached2, err2 := agg.AddSignature(blockHeight, blockHash, attesterID, signature)

	// Assert: No error, quorum status should be same as after the first call
	require.NoError(t, err2, "Adding a duplicate signature should not return an error")
	require.False(t, quorumReached2, "Quorum status should not change after duplicate")

	// Verify only one signature is stored
	sigs, ok := agg.GetAggregatedSignatures(blockHeight)
	require.False(t, ok, "GetAggregatedSignatures should still return false (quorum not met)")
	require.Len(t, sigs, 0, "Signature list should be empty as quorum is not met")

	// --- Test duplicate after quorum reached ---
	blockHeight2 := uint64(15)
	blockHash2 := []byte("block_hash_15")
	dataToSign2 := []byte("some data for block 15")
	agg.SetBlockData(blockHash2, dataToSign2)
	sig0 := ed25519.Sign(privKeys["attester-0"], dataToSign2)
	sig1 := ed25519.Sign(privKeys["attester-1"], dataToSign2)

	_, err = agg.AddSignature(blockHeight2, blockHash2, "attester-0", sig0)
	require.NoError(t, err)
	qReached1, err := agg.AddSignature(blockHeight2, blockHash2, "attester-1", sig1)
	require.NoError(t, err)
	require.True(t, qReached1, "Quorum should be reached")

	// Add duplicate for attester-1
	qReachedDup, errDup := agg.AddSignature(blockHeight2, blockHash2, "attester-1", sig1)
	require.NoError(t, errDup, "Adding duplicate after quorum should not error")
	require.True(t, qReachedDup, "Quorum should remain true after duplicate")

	// Verify still only 2 signatures are stored
	sigs2, ok2 := agg.GetAggregatedSignatures(blockHeight2)
	require.True(t, ok2, "GetAggregatedSignatures should return true after quorum")
	require.Len(t, sigs2, 2, "Should only have 2 signatures despite duplicate add")
}

func TestGetAggregatedSignatures(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	pubKeys, privKeys := generateTestKeys(t, 3)
	quorum := 2
	agg, err := aggregator.NewSignatureAggregator(logger, quorum, pubKeys)
	require.NoError(t, err)

	blockHeightQuorum := uint64(20)
	blockHashQuorum := []byte("block_hash_20")
	dataQuorum := []byte("data block 20")
	agg.SetBlockData(blockHashQuorum, dataQuorum)

	blockHeightPartial := uint64(21)
	blockHashPartial := []byte("block_hash_21")
	dataPartial := []byte("data block 21")
	agg.SetBlockData(blockHashPartial, dataPartial)

	// Signatures for the block that will reach quorum
	sigQuorum0 := ed25519.Sign(privKeys["attester-0"], dataQuorum)
	sigQuorum1 := ed25519.Sign(privKeys["attester-1"], dataQuorum)

	// Signature for the block that won't reach quorum
	sigPartial0 := ed25519.Sign(privKeys["attester-0"], dataPartial)

	// Add signatures
	_, err = agg.AddSignature(blockHeightQuorum, blockHashQuorum, "attester-0", sigQuorum0)
	require.NoError(t, err)
	_, err = agg.AddSignature(blockHeightQuorum, blockHashQuorum, "attester-1", sigQuorum1)
	require.NoError(t, err)
	_, err = agg.AddSignature(blockHeightPartial, blockHashPartial, "attester-0", sigPartial0)
	require.NoError(t, err)

	t.Run("QuorumMet", func(t *testing.T) {
		sigs, ok := agg.GetAggregatedSignatures(blockHeightQuorum)
		require.True(t, ok, "Expected ok=true when quorum is met")
		require.NotNil(t, sigs)
		require.Len(t, sigs, 2, "Expected 2 signatures when quorum is met")
		// Check if the returned signatures match the ones added (order might vary)
		require.Contains(t, sigs, sigQuorum0)
		require.Contains(t, sigs, sigQuorum1)
	})

	t.Run("QuorumNotMet", func(t *testing.T) {
		sigs, ok := agg.GetAggregatedSignatures(blockHeightPartial)
		require.False(t, ok, "Expected ok=false when quorum is not met")
		require.Nil(t, sigs, "Expected nil slice when quorum is not met") // Function returns nil explicitly
	})

	t.Run("HeightNotFound", func(t *testing.T) {
		unknownHeight := uint64(99)
		sigs, ok := agg.GetAggregatedSignatures(unknownHeight)
		require.False(t, ok, "Expected ok=false for unknown height")
		require.Nil(t, sigs, "Expected nil slice for unknown height")
	})
}

// TODO: Add test for PruneSignatures (if implemented)
