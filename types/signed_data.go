package types

import (
	"bytes"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
)

// SignedData combines Data and its signature.
type SignedData struct {
	Data
	Signature Signature
	Signer    Signer
}

// New returns a new SignedData.
func (d *SignedData) New() *SignedData {
	return new(SignedData)
}

// IsZero returns true if the SignedData is nil.
func (d *SignedData) IsZero() bool {
	return d == nil
}

// ChainID returns chain ID of the SignedData.
func (d *SignedData) ChainID() string {
	return d.Metadata.ChainID
}

// Height returns height of the SignedData.
func (d *SignedData) Height() uint64 {
	return d.Metadata.Height
}

// LastHeader returns last header hash of the SignedData.
func (d *SignedData) LastHeader() Hash {
	return d.LastDataHash
}

// Time returns time of the SignedData.
func (d *SignedData) Time() time.Time {
	return time.Unix(0, int64(d.Metadata.Time))
}

// Verify Verifies a new, untrusted SignedData against a trusted SignedData.
func (d *SignedData) Verify(untrustedData *SignedData) error {
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
func (d *SignedData) Validate() error {
	return nil
}

// Size returns size of the block in bytes.
func (d *SignedData) Size() int {
	protoData, err := d.ToProto()
	if err != nil {
		return 0
	}
	return proto.Size(protoData)
}
