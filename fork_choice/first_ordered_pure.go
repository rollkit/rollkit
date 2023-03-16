package fork_choice

import "github.com/rollkit/rollkit/types"

var _ ForkChoiceRule = firstOrderedPure{}

type firstOrderedPure struct{}

func (f firstOrderedPure) Apply(candidates []*types.Block) (*types.Block, bool) {
	// Return first non-nil block from the candidates
	for _, b := range candidates {
		if b != nil {
			return b, true
		}
	}
	return nil, false
}
