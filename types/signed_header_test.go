package types

import (
	"fmt"
	"testing"

	"github.com/celestiaorg/go-header"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignedHeader(t *testing.T) {
	// Generate a random signed header
	trusted, privKey, err := GetRandomSignedHeader()
	require.NoError(t, err)
	// Get the next random header
	untrustedAdj, err := GetRandomNextSignedHeader(trusted, privKey)
	require.NoError(t, err)
	t.Run("Test Verify", func(t *testing.T) {
		testVerify(t, trusted, untrustedAdj, privKey)
	})
	t.Run("Test ValidateBasic", func(t *testing.T) {
		testValidateBasic(t, untrustedAdj, privKey)
	})
}

func testVerify(t *testing.T, trusted *SignedHeader, untrustedAdj *SignedHeader, privKey ed25519.PrivKey) {
	tests := []struct {
		prepare func() (*SignedHeader, bool) // Function to prepare the test case
		err     error                        // Expected error
	}{
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     nil,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.LastHeaderHash = header.Hash(GetRandomBytes(32))
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrLastHeaderHashMismatch,
			},
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.LastCommitHash = header.Hash(GetRandomBytes(32))
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrLastCommitHashMismatch,
			},
		},
		{
			prepare: func() (*SignedHeader, bool) {
				// Checks for non-adjacency
				untrusted := *untrustedAdj
				untrusted.Header.BaseHeader.Height++
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrNonAdjacentHeaders,
			},
		},
	}

	for testIndex, test := range tests {
		t.Run(fmt.Sprintf("Test #%d", testIndex), func(t *testing.T) {
			preparedHeader, shouldRecomputeCommit := test.prepare()

			if shouldRecomputeCommit {
				commit, err := getCommit(preparedHeader.Header, privKey)
				require.NoError(t, err)
				preparedHeader.Commit = *commit
			}

			err := trusted.Verify(preparedHeader)

			if test.err == nil {
				assert.NoError(t, err)
				return
			}

			if err == nil {
				t.Errorf("expected error: %v, but got nil", test.err)
				return
			}

			reason := err.(*header.VerifyError).Reason
			expectedReason := test.err.(*header.VerifyError).Reason
			assert.ErrorIs(t, reason, expectedReason)
		})
	}
}

func testValidateBasic(t *testing.T, untrustedAdj *SignedHeader, privKey ed25519.PrivKey) {
	// Define test cases
	tests := []struct {
		prepare func() (*SignedHeader, bool) // Function to prepare the test case
		err     error                        // Expected error
	}{
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     nil,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.AggregatorsHash = header.Hash(GetRandomBytes(32))
				return &untrusted, false
			},
			err: ErrAggregatorSetHashMismatch,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.BaseHeader.ChainID = "toaster"
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Version.App = untrusted.Version.App + 1
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = nil
				return &untrusted, true
			},
			err: ErrNoProposerAddress,
		},
	}

	for testIndex, test := range tests {
		t.Run(fmt.Sprintf("Test #%d", testIndex), func(t *testing.T) {
			preparedHeader, shouldRecomputeCommit := test.prepare()

			if shouldRecomputeCommit {
				commit, err := getCommit(preparedHeader.Header, privKey)
				require.NoError(t, err)
				preparedHeader.Commit = *commit
			}

			err := preparedHeader.ValidateBasic()

			if test.err == nil {
				assert.NoError(t, err)
				return
			}

			if err == nil {
				t.Errorf("expected error: %v, but got nil", test.err)
				return
			}

			assert.ErrorIs(t, err, test.err)
		})
	}
}
