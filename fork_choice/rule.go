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

func GetRule(ruleName string) (ForkChoiceRule, bool) {
	switch ruleName {
	case FirstOrderedPure:
		return firstOrderedPure{}, true
	default:
		return nil, false
	}
}

type ForkChoiceRule interface {
	Apply(candidates []*types.Block) (*types.Block, bool)
}
