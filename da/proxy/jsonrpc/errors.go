package jsonrpc

import (
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/rollkit/rollkit/da"
)

// getKnownErrorsMapping returns a mapping of known error codes to their corresponding error types.
func getKnownErrorsMapping() jsonrpc.Errors {
	errs := jsonrpc.NewErrors()
	errs.Register(jsonrpc.ErrorCode(da.CodeBlobNotFound), new(*da.ErrBlobNotFound))
	errs.Register(jsonrpc.ErrorCode(da.CodeBlobSizeOverLimit), new(*da.ErrBlobSizeOverLimit))
	errs.Register(jsonrpc.ErrorCode(da.CodeTxTimedOut), new(*da.ErrTxTimedOut))
	errs.Register(jsonrpc.ErrorCode(da.CodeTxAlreadyInMempool), new(*da.ErrTxAlreadyInMempool))
	errs.Register(jsonrpc.ErrorCode(da.CodeTxIncorrectAccountSequence), new(*da.ErrTxIncorrectAccountSequence))
	errs.Register(jsonrpc.ErrorCode(da.CodeTxTooLarge), new(*da.ErrTxTooLarge))
	errs.Register(jsonrpc.ErrorCode(da.CodeContextDeadline), new(*da.ErrContextDeadline))
	errs.Register(jsonrpc.ErrorCode(da.CodeFutureHeight), new(*da.ErrFutureHeight))
	return errs
}
