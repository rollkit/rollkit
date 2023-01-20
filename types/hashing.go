package types

import (
	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tmtypes "github.com/tendermint/tendermint/types"
)

// Hash returns ABCI-compatible hash of a header.
func (h *Header) Hash() Hash {
	abciHeader := tmtypes.Header{
		Version: tmversion.Consensus{
			Block: h.Version.Block,
			App:   h.Version.App,
		},
		Height: int64(h.Height()),
		Time:   h.Time(),
		LastBlockID: tmtypes.BlockID{
			Hash: tmbytes.HexBytes(h.LastHeaderHash),
			PartSetHeader: tmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     tmbytes.HexBytes(h.LastCommitHash),
		DataHash:           tmbytes.HexBytes(h.DataHash),
		ValidatorsHash:     tmbytes.HexBytes(h.AggregatorsHash),
		NextValidatorsHash: nil,
		ConsensusHash:      tmbytes.HexBytes(h.ConsensusHash),
		AppHash:            tmbytes.HexBytes(h.AppHash),
		LastResultsHash:    tmbytes.HexBytes(h.LastResultsHash),
		EvidenceHash:       new(tmtypes.EvidenceData).Hash(),
		ProposerAddress:    h.ProposerAddress,
		ChainID:            h.ChainID(),
	}
	return Hash(abciHeader.Hash())
}

// Hash returns ABCI-compatible hash of a block.
func (b *Block) Hash() Hash {
	return b.Header.Hash()
}
