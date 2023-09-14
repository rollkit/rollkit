package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/celestiaorg/go-header"
	"github.com/cometbft/cometbft/crypto/ed25519"
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

func (sh *SignedHeader) New() *SignedHeader {
	return new(SignedHeader)
}

func (sh *SignedHeader) IsZero() bool {
	return sh == nil
}

var (
	ErrNonAdjacentHeaders     = errors.New("non-adjacent headers")
	ErrNoProposerAddress      = errors.New("no proposer address")
	ErrLastHeaderHashMismatch = errors.New("last header hash mismatch")
	ErrLastCommitHashMismatch = errors.New("last commit hash mismatch")
)

func (sh *SignedHeader) Verify(untrstH *SignedHeader) error {
	// TODO(@Wondertan):
	//  We keep this check because of how unit tests are structured where TestVerify tests both ValidateBasic and Verify.
	//  While the check is redundant as go-header ensures untrustH passed ValidateBasic before.
	//  Decoupling TestVerify into TestValidateBasic and TestVerify would allow to remove this check.
	if err := untrstH.ValidateBasic(); err != nil {
		return &header.VerifyError{
			Reason: err,
		}
	}

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
	ErrAggregatorSetHashMismatch   = errors.New("aggregator set hash in signed header and hash of validator set do not match")
	ErrSignatureVerificationFailed = errors.New("signature verification failed")
)

// ValidateBasic performs basic validation of a signed header.
func (sh *SignedHeader) ValidateBasic() error {
	if err := sh.Header.ValidateBasic(); err != nil {
		return err
	}

	if err := sh.Commit.ValidateBasic(); err != nil {
		return err
	}

	// Handle Based Rollup case
	if sh.Validators == nil || len(sh.Validators.Validators) == 0 {
		return nil
	}

	if err := sh.Validators.ValidateBasic(); err != nil {
		return err
	}

	if !bytes.Equal(sh.Validators.Hash(), sh.AggregatorsHash[:]) {
		return ErrAggregatorSetHashMismatch
	}

	// Make sure there is exactly one signature
	if len(sh.Commit.Signatures) != 1 {
		return errors.New("expected exactly one signature")
	}

	signature := sh.Commit.Signatures[0]
	proposer := sh.Validators.GetProposer()
	var pubKey ed25519.PubKey = proposer.PubKey.Bytes()
	msg, err := sh.Header.MarshalBinary()
	if err != nil {
		return errors.New("signature verification failed, unable to marshal header")
	}
	if !pubKey.VerifySignature(msg, signature) {
		return ErrSignatureVerificationFailed
	}

	return nil
}

var _ header.Header[*SignedHeader] = &SignedHeader{}
