package types

import (
	"bytes"
	"errors"
)

// DataCommitment represents a commitment to data stored on the DA layer.
// It is used to link a header to its corresponding data.
type DataCommitment struct {
	// Hash of the data
	Hash Hash
	// Optional reference to the location of the data on the DA layer
	DAReference []byte
}

// Validate checks if the DataCommitment is valid.
func (dc *DataCommitment) Validate() error {
	if dc == nil {
		return errors.New("data commitment cannot be nil")
	}

	// Check if the hash is empty
	emptyHash := Hash{}
	if bytes.Equal(dc.Hash[:], emptyHash[:]) {
		return errors.New("data commitment hash cannot be empty")
	}

	return nil
}

// SetDataCommitment sets the DataCommitment in a header.
func (h *Header) SetDataCommitment(data *Data) {
	dataHash := data.Hash()
	h.DataHash = dataHash
}

// ValidateDataCommitment validates that the header's DataHash matches the hash of the provided data.
func (h *Header) ValidateDataCommitment(data *Data) bool {
	return bytes.Equal(h.DataHash[:], data.Hash()[:])
}
