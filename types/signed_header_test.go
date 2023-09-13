package types

import (
	"strconv"
	"testing"
	"time"

	"github.com/celestiaorg/go-header"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	trusted, privKey, err := GetRandomSignedHeader()
	require.NoError(t, err)
	time.Sleep(time.Second)
	untrustedAdj, err := GetNextRandomHeader(trusted, privKey)
	require.NoError(t, err)
	fakeAggregatorsHash := header.Hash(GetRandomBytes(32))
	fakeLastHeaderHash := header.Hash(GetRandomBytes(32))
	fakeLastCommitHash := header.Hash(GetRandomBytes(32))
	tests := []struct {
		prepare func() (*SignedHeader, bool)
		err     error
	}{
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     nil,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.AggregatorsHash = fakeAggregatorsHash
				return &untrusted, false
			},
			err: &header.VerifyError{
				Reason: ErrAggregatorSetHashMismatch,
			},
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.LastHeaderHash = fakeLastHeaderHash
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrLastHeaderHashMismatch,
			},
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.LastCommitHash = fakeLastCommitHash
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
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.BaseHeader.ChainID = "toaster"
				return &untrusted, false // Signature verification should fail
			},
			err: &header.VerifyError{
				Reason: ErrSignatureVerificationFailed,
			},
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Version.App = untrusted.Version.App + 1
				return &untrusted, false // Signature verification should fail
			},
			err: &header.VerifyError{
				Reason: ErrSignatureVerificationFailed,
			},
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = nil
				return &untrusted, true
			},
			err: &header.VerifyError{
				Reason: ErrNoProposerAddress,
			},
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			preparedHeader, recomputeCommit := test.prepare()
			if recomputeCommit {
				commit, err := getCommit(preparedHeader.Header, privKey)
				require.NoError(t, err)
				preparedHeader.Commit = *commit
			}
			err = trusted.Verify(preparedHeader)
			if test.err == nil {
				assert.NoError(t, err)
				return
			}
			if err == nil {
				t.Errorf("expected err: %v, got nil", test.err)
				return
			}
			reason := err.(*header.VerifyError).Reason
			testReason := test.err.(*header.VerifyError).Reason
			assert.ErrorIs(t, reason, testReason)
		})
	}
}
