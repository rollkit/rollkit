package types

import (
	"bytes"
	"encoding"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
)

var (
	// ErrHeaderDataMismatch is returned when header and data metadata do not describe the same block.
	ErrHeaderDataMismatch = errors.New("header and data do not match")

	// ErrDataHashMismatch is returned when a header's data hash does not match its block data.
	ErrDataHashMismatch = errors.New("dataHash from the header does not match with hash of the block's data")
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

// Data defines Evolve block data.
type Data struct {
	*Metadata
	Txs Txs
}

// SignedData combines Data and its signature.
type SignedData struct {
	Data
	Signature Signature
	Signer    Signer
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
			return ErrHeaderDataMismatch
		}
	}
	// exclude Metadata while computing the data hash for comparison
	d := Data{Txs: data.Txs}
	dataHash := d.DACommitment()
	if !bytes.Equal(dataHash[:], header.DataHash[:]) {
		return ErrDataHashMismatch
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

// Verify a new, untrusted block against a trusted block.
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
	// Validate that Metadata is set
	if d.Metadata == nil {
		return errors.New("data metadata cannot be nil")
	}

	// Validate ChainID is not empty
	if d.Metadata.ChainID == "" {
		return errors.New("chain ID cannot be empty")
	}

	// Validate Height is non-zero for non-genesis blocks
	if d.Metadata.Height == 0 {
		return errors.New("height cannot be zero")
	}

	// Validate timestamp - must be non-zero
	if d.Metadata.Time == 0 {
		return errors.New("timestamp cannot be zero")
	}

	// Check timestamp is not too far in the future (allow some clock drift)
	maxAllowedTime := time.Now().Add(MaxClockDrift)
	dataTime := d.Time()
	if dataTime.After(maxAllowedTime) {
		return errors.New("timestamp too far in future")
	}

	return nil
}

// Size returns size of the block in bytes.
func (d *Data) Size() int {
	return proto.Size(d.ToProto())
}

// TxsByteSize returns total byte size of all transactions without proto marshalling.
// This is much cheaper than Size() and useful for metrics/reporting.
func (d *Data) TxsByteSize() int {
	n := 0
	for _, tx := range d.Txs {
		n += len(tx)
	}
	return n
}
