package types

import (
	"crypto/sha256"
	"hash"

	"github.com/libp2p/go-libp2p/core/crypto"
)

var (
	leafPrefix = []byte{0}
	// DefaultHeaderHasher is the global header hasher that can be overridden by different implementations
	DefaultHeaderHasher HeaderHasher = defaultHeaderHash
)

// defaultHeaderHash is the default implementation for header hashing using SHA256
func defaultHeaderHash(header *Header) (Hash, error) {
	bytes, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(bytes)

	return hash[:], nil
}

// SetHeaderHasher allows different implementations to override the default header hasher
func SetHeaderHasher(hasher HeaderHasher) {
	DefaultHeaderHasher = hasher
}

// Hash returns hash of the header using the configured header hasher
func (h *Header) Hash() Hash {
	hash, err := DefaultHeaderHasher(h)
	if err != nil {
		// For backward compatibility, return nil on error
		// In the future, this method signature could be changed to return (Hash, error)
		return nil
	}

	return hash
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

// CommitHashProvider defines the function signature for a component that can calculate
// the CommitHash for a block header.
// It takes the header and is expected to return a types.Hash compatible with the target consensus system (e.g., CometBFT).
// If the hasher is not configured or an error occurs, it should be handled appropriately
// by the caller (e.g., using a zero hash or returning an error).
type CommitHashProvider func(signature *Signature, header *Header, proposerAddress []byte) (Hash, error)
