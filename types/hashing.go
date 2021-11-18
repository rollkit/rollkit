package types

import (
	"encoding"

	"github.com/minio/sha256-simd"
)

func (h *Header) Hash() [32]byte {
	return hash(h)
}

func (b *Block) Hash() [32]byte {
	return hash(b)
}

func (c *Commit) Hash() [32]byte {
	return hash(c)
}

func hash(obj encoding.BinaryMarshaler) [32]byte {
	blob, err := obj.MarshalBinary()
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(blob)

}
