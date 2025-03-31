package jsonrpc

import (
	"github.com/filecoin-project/go-jsonrpc"

	coreda "github.com/rollkit/rollkit/core/da"
	da "github.com/rollkit/rollkit/da"
)

// getKnownErrorsMapping returns a mapping of known error codes to their corresponding error types.
func getKnownErrorsMapping() jsonrpc.Errors {
	errs := jsonrpc.NewErrors()
	errs.Register(jsonrpc.ErrorCode(coreda.StatusNotFound), &da.ErrBlobNotFound)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusTooBig), &da.ErrBlobSizeOverLimit)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusContextDeadline), &da.ErrTxTimedOut)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusAlreadyInMempool), &da.ErrTxAlreadyInMempool)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusIncorrectAccountSequence), &da.ErrTxIncorrectAccountSequence)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusTooBig), &da.ErrTxTooLarge)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusContextDeadline), &da.ErrContextDeadline)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusFutureHeight), &da.ErrFutureHeight)
	return errs
}
