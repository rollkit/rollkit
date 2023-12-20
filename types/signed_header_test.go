package types

import (
	"fmt"
	"testing"

	"github.com/celestiaorg/go-header"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtypes "github.com/cometbft/cometbft/types"
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
		// 1. Test valid
		// Verify valid adjacent headers
		// Expect success
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     nil,
		},
		// 2. Test invalid LastHeaderHash link
		// break the LastHeaderHash link between the trusted and untrusted header
		// Expect failure
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
		// 3. Test LastCommitHash link between trusted and untrusted header
		// break the LastCommitHash link between the trusted and untrusted header
		// Expect failure
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
		// 4. Test non-adjacent
		// increments the BaseHeader.Height so it's unexpected
		// Expect failure
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
		// 5. Test proposer verification
		// changes the proposed address to a random address
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Header.ProposerAddress = GetRandomBytes(32)
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrProposerVerificationFailed,
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
		// 1. Test valid
		// Validate block
		// Expect success
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     nil,
		},
		// 2. Test chain ID changed
		// breaks signature verification by changing the chain ID
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.BaseHeader.ChainID = "toaster"
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		// 3. Test app version changed
		// breaks signature verification by changing app version
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Version.App = untrusted.Version.App + 1
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		// 4. Test invalid signature fails
		// breaks signature verification by changing the signature
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Commit.Signatures[0] = GetRandomBytes(32)
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		// 5. Test nil proposer address
		// Sets the proposer address to nil
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = nil
				return &untrusted, true
			},
			err: ErrNoProposerAddress,
		},
		// 6. Test invalid validator set length
		// Set the validator set length to be not one
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				v1Key, v2Key := ed25519.GenPrivKey(), ed25519.GenPrivKey()
				validators := []*cmtypes.Validator{
					{
						Address:          v1Key.PubKey().Address(),
						PubKey:           v1Key.PubKey(),
						VotingPower:      int64(50),
						ProposerPriority: int64(1),
					},
					{
						Address:          v2Key.PubKey().Address(),
						PubKey:           v2Key.PubKey(),
						VotingPower:      int64(50),
						ProposerPriority: int64(1),
					},
				}
				untrusted.Validators = cmtypes.NewValidatorSet(validators)
				return &untrusted, true
			},
			err: ErrInvalidValidatorSetLengthMismatch,
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
