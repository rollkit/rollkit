package types

import (
	"github.com/libp2p/go-libp2p/core/crypto"
)

// ValidatorHasherProvider defines the function type for hashing a validator's public key and address.
type ValidatorHasherProvider func(
	proposerAddress []byte,
	pubKey crypto.PubKey,
) (Hash, error)

// DefaultValidatorHasherProvider is the default implementation of ValidatorHasherProvider.
// It returns an empty Hash, as rollkit does not use validator hashes itself.
func DefaultValidatorHasherProvider(
	_ []byte,
	_ crypto.PubKey,
) (Hash, error) {
	return Hash{}, nil
}
