package fork_choice

import (
	"github.com/rollkit/rollkit/types"
)

const (
	//FirstOrderedCentralized = "FOC"
	FirstOrderedPure = "FOP"
	//HighestGasPure = "HGP"
	//Astria = "ASTRIA"
)

var rules = map[string]func([]*types.Block) (*types.Block, bool){
	FirstOrderedPure: ApplyFirstOrderedPure,
}

func GetRule(ruleName string) (func([]*types.Block) (*types.Block, bool), bool) {
	fcr, ok := rules[ruleName]
	return fcr, ok
}

// func ApplyFirstOrderedCentralized {
/* Need to validate signature from aggregator
	 * but blocks don't appear to currently have a
	 * proposer signature... they just have LastCommit
	 * which is the previous block.
	 * For now, this just selects the first without checking the signature.
	 * https://github.com/rollkit/rollkit/issues/735
}*/

func ApplyFirstOrderedPure(candidates []*types.Block) (*types.Block, bool) {
	// Return first non-nil block from the candidates
	for _, b := range candidates {
		if b != nil {
			return b, true
		}
	}
	return nil, false
}
