package types

import (
	"bytes"
	"errors"
	"time"
)

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
type Version struct {
	Block uint64
	App   uint64
}

// Block represents a complete block in the blockchain
type Block struct {
	Header    Header
	Data      *Data
	Signature *Signature
}

// Metadata defines metadata for Data struct to help with p2p gossiping
type Metadata struct {
	ChainID      string
	Height       uint64
	Time         uint64
	LastDataHash Hash
}

// Data defines block data
type Data struct {
	*Metadata
	Txs Txs
}

// Signature represents signature of block creator
type Signature []byte

// GetCommitHash returns a commit hash associated with a signature
func (signature *Signature) GetCommitHash(header *Header, proposerAddress []byte) []byte {
	commit := GetCommit(header.Height(), header.Hash(), proposerAddress, header.Time(), *signature)
	return commit.Hash
}

// ValidateBasic performs basic validation of block data
func (d *Data) ValidateBasic() error {
	if d == nil {
		return errors.New("data cannot be nil")
	}
	if d.Metadata != nil {
		if d.ChainID() == "" {
			return errors.New("chain ID cannot be empty")
		}
		if d.Height() == 0 {
			return errors.New("height cannot be zero")
		}
		if d.Time().IsZero() {
			return errors.New("time cannot be zero")
		}
	}
	return nil
}

// ValidateBasic performs basic validation of a signature
func (signature *Signature) ValidateBasic() error {
	if len(*signature) == 0 {
		return errors.New("signature cannot be empty")
	}
	return nil
}

// Validate performs validation of data with respect to its header
func (b *Block) Validate() error {
	// Validate basic structure
	if b.Header.IsZero() {
		return errors.New("header cannot be empty")
	}
	if b.Data == nil {
		return errors.New("data cannot be nil")
	}
	if err := b.Signature.ValidateBasic(); err != nil {
		return err
	}

	// Validate metadata matches header
	if b.Data.Metadata != nil {
		if b.Header.ChainID() != b.Data.ChainID() ||
			b.Header.Height() != b.Data.Height() ||
			b.Header.Time().UnixNano() != b.Data.Time().UnixNano() {
			return errors.New("header and data metadata do not match")
		}
	}

	// Validate data hash
	dataHash := b.Data.Hash()
	if !bytes.Equal(dataHash[:], b.Header.DataHash[:]) {
		return errors.New("data hash mismatch")
	}

	return nil
}

// Verify verifies a new, untrusted block against a trusted block
func (b *Block) Verify(untrustedBlock *Block) error {
	if untrustedBlock == nil {
		return errors.New("untrusted block cannot be nil")
	}

	// Verify heights are consecutive
	if b.Header.Height()+1 != untrustedBlock.Header.Height() {
		return errors.New("blocks are not consecutive")
	}

	// Verify chain IDs match
	if b.Header.ChainID() != untrustedBlock.Header.ChainID() {
		return errors.New("chain ID mismatch")
	}

	// Verify header hash links
	if !bytes.Equal(b.Header.Hash(), untrustedBlock.Header.LastHeaderHash) {
		return errors.New("header hash link mismatch")
	}

	// Verify timestamps are monotonic
	if !untrustedBlock.Header.Time().After(b.Header.Time()) {
		return errors.New("time must be monotonically increasing")
	}

	return nil
}

// Helper methods for Data
func (d *Data) ChainID() string {
	if d.Metadata == nil {
		return ""
	}
	return d.Metadata.ChainID
}

func (d *Data) Height() uint64 {
	if d.Metadata == nil {
		return 0
	}
	return d.Metadata.Height
}

func (d *Data) Time() time.Time {
	if d.Metadata == nil {
		return time.Time{}
	}
	return time.Unix(0, int64(d.Metadata.Time))
}

func (d *Data) LastHeader() Hash {
	if d.Metadata == nil {
		return Hash{}
	}
	return d.Metadata.LastDataHash
}

func (d *Data) IsZero() bool {
	return d == nil
}
