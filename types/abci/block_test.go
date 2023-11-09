package abci

import (
	cmbytes "github.com/cometbft/cometbft/libs/bytes"

	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"testing"

	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/types"

	"github.com/stretchr/testify/assert"
)

func TestToABCIHeaderPB(t *testing.T) {
	header := types.GetRandomHeader()
	expected := cmproto.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockId: cmproto.BlockID{
			Hash: header.LastHeaderHash[:],
			PartSetHeader: cmproto.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     header.LastHeaderHash[:],
		DataHash:           header.DataHash[:],
		ValidatorsHash:     header.AggregatorsHash[:],
		NextValidatorsHash: nil,
		ConsensusHash:      header.ConsensusHash[:],
		AppHash:            header.AppHash[:],
		LastResultsHash:    header.LastResultsHash[:],
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}

	actual, err := ToABCIHeaderPB(&header)
	if err != nil {
		t.Fatalf("ToABCIHeaderPB returned an error: %v", err)
	}

	assert.Equal(t, expected, actual)
}

func TestToABCIHeader(t *testing.T) {
	header := types.GetRandomHeader()
	expected := cmtypes.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()),
		Time:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.LastHeaderHash[:]),
			PartSetHeader: cmtypes.PartSetHeader{
				Total: 0,
				Hash:  nil,
			},
		},
		LastCommitHash:     cmbytes.HexBytes(header.LastCommitHash),
		DataHash:           cmbytes.HexBytes(header.DataHash),
		ValidatorsHash:     cmbytes.HexBytes(header.AggregatorsHash),
		NextValidatorsHash: nil,
		ConsensusHash:      cmbytes.HexBytes(header.ConsensusHash),
		AppHash:            cmbytes.HexBytes(header.AppHash),
		LastResultsHash:    cmbytes.HexBytes(header.LastResultsHash),
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
	}

	actual, err := ToABCIHeader(&header)
	if err != nil {
		t.Fatalf("ToABCIHeaderPB returned an error: %v", err)
	}

	assert.Equal(t, expected, actual)
}

func TestToABCIBlock(t *testing.T) {
	blockHeight, nTxs := uint64(1), 2
	block := types.GetRandomBlock(blockHeight, nTxs)
	abciHeader, err := ToABCIHeader(&block.SignedHeader.Header)
	if err != nil {
		t.Fatal(err)
	}

	abciCommit := block.SignedHeader.Commit.ToABCICommit(block.Height(), block.Hash())

	if len(abciCommit.Signatures) == 1 {
		abciCommit.Signatures[0].ValidatorAddress = block.SignedHeader.ProposerAddress
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
	abciBlock.Header.DataHash = cmbytes.HexBytes(block.SignedHeader.DataHash)

	actual, err := ToABCIBlock(block)
	if err != nil {
		t.Fatalf("ToABCIBlock returned an error: %v", err)
	}
	assert.Equal(t, &abciBlock, actual)
}

func TestToABCIBlockMeta(t *testing.T) {
	blockHeight, nTxs := uint64(1), 2
	block := types.GetRandomBlock(blockHeight, nTxs)
	cmblock, err := ToABCIBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	blockID := cmtypes.BlockID{Hash: cmblock.Hash()}

	expected := &cmtypes.BlockMeta{
		BlockID:   blockID,
		BlockSize: cmblock.Size(),
		Header:    cmblock.Header,
		NumTxs:    len(cmblock.Txs),
	}

	actual, err := ToABCIBlockMeta(block)
	if err != nil {
		t.Fatalf("ToABCIBlock returned an error: %v", err)
	}
	assert.Equal(t, expected, actual)

}
