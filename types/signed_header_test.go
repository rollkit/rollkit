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
		prepare func() *SignedHeader
		err     bool
	}{
		{
			prepare: func() *SignedHeader { return untrustedAdj },
			err:     false,
		},
		{
			prepare: func() *SignedHeader {
				untrusted := *untrustedAdj
				untrusted.AggregatorsHash = GetRandomBytes(32)
				return &untrusted
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrusted := *untrustedAdj
				untrusted.LastHeaderHash = GetRandomBytes(32)
				return &untrusted
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrusted := *untrustedAdj
				untrusted.LastCommitHash = GetRandomBytes(32)
				return &untrusted
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrustedAdj.Header.BaseHeader.Height++
				return untrustedAdj
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrustedAdj.Header.BaseHeader.Time = uint64(untrustedAdj.Header.Time().Truncate(time.Hour).Unix())
				return untrustedAdj
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrustedAdj.Header.BaseHeader.Time = uint64(untrustedAdj.Header.Time().Add(time.Minute).Unix())
				return untrustedAdj
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrustedAdj.BaseHeader.ChainID = "toaster"
				return untrustedAdj
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrustedAdj.Version.App = untrustedAdj.Version.App + 1
				return untrustedAdj
			},
			err: true,
		},
		{
			prepare: func() *SignedHeader {
				untrustedAdj.ProposerAddress = nil
				return untrustedAdj
			},
			err: true,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			err := trusted.Verify(test.prepare())
			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
