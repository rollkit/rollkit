package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/celestiaorg/go-header"
	cmtypes "github.com/cometbft/cometbft/types"
)

// SignedHeader combines Header and its Commit.
//
// Used mostly for gossiping.
type SignedHeader struct {
	Header
	Commit     Commit
	Validators *cmtypes.ValidatorSet
}

// New creates a new SignedHeader.
func (sh *SignedHeader) New() *SignedHeader {
	return new(SignedHeader)
}

// IsZero returns true if the SignedHeader is nil
func (sh *SignedHeader) IsZero() bool {
	return sh == nil
}

var (
	// ErrNonAdjacentHeaders is returned when the headers are not adjacent.
	ErrNonAdjacentHeaders = errors.New("non-adjacent headers")

	// ErrLastHeaderHashMismatch is returned when the last header hash doesn't match.
	ErrLastHeaderHashMismatch = errors.New("last header hash mismatch")

	// ErrLastCommitHashMismatch is returned when the last commit hash doesn't match.
	ErrLastCommitHashMismatch = errors.New("last commit hash mismatch")
)

// Verify verifies the signed header.
func (sh *SignedHeader) Verify(untrstH *SignedHeader) error {
	// go-header ensures untrustH already passed ValidateBasic.
	if err := sh.Header.Verify(&untrstH.Header); err != nil {
		return &header.VerifyError{
			Reason: err,
		}
	}

	if sh.Height()+1 < untrstH.Height() {
		return &header.VerifyError{
			Reason: fmt.Errorf("%w: untrusted %d, trusted %d",
				ErrNonAdjacentHeaders,
				untrstH.Height(),
				sh.Height(),
			),
			SoftFailure: true,
		}
	}

	sHHash := sh.Header.Hash()
	if !bytes.Equal(untrstH.LastHeaderHash[:], sHHash) {
		return &header.VerifyError{
			Reason: fmt.Errorf("%w: expected %v, but got %v",
				ErrLastHeaderHashMismatch,
				untrstH.LastHeaderHash[:], sHHash,
			),
		}
	}
	sHLastCommitHash := sh.Commit.GetCommitHash(&untrstH.Header, sh.ProposerAddress)
	if !bytes.Equal(untrstH.LastCommitHash[:], sHLastCommitHash) {
		return &header.VerifyError{
			Reason: fmt.Errorf("%w: expected %v, but got %v",
				ErrLastCommitHashMismatch,
				untrstH.LastCommitHash[:], sHHash,
			),
		}
	}
	return nil
}

var (
	// ErrAggregatorSetHashMismatch is returned when the aggregator set hash
	// in the signed header doesn't match the hash of the validator set.
	ErrAggregatorSetHashMismatch = errors.New("aggregator set hash in signed header and hash of validator set do not match")

	// ErrSignatureVerificationFailed is returned when the signature
	// verification fails
	ErrSignatureVerificationFailed = errors.New("signature verification failed")

	// ErrInvalidValidatorSetLengthMismatch is returned when the validator set length is not exactly one
	ErrInvalidValidatorSetLengthMismatch = errors.New("must have exactly one validator (the centralized sequencer)")

	// ErrProposerAddressMismatch is returned when the proposer address in the signed header does not match the proposer address in the validator set
	ErrProposerAddressMismatch = errors.New("proposer address in SignedHeader does not match the proposer address in the validator set")

	// ErrProposerNotInValSet is returned when the proposer address in the validator set is not in the validator set
	ErrProposerNotInValSet = errors.New("proposer address in the validator set is not in the validator set")
)

// validatorsEqual compares validator pointers. Starts with the happy case, then falls back to field-by-field comparison.
func validatorsEqual(val1 *cmtypes.Validator, val2 *cmtypes.Validator) bool {
	if val1 == val2 {
		// happy case is if they are pointers to the same struct.
		return true
	}
	// if not, do a field-by-field comparison
	return val1.PubKey.Equals(val2.PubKey) &&
		bytes.Equal(val1.Address.Bytes(), val2.Address.Bytes()) &&
		val1.VotingPower == val2.VotingPower &&
		val1.ProposerPriority == val2.ProposerPriority

}

// ValidateBasic performs basic validation of a signed header.
func (sh *SignedHeader) ValidateBasic() error {
	if err := sh.Header.ValidateBasic(); err != nil {
		return err
	}

	if err := sh.Commit.ValidateBasic(); err != nil {
		return err
	}

	if err := sh.Validators.ValidateBasic(); err != nil {
		return err
	}

	// Rollkit vA uses a centralized sequencer, so there should only be one validator
	if len(sh.Validators.Validators) != 1 {
		return errors.New("cannot have more than 1 validator (the centralized sequencer)")
	}

	// Check that the proposer address in the signed header matches the proposer address in the validator set
	if !bytes.Equal(sh.ProposerAddress, sh.Validators.Proposer.Address.Bytes()) {
		return ErrProposerAddressMismatch
	}

	// Check that the proposer is the only validator in the validator set
	if !validatorsEqual(sh.Validators.Proposer, sh.Validators.Validators[0]) {
		return ErrProposerNotInValSet
	}

	// Make sure there is exactly one signature
	if len(sh.Commit.Signatures) != 1 {
		return errors.New("expected exactly one signature")
	}

	signature := sh.Commit.Signatures[0]

	msg, err := sh.Header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("signature verification failed, unable to marshal header: %v", err)
	}
	if !sh.Validators.Validators[0].PubKey.VerifySignature(msg, signature) {
		return ErrSignatureVerificationFailed
	}
	return nil
}

var _ header.Header[*SignedHeader] = &SignedHeader{}
