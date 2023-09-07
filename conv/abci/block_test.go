package abci

import (
	cmbytes "github.com/cometbft/cometbft/libs/bytes"

	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"testing"
	"time"

	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
)

func TestToABCIHeaderPB(t *testing.T) {
	header := &types.Header{
		BaseHeader: types.BaseHeader{
			Height:  12,
			Time:    uint64(time.Now().Local().Day()),
			ChainID: "test",
		},
		Version: types.Version{
			Block: 1,
			App:   2,
		},
		LastHeaderHash:  types.GetRandomBytes(32),
		LastCommitHash:  types.GetRandomBytes(32),
		DataHash:        types.GetRandomBytes(32),
		ConsensusHash:   types.GetRandomBytes(32),
		AppHash:         types.GetRandomBytes(32),
		LastResultsHash: types.GetRandomBytes(32),
		ProposerAddress: types.GetRandomBytes(32),
		AggregatorsHash: types.GetRandomBytes(32),
	}

	expected := cmproto.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockId: cmproto.BlockID{
			Hash: header.LastHeaderHash[:],
			PartSetHeader: cmproto.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.LastHeaderHash[:],
		DataHash:           header.DataHash[:],
		ValidatorsHash:     header.AggregatorsHash[:],
		NextValidatorsHash: nil,
		ConsensusHash:      header.ConsensusHash[:],
		AppHash:            header.AppHash[:],
		LastResultsHash:    header.LastResultsHash[:],
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}

	actual, err := ToABCIHeaderPB(header)
	if err != nil {
		t.Fatalf("ToABCIHeaderPB returned an error: %v", err)
	}

	assert.Equal(t, expected, actual)
}

func TestToABCIHeader(t *testing.T) {
	header := &types.Header{
		BaseHeader: types.BaseHeader{
			Height:  12,
			Time:    uint64(time.Now().Local().Day()),
			ChainID: "test",
		},
		Version: types.Version{
			Block: 1,
			App:   2,
		},
		LastHeaderHash:  types.GetRandomBytes(32),
		LastCommitHash:  types.GetRandomBytes(32),
		DataHash:        types.GetRandomBytes(32),
		ConsensusHash:   types.GetRandomBytes(32),
		AppHash:         types.GetRandomBytes(32),
		LastResultsHash: types.GetRandomBytes(32),
		ProposerAddress: types.GetRandomBytes(32),
		AggregatorsHash: types.GetRandomBytes(32),
	}
	expected := cmtypes.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.LastHeaderHash[:]),
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     cmbytes.HexBytes(header.LastCommitHash),
		DataHash:           cmbytes.HexBytes(header.DataHash),
		ValidatorsHash:     cmbytes.HexBytes(header.AggregatorsHash),
		NextValidatorsHash: nil,
		ConsensusHash:      cmbytes.HexBytes(header.ConsensusHash),
		AppHash:            cmbytes.HexBytes(header.AppHash),
		LastResultsHash:    cmbytes.HexBytes(header.LastResultsHash),
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}

	actual, err := ToABCIHeaderPB(header)
	if err != nil {
		t.Fatalf("ToABCIHeaderPB returned an error: %v", err)
	}

	assert.Equal(t, expected, actual)
}
