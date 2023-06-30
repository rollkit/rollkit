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
		// {
		// 	prepare: func() libhead.Header {
		// 		return untrustedNonAdj
		// 	},
		// 	err: false,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrusted := *untrustedAdj
		// 		untrusted.ValidatorsHash = tmrand.Bytes(32)
		// 		return &untrusted
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrusted := *untrustedNonAdj
		// 		untrusted.Commit = NewTestSuite(t, 2).Commit(RandRawHeader(t))
		// 		return &untrusted
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrusted := *untrustedAdj
		// 		untrusted.RawHeader.LastBlockID.Hash = tmrand.Bytes(32)
		// 		return &untrusted
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrustedAdj.RawHeader.Time = untrustedAdj.RawHeader.Time.Add(time.Minute)
		// 		return untrustedAdj
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrustedAdj.RawHeader.Time = untrustedAdj.RawHeader.Time.Truncate(time.Hour)
		// 		return untrustedAdj
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrustedAdj.RawHeader.ChainID = "toaster"
		// 		return untrustedAdj
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrustedAdj.RawHeader.Height++
		// 		return untrustedAdj
		// 	},
		// 	err: true,
		// },
		// {
		// 	prepare: func() libhead.Header {
		// 		untrustedAdj.RawHeader.Version.App = appconsts.LatestVersion + 1
		// 		return untrustedAdj
		// 	},
		// 	err: true,
		// },
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
