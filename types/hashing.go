package types

import (
	"github.com/cometbft/cometbft/crypto/merkle"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtypes "github.com/cometbft/cometbft/types"
)

var (
	// EmptyEvidenceHash is the hash of an empty EvidenceData
	EmptyEvidenceHash = new(cmtypes.EvidenceData).Hash()
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
		LastCommitHash:  cmbytes.HexBytes(h.LastCommitHash),
		DataHash:        cmbytes.HexBytes(h.DataHash),
		ConsensusHash:   cmbytes.HexBytes(h.ConsensusHash),
		AppHash:         cmbytes.HexBytes(h.AppHash),
		LastResultsHash: cmbytes.HexBytes(h.LastResultsHash),
		EvidenceHash:    EmptyEvidenceHash,
		ProposerAddress: h.ProposerAddress,
		// Backward compatibility
		ValidatorsHash:     cmbytes.HexBytes(h.ValidatorHash),
		NextValidatorsHash: cmbytes.HexBytes(h.ValidatorHash),
		ChainID:            h.ChainID(),
	}
	return Hash(abciHeader.Hash())
}

// Hash returns hash of the Data
func (d *Data) Hash() Hash {
	// Ignoring the marshal error for now to satisfy the go-header interface
	// Later on the usage of Hash should be replaced with DA commitment
	dBytes, _ := d.MarshalBinary()
	return merkle.HashFromByteSlices([][]byte{
		dBytes,
	})
}
