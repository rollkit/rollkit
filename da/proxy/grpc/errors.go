package grpc

import (
	"errors"

	"google.golang.org/grpc/status"

	"github.com/rollkit/go-da"
	pbda "github.com/rollkit/go-da/types/pb/da"
)

func tryToMapError(err error) error {
	if err == nil {
		return nil
	}

	s, ok := status.FromError(err)
	if ok {
		details := s.Proto().Details
		if len(details) == 1 {
			var errorDetail pbda.ErrorDetails
			unmarshalError := errorDetail.Unmarshal(details[0].Value)
			if unmarshalError != nil {
				return err
			}
			return errorForCode(errorDetail.Code)
		}
	}
	return err
}

func errorForCode(code pbda.ErrorCode) error {
	switch code {
	case pbda.ErrorCode_ERROR_CODE_BLOB_NOT_FOUND:
		return &da.ErrBlobNotFound{}
	case pbda.ErrorCode_ERROR_CODE_BLOB_SIZE_OVER_LIMIT:
		return &da.ErrBlobSizeOverLimit{}
	case pbda.ErrorCode_ERROR_CODE_TX_TIMED_OUT:
		return &da.ErrTxTimedOut{}
	case pbda.ErrorCode_ERROR_CODE_TX_ALREADY_IN_MEMPOOL:
		return &da.ErrTxAlreadyInMempool{}
	case pbda.ErrorCode_ERROR_CODE_TX_INCORRECT_ACCOUNT_SEQUENCE:
		return &da.ErrTxIncorrectAccountSequence{}
	case pbda.ErrorCode_ERROR_CODE_TX_TOO_LARGE:
		return &da.ErrTxTooLarge{}
	case pbda.ErrorCode_ERROR_CODE_CONTEXT_DEADLINE:
		return &da.ErrContextDeadline{}
	case pbda.ErrorCode_ERROR_CODE_FUTURE_HEIGHT:
		return &da.ErrFutureHeight{}
	default:
		return errors.New("unknown error code")
	}
}
