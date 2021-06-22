package abci

import (
	tmproto "github.com/lazyledger/lazyledger-core/proto/tendermint/types"
	tmversion "github.com/lazyledger/lazyledger-core/proto/tendermint/version"
	"github.com/lazyledger/optimint/types"
	"time"
)

func ToABCIHeader(header *types.Header) tmproto.Header {
	return tmproto.Header{
		Version: tmversion.Consensus{
			Block: uint64(header.Version.Block),
			App:   uint64(header.Version.App),
		},
		ChainID: "", // TODO(tzdybal)
		Height: int64(header.Height),
		Time:    time.Unix(int64(header.Time), 0),
		LastBlockId: tmproto.BlockID{
			Hash: nil,
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
		EvidenceHash:       nil,
		ProposerAddress:    header.ProposerAddress,
	}
}
