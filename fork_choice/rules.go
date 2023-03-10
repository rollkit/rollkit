package fork_choice

import "github.com/rollkit/rollkit/types"

const (
	//FirstOrderedCentralized = "FOC"
	FirstOrderedPure = "FOP"
	//HighestGasPure = "HGP"
	//Astria = "ASTRIA"
)

var rules = map[string]func([]*types.Block) (*types.Block, bool){
	FirstOrderedPure: ApplyFirstOrderedPure,
}

func GetRule(ruleName string) func([]*types.Block) (*types.Block, bool) {
	return rules[ruleName]
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
	if len(candidates) > 0 && candidates[0] != nil {
		return candidates[0], true
	} else {
		return &types.Block{}, false
	}
}
