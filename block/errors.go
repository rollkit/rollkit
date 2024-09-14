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

// SaveBlockError is used when failed to save block
type SaveBlockError struct {
	Err error
}

func (e SaveBlockError) Error() string {
	return fmt.Sprintf("failed to save block: %v", e.Err)
}

func (e SaveBlockError) Unwrap() error {
	return e.Err
}

// SaveBlockResponsesError is used when failed to save block responses
type SaveBlockResponsesError struct {
	Err error
}

func (e SaveBlockResponsesError) Error() string {
	return fmt.Sprintf("failed to save block responses: %v", e.Err)
}

func (e SaveBlockResponsesError) Unwrap() error {
	return e.Err
}
