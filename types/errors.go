package types

import "errors"

var (
	ErrInvalidNamespaceId          = errors.New("invalid lenght of 'NamespaceId'")
	ErrInvalidLastHeaderHash       = errors.New("invalid lenght of 'InvalidLastHeaderHash'")
	ErrInvalidLastCommitHeaderHash = errors.New("invalid lenght of 'InvalidLastCommit.HeaderHash'")
	ErrInvalidLastCommitHash       = errors.New("invalid lenght of 'InvalidLastCommitHash'")
	ErrInvalidDataHash             = errors.New("invalid lenght of 'InvalidDataHash'")
	ErrInvalidConsensusHash        = errors.New("invalid lenght of 'InvalidConsensusHash'")
	ErrInvalidAppHash              = errors.New("invalid lenght of 'InvalidAppHash'")
	ErrInvalidLastResultsHash      = errors.New("invalid lenght of 'InvalidLastResultsHash'")
	ErrInvalidProposerAddress      = errors.New("invalid lenght of 'InvalidProposerAddress'")
)
