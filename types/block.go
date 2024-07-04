package types

import (
	"bytes"
	"encoding"
	"errors"
	"time"

	cmtypes "github.com/cometbft/cometbft/types"
)

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
	Txs Txs
	// IntermediateStateRoots IntermediateStateRoots
	// Note: Temporarily remove Evidence #896
	// Evidence               EvidenceData
}

// EvidenceData defines how evidence is stored in block.
type EvidenceData struct {
	Evidence []cmtypes.Evidence
}

// Signature represents signature of block creator.
type Signature []byte

// IntermediateStateRoots describes the state between transactions.
// They are required for fraud proofs.
type IntermediateStateRoots struct {
	RawRootsList [][]byte
}

// GetCommitHash returns an ABCI commit equivalent hash associated to a signature.
func (signature *Signature) GetCommitHash(header *Header, proposerAddress []byte) []byte {
	abciCommit := GetABCICommit(header.Height(), header.Hash(), proposerAddress, header.Time(), *signature)
	abciCommit.Signatures[0].ValidatorAddress = proposerAddress
	abciCommit.Signatures[0].Timestamp = header.Time()
	return abciCommit.Hash()
}

// ValidateBasic performs basic validation of block data.
// Actually it's a placeholder, because nothing is checked.
func (d *Data) ValidateBasic() error {
	return nil
}

// ValidateBasic performs basic validation of a signature.
func (signature *Signature) ValidateBasic() error {
	if len(*signature) == 0 {
		return ErrSignatureEmpty
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
	return b.SignedHeader.ChainID()
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

// Size returns size of the block in bytes.
func (b *Block) Size() int {
	pbb, err := b.ToProto()
	if err != nil {
		return 0
	}

	return pbb.Size()
}
