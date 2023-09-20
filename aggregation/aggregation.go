package aggregation

import (
	"github.com/rollkit/rollkit/types"
)

const (
	Ok uint = iota
	Junk
	ConsensusFault
)

type Aggregation interface {
	CheckSafetyInvariant(newBlock *types.Block, oldBlocks []*types.Block) uint
	//ApplyForkChoiceRule(blocks []*types.Block) (*types.Block, error)
}
