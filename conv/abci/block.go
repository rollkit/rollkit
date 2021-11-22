package abci

import (
	"time"

	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/types"
)

// ToABCIHeaderPB converts Optimint header to Header format defined in ABCI.
// Caller should fill all the fields that are not available in Optimint header (like ChainID).
func ToABCIHeaderPB(header *types.Header) (tmproto.Header, error) {
	hash := header.Hash()
	return tmproto.Header{
		Version: tmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height),
		Time:   time.Unix(int64(header.Time), 0),
		LastBlockId: tmproto.BlockID{
			Hash: hash[:],
			PartSetHeader: tmproto.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.LastCommitHash[:],
		DataHash:           header.DataHash[:],
		ValidatorsHash:     nil,
		NextValidatorsHash: nil,
		ConsensusHash:      header.ConsensusHash[:],
		AppHash:            header.AppHash[:],
		LastResultsHash:    header.LastResultsHash[:],
		EvidenceHash:       tmtypes.EvidenceList{}.Hash(),
		ProposerAddress:    header.ProposerAddress,
	}, nil
}

// ToABCIHeader converts Optimint header to Header format defined in ABCI.
// Caller should fill all the fields that are not available in Optimint header (like ChainID).
func ToABCIHeader(header *types.Header) (tmtypes.Header, error) {
	hash := header.Hash()
	return tmtypes.Header{
		Version: tmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height),
		Time:   time.Unix(int64(header.Time), 0),
		LastBlockID: tmtypes.BlockID{
			Hash: hash[:],
			PartSetHeader: tmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.LastCommitHash[:],
		DataHash:           header.DataHash[:],
		ValidatorsHash:     nil,
		NextValidatorsHash: nil,
		ConsensusHash:      header.ConsensusHash[:],
		AppHash:            header.AppHash[:],
		LastResultsHash:    header.LastResultsHash[:],
		EvidenceHash:       tmtypes.EvidenceList{}.Hash(),
		ProposerAddress:    header.ProposerAddress,
	}, nil
}

// ToABCIBlock converts Optimint block into block format defined by ABCI.
// Returned block should pass `ValidateBasic`.
func ToABCIBlock(block *types.Block) (*tmtypes.Block, error) {
	abciHeader, err := ToABCIHeader(&block.Header)
	if err != nil {
		return nil, err
	}
	abciCommit := ToABCICommit(&block.LastCommit)
	// This assumes that we have only one signature
	if len(abciCommit.Signatures) == 1 {
		abciCommit.Signatures[0].ValidatorAddress = block.Header.ProposerAddress
	}
	abciBlock := tmtypes.Block{
		Header: abciHeader,
		Evidence: tmtypes.EvidenceData{
			Evidence: nil,
		},
		LastCommit: abciCommit,
	}
	abciBlock.Data.Txs = make([]tmtypes.Tx, len(block.Data.Txs))
	for i := range block.Data.Txs {
		abciBlock.Data.Txs[i] = tmtypes.Tx(block.Data.Txs[i])
	}
	abciBlock.Header.DataHash = abciBlock.Data.Txs.Hash()

	return &abciBlock, nil
}

// ToABCICommit converts Optimint commit into commit format defined by ABCI.
// This function only converts fields that are available in Optimint commit.
// Other fields (especially ValidatorAddress and Timestamp of Signature) has to be filled by caller.
func ToABCICommit(commit *types.Commit) *tmtypes.Commit {
	tmCommit := tmtypes.Commit{
		Height: int64(commit.Height),
		Round:  0,
		BlockID: tmtypes.BlockID{
			Hash:          commit.HeaderHash[:],
			PartSetHeader: tmtypes.PartSetHeader{},
		},
	}
	for _, sig := range commit.Signatures {
		commitSig := tmtypes.CommitSig{
			BlockIDFlag: tmtypes.BlockIDFlagCommit,
			Signature:   sig,
		}
		tmCommit.Signatures = append(tmCommit.Signatures, commitSig)
	}

	return &tmCommit
}
