package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/celestiaorg/go-header"
)

var (
	// ErrLastHeaderHashMismatch is returned when the last header hash doesn't match.
	ErrLastHeaderHashMismatch = errors.New("last header hash mismatch")

	// ErrLastCommitHashMismatch is returned when the last commit hash doesn't match.
	ErrLastCommitHashMismatch = errors.New("last commit hash mismatch")
)

// SignedHeader combines Header and its signature.
//
// Used mostly for gossiping.
type SignedHeader struct {
	Header
	// Note: This is backwards compatible as ABCI exported types are not affected.
	Signature Signature
	Signer    Signer
}

// New creates a new SignedHeader.
func (sh *SignedHeader) New() *SignedHeader {
	return new(SignedHeader)
}

// IsZero returns true if the SignedHeader is nil
func (sh *SignedHeader) IsZero() bool {
	return sh == nil
}

// Verify verifies the signed header.
func (sh *SignedHeader) Verify(untrstH *SignedHeader) error {
	// go-header ensures untrustH already passed ValidateBasic.
	if err := sh.Header.Verify(&untrstH.Header); err != nil {
		return &header.VerifyError{
			Reason: err,
		}
	}

	if sh.isAdjacent(untrstH) {
		if err := sh.verifyHeaderHash(untrstH); err != nil {
			return err
		}
	}

	return nil
}

// verifyHeaderHash verifies the header hash.
func (sh *SignedHeader) verifyHeaderHash(untrstH *SignedHeader) error {
	hash := sh.Hash()
	if !bytes.Equal(hash, untrstH.LastHeader()) {
		return sh.newVerifyError(ErrLastHeaderHashMismatch, hash, untrstH.LastHeader())
	}
	return nil
}

// isAdjacent checks if the height of headers is adjacent.
func (sh *SignedHeader) isAdjacent(untrstH *SignedHeader) bool {
	return sh.Height()+1 == untrstH.Height()
}

// newVerifyError creates and returns a new error verification.
func (sh *SignedHeader) newVerifyError(err error, expected, got []byte) *header.VerifyError {
	return &header.VerifyError{
		Reason: fmt.Errorf("verification error at height %d: %w: expected %X, but got %X", sh.Height(), err, expected, got),
	}
}

var (
	// ErrAggregatorSetHashMismatch is returned when the aggregator set hash
	// in the signed header doesn't match the hash of the validator set.
	ErrAggregatorSetHashMismatch = errors.New("aggregator set hash in signed header and hash of validator set do not match")

	// ErrSignatureVerificationFailed is returned when the signature
	// verification fails
	ErrSignatureVerificationFailed = errors.New("signature verification failed")

	// ErrProposerAddressMismatch is returned when the proposer address in the signed header does not match the proposer address in the validator set
	ErrProposerAddressMismatch = errors.New("proposer address in SignedHeader does not match the proposer address in the validator set")

	// ErrSignatureEmpty is returned when signature is empty
	ErrSignatureEmpty = errors.New("signature is empty")
)

// ValidateBasic performs basic validation of a signed header.
func (sh *SignedHeader) ValidateBasic() error {
	if err := sh.Header.ValidateBasic(); err != nil {
		return err
	}

	if err := sh.Signature.ValidateBasic(); err != nil {
		return err
	}

	// Check that the proposer address in the signed header matches the proposer address in the validator set
	if !bytes.Equal(sh.ProposerAddress, sh.Signer.Address) {
		return ErrProposerAddressMismatch
	}

	bz, err := sh.Header.MarshalBinary()
	if err != nil {
		return err
	}

	verified, err := sh.Signer.PubKey.Verify(bz, sh.Signature)
	if err != nil {
		return err
	}
	if !verified {
		return ErrSignatureVerificationFailed
	}

	return nil
}

var _ header.Header[*SignedHeader] = &SignedHeader{}
