package types

import (
	"github.com/celestiaorg/go-header"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tmtypes "github.com/tendermint/tendermint/types"
)

// Hash returns ABCI-compatible hash of a header.
func (h *Header) Hash() header.Hash {
	abciHeader := tmtypes.Header{
		Version: tmversion.Consensus{
			Block: h.Version.Block,
			App:   h.Version.App,
		},
		Height: int64(h.Height()),
		Time:   h.Time(),
		LastBlockID: tmtypes.BlockID{
			Hash: ConvertToHexBytes(h.LastHeaderHash[:]),
			PartSetHeader: tmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     ConvertToHexBytes(h.LastCommitHash[:]),
		DataHash:           ConvertToHexBytes(h.DataHash[:]),
		ValidatorsHash:     ConvertToHexBytes(h.AggregatorsHash[:]),
		NextValidatorsHash: nil,
		ConsensusHash:      ConvertToHexBytes(h.ConsensusHash[:]),
		AppHash:            ConvertToHexBytes(h.AppHash[:]),
		LastResultsHash:    ConvertToHexBytes(h.LastResultsHash[:]),
		EvidenceHash:       new(tmtypes.EvidenceData).Hash(),
		ProposerAddress:    h.ProposerAddress,
		ChainID:            h.ChainID(),
	}
	return Hash(abciHeader.Hash())
}

// Hash returns ABCI-compatible hash of a block.
func (b *Block) Hash() Hash {
	var hash Hash
	copy(hash[:], b.Header.Hash())
	return hash
}
