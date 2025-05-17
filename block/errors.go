package block

import (
	"errors"
)

// These errors are used by Manager.
var (
	// ErrNoValidatorsInState is used when no validators/proposers are found in state
	ErrNoValidatorsInState = errors.New("no validators found in state")

	// ErrNotProposer is used when the manager is not a proposer
	ErrNotProposer = errors.New("not a proposer")

	// ErrNoBatch indicate no batch is available for creating block
	ErrNoBatch = errors.New("no batch to process")

	// ErrHeightFromFutureStr is the error message for height from future returned by da
	ErrHeightFromFutureStr = errors.New("given height is from the future")
)
