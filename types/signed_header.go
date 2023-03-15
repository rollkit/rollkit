package types

import (
	"fmt"

	"github.com/celestiaorg/go-header"
)

func (sH *SignedHeader) New() header.Header {
	return new(SignedHeader)
}

func (sH *SignedHeader) IsZero() bool {
	return sH == nil
}

func (sH *SignedHeader) VerifyAdjacent(untrst header.Header) error {
	// Explicit type checks are required due to embedded Header which also does the explicit type check
	untrstH, ok := untrst.(*SignedHeader)
	if !ok {
		return &header.VerifyError{
			Reason: fmt.Errorf("%T is not of type %T", untrst, sH),
		}
	}
	return sH.Header.VerifyAdjacent(&untrstH.Header)
}

func (sH *SignedHeader) VerifyNonAdjacent(untrst header.Header) error {
	untrstH, ok := untrst.(*SignedHeader)
	if !ok {
		return &header.VerifyError{
			Reason: fmt.Errorf("%T is not of type %T", untrst, sH),
		}
	}
	return sH.Header.VerifyNonAdjacent(&untrstH.Header)
}
