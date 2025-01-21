package types

import (
	"crypto/sha256"

	"github.com/cometbft/cometbft/crypto/merkle"
	cmtypes "github.com/cometbft/cometbft/types"
)

var (
	// EmptyEvidenceHash is the hash of an empty EvidenceData
	EmptyEvidenceHash = new(cmtypes.EvidenceData).Hash()
)

// Hash returns hash of the header
func (h *Header) Hash() Hash {
	bytes, err := h.MarshalBinary()
	if err != nil {
		return nil
	}
	hash := sha256.Sum256(bytes)
	return hash[:]

	//abciHeader := cmtypes.Header{
	//	Version: cmversion.Consensus{
	//		Block: h.Version.Block,
	//		App:   h.Version.App,
	//	},
	//	Height: int64(h.Height()), //nolint:gosec
	//	Time:   h.Time(),
	//	LastBlockID: cmtypes.BlockID{
	//		Hash: cmbytes.HexBytes(h.LastHeaderHash),
	//		PartSetHeader: cmtypes.PartSetHeader{
	//			Total: 0,
	//			Hash:  nil,
	//		},
	//	},
	//	LastCommitHash:  cmbytes.HexBytes(h.LastCommitHash),
	//	DataHash:        cmbytes.HexBytes(h.DataHash),
	//	ConsensusHash:   cmbytes.HexBytes(h.ConsensusHash),
	//	AppHash:         cmbytes.HexBytes(h.AppHash),
	//	LastResultsHash: cmbytes.HexBytes(h.LastResultsHash),
	//	EvidenceHash:    EmptyEvidenceHash,
	//	ProposerAddress: h.ProposerAddress,
	//	// Backward compatibility
	//	ValidatorsHash:     cmbytes.HexBytes(h.ValidatorHash),
	//	NextValidatorsHash: cmbytes.HexBytes(h.ValidatorHash),
	//	ChainID:            h.ChainID(),
	//}
	//return Hash(abciHeader.Hash())
}

// Hash returns hash of the Data
func (d *Data) Hash() Hash {
	// Ignoring the marshal error for now to satify the go-header interface
	// Later on the usage of Hash should be replaced with DA commitment
	dBytes, _ := d.MarshalBinary()
	return merkle.HashFromByteSlices([][]byte{
		dBytes,
	})
}
