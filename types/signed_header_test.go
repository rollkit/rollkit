package types

import (
	"fmt"
	"testing"

	"github.com/celestiaorg/go-header"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/signer/noop"
)

func TestSignedHeader(t *testing.T) {
	chainID := "TestSignedHeader"
	// Generate a random signed header
	trusted, privKey, err := GetRandomSignedHeader(chainID)
	require.NoError(t, err)

	signer, err := noop.NewNoopSigner(privKey)
	require.NoError(t, err)
	// Get the next random header
	untrustedAdj, err := GetRandomNextSignedHeader(trusted, signer, chainID)
	require.NoError(t, err)
	t.Run("Test Verify", func(t *testing.T) {
		testVerify(t, trusted, untrustedAdj, privKey)
	})
	t.Run("Test ValidateBasic", func(t *testing.T) {
		testValidateBasic(t, untrustedAdj, privKey)
	})
}

func testVerify(t *testing.T, trusted *SignedHeader, untrustedAdj *SignedHeader, privKey crypto.PrivKey) {
	tests := []struct {
		prepare func() (*SignedHeader, bool) // Function to prepare the test case
		err     error                        // Expected error
	}{
		// 0. Test valid
		// Verify valid adjacent headers
		// Expect success
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, true },
			err:     nil,
		},
		// 1. Test invalid LastHeaderHash link
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
		// 2. Test non-adjacent
		// increments the BaseHeader.Height so it's headers are non-adjacent
		// Expect success
		{
			prepare: func() (*SignedHeader, bool) {
				// Checks for non-adjacency
				untrusted := *untrustedAdj
				untrusted.BaseHeader.Height++
				return &untrusted, true
			},
			err: nil,
		},
		// 4. Test proposer verification
		// changes the proposed address to a random address
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = GetRandomBytes(32)
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrProposerVerificationFailed,
			},
		},
		// 5. Test proposer verification for non-adjacent headers
		// changes the proposed address to a random address and updates height
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = GetRandomBytes(32)
				untrusted.BaseHeader.Height++
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
				signature, err := GetSignature(preparedHeader.Header, privKey)
				require.NoError(t, err)
				preparedHeader.Signature = signature
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

			var verifyError *header.VerifyError
			assert.ErrorAs(t, err, &verifyError)
			reason := verifyError.Reason
			assert.ErrorAs(t, test.err, &verifyError)
			expectedReason := verifyError.Reason
			assert.ErrorIs(t, reason, expectedReason)
		})
	}
}

func testValidateBasic(t *testing.T, untrustedAdj *SignedHeader, privKey crypto.PrivKey) {
	// Define test cases
	tests := []struct {
		prepare func() (*SignedHeader, bool) // Function to prepare the test case
		err     error                        // Expected error
	}{
		// 0. Test valid
		// Validate block
		// Expect success
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     nil,
		},
		// 1. Test chain ID changed
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
		// 2. Test app version changed
		// breaks signature verification by changing app version
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.BaseHeader.Height = untrusted.BaseHeader.Height + 1
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		// 3. Test invalid signature fails
		// breaks signature verification by changing the signature
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Signature = GetRandomBytes(32)
				return &untrusted, false // Signature verification should fail
			},
			err: ErrSignatureVerificationFailed,
		},
		// 4. Test nil proposer address
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

		// 5. Test proposer address mismatch between that of signed header and validator set
		// Set the proposer address in the signed header to be different from that of the validator set
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = GetRandomBytes(32)
				return &untrusted, true
			},
			err: ErrProposerAddressMismatch,
		},
		//  6. Test empty signature values in signature list
		// Set the signature to be an empty value in signature list
		// Expect failure
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Signature = Signature{}
				return &untrusted, false
			},
			err: ErrSignatureEmpty,
		},
	}

	for testIndex, test := range tests {
		t.Run(fmt.Sprintf("Test #%d", testIndex), func(t *testing.T) {
			preparedHeader, shouldRecomputeCommit := test.prepare()

			if shouldRecomputeCommit {
				signature, err := GetSignature(preparedHeader.Header, privKey)
				require.NoError(t, err)
				preparedHeader.Signature = signature
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
			if testIndex == 7 {
				assert.Contains(t, err.Error(), test.err.Error())
			} else {
				assert.ErrorIs(t, err, test.err)
			}
		})
	}
}
