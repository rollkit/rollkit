package da

import "github.com/lazyledger/optimint/types"

// TODO define an enum of different non-happy-path cases
// that might need to be handled by Optimint independent of
// the underlying DA chain.
type StatusCode uint64

const (
	StatusSuccess StatusCode = iota
	StatusError
)

type ResultSubmitBlock struct {
	// Code is to determine if the action succeeded.
	Code StatusCode
	// Not sure if this needs to be bubbled up to other
	// parts of Optimint.
	// Hash hash.Hash
}

type DataAvailabilityLayerClient interface {
	Start() error
	Stop() error

	// SubmitBlock submits the passed in block to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlock(block *types.Block) ResultSubmitBlock
}
