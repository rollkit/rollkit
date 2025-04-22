package da

import (
	"errors"
)

var (
	ErrBlobNotFound               = errors.New("blob: not found")
	ErrBlobSizeOverLimit          = errors.New("blob: over size limit")
	ErrTxTimedOut                 = errors.New("timed out waiting for tx to be included in a block")
	ErrTxAlreadyInMempool         = errors.New("tx already in mempool")
	ErrTxIncorrectAccountSequence = errors.New("incorrect account sequence")
	ErrTxTooLarge                 = errors.New("tx too large")
	ErrContextDeadline            = errors.New("context deadline")
	ErrFutureHeight               = errors.New("future height")
)
