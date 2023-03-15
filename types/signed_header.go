package types

import (
	"encoding"
	"time"

	"github.com/celestiaorg/go-header"
)

func (sH *SignedHeader) New() header.Header {
	return new(SignedHeader)
}

func (sH *SignedHeader) Hash() Hash {
	return sH.Header.Hash()
}

func (sH *SignedHeader) IsZero() bool {
	return sH == nil
}

func (sH *SignedHeader) ChainID() string {
	return sH.Header.ChainID()
}

func (sH *SignedHeader) Height() int64 {
	return sH.Header.Height()
}

func (sH *SignedHeader) LastHeader() Hash {
	return sH.Header.LastHeader()
}

func (sH *SignedHeader) Time() time.Time {
	return sH.Header.Time()
}

func (sH *SignedHeader) VerifyAdjacent(untrst header.Header) error {
	return sH.Header.VerifyAdjacent(untrst)
}

func (sH *SignedHeader) VerifyNonAdjacent(untrst header.Header) error {
	return sH.Header.VerifyNonAdjacent(untrst)
}

func (sH *SignedHeader) Validate() error {
	return sH.ValidateBasic()
}

var _ header.Header = &SignedHeader{}
var _ encoding.BinaryMarshaler = &SignedHeader{}
var _ encoding.BinaryUnmarshaler = &SignedHeader{}
