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
	header := types.GetRandomHeader("TestToABCIHeaderPB")
	expected := cmproto.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()), //nolint:gosec
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
		ConsensusHash:      header.ConsensusHash[:],
		AppHash:            header.AppHash[:],
		LastResultsHash:    header.LastResultsHash[:],
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
		ValidatorsHash:     header.ValidatorHash,
		NextValidatorsHash: header.ValidatorHash,
	}

	actual, err := ToABCIHeaderPB(&header)
	if err != nil {
		t.Fatalf("ToABCIHeaderPB returned an error: %v", err)
	}

	assert.Equal(t, expected, actual)
}

func TestToABCIHeader(t *testing.T) {
	header := types.GetRandomHeader("TestToABCIHeader")
	expected := cmtypes.Header{
		Version: cmversion.Consensus{
			Block: header.Version.Block,
			App:   header.Version.App,
		},
		Height: int64(header.Height()), //nolint:gosec
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
		ConsensusHash:      cmbytes.HexBytes(header.ConsensusHash),
		AppHash:            cmbytes.HexBytes(header.AppHash),
		LastResultsHash:    cmbytes.HexBytes(header.LastResultsHash),
		EvidenceHash:       new(cmtypes.EvidenceData).Hash(),
		ProposerAddress:    header.ProposerAddress,
		ChainID:            header.ChainID(),
		ValidatorsHash:     cmbytes.HexBytes(header.ValidatorHash),
		NextValidatorsHash: cmbytes.HexBytes(header.ValidatorHash),
	}

	actual, err := ToABCIHeader(&header)
	if err != nil {
		t.Fatalf("ToABCIHeaderPB returned an error: %v", err)
	}

	assert.Equal(t, expected, actual)
}

func TestToABCIBlock(t *testing.T) {
	blockHeight, nTxs := uint64(1), 2
	header, data := types.GetRandomBlock(blockHeight, nTxs, "TestToABCIBlock")
	abciHeader, err := ToABCIHeader(&header.Header)
	if err != nil {
		t.Fatal(err)
	}

	// we only have one centralize sequencer
	assert.Equal(t, 1, len(header.Validators.Validators))
	val := header.Validators.Validators[0].Address

	abciCommit := types.GetABCICommit(header.Height(), header.Hash(), val, header.Time(), header.Signature)

	if len(abciCommit.Signatures) == 1 {
		abciCommit.Signatures[0].ValidatorAddress = header.ProposerAddress
	}

	abciBlock := cmtypes.Block{
		Header: abciHeader,
		Evidence: cmtypes.EvidenceData{
			Evidence: nil,
		},
		LastCommit: abciCommit,
	}
	abciBlock.Data.Txs = make([]cmtypes.Tx, len(data.Txs))
	for i := range data.Txs {
		abciBlock.Data.Txs[i] = cmtypes.Tx(data.Txs[i])
	}
	abciBlock.Header.DataHash = cmbytes.HexBytes(header.DataHash)

	actual, err := ToABCIBlock(header, data)
	if err != nil {
		t.Fatalf("ToABCIBlock returned an error: %v", err)
	}
	assert.Equal(t, &abciBlock, actual)
}

func TestToABCIBlockMeta(t *testing.T) {
	blockHeight, nTxs := uint64(1), 2
	header, data := types.GetRandomBlock(blockHeight, nTxs, "TestToABCIBlockMeta")
	cmblock, err := ToABCIBlock(header, data)
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

	actual, err := ToABCIBlockMeta(header, data)
	if err != nil {
		t.Fatalf("ToABCIBlock returned an error: %v", err)
	}
	assert.Equal(t, expected, actual)

}
