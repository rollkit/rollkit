package types

import (
	"encoding"
	tmversion "github.com/tendermint/tendermint/proto/tendermint/version"
	tmtypes "github.com/tendermint/tendermint/types"
	"time"

	"github.com/minio/sha256-simd"
)

func (header *Header) Hash() [32]byte {
	abciHeader := tmtypes.Header{
		Version: tmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height),
		Time:   time.Unix(int64(header.Time), 0),
		LastBlockID: tmtypes.BlockID{
			Hash: header.LastHeaderHash[:],
			PartSetHeader: tmtypes.PartSetHeader{
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
	}
	var hash [32]byte
	copy(hash[:], abciHeader.Hash())
	return hash
}

func (b *Block) Hash() [32]byte {
	return b.Header.Hash()
}

func hash(obj encoding.BinaryMarshaler) [32]byte {
	blob, err := obj.MarshalBinary()
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(blob)

}
