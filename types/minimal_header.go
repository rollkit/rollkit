package types

import (
	"encoding"
	"errors"
	"time"

	"github.com/celestiaorg/go-header"
)

// MinimalHeader defines the structure of a minimal Rollkit block header as defined in the ADR:
// https://raw.githubusercontent.com/rollkit/rollkit/72ce13c222a715ae6f9fb30790e54dc44c9065eb/block/rollkit-minimal-header.md
type MinimalHeader struct {
	// ParentHash is the hash of the previous rollup block header
	ParentHash Hash

	// BlockHeight represents the block height (aka block number) of a given header
	BlockHeight uint64

	// BlockTimestamp is the block creation time
	BlockTimestamp uint64

	// BlockChainID is the chain identifier
	BlockChainID string

	// DataCommitment is a pointer to block data on DA layer
	DataCommitment Hash

	// StateRoot is a commitment representing the state linked to the header
	StateRoot Hash

	// ExtraData is an arbitrary field for additional metadata (e.g. sequencer info)
	ExtraData []byte
}

// New creates a new MinimalHeader.
func (h *MinimalHeader) New() *MinimalHeader {
	return new(MinimalHeader)
}

// IsZero returns true if the header is nil.
func (h *MinimalHeader) IsZero() bool {
	return h == nil
}

// ChainID returns chain ID of the header.
func (h *MinimalHeader) ChainID() string {
	return h.BlockChainID
}

// Height returns height of the header.
func (h *MinimalHeader) Height() uint64 {
	return h.BlockHeight
}

// LastHeader returns parent header hash to implement header.Header interface.
func (h *MinimalHeader) LastHeader() Hash {
	return h.ParentHash
}

// ParentHeader returns parent header hash.
func (h *MinimalHeader) ParentHeader() Hash {
	return h.ParentHash
}

// Time returns timestamp as unix time with nanosecond precision
func (h *MinimalHeader) Time() time.Time {
	return time.Unix(0, int64(h.BlockTimestamp)) //nolint:gosec
}

// ValidateBasic performs basic validation of a header.
func (h *MinimalHeader) ValidateBasic() error {
	// Basic validation can be expanded as needed
	if h.BlockHeight == 0 {
		return errors.New("height cannot be zero")
	}

	if h.BlockChainID == "" {
		return errors.New("chain ID cannot be empty")
	}

	return nil
}

// Validate performs validation of the header, implementing the header.Header interface.
func (h *MinimalHeader) Validate() error {
	return h.ValidateBasic()
}

// Verify verifies the header against an untrusted header.
func (h *MinimalHeader) Verify(untrstH *MinimalHeader) error {
	// Placeholder implementation
	// In a real implementation, additional verification could be performed here
	return nil
}

// Hash returns the hash of the minimal header
func (h *MinimalHeader) Hash() Hash {
	// Implementation would depend on how you compute the hash
	// This is a placeholder
	var hash Hash
	// TODO: Implement proper hashing
	return hash
}

// MarshalBinary encodes MinimalHeader into binary form and returns it.
func (h *MinimalHeader) MarshalBinary() ([]byte, error) {
	// Implementation would depend on how you serialize the header
	// This is a placeholder
	return nil, errors.New("not implemented - will be implemented in serialization.go")
}

// UnmarshalBinary decodes binary form of MinimalHeader into object.
func (h *MinimalHeader) UnmarshalBinary(data []byte) error {
	// Implementation would depend on how you deserialize the header
	// This is a placeholder
	return errors.New("not implemented - will be implemented in serialization.go")
}

// FromHeader converts a standard Header to a MinimalHeader
func FromHeader(h *Header) *MinimalHeader {
	return &MinimalHeader{
		ParentHash:     h.LastHeaderHash,
		BlockHeight:    h.Height(),
		BlockTimestamp: h.BaseHeader.Time,
		BlockChainID:   h.ChainID(),
		DataCommitment: h.DataHash,
		StateRoot:      h.AppHash,
		ExtraData:      h.ProposerAddress, // You might want to include more data here
	}
}

// ToHeader converts a MinimalHeader to a standard Header
func (h *MinimalHeader) ToHeader() *Header {
	header := &Header{
		BaseHeader: BaseHeader{
			Height:  h.BlockHeight,
			Time:    h.BlockTimestamp,
			ChainID: h.BlockChainID,
		},
		LastHeaderHash:  h.ParentHash,
		DataHash:        h.DataCommitment,
		AppHash:         h.StateRoot,
		ProposerAddress: h.ExtraData,
	}

	// Set some sensible defaults for the remaining fields
	// This is just a placeholder and might need to be adjusted

	return header
}

var _ header.Header[*MinimalHeader] = &MinimalHeader{}
var _ encoding.BinaryMarshaler = &MinimalHeader{}
var _ encoding.BinaryUnmarshaler = &MinimalHeader{}
