package types

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cosmossdk.io/log"

	coreda "github.com/rollkit/rollkit/core/da"
)

var placeholder = []byte("placeholder")

// SubmitWithHelpers performs blob submission using the underlying DA layer,
// handling error mapping to produce a ResultSubmit.
// It assumes blob size filtering is handled within the DA implementation's Submit.
// It mimics the logic previously found in da.DAClient.Submit.
func SubmitWithHelpers(
	ctx context.Context,
	da coreda.DA, // Use the core DA interface
	logger log.Logger,
	data [][]byte,
	gasPrice float64,
	options []byte,
) coreda.ResultSubmit { // Return core ResultSubmit type
	ids, err := da.SubmitWithOptions(ctx, data, gasPrice, placeholder, options)

	// Handle errors returned by Submit
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Debug("DA submission canceled via helper due to context cancellation")
			return coreda.ResultSubmit{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusContextCanceled,
					Message: "submission canceled",
					IDs:     ids,
				},
			}
		}
		status := coreda.StatusError
		switch {
		case errors.Is(err, coreda.ErrTxTimedOut):
			status = coreda.StatusNotIncludedInBlock
		case errors.Is(err, coreda.ErrTxAlreadyInMempool):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, coreda.ErrTxIncorrectAccountSequence):
			status = coreda.StatusIncorrectAccountSequence
		case errors.Is(err, coreda.ErrBlobSizeOverLimit):
			status = coreda.StatusTooBig
		case errors.Is(err, coreda.ErrContextDeadline):
			status = coreda.StatusContextDeadline
		}
		logger.Error("DA submission failed via helper", "error", err, "status", status)
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:           status,
				Message:        "failed to submit blobs: " + err.Error(),
				IDs:            ids,
				SubmittedCount: uint64(len(ids)),
				Height:         0,
			},
		}
	}

	if len(ids) == 0 && len(data) > 0 {
		logger.Warn("DA submission via helper returned no IDs for non-empty input data")
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: "failed to submit blobs: no IDs returned despite non-empty input",
			},
		}
	}

	// Get height from the first ID
	var height uint64
	if len(ids) > 0 {
		height, _, err = coreda.SplitID(ids[0])
		if err != nil {
			logger.Error("failed to split ID", "error", err)
		}
	}

	logger.Debug("DA submission successful via helper", "num_ids", len(ids))
	return coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			IDs:            ids,
			SubmittedCount: uint64(len(ids)),
			Height:         height,
			BlobSize:       0,
		},
	}
}

// RetrieveWithHelpers performs blob retrieval using the underlying DA layer,
// handling error mapping to produce a ResultRetrieve.
// It mimics the logic previously found in da.DAClient.Retrieve.
func RetrieveWithHelpers(
	ctx context.Context,
	da coreda.DA,
	logger log.Logger,
	dataLayerHeight uint64,
	namespace []byte,
) coreda.ResultRetrieve {

	// 1. Get IDs with timeout handling
	var idsResult *coreda.GetIDsResult
	var err error

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusContextCanceled,
				Message: "context cancelled before GetIDs call",
				Height:  dataLayerHeight,
			},
		}
	default:
	}

	logger.Debug("Making RPC call", "height", dataLayerHeight, "method", "GetIDs", "module", "main", "namespace", fmt.Sprintf("%q", namespace))

	idsResult, err = da.GetIDs(ctx, dataLayerHeight, namespace)
	if err != nil {
		// Check if context was cancelled during the call
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			logger.Debug("Retrieve helper: GetIDs cancelled", "height", dataLayerHeight)
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusContextCanceled,
					Message: "GetIDs call was cancelled",
					Height:  dataLayerHeight,
				},
			}
		}

		// Handle specific "not found" error
		if strings.Contains(err.Error(), coreda.ErrBlobNotFound.Error()) {
			logger.Debug("Retrieve helper: Blobs not found at height", "height", dataLayerHeight)
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusNotFound,
					Message: coreda.ErrBlobNotFound.Error(),
					Height:  dataLayerHeight,
				},
			}
		}
		if strings.Contains(err.Error(), coreda.ErrHeightFromFuture.Error()) {
			logger.Debug("Retrieve helper: Blobs not found at height", "height", dataLayerHeight)
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusHeightFromFuture,
					Message: coreda.ErrHeightFromFuture.Error(),
					Height:  dataLayerHeight,
				},
			}
		}

		// Check for timeout/deadline errors
		if strings.Contains(err.Error(), "context deadline exceeded") ||
		   strings.Contains(err.Error(), "timeout") ||
		   errors.Is(err, coreda.ErrContextDeadline) {
			logger.Debug("Retrieve helper: GetIDs timeout", "height", dataLayerHeight, "error", err)
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusContextDeadline,
					Message: fmt.Sprintf("GetIDs timeout: %s", err.Error()),
					Height:  dataLayerHeight,
				},
			}
		}

		// Handle other errors during GetIDs
		logger.Error("Retrieve helper: Failed to get IDs", "height", dataLayerHeight, "error", err)
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: fmt.Sprintf("failed to get IDs: %s", err.Error()),
				Height:  dataLayerHeight,
			},
		}
	}

	logger.Debug("RPC call successful", "method", "GetIDs", "module", "main")

	// This check should technically be redundant if GetIDs correctly returns ErrBlobNotFound
	if idsResult == nil || len(idsResult.IDs) == 0 {
		logger.Debug("Retrieve helper: No IDs found at height", "height", dataLayerHeight)
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusNotFound,
				Message: coreda.ErrBlobNotFound.Error(),
				Height:  dataLayerHeight,
			},
		}
	}

	// 2. Get Blobs using the retrieved IDs
	logger.Debug("Making RPC call", "method", "Get", "module", "main", "namespace", fmt.Sprintf("%q", namespace), "num_ids", len(idsResult.IDs))

	blobs, err := da.Get(ctx, idsResult.IDs, namespace)
	if err != nil {
		// Check if context was cancelled during the call
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			logger.Debug("Retrieve helper: Get blobs cancelled", "height", dataLayerHeight, "num_ids", len(idsResult.IDs))
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusContextCanceled,
					Message: "Get blobs call was cancelled",
					Height:  dataLayerHeight,
				},
			}
		}

		// Check for timeout/deadline errors
		if strings.Contains(err.Error(), "context deadline exceeded") ||
		   strings.Contains(err.Error(), "timeout") ||
		   errors.Is(err, coreda.ErrContextDeadline) {
			logger.Error("Retrieve helper: Failed to get blobs", "height", dataLayerHeight, "num_ids", len(idsResult.IDs), "error", err)
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusContextDeadline,
					Message: fmt.Sprintf("failed to get blobs: %s", err.Error()),
					Height:  dataLayerHeight,
				},
			}
		}

		// Handle other errors during Get
		logger.Error("Retrieve helper: Failed to get blobs", "height", dataLayerHeight, "num_ids", len(idsResult.IDs), "error", err)
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: fmt.Sprintf("failed to get blobs: %s", err.Error()),
				Height:  dataLayerHeight,
			},
		}
	}

	logger.Debug("RPC call successful", "method", "Get", "module", "main")

	// Success
	logger.Debug("Retrieve helper: Successfully retrieved blobs", "height", dataLayerHeight, "num_blobs", len(blobs))
	return coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:      coreda.StatusSuccess,
			Height:    dataLayerHeight,
			IDs:       idsResult.IDs,
			Timestamp: idsResult.Timestamp,
		},
		Data: blobs,
	}
}
