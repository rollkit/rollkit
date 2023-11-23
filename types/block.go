package types

import (
	"bytes"
	"encoding"
	"errors"
	"time"

	cmbytes "github.com/cometbft/cometbft/libs/bytes"

	cmtypes "github.com/cometbft/cometbft/types"
)

// NamespaceID is a unique identifier of a namespace.
type NamespaceID [8]byte

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
// This is equivalent to the tmversion.Consensus type in Tendermint.
type Version struct {
	Block uint64
	App   uint64
}

// Block defines the structure of Rollkit block.
type Block struct {
	SignedHeader SignedHeader
	Data         Data
}

var _ encoding.BinaryMarshaler = &Block{}
var _ encoding.BinaryUnmarshaler = &Block{}

// Data defines Rollkit block data.
type Data struct {
	Txs                    Txs
	IntermediateStateRoots IntermediateStateRoots
	// Note: Temporarily remove Evidence #896
	// Evidence               EvidenceData
}

// EvidenceData defines how evidence is stored in block.
type EvidenceData struct {
	Evidence []cmtypes.Evidence
}

// Commit contains evidence of block creation.
type Commit struct {
	Signatures []Signature // most of the time this is a single signature
}

// Signature represents signature of block creator.
type Signature []byte

// IntermediateStateRoots describes the state between transactions.
// They are required for fraud proofs.
type IntermediateStateRoots struct {
	RawRootsList [][]byte
}

// ToABCICommit converts Rollkit commit into commit format defined by ABCI.
// This function only converts fields that are available in Rollkit commit.
// Other fields (especially ValidatorAddress and Timestamp of Signature) has to be filled by caller.
func (c *Commit) ToABCICommit(height uint64, hash Hash, val cmtypes.Address, time time.Time) *cmtypes.Commit {
	tmCommit := cmtypes.Commit{
		Height: int64(height),
		Round:  0,
		BlockID: cmtypes.BlockID{
			Hash:          cmbytes.HexBytes(hash),
			PartSetHeader: cmtypes.PartSetHeader{},
		},
		Signatures: make([]cmtypes.CommitSig, len(c.Signatures)),
	}
	for i, sig := range c.Signatures {
		commitSig := cmtypes.CommitSig{
			BlockIDFlag:      cmtypes.BlockIDFlagCommit,
			Signature:        sig,
			ValidatorAddress: val,
			Timestamp:        time,
		}
		tmCommit.Signatures[i] = commitSig
	}

	return &tmCommit
}

// GetCommitHash returns hash of the commit.
func (c *Commit) GetCommitHash(header *Header, proposerAddress []byte) []byte {
	lastABCICommit := c.ToABCICommit(header.Height(), header.Hash(), proposerAddress, header.Time())
	// Rollkit does not support a multi signature scheme so there can only be one signature
	if len(c.Signatures) == 1 {
		lastABCICommit.Signatures[0].ValidatorAddress = proposerAddress
		lastABCICommit.Signatures[0].Timestamp = header.Time()
	}
	return lastABCICommit.Hash()
}

// ValidateBasic performs basic validation of block data.
// Actually it's a placeholder, because nothing is checked.
func (d *Data) ValidateBasic() error {
	return nil
}

// ValidateBasic performs basic validation of a commit.
func (c *Commit) ValidateBasic() error {
	if len(c.Signatures) == 0 {
		return errors.New("no signatures")
	}
	return nil
}

// ValidateBasic performs basic validation of a block.
func (b *Block) ValidateBasic() error {
	if err := b.SignedHeader.ValidateBasic(); err != nil {
		return err
	}
	if err := b.Data.ValidateBasic(); err != nil {
		return err
	}
	dataHash, err := b.Data.Hash()
	if err != nil {
		return err
	}
	if !bytes.Equal(dataHash[:], b.SignedHeader.DataHash[:]) {
		return errors.New("dataHash from the header does not match with hash of the block's data")
	}
	return nil
}

// New returns a new Block.
func (b *Block) New() *Block {
	return new(Block)
}

// IsZero returns true if the block is nil.
func (b *Block) IsZero() bool {
	return b == nil
}

// ChainID returns chain ID of the block.
func (b *Block) ChainID() string {
	return b.SignedHeader.ChainID() + "-block"
}

// Height returns height of the block.
func (b *Block) Height() uint64 {
	return b.SignedHeader.Height()
}

// LastHeader returns last header hash of the block.
func (b *Block) LastHeader() Hash {
	return b.SignedHeader.LastHeader()
}

// Time returns time of the block.
func (b *Block) Time() time.Time {
	return b.SignedHeader.Time()
}

// Verify Verifies a new, untrusted block against a trusted block.
func (b *Block) Verify(untrustedBlock *Block) error {
	if untrustedBlock == nil {
		return errors.New("untrusted block cannot be nil")
	}
	return b.SignedHeader.Verify(&untrustedBlock.SignedHeader)
}

// Validate performs basic validation of a block.
func (b *Block) Validate() error {
	return b.ValidateBasic()
}
