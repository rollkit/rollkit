package da

import (
	"errors"
	"fmt"
)

var (
	ErrBlobSizeOverLimit          = fmt.Errorf("blob size over limit")
	ErrTxTimedOut                 = errors.New("transaction timed out")
	ErrTxAlreadyInMempool         = errors.New("transaction already in mempool")
	ErrTxIncorrectAccountSequence = errors.New("incorrect account sequence")
	ErrTxTooLarge                 = errors.New("transaction too large")
	ErrContextDeadline            = errors.New("context deadline exceeded")
	ErrBlobNotFound               = errors.New("blob not found")
)
