package types

import (
	"errors"
	"fmt"
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
