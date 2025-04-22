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

// GetNamespace returns the namespace for the DA layer.
func (dac *DAClient) GetNamespace(ctx context.Context) ([]byte, error) {
	return dac.Namespace, nil
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
func (dac *DAClient) Submit(ctx context.Context, data [][]byte, maxBlobSize uint64, gasPrice float64) coreda.ResultSubmit {
	var (
		blobs    [][]byte = make([][]byte, 0, len(data))
		blobSize uint64
		message  string
	)
	for i := range data {
		blob := data[i]
		if blobSize+uint64(len(blob)) > maxBlobSize {
			message = fmt.Sprint(coreda.ErrBlobSizeOverLimit.Error(), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
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
				Message: "failed to submit blobs: no blobs generated " + message,
			},
		}
	}

	ids, err := dac.submit(ctx, blobs, gasPrice, dac.Namespace)
	if err != nil {
		status := coreda.StatusError
		switch {
		case errors.Is(err, coreda.ErrTxTimedOut):
			status = coreda.StatusNotIncludedInBlock
		case errors.Is(err, coreda.ErrTxAlreadyInMempool):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, coreda.ErrTxIncorrectAccountSequence):
			status = coreda.StatusAlreadyInMempool
		case errors.Is(err, coreda.ErrBlobSizeOverLimit):
			status = coreda.StatusTooBig
		case errors.Is(err, coreda.ErrContextDeadline):
			status = coreda.StatusContextDeadline
		}
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    status,
				Message: "failed to submit blobs: " + err.Error(),
			},
		}
	}

	if len(ids) == 0 {
		return coreda.ResultSubmit{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: "failed to submit blobs: unexpected len(ids): 0",
			},
		}
	}

	return coreda.ResultSubmit{
		BaseResult: coreda.BaseResult{
			Code:           coreda.StatusSuccess,
			IDs:            ids,
			SubmittedCount: uint64(len(ids)),
		},
	}
}

// RetrieveHeaders retrieves block headers from DA.
// It is on the caller to decode the headers
func (dac *DAClient) Retrieve(ctx context.Context, dataLayerHeight uint64) coreda.ResultRetrieve {
	result, err := dac.DA.GetIDs(ctx, dataLayerHeight, dac.Namespace)
	if err != nil {
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: fmt.Sprintf("failed to get IDs: %s", err.Error()),
				Height:  dataLayerHeight,
			},
		}
	}

	// If no blocks are found, return a non-blocking error.
	if result == nil || len(result.IDs) == 0 {
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusNotFound,
				Message: coreda.ErrBlobNotFound.Error(),
				Height:  dataLayerHeight,
			},
		}
	}

	blobs, err := dac.DA.Get(ctx, result.IDs, dac.Namespace)
	if err != nil {
		return coreda.ResultRetrieve{
			BaseResult: coreda.BaseResult{
				Code:    coreda.StatusError,
				Message: fmt.Sprintf("failed to get blobs: %s", err.Error()),
				Height:  dataLayerHeight,
			},
		}
	}

	return coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:      coreda.StatusSuccess,
			Height:    dataLayerHeight,
			IDs:       result.IDs,
			Timestamp: result.Timestamp,
		},
		Data: blobs,
	}
}

func (dac *DAClient) submit(ctx context.Context, blobs []coreda.Blob, gasPrice float64, namespace []byte) ([]coreda.ID, error) {
	return dac.DA.SubmitWithOptions(ctx, blobs, gasPrice, namespace, dac.SubmitOptions)
}
