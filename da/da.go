package da

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/log"

	coreda "github.com/rollkit/rollkit/core/da"
)

// DAClient is a new DA implementation.
type DAClient struct {
	DA            coreda.DA
	gasPrice      float64
	gasMultiplier float64
	Namespace     []byte
	SubmitOptions []byte
	Logger        log.Logger
}

// NewDAClient returns a new DA client.
func NewDAClient(da coreda.DA, gasPrice, gasMultiplier float64, ns []byte, options []byte, logger log.Logger) coreda.Client {
	return &DAClient{
		DA:            da,
		gasPrice:      gasPrice,
		gasMultiplier: gasMultiplier,
		Namespace:     ns,
		SubmitOptions: options,
		Logger:        logger,
	}
}

// MaxBlobSize returns the maximum blob size for the DA layer.
func (dac *DAClient) MaxBlobSize(ctx context.Context) (uint64, error) {
	return dac.DA.MaxBlobSize(ctx)
}

// GasPrice returns the gas price for the DA layer.
func (dac *DAClient) GasPrice(ctx context.Context) (float64, error) {
	return dac.DA.GasPrice(ctx)
}

// GasMultiplier returns the gas multiplier for the DA layer.
func (dac *DAClient) GasMultiplier(ctx context.Context) (float64, error) {
	return dac.DA.GasMultiplier(ctx)
}

// SubmitHeaders submits block headers to DA.
func (dac *DAClient) SubmitHeaders(ctx context.Context, headers [][]byte, maxBlobSize uint64, gasPrice float64) coreda.ResultSubmit {
	var (
		blobs    [][]byte
		blobSize uint64
		message  string
	)
	for i := range headers {
		blob := headers[i]
		if blobSize+uint64(len(blob)) > maxBlobSize {
			message = fmt.Sprint(ErrBlobSizeOverLimit.Error(), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
			dac.Logger.Info(message)
			break
		}
		blobSize += uint64(len(blob))
		blobs = append(blobs, blob)
	}
	if len(blobs) == 0 {
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: "failed to submit headers: no blobs generated " + message,
			},
		}
	}

	ids, height, err := dac.submit(ctx, blobs, gasPrice, dac.Namespace)
	if err != nil {
		status := coreda.StatusError
		switch {
		case errors.Is(err, ErrTxTimedOut):
			status = coreda.StatusNotIncludedInBlock
		case errors.Is(err, ErrTxAlreadyInMempool):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, ErrTxIncorrectAccountSequence):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, ErrTxTooLarge):
			status = coreda.StatusTooBig
		case errors.Is(err, ErrContextDeadline):
			status = coreda.StatusContextDeadline
		}
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    status,
				Message: "failed to submit headers: " + err.Error(),
			},
		}
	}

	if len(ids) == 0 {
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: "failed to submit headers: unexpected len(ids): 0",
			},
		}
	}

	return coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			DAHeight:       height,
			SubmittedCount: uint64(len(ids)),
		},
	}
}

// RetrieveHeaders retrieves block headers from DA.
// It is on the caller to decode the headers
func (dac *DAClient) RetrieveHeaders(ctx context.Context, dataLayerHeight uint64) coreda.ResultRetrieveHeaders {
	result, err := dac.DA.GetIDs(ctx, dataLayerHeight, dac.Namespace)
	if err != nil {
		return coreda.ResultRetrieveHeaders{
			BaseResult: coreda.BaseResult{
				Code:     coreda.StatusError,
				Message:  fmt.Sprintf("failed to get IDs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	// If no blocks are found, return a non-blocking error.
	if result == nil || len(result.IDs) == 0 {
		return coreda.ResultRetrieveHeaders{
			BaseResult: coreda.BaseResult{
				Code:     coreda.StatusNotFound,
				Message:  ErrBlobNotFound.Error(),
				DAHeight: dataLayerHeight,
			},
		}
	}

	blobs, err := dac.DA.Get(ctx, result.IDs, dac.Namespace)
	if err != nil {
		return coreda.ResultRetrieveHeaders{
			BaseResult: coreda.BaseResult{
				Code:     coreda.StatusError,
				Message:  fmt.Sprintf("failed to get blobs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	headers := make([][]byte, len(blobs))
	for i, blob := range blobs {
		headers[i] = blob
		dac.Logger.Error("failed to unmarshal block", "daHeight", dataLayerHeight, "position", i, "error", err)
		continue
	}

	return coreda.ResultRetrieveHeaders{
		BaseResult: coreda.BaseResult{
			Code:     coreda.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Headers: blobs,
	}
}

func (dac *DAClient) submit(ctx context.Context, blobs []coreda.Blob, gasPrice float64, namespace []byte) ([]coreda.ID, uint64, error) {
	return dac.DA.Submit(ctx, blobs, gasPrice, namespace, dac.SubmitOptions)
}

//--------------------------------
// Batches
//--------------------------------

// SubmitBatch submits block data to DA.
func (dac *DAClient) SubmitBatch(ctx context.Context, data [][]byte, maxBlobSize uint64, gasPrice float64) coreda.ResultSubmitBatch {
	var (
		blobs    [][]byte
		blobSize uint64
		message  string
	)
	for i := range data {
		blob := data[i]
		if blobSize+uint64(len(blob)) > maxBlobSize {
			message = fmt.Sprint((&ErrBlobSizeOverLimit), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
			dac.Logger.Info(message)
			break
		}
		blobSize += uint64(len(blob))
		blobs = append(blobs, blob)
	}
	if len(blobs) == 0 {
		return coreda.ResultSubmitBatch{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: "failed to submit blocks: no blobs generated " + message,
			},
		}
	}

	ids, height, err := dac.submit(ctx, blobs, gasPrice, dac.Namespace)
	if err != nil {
		status := coreda.StatusError
		switch {
		case errors.Is(err, ErrTxTimedOut):
			status = coreda.StatusNotIncludedInBlock
		case errors.Is(err, ErrTxAlreadyInMempool):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, ErrTxIncorrectAccountSequence):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, ErrTxTooLarge):
			status = coreda.StatusTooBig
		case errors.Is(err, ErrContextDeadline):
			status = coreda.StatusContextDeadline
		}
		return coreda.ResultSubmitBatch{
			BaseResult: coreda.BaseResult{
				Code:    status,
				Message: "failed to submit block data: " + err.Error(),
			},
		}
	}

	// check if the ids are empty and if the length of ids is not equal to the length of blobs
	if len(ids) == 0 || len(ids) != len(blobs) {
		return coreda.ResultSubmitBatch{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: fmt.Sprintf("failed to submit data: unexpected len(ids): %d, len(blobs): %d", len(ids), len(blobs)),
			},
		}
	}

	return coreda.ResultSubmitBatch{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			DAHeight:       height,
			SubmittedCount: uint64(len(ids)),
		},
	}
}

// RetrieveBatch retrieves block data from DA.
func (dac *DAClient) RetrieveBatch(ctx context.Context, dataLayerHeight uint64) coreda.ResultRetrieveBatch {
	idsResult, err := dac.DA.GetIDs(ctx, dataLayerHeight, dac.Namespace)
	if err != nil {
		return coreda.ResultRetrieveBatch{
			BaseResult: coreda.BaseResult{
				Code:     coreda.StatusError,
				Message:  fmt.Sprintf("failed to get IDs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}
	ids := idsResult.IDs

	// If no block data are found, return a non-blocking error.
	if len(ids) == 0 {
		return coreda.ResultRetrieveBatch{
			BaseResult: coreda.BaseResult{
				Code:     coreda.StatusNotFound,
				Message:  ErrBlobNotFound.Error(),
				DAHeight: dataLayerHeight,
			},
		}
	}

	blobs, err := dac.DA.Get(ctx, ids, dac.Namespace)
	if err != nil {
		return coreda.ResultRetrieveBatch{
			BaseResult: coreda.BaseResult{
				Code:     coreda.StatusError,
				Message:  fmt.Sprintf("failed to get blobs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	return coreda.ResultRetrieveBatch{
		BaseResult: coreda.BaseResult{
			Code:     coreda.StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Data: blobs,
	}
}
