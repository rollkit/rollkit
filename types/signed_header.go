package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/celestiaorg/go-header"
	"github.com/cometbft/cometbft/crypto/ed25519"
)

var ErrAggregatorSetHashMismatch = errors.New("aggregator set hash in signed header and hash of validator set do not match")

var ErrSignatureVerificationFailed = errors.New("signature verification failed")

var ErrNoProposerAddress = errors.New("no proposer address")

type ErrLastHeaderHashMismatch struct {
	Reason error
}

func (mr *ErrLastHeaderHashMismatch) Error() string {
	return fmt.Sprintf("lastHeaderMismatch: %s", mr.Reason.Error())
}

type ErrLastCommitHashMismatch struct {
	Reason error
}

func (mr *ErrLastCommitHashMismatch) Error() string {
	return fmt.Sprintf("lastCommitMismatch: %s", mr.Reason.Error())
}

type ErrNewHeaderTimeBeforeOldHeaderTime struct {
	Reason error
}

func (mr *ErrNewHeaderTimeBeforeOldHeaderTime) Error() string {
	return fmt.Sprintf("newHeaderTimeBeforeOldHeaderTime: %s", mr.Reason.Error())
}

type ErrNewHeaderTimeFromFuture struct {
	Reason error
}

func (mr *ErrNewHeaderTimeFromFuture) Error() string {
	return fmt.Sprintf("newHeaderTimeFromFuture: %s", mr.Reason.Error())
}

func (sH *SignedHeader) New() header.Header {
	return new(SignedHeader)
}

func (sH *SignedHeader) IsZero() bool {
	return sH == nil
}

func (sH *SignedHeader) Verify(untrst header.Header) error {
	// Explicit type checks are required due to embedded Header which also does the explicit type check
	untrstH, ok := untrst.(*SignedHeader)
	if !ok {
		// if the header type is wrong, something very bad is going on
		// and is a programmer bug
		panic(fmt.Errorf("%T is not of type %T", untrst, untrstH))
	}
	if err := untrstH.ValidateBasic(); err != nil {
		return &header.VerifyError{
			Reason: err,
		}
	}
	if err := sH.Header.Verify(&untrstH.Header); err != nil {
		return &header.VerifyError{
			Reason: err,
		}
	}

	// TODO: Accept non-adjacent headers until go-header implements feature to accept non-adjacent
	if sH.Height()+1 < untrst.Height() {
		return nil
	}

	sHHash := sH.Header.Hash()
	if !bytes.Equal(untrstH.LastHeaderHash[:], sHHash) {
		return &header.VerifyError{
			Reason: &ErrLastHeaderHashMismatch{
				fmt.Errorf("last header hash %v does not match hash of previous header %v", untrstH.LastHeaderHash[:], sHHash),
			},
		}
	}
	sHLastCommitHash := sH.Commit.GetCommitHash(&untrstH.Header, sH.ProposerAddress)
	if !bytes.Equal(untrstH.LastCommitHash[:], sHLastCommitHash) {
		return &header.VerifyError{
			Reason: &ErrLastCommitHashMismatch{
				fmt.Errorf("last commit hash %v does not match hash of previous header %v", untrstH.LastCommitHash[:], sHHash),
			},
		}
	}
	return nil
}

var _ header.Header = &SignedHeader{}

// ValidateBasic performs basic validation of a signed header.
func (h *SignedHeader) ValidateBasic() error {
	if err := h.Header.ValidateBasic(); err != nil {
		return err
	}

	if err := h.Commit.ValidateBasic(); err != nil {
		return err
	}

	// Handle Based Rollup case
	if h.Validators == nil || len(h.Validators.Validators) == 0 {
		return nil
	}

	if err := h.Validators.ValidateBasic(); err != nil {
		return err
	}

	if !bytes.Equal(h.Validators.Hash(), h.AggregatorsHash[:]) {
		return ErrAggregatorSetHashMismatch
	}

	// Make sure there is exactly one signature
	if len(h.Commit.Signatures) != 1 {
		return errors.New("expected exactly one signature")
	}

	signature := h.Commit.Signatures[0]
	proposer := h.Validators.GetProposer()
	var pubKey ed25519.PubKey = proposer.PubKey.Bytes()
	msg, err := h.Header.MarshalBinary()
	if err != nil {
		return errors.New("signature verification failed, unable to marshal header")
	}
	if !pubKey.VerifySignature(msg, signature) {
		return ErrSignatureVerificationFailed
	}

	return nil
}
