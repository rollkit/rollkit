package types

import (
	"bytes"
)

// SetDataCommitment sets the data hash in a header.
func (h *Header) SetDataCommitment(data *Data) {
	dataHash := data.Hash()
	h.DataHash = dataHash
}

// ValidateDataCommitment validates that the header's DataHash matches the hash of the provided data.
func (h *Header) ValidateDataCommitment(data *Data) bool {
	return bytes.Equal(h.DataHash[:], data.Hash()[:])
}
