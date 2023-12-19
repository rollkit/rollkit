package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlock_ValidateBasic(t *testing.T) {
	validBlock := GetRandomBlock(1, 0)
	err := validBlock.ValidateBasic()
	require.NoError(t, err)

	blockWithJunkProposer := validBlock
	blockWithJunkProposer.SignedHeader.ProposerAddress = GetRandomBytes(32)
	err = blockWithJunkProposer.ValidateBasic()
	require.Error(t, err)

	blockWithInvalidSig := validBlock
	blockWithInvalidSig.SignedHeader.Commit.Signatures[0] = GetRandomBytes(64)
	err = blockWithInvalidSig.ValidateBasic()
	require.Error(t, err)
}
