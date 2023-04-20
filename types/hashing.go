package types

import (
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtypes "github.com/cometbft/cometbft/types"
)

// Hash returns ABCI-compatible hash of a header.
func (h *Header) Hash() Hash {
	abciHeader := cmtypes.Header{
		Version: cmversion.Consensus{
			Block: h.Version.Block,
			App:   h.Version.App,
		},
		Height: int64(h.Height()),
		Time:   h.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(h.LastHeaderHash),
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     cmbytes.HexBytes(h.LastCommitHash),
		DataHash:           cmbytes.HexBytes(h.DataHash),
		ValidatorsHash:     cmbytes.HexBytes(h.AggregatorsHash),
		NextValidatorsHash: nil,
		ConsensusHash:      cmbytes.HexBytes(h.ConsensusHash),
		AppHash:            cmbytes.HexBytes(h.AppHash),
		LastResultsHash:    cmbytes.HexBytes(h.LastResultsHash),
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    h.ProposerAddress,
		ChainID:            h.ChainID(),
	}
	return Hash(abciHeader.Hash())
}

// Hash returns ABCI-compatible hash of a block.
func (b *Block) Hash() Hash {
	return b.SignedHeader.Header.Hash()
}
