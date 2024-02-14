package types

import (
	"errors"
	"fmt"

	cmtypes "github.com/cometbft/cometbft/types"
)

var (
	// ErrMaxBytesCannotBeZero indicates that the block's MaxBytes cannot be 0.
	ErrMaxBytesCannotBeZero = errors.New("block.MaxBytes cannot be 0")

	// ErrMaxBytesInvalid indicates that the block's MaxBytes must be either -1 or greater than 0.
	ErrMaxBytesInvalid = errors.New("block.MaxBytes must be -1 or greater than 0")

	// ErrMaxBytesTooBig indicates that the block's MaxBytes exceeds the maximum allowed size.
	// See https://github.com/cometbft/cometbft/blob/2cd0d1a33cdb6a2c76e6e162d892624492c26290/types/params.go#L16
	ErrMaxBytesTooBig = errors.New("block.MaxBytes is bigger than the maximum permitted block size")

	// ErrMaxGasInvalid indicates that the block's MaxGas must be greater than or equal to -1.
	ErrMaxGasInvalid = errors.New("block.MaxGas must be greater or equal to -1")

	// ErrVoteExtensionsEnableHeightNegative indicates that the ABCI.VoteExtensionsEnableHeight cannot be negative.
	ErrVoteExtensionsEnableHeightNegative = errors.New("ABCI.VoteExtensionsEnableHeight cannot be negative")

	// ErrPubKeyTypesEmpty indicates that the Validator.PubKeyTypes length must be greater than 0.
	ErrPubKeyTypesEmpty = errors.New("len(Validator.PubKeyTypes) must be greater than 0")

	// ErrPubKeyTypeUnknown indicates an encounter with an unknown public key type.
	ErrPubKeyTypeUnknown = errors.New("unknown pubkey type")
)

// ConsensusParamsValidateBasic validates the ConsensusParams to ensure all values are within their
// allowed limits, and returns an error if they are not.
func ConsensusParamsValidateBasic(params cmtypes.ConsensusParams) error {
	if params.Block.MaxBytes == 0 {
		return ErrMaxBytesCannotBeZero
	}
	if params.Block.MaxBytes < -1 {
		return fmt.Errorf("%w: Got %d",
			ErrMaxBytesInvalid,
			params.Block.MaxBytes)
	}
	if params.Block.MaxBytes > cmtypes.MaxBlockSizeBytes {
		return fmt.Errorf("%w: %d > %d",
			ErrMaxBytesTooBig,
			params.Block.MaxBytes, cmtypes.MaxBlockSizeBytes)
	}

	if params.Block.MaxGas < -1 {
		return fmt.Errorf("%w: Got %d",
			ErrMaxGasInvalid,
			params.Block.MaxGas)
	}

	if params.ABCI.VoteExtensionsEnableHeight < 0 {
		return fmt.Errorf("%w: Got: %d",
			ErrVoteExtensionsEnableHeightNegative,
			params.ABCI.VoteExtensionsEnableHeight)
	}

	if len(params.Validator.PubKeyTypes) == 0 {
		return ErrPubKeyTypesEmpty
	}

	// Check if keyType is a known ABCIPubKeyType
	for i, keyType := range params.Validator.PubKeyTypes {
		if _, ok := cmtypes.ABCIPubKeyTypesToNames[keyType]; !ok {
			return fmt.Errorf("%w: params.Validator.PubKeyTypes[%d], %s,",
				ErrPubKeyTypeUnknown,
				i, keyType)
		}
	}

	return nil
}
