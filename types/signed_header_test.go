package types

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerify(t *testing.T) {
	trusted, privKey, err := GetRandomSignedHeader()
	require.NoError(t, err)
	time.Sleep(time.Second)
	untrustedAdj, err := GetNextRandomHeader(trusted, privKey)
	require.NoError(t, err)
	tests := []struct {
		prepare func() (*SignedHeader, bool)
		err     bool
	}{
		{
			prepare: func() (*SignedHeader, bool) { return untrustedAdj, false },
			err:     false,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.AggregatorsHash = GetRandomBytes(32)
				return &untrusted, true
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.LastHeaderHash = GetRandomBytes(32)
				return &untrusted, true
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.LastCommitHash = GetRandomBytes(32)
				return &untrusted, true
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				// Checks for non-adjacency
				untrusted := *untrustedAdj
				untrusted.Header.BaseHeader.Height++
				return &untrusted, true
			},
			err: false, // Accepts non-adjacent headers
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Header.BaseHeader.Time = uint64(untrusted.Header.Time().Truncate(time.Hour).UnixNano())
				return &untrusted, true
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Header.BaseHeader.Time = uint64(untrusted.Header.Time().Add(time.Minute).UnixNano())
				return &untrusted, true
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.BaseHeader.ChainID = "toaster"
				return &untrusted, false // Signature verification should fail
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.Version.App = untrusted.Version.App + 1
				return &untrusted, false // Signature verification should fail
			},
			err: true,
		},
		{
			prepare: func() (*SignedHeader, bool) {
				untrusted := *untrustedAdj
				untrusted.ProposerAddress = nil
				return &untrusted, true
			},
			err: true,
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
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
