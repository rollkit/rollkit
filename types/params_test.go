package types

import (
	"testing"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/assert"
)

func TestConsensusParamsValidateBasic(t *testing.T) {
	testCases := []struct {
		name    string
		prepare func() cmtypes.ConsensusParams // Function to prepare the test case
		err     error
	}{
		// 1. Test valid params
		{
			name: "valid params",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: 12345,
						MaxGas:   6543234,
					},
					Validator: cmtypes.ValidatorParams{
						PubKeyTypes: []string{cmtypes.ABCIPubKeyTypeEd25519},
					},
					Version: cmtypes.VersionParams{
						App: 42,
					},
				}
			},
			err: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ConsensusParamsValidateBasic(tc.prepare())
			assert.ErrorIs(t, err, tc.err)
		})
	}
}
