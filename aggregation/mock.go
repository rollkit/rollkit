package aggregation

import (
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/types"
)

var _ Aggregation = MockAggregation{}

func NewMockAggregation(genesis *cmtypes.GenesisDoc) (Aggregation, error) {
	return MockAggregation{}, nil
}

type MockAggregation struct{}

func (s MockAggregation) CheckSafetyInvariant(newBlock *types.Block, oldBlocks []*types.Block) uint {
	return Ok
}

/*func (s MockAggregation) ApplyForkChoiceRule(blocks []*types.Block) (*types.Block, error) {
	return blocks[0], nil
}*/
