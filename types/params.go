package types

import (
	"errors"
	"fmt"

	cmtypes "github.com/cometbft/cometbft/types"
)

// ConsensusParamsValidateBasic validates the ConsensusParams to ensure all values are within their
// allowed limits, and returns an error if they are not.
func ConsensusParamsValidateBasic(params cmtypes.ConsensusParams) error {
	if params.Block.MaxBytes == 0 {
		return fmt.Errorf("block.MaxBytes cannot be 0")
	}
	if params.Block.MaxBytes < -1 {
		return fmt.Errorf("block.MaxBytes must be -1 or greater than 0. Got %d",

			params.Block.MaxBytes)
	}
	if params.Block.MaxBytes > cmtypes.MaxBlockSizeBytes {
		return fmt.Errorf("block.MaxBytes is too big. %d > %d",
			params.Block.MaxBytes, cmtypes.MaxBlockSizeBytes)
	}

	if params.Block.MaxGas < -1 {
		return fmt.Errorf("block.MaxGas must be greater or equal to -1. Got %d",
			params.Block.MaxGas)
	}

	if params.ABCI.VoteExtensionsEnableHeight < 0 {
		return fmt.Errorf("ABCI.VoteExtensionsEnableHeight cannot be negative. Got: %d", params.ABCI.VoteExtensionsEnableHeight)
	}

	if len(params.Validator.PubKeyTypes) == 0 {
		return errors.New("len(Validator.PubKeyTypes) must be greater than 0")
	}

	// Check if keyType is a known ABCIPubKeyType
	for i := 0; i < len(params.Validator.PubKeyTypes); i++ {
		keyType := params.Validator.PubKeyTypes[i]
		if _, ok := cmtypes.ABCIPubKeyTypesToNames[keyType]; !ok {
			return fmt.Errorf("params.Validator.PubKeyTypes[%d], %s, is an unknown pubkey type",
				i, keyType)
		}
	}

	return nil
}
