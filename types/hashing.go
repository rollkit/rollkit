package types

import (
	"crypto/sha256"

	"github.com/cometbft/cometbft/crypto/merkle"
)

// Hash returns hash of the header
func (h *Header) Hash() Hash {
	bytes, err := h.MarshalBinary()
	if err != nil {
		return nil
	}
	hash := sha256.Sum256(bytes)
	return hash[:]
}

// Hash returns hash of the Data
func (d *Data) Hash() Hash {
	// Ignoring the marshal error for now to satisfy the go-header interface
	// Later on the usage of Hash should be replaced with DA commitment
	dBytes, _ := d.MarshalBinary()
	return merkle.HashFromByteSlices([][]byte{
		dBytes,
	})
}
