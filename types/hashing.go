package types

import (
	"encoding"

	"github.com/minio/sha256-simd"
)

func Hash(object encoding.BinaryMarshaler) ([32]byte, error) {
	blob, err := object.MarshalBinary()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(blob), nil
}
