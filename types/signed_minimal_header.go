package types

import (
	"time"
)

// SignedMinimalHeader contains a MinimalHeader with its signature.
type SignedMinimalHeader struct {
	Header    MinimalHeader
	Signature []byte
}

// Hash returns the hash of the header.
func (sh *SignedMinimalHeader) Hash() Hash {
	return sh.Header.Hash()
}

// Height returns the height of the header.
func (sh *SignedMinimalHeader) Height() uint64 {
	return sh.Header.Height()
}

// Time returns the time of the header.
func (sh *SignedMinimalHeader) Time() time.Time {
	return sh.Header.Time()
}

// ChainID returns the chain ID of the header.
func (sh *SignedMinimalHeader) ChainID() string {
	return sh.Header.ChainID()
}
