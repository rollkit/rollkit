package abci

import (
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/types"
)

// MinimalToABCIHeaderPB converts Rollkit minimal header to Header format defined in ABCI.
func MinimalToABCIHeaderPB(header *types.MinimalHeader) (cmtproto.Header, error) {
	// Create a default version since MinimalHeader doesn't contain version info
	version := cmversion.Consensus{
		Block: 0,
		App:   0,
	}

	// Prepare empty hashes for required ABCI header fields not present in MinimalHeader
	emptyHash := make([]byte, 32)

	return cmtproto.Header{
		Version: version,
		Height:  int64(header.Height()), //nolint:gosec
		Time:    header.Time(),
		LastBlockId: cmtproto.BlockID{
			Hash: header.ParentHash[:],
			PartSetHeader: cmtproto.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.ParentHash[:],
		DataHash:           header.DataCommitment[:],
		ConsensusHash:      emptyHash,
		AppHash:            header.StateRoot[:],
		LastResultsHash:    emptyHash,
		EvidenceHash:       emptyHash,
		ProposerAddress:    header.ExtraData,
		ChainID:            header.ChainID(),
		ValidatorsHash:     emptyHash,
		NextValidatorsHash: emptyHash,
	}, nil
}

// MinimalToABCIHeader converts Rollkit minimal header to Header format defined in ABCI.
func MinimalToABCIHeader(header *types.MinimalHeader) (cmtypes.Header, error) {
	// Create a default version since MinimalHeader doesn't contain version info
	version := cmversion.Consensus{
		Block: 0,
		App:   0,
	}

	// Prepare empty hashes for required ABCI header fields not present in MinimalHeader
	emptyHash := cmbytes.HexBytes(make([]byte, 32))

	return cmtypes.Header{
		Version: version,
		Height:  int64(header.Height()), //nolint:gosec
		Time:    header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.ParentHash[:]),
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     cmbytes.HexBytes(header.ParentHash[:]),
		DataHash:           cmbytes.HexBytes(header.DataCommitment[:]),
		ConsensusHash:      emptyHash,
		AppHash:            cmbytes.HexBytes(header.StateRoot[:]),
		LastResultsHash:    emptyHash,
		EvidenceHash:       emptyHash,
		ProposerAddress:    header.ExtraData,
		ChainID:            header.ChainID(),
		ValidatorsHash:     emptyHash,
		NextValidatorsHash: emptyHash,
	}, nil
}

// MinimalToABCIBlock converts minimal header and data into block format defined by ABCI.
func MinimalToABCIBlock(header *types.SignedMinimalHeader, data *types.Data) (*cmtypes.Block, error) {
	abciHeader, err := MinimalToABCIHeader(&header.Header)
	if err != nil {
		return nil, err
	}

	// For simplicity, assume a single validator
	val := make([]byte, 20) // typical validator address size

	// Create a basic commit with the header's signature
	abciCommit := &cmtypes.Commit{
		Height: int64(header.Header.Height()), //nolint:gosec
		Round:  0,
		BlockID: cmtypes.BlockID{
			Hash:          cmbytes.HexBytes(header.Header.Hash()),
			PartSetHeader: cmtypes.PartSetHeader{},
		},
		Signatures: []cmtypes.CommitSig{
			{
				BlockIDFlag:      cmtypes.BlockIDFlagCommit,
				ValidatorAddress: val,
				Timestamp:        header.Header.Time(),
				Signature:        header.Signature,
			},
		},
	}

	abciBlock := cmtypes.Block{
		Header: abciHeader,
		Evidence: cmtypes.EvidenceData{
			Evidence: nil,
		},
		LastCommit: abciCommit,
	}

	// Convert the transactions
	abciBlock.Data.Txs = make([]cmtypes.Tx, len(data.Txs))
	for i := range data.Txs {
		abciBlock.Data.Txs[i] = cmtypes.Tx(data.Txs[i])
	}

	return &abciBlock, nil
}
