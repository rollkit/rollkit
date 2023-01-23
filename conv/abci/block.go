package abci

import (
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/types"
)

// ToABCIHeaderPB converts Rollkit header to Header format defined in ABCI.
// Caller should fill all the fields that are not available in Rollkit header (like ChainID).
func ToABCIHeaderPB(header *types.Header) (tmproto.Header, error) {
	return tmproto.Header{
		Version: tmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockId: tmproto.BlockID{
			Hash: header.LastHeaderHash[:],
			PartSetHeader: tmproto.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.LastCommitHash[:],
		DataHash:           header.DataHash[:],
		ValidatorsHash:     header.AggregatorsHash[:],
		NextValidatorsHash: nil,
		ConsensusHash:      header.ConsensusHash[:],
		AppHash:            header.AppHash[:],
		LastResultsHash:    header.LastResultsHash[:],
		EvidenceHash:       new(tmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}, nil
}

// ToABCIHeader converts Rollkit header to Header format defined in ABCI.
// Caller should fill all the fields that are not available in Rollkit header (like ChainID).
func ToABCIHeader(header *types.Header) (tmtypes.Header, error) {
	return tmtypes.Header{
		Version: tmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockID: tmtypes.BlockID{
			Hash: tmbytes.HexBytes(header.LastHeaderHash),
			PartSetHeader: tmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     tmbytes.HexBytes(header.LastCommitHash),
		DataHash:           tmbytes.HexBytes(header.DataHash),
		ValidatorsHash:     tmbytes.HexBytes(header.AggregatorsHash),
		NextValidatorsHash: nil,
		ConsensusHash:      tmbytes.HexBytes(header.ConsensusHash),
		AppHash:            tmbytes.HexBytes(header.AppHash),
		LastResultsHash:    tmbytes.HexBytes(header.LastResultsHash),
		EvidenceHash:       new(tmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}, nil
}

// ToABCIBlock converts Rolkit block into block format defined by ABCI.
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
	abciBlock.Header.DataHash = tmbytes.HexBytes(block.Header.DataHash)

	return &abciBlock, nil
}

// ToABCIBlockMeta converts Rollkit block into BlockMeta format defined by ABCI
func ToABCIBlockMeta(block *types.Block) (*tmtypes.BlockMeta, error) {
	tmblock, err := ToABCIBlock(block)
	if err != nil {
		return nil, err
	}
	blockID := tmtypes.BlockID{Hash: tmblock.Hash()}

	return &tmtypes.BlockMeta{
		BlockID:   blockID,
		BlockSize: tmblock.Size(),
		Header:    tmblock.Header,
		NumTxs:    len(tmblock.Txs),
	}, nil
}

// ToABCICommit converts Rollkit commit into commit format defined by ABCI.
// This function only converts fields that are available in Rollkit commit.
// Other fields (especially ValidatorAddress and Timestamp of Signature) has to be filled by caller.
func ToABCICommit(commit *types.Commit) *tmtypes.Commit {
	tmCommit := tmtypes.Commit{
		Height: int64(commit.Height),
		Round:  0,
		BlockID: tmtypes.BlockID{
			Hash:          tmbytes.HexBytes(commit.HeaderHash),
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
