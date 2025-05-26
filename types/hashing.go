package types

import (
	"crypto/sha256"
	"hash"

	"github.com/libp2p/go-libp2p/core/crypto"
)

var (
	leafPrefix = []byte{0}
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
	return leafHashOpt(sha256.New(), dBytes)
}

// DACommitment returns the DA commitment of the Data excluding the Metadata
func (d *Data) DACommitment() Hash {
	// Prune the Data to only include the Txs
	prunedData := &Data{
		Txs: d.Txs,
	}
	dBytes, _ := prunedData.MarshalBinary()
	return leafHashOpt(sha256.New(), dBytes)
}

func leafHashOpt(s hash.Hash, leaf []byte) []byte {
	s.Reset()
	s.Write(leafPrefix)
	s.Write(leaf)
	return s.Sum(nil)
}

// ValidatorHasher defines the function signature for a component that can calculate
// the ValidatorHash for a block header.
// It takes the proposer's address and public key (as known by Rollkit's signer)
// and is expected to return a types.Hash compatible with the target consensus system (e.g., CometBFT).
// If the hasher is not configured or an error occurs, it should be handled appropriately
// by the caller (e.g., using a zero hash or returning an error).
type ValidatorHasher func(proposerAddress []byte, pubKey crypto.PubKey) (Hash, error)

// HeaderHasher defines the function signature for a component that can calculate
// the HeaderHash for a block header.
// It takes the header and is expected to return a types.Hash compatible with the target consensus system (e.g., CometBFT).
// If the hasher is not configured or an error occurs, it should be handled appropriately
// by the caller (e.g., using a zero hash or returning an error).
type HeaderHasher func(header *Header) (Hash, error)
