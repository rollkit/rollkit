package aggregation

import (
	"fmt"

	"bytes"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/rollkit/rollkit/types"
)

var _ Aggregation = CentralizedAggregation{}

type CentralizedAggregation struct {
	AggregatorAddress []byte
}

func NewCentralizedAggregation(genesis *cmtypes.GenesisDoc) (Aggregation, error) {
	if genesis == nil {
		return nil, fmt.Errorf("genesis can't be nil for centralized sequencer")
	}
	if len(genesis.Validators) != 1 {
		return nil, fmt.Errorf("number of validators in genesis != 1")
	}
	return CentralizedAggregation{
		AggregatorAddress: genesis.Validators[0].Address.Bytes(),
	}, nil
}

func (s CentralizedAggregation) CheckSafetyInvariant(newBlock *types.Block, oldBlocks []*types.Block) uint {
	if !bytes.Equal(newBlock.SignedHeader.Header.ProposerAddress, s.AggregatorAddress) {
		return Junk
	}
	if len(oldBlocks) > 0 {
		return ConsensusFault
	}
	return Ok
}

/*func (s CentralizedAggregation) ApplyForkChoiceRule(blocks []*types.Block) (*types.Block, error) {
	// Centralized sequencers shouldn't ever fork.
	if len(blocks) > 1 {
		return nil, fmt.Errorf("apparent safety violation")
	}
	return blocks[0], nil
}*/
