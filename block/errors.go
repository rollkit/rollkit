package block

import (
	"errors"
	"fmt"
)

// These errors are used by Manager.
var (
	// ErrNoValidatorsInState is used when no validators/proposers are found in state
	ErrNoValidatorsInState = errors.New("no validators found in state")

	// ErrNotProposer is used when the manager is not a proposer
	ErrNotProposer = errors.New("not a proposer")
)

// SaveBlockError is returned on failure to save block data
type SaveBlockError struct {
	Err error
}

func (e SaveBlockError) Error() string {
	return fmt.Sprintf("failed to save block: %v", e.Err)
}

func (e SaveBlockError) Unwrap() error {
	return e.Err
}
