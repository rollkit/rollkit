package jsonrpc

import (
	"github.com/filecoin-project/go-jsonrpc"

	coreda "github.com/evstack/ev-node/core/da"
)

// getKnownErrorsMapping returns a mapping of known error codes to their corresponding error types.
func getKnownErrorsMapping() jsonrpc.Errors {
	errs := jsonrpc.NewErrors()
	errs.Register(jsonrpc.ErrorCode(coreda.StatusNotFound), &coreda.ErrBlobNotFound)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusTooBig), &coreda.ErrBlobSizeOverLimit)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusContextDeadline), &coreda.ErrTxTimedOut)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusAlreadyInMempool), &coreda.ErrTxAlreadyInMempool)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusIncorrectAccountSequence), &coreda.ErrTxIncorrectAccountSequence)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusContextDeadline), &coreda.ErrContextDeadline)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusContextCanceled), &coreda.ErrContextCanceled)
	errs.Register(jsonrpc.ErrorCode(coreda.StatusHeightFromFuture), &coreda.ErrHeightFromFuture)
	return errs
}
