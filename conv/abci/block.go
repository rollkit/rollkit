package abci

import (
	"time"

	tmproto "github.com/lazyledger/lazyledger-core/proto/tendermint/types"
	tmversion "github.com/lazyledger/lazyledger-core/proto/tendermint/version"

	"github.com/lazyledger/optimint/hash"
	"github.com/lazyledger/optimint/types"
)

// ToABCIHeader converts Optimint header to Header format defined in ABCI.
func ToABCIHeader(header *types.Header) (tmproto.Header, error) {
	h, err := hash.Hash(header)
	if err != nil {
		return tmproto.Header{}, err
	}
	return tmproto.Header{
		Version: tmversion.Consensus{
			Block: uint64(header.Version.Block),
			App:   uint64(header.Version.App),
		},
		ChainID: "", // TODO(tzdybal)
		Height:  int64(header.Height),
		Time:    time.Unix(int64(header.Time), 0),
		LastBlockId: tmproto.BlockID{
			Hash: h[:],
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
	}, nil
}
