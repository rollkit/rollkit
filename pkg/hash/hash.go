package hash

import "crypto/sha256"

const (
	// TruncatedSize is the size of the truncated hash.
	TruncatedSize = 32
)

// SumTruncated returns the bytes of SHA256 of the bz.
func SumTruncated(bz []byte) []byte {
	hash := sha256.Sum256(bz)
	return hash[:TruncatedSize]
}
