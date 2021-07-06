package hash

import (
	"encoding"

	"github.com/minio/sha256-simd"
)

// Hash creates a SHA-256 hash of object that can be serialized into binary form.
func Hash(object encoding.BinaryMarshaler) ([32]byte, error) {
	blob, err := object.MarshalBinary()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(blob), nil
}
