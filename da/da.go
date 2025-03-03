package da

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/log"

	coreda "github.com/rollkit/rollkit/core/da"
)

const (
	// defaultSubmitTimeout is the timeout for block submission
	defaultSubmitTimeout = 60 * time.Second

	// defaultRetrieveTimeout is the timeout for block retrieval
	defaultRetrieveTimeout = 60 * time.Second
)

// DAClient is a new DA implementation.
type DAClient struct {
	DA              coreda.DA
	GasPrice        float64
	GasMultiplier   float64
	Namespace       []byte
	SubmitOptions   []byte
	SubmitTimeout   time.Duration
	RetrieveTimeout time.Duration
	Logger          log.Logger
}

// NewDAClient returns a new DA client.
func NewDAClient(da coreda.DA, gasPrice, gasMultiplier float64, ns []byte, options []byte, logger log.Logger) coreda.Client {
	return &DAClient{
		DA:              da,
		GasPrice:        gasPrice,
		GasMultiplier:   gasMultiplier,
		Namespace:       ns,
		SubmitOptions:   options,
		SubmitTimeout:   defaultSubmitTimeout,
		RetrieveTimeout: defaultRetrieveTimeout,
		Logger:          logger,
	}
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

	ctx, cancel := context.WithTimeout(ctx, dac.SubmitTimeout)
	defer cancel()
	ids, err := dac.submit(ctx, blobs, gasPrice, dac.Namespace)
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
			DAHeight:       binary.LittleEndian.Uint64(ids[0]),
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

	ctx, cancel := context.WithTimeout(ctx, dac.RetrieveTimeout)
	defer cancel()
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

func (dac *DAClient) submit(ctx context.Context, blobs []coreda.Blob, gasPrice float64, namespace []byte) ([]coreda.ID, error) {
	if len(dac.SubmitOptions) == 0 {
		return dac.DA.Submit(ctx, blobs, gasPrice, namespace)
	}
	return dac.DA.SubmitWithOptions(ctx, blobs, gasPrice, namespace, dac.SubmitOptions)
}
