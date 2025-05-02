package types

import (
	"bytes"
	"encoding"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
)

// Version captures the consensus rules for processing a block in the blockchain,
// including all blockchain data structures and the rules of the application's
// state transition machine.
// This is equivalent to the tmversion.Consensus type in Tendermint.
type Version struct {
	Block uint64
	App   uint64
}

var _ encoding.BinaryMarshaler = &Data{}
var _ encoding.BinaryUnmarshaler = &Data{}

// Metadata defines metadata for Data struct to help with p2p gossiping.
type Metadata struct {
	ChainID      string
	Height       uint64
	Time         uint64
	LastDataHash Hash
}

// Data defines Rollkit block data.
type Data struct {
	*Metadata
	Txs Txs
}

// Signature represents signature of block creator.
type Signature []byte

// ValidateBasic performs basic validation of a signature.
func (signature *Signature) ValidateBasic() error {
	if len(*signature) == 0 {
		return ErrSignatureEmpty
	}
	return nil
}

// Validate performs validation of data with respect to its header
func Validate(header *SignedHeader, data *Data) error {
	// Validate Metadata only when available
	if data.Metadata != nil {
		if header.ChainID() != data.ChainID() ||
			header.Height() != data.Height() ||
			header.Time() != data.Time() { // skipping LastDataHash comparison as it needs access to previous header
			return errors.New("header and data do not match")
		}
	}
	// exclude Metadata while computing the data hash for comparison
	d := Data{Txs: data.Txs}
	dataHash := d.DACommitment()
	if !bytes.Equal(dataHash[:], header.DataHash[:]) {
		return errors.New("dataHash from the header does not match with hash of the block's data")
	}
	return nil
}

// New returns a new Block.
func (d *Data) New() *Data {
	return new(Data)
}

// IsZero returns true if the block is nil.
func (d *Data) IsZero() bool {
	return d == nil
}

// ChainID returns chain ID of the block.
func (d *Data) ChainID() string {
	return d.Metadata.ChainID
}

// Height returns height of the block.
func (d *Data) Height() uint64 {
	return d.Metadata.Height
}

// LastHeader returns last header hash of the block.
func (d *Data) LastHeader() Hash {
	return d.LastDataHash
}

// Time returns time of the block.
func (d *Data) Time() time.Time {
	return time.Unix(0, int64(d.Metadata.Time))
}

// Verify Verifies a new, untrusted block against a trusted block.
func (d *Data) Verify(untrustedData *Data) error {
	if untrustedData == nil {
		return errors.New("untrusted block cannot be nil")
	}
	dataHash := d.Hash()
	// Check if the data hash of the untrusted block matches the last data hash of the trusted block
	if !bytes.Equal(dataHash[:], untrustedData.LastDataHash[:]) {
		return errors.New("data hash of the trusted data does not match with last data hash of the untrusted data")
	}
	return nil
}

// Validate performs basic validation of a block.
// this is used to implement the header interface for go header
func (d *Data) Validate() error {
	return nil
}

// Size returns size of the block in bytes.
func (d *Data) Size() int {
	return proto.Size(d.ToProto())
}
