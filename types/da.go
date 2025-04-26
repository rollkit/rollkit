package types

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/log"

	coreda "github.com/rollkit/rollkit/core/da"
)

// SubmitWithHelpers performs blob submission using the underlying DA layer,
// handling error mapping to produce a ResultSubmit.
// It assumes blob size filtering is handled within the DA implementation's SubmitWithOptions.
// It mimics the logic previously found in da.DAClient.Submit.
func SubmitWithHelpers(
	ctx context.Context,
	da coreda.DA, // Use the core DA interface
	logger log.Logger,
	data [][]byte,
	gasPrice float64,
	namespace []byte,
	options []byte,
) coreda.ResultSubmit { // Return core ResultSubmit type

	ids, err := da.SubmitWithOptions(ctx, data, gasPrice, namespace, options)

	// Handle errors returned by SubmitWithOptions
	if err != nil {
		status := coreda.StatusError
		switch {
		case errors.Is(err, coreda.ErrTxTimedOut):
			status = coreda.StatusNotIncludedInBlock
		case errors.Is(err, coreda.ErrTxAlreadyInMempool):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, coreda.ErrTxIncorrectAccountSequence):
			status = coreda.StatusAlreadyInMempool // Assuming same handling
		case errors.Is(err, coreda.ErrBlobSizeOverLimit):
			status = coreda.StatusTooBig
		case errors.Is(err, coreda.ErrContextDeadline):
			status = coreda.StatusContextDeadline
		}
		logger.Error("DA submission failed via helper", "error", err, "status", status)
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    status,
				Message: "failed to submit blobs: " + err.Error(),
				// Include IDs if available, even on error (might indicate partial success if DA impl returns them)
				IDs:            ids,
				SubmittedCount: uint64(len(ids)),
			},
		}
	}

	// Handle successful submission (potentially partial if DA impl filtered blobs)
	if len(ids) == 0 && len(data) > 0 {
		// If no IDs were returned but we submitted data, it implies an issue (e.g., all filtered out)
		logger.Warn("DA submission via helper returned no IDs for non-empty input data")
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError, // Or potentially StatusTooBig if filtering is the only reason
				Message: "failed to submit blobs: no IDs returned despite non-empty input",
			},
		}
	}

	logger.Debug("DA submission successful via helper", "num_ids", len(ids))
	return coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			IDs:            ids,              // Set IDs within BaseResult
			SubmittedCount: uint64(len(ids)), // Set SubmittedCount within BaseResult
			// Height and BlobSize might not be available from SubmitWithOptions interface.
			Height:   0,
			BlobSize: 0,
		},
	}
}

// RetrieveWithHelpers performs blob retrieval using the underlying DA layer,
// handling error mapping to produce a ResultRetrieve.
// It mimics the logic previously found in da.DAClient.Retrieve.
func RetrieveWithHelpers(
	ctx context.Context,
	da coreda.DA, // Use the core DA interface
	logger log.Logger,
	dataLayerHeight uint64,
	namespace []byte,
) coreda.ResultRetrieve { // Return core ResultRetrieve type

	// 1. Get IDs
	idsResult, err := da.GetIDs(ctx, dataLayerHeight, namespace)
	if err != nil {
		// Handle specific "not found" error
		if errors.Is(err, coreda.ErrBlobNotFound) {
			logger.Debug("Retrieve helper: Blobs not found at height", "height", dataLayerHeight)
			return coreda.ResultRetrieve{
				BaseResult: coreda.BaseResult{
					Code:    coreda.StatusNotFound,
					Message: coreda.ErrBlobNotFound.Error(),
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
	blobs, err := da.Get(ctx, idsResult.IDs, namespace)
	if err != nil {
		// Handle errors during Get
		logger.Error("Retrieve helper: Failed to get blobs", "height", dataLayerHeight, "num_ids", len(idsResult.IDs), "error", err)
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: fmt.Sprintf("failed to get blobs: %s", err.Error()),
				Height:  dataLayerHeight,
			},
		}
	}

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
