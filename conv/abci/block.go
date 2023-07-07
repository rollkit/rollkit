package abci

import (
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/types"
)

// ToABCIHeaderPB converts Rollkit header to Header format defined in ABCI.
// Caller should fill all the fields that are not available in Rollkit header (like ChainID).
func ToABCIHeaderPB(header *types.Header) (cmproto.Header, error) {
	return cmproto.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockId: cmproto.BlockID{
			Hash: header.ToProto().LastHeaderHash[:],
			PartSetHeader: cmproto.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.ToProto().LastHeaderHash[:],
		DataHash:           header.ToProto().DataHash[:],
		ValidatorsHash:     header.ToProto().AggregatorsHash[:],
		NextValidatorsHash: nil,
		ConsensusHash:      header.ToProto().ConsensusHash[:],
		AppHash:            header.ToProto().AppHash[:],
		LastResultsHash:    header.ToProto().LastResultsHash[:],
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}, nil
}

// ToABCIHeader converts Rollkit header to Header format defined in ABCI.
// Caller should fill all the fields that are not available in Rollkit header (like ChainID).
func ToABCIHeader(header *types.Header) (cmtypes.Header, error) {
	return cmtypes.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.ToProto().LastHeaderHash),
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     cmbytes.HexBytes(header.ToProto().LastCommitHash),
		DataHash:           cmbytes.HexBytes(header.ToProto().DataHash),
		ValidatorsHash:     cmbytes.HexBytes(header.ToProto().AggregatorsHash),
		NextValidatorsHash: nil,
		ConsensusHash:      cmbytes.HexBytes(header.ToProto().ConsensusHash),
		AppHash:            cmbytes.HexBytes(header.ToProto().AppHash),
		LastResultsHash:    cmbytes.HexBytes(header.ToProto().LastResultsHash),
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}, nil
}

// ToABCIBlock converts Rolkit block into block format defined by ABCI.
// Returned block should pass `ValidateBasic`.
func ToABCIBlock(block *types.Block) (*cmtypes.Block, error) {
	abciHeader, err := ToABCIHeader(&block.SignedHeader.Header)
	if err != nil {
		return nil, err
	}
	abciCommit := block.SignedHeader.Commit.ToABCICommit(int64(block.SignedHeader.Header.BaseHeader.Height), block.SignedHeader.Hash())
	// This assumes that we have only one signature
	if len(abciCommit.Signatures) == 1 {
		abciCommit.Signatures[0].ValidatorAddress = block.SignedHeader.Header.ProposerAddress
	}
	abciBlock := cmtypes.Block{
		Header: abciHeader,
		Evidence: cmtypes.EvidenceData{
			Evidence: nil,
		},
		LastCommit: abciCommit,
	}
	abciBlock.Data.Txs = make([]cmtypes.Tx, len(block.Data.Txs))
	for i := range block.Data.Txs {
		abciBlock.Data.Txs[i] = cmtypes.Tx(block.Data.Txs[i])
	}
	abciBlock.Header.DataHash = cmbytes.HexBytes(block.SignedHeader.Header.DataHash)

	return &abciBlock, nil
}

// ToABCIBlockMeta converts Rollkit block into BlockMeta format defined by ABCI
func ToABCIBlockMeta(block *types.Block) (*cmtypes.BlockMeta, error) {
	cmblock, err := ToABCIBlock(block)
	if err != nil {
		return nil, err
	}
	blockID := cmtypes.BlockID{Hash: cmblock.Hash()}

	return &cmtypes.BlockMeta{
		BlockID:   blockID,
		BlockSize: cmblock.Size(),
		Header:    cmblock.Header,
		NumTxs:    len(cmblock.Txs),
	}, nil
}
