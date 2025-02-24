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
		// 2. Test MaxBytes cannot be 0
		{
			name: "MaxBytes cannot be 0",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: 0,
						MaxGas:   123456,
					},
				}
			},
			err: ErrMaxBytesCannotBeZero,
		},
		// 3. Test MaxBytes invalid
		{
			name: "MaxBytes invalid",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: -2,
						MaxGas:   123456,
					},
				}
			},
			err: ErrMaxBytesInvalid,
		},
		// 4. Test MaxBytes too big
		{
			name: "MaxBytes too big",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: cmtypes.MaxBlockSizeBytes + 1,
						MaxGas:   123456,
					},
				}
			},
			err: ErrMaxBytesTooBig,
		},
		// 5. Test MaxGas invalid
		{
			name: "MaxGas invalid",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: 1000,
						MaxGas:   -2,
					},
				}
			},
			err: ErrMaxGasInvalid,
		},
		// 6. Test VoteExtensionsEnableHeight negative
		{
			name: "VoteExtensionsEnableHeight negative",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: 12345,
						MaxGas:   6543234,
					},
					ABCI: cmtypes.ABCIParams{
						VoteExtensionsEnableHeight: -1,
					},
				}
			},
			err: ErrVoteExtensionsEnableHeightNegative,
		},
		// 7. Test PubKeyTypes empty
		{
			name: "PubKeyTypes empty",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: 12345,
						MaxGas:   6543234,
					},
					Validator: cmtypes.ValidatorParams{
						PubKeyTypes: []string{},
					},
				}
			},
			err: ErrPubKeyTypesEmpty,
		},
		// 8. Test PubKeyTypes unknown
		{
			name: "PubKeyTypes unknown",
			prepare: func() cmtypes.ConsensusParams {
				return cmtypes.ConsensusParams{
					Block: cmtypes.BlockParams{
						MaxBytes: 12345,
						MaxGas:   6543234,
					},
					Validator: cmtypes.ValidatorParams{
						PubKeyTypes: []string{"unknownType"},
					},
				}
			},
			err: ErrPubKeyTypeUnknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ConsensusParamsValidateBasic(tc.prepare())
			assert.ErrorIs(t, err, tc.err)
		})
	}
}
