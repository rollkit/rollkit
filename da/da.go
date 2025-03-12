package da

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/log"
	"github.com/gogo/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

const (
	// defaultSubmitTimeout is the timeout for block submission
	defaultSubmitTimeout = 60 * time.Second

	// defaultRetrieveTimeout is the timeout for block retrieval
	defaultRetrieveTimeout = 60 * time.Second
)

// StatusCode is a type for DA layer return status.
// TODO: define an enum of different non-happy-path cases
// that might need to be handled by Rollkit independent of
// the underlying DA chain.
type StatusCode uint64

// Data Availability return codes.
const (
	StatusUnknown StatusCode = iota
	StatusSuccess
	StatusNotFound
	StatusNotIncludedInBlock
	StatusAlreadyInMempool
	StatusTooBig
	StatusContextDeadline
	StatusError
)

// BaseResult contains basic information returned by DA layer.
type BaseResult struct {
	// Code is to determine if the action succeeded.
	Code StatusCode
	// Message may contain DA layer specific information (like DA block height/hash, detailed error message, etc)
	Message string
	// DAHeight informs about a height on Data Availability Layer for given result.
	DAHeight uint64
	// SubmittedCount is the number of successfully submitted blocks.
	SubmittedCount uint64
	// BlobSize is the size of the blob submitted.
	BlobSize uint64
}

// ResultSubmit contains information returned from DA layer after block headers/data submission.
type ResultSubmit struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}

// ResultRetrieveHeaders contains batch of block headers returned from DA layer client.
type ResultRetrieveHeaders struct {
	BaseResult
	// Header is the block header retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Headers []*types.SignedHeader
}

// DAClient is a new DA implementation.
type DAClient struct {
	DA              coreda.DA
	GasPrice        float64
	GasMultiplier   float64
	Namespace       coreda.Namespace
	SubmitOptions   []byte
	SubmitTimeout   time.Duration
	RetrieveTimeout time.Duration
	logger          log.Logger
}

// NewDAClient returns a new DA client.
func NewDAClient(da coreda.DA, gasPrice, gasMultiplier float64, ns coreda.Namespace, logger log.Logger, options []byte) *DAClient {
	return &DAClient{
		DA:              da,
		GasPrice:        gasPrice,
		GasMultiplier:   gasMultiplier,
		Namespace:       ns,
		SubmitOptions:   options,
		SubmitTimeout:   defaultSubmitTimeout,
		RetrieveTimeout: defaultRetrieveTimeout,
		logger:          logger,
	}
}

// SubmitHeaders submits block headers to DA.
func (dac *DAClient) SubmitHeaders(ctx context.Context, headers []*types.SignedHeader, maxBlobSize uint64, gasPrice float64) ResultSubmit {
	var (
		blobs    [][]byte
		blobSize uint64
		message  string
	)
	for i := range headers {
		blob, err := headers[i].MarshalBinary()
		if err != nil {
			message = fmt.Sprint("failed to serialize header", err)
			dac.logger.Info(message)
			break
		}
		if blobSize+uint64(len(blob)) > maxBlobSize {
			message = fmt.Sprint((&ErrBlobSizeOverLimit{}).Error(), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
			dac.logger.Info(message)
			break
		}
		blobSize += uint64(len(blob))
		blobs = append(blobs, blob)
	}
	if len(blobs) == 0 {
		return ResultSubmit{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: "failed to submit headers: no blobs generated " + message,
			},
		}
	}

	ctx, cancel := context.WithTimeout(ctx, dac.SubmitTimeout)
	defer cancel()
	ids, err := dac.submit(ctx, blobs, gasPrice, dac.Namespace)
	if err != nil {
		status := StatusError
		switch {
		case errors.Is(err, &ErrTxTimedOut{}):
			status = StatusNotIncludedInBlock
		case errors.Is(err, &ErrTxAlreadyInMempool{}):
			status = StatusAlreadyInMempool
		case errors.Is(err, &ErrTxIncorrectAccountSequence{}):
			status = StatusAlreadyInMempool
		case errors.Is(err, &ErrTxTooLarge{}):
			status = StatusTooBig
		case errors.Is(err, &ErrContextDeadline{}):
			status = StatusContextDeadline
		}
		return ResultSubmit{
			BaseResult: BaseResult{
				Code:    status,
				Message: "failed to submit headers: " + err.Error(),
			},
		}
	}

	if len(ids) == 0 {
		return ResultSubmit{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: "failed to submit headers: unexpected len(ids): 0",
			},
		}
	}

	return ResultSubmit{
		BaseResult: BaseResult{
			Code:           StatusSuccess,
			DAHeight:       binary.LittleEndian.Uint64(ids[0]),
			SubmittedCount: uint64(len(ids)),
		},
	}
}

// RetrieveHeaders retrieves block headers from DA.
func (dac *DAClient) RetrieveHeaders(ctx context.Context, dataLayerHeight uint64) ResultRetrieveHeaders {
	result, err := dac.DA.GetIDs(ctx, dataLayerHeight, dac.Namespace)
	if err != nil {
		return ResultRetrieveHeaders{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get IDs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	// If no blocks are found, return a non-blocking error.
	if result == nil || len(result.IDs) == 0 {
		return ResultRetrieveHeaders{
			BaseResult: BaseResult{
				Code:     StatusNotFound,
				Message:  (&ErrBlobNotFound{}).Error(),
				DAHeight: dataLayerHeight,
			},
		}
	}

	ctx, cancel := context.WithTimeout(ctx, dac.RetrieveTimeout)
	defer cancel()
	blobs, err := dac.DA.Get(ctx, result.IDs, dac.Namespace)
	if err != nil {
		return ResultRetrieveHeaders{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get blobs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	headers := make([]*types.SignedHeader, len(blobs))
	for i, blob := range blobs {
		var header pb.SignedHeader
		err = proto.Unmarshal(blob, &header)
		if err != nil {
			dac.logger.Error("failed to unmarshal block", "daHeight", dataLayerHeight, "position", i, "error", err)
			continue
		}
		headers[i] = new(types.SignedHeader)
		err := headers[i].FromProto(&header)
		if err != nil {
			return ResultRetrieveHeaders{
				BaseResult: BaseResult{
					Code:    StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return ResultRetrieveHeaders{
		BaseResult: BaseResult{
			Code:     StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Headers: headers,
	}
}

func (dac *DAClient) submit(ctx context.Context, blobs []coreda.Blob, gasPrice float64, namespace coreda.Namespace) ([]coreda.ID, error) {
	if len(dac.SubmitOptions) == 0 {
		return dac.DA.Submit(ctx, blobs, gasPrice, namespace)
	}
	return dac.DA.SubmitWithOptions(ctx, blobs, gasPrice, namespace, dac.SubmitOptions)
}

//--------------------------------
// Batches
//--------------------------------

// ResultSubmitBatch contains information returned from DA layer after block headers/data submission.
type ResultSubmitBatch struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}

// ResultRetrieveBatch contains batch of block data returned from DA layer client.
type ResultRetrieveBatch struct {
	BaseResult
	// Data is the block data retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Data []*types.Data
}

// SubmitBatch submits block data to DA.
func (dac *DAClient) SubmitBatch(ctx context.Context, data []*coresequencer.Batch, maxBlobSize uint64, gasPrice float64) ResultSubmitBatch {
	var (
		blobs    [][]byte
		blobSize uint64
		message  string
	)
	for i := range data {
		protoBatch := &pb.Batch{
			Txs: data[i].Transactions,
		}
		blob, err := proto.Marshal(protoBatch)
		if err != nil {
			message = fmt.Sprint("failed to serialize block", err)
			dac.logger.Info(message)
			break
		}
		if blobSize+uint64(len(blob)) > maxBlobSize {
			message = fmt.Sprint((&ErrBlobSizeOverLimit{}).Error(), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
			dac.logger.Info(message)
			break
		}
		blobSize += uint64(len(blob))
		blobs = append(blobs, blob)
	}
	if len(blobs) == 0 {
		return ResultSubmitBatch{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: "failed to submit blocks: no blobs generated " + message,
			},
		}
	}

	ctx, cancel := context.WithTimeout(ctx, dac.SubmitTimeout)
	defer cancel()
	ids, err := dac.DA.Submit(ctx, blobs, gasPrice, dac.Namespace)
	if err != nil {
		status := StatusError
		switch {
		case errors.Is(err, &ErrTxTimedOut{}):
			status = StatusNotIncludedInBlock
		case errors.Is(err, &ErrTxAlreadyInMempool{}):
			status = StatusAlreadyInMempool
		case errors.Is(err, &ErrTxIncorrectAccountSequence{}):
			status = StatusAlreadyInMempool
		case errors.Is(err, &ErrTxTooLarge{}):
			status = StatusTooBig
		case errors.Is(err, &ErrContextDeadline{}):
			status = StatusContextDeadline
		}
		return ResultSubmitBatch{
			BaseResult: BaseResult{
				Code:    status,
				Message: "failed to submit block data: " + err.Error(),
			},
		}
	}

	// check if the ids are empty and if the length of ids is not equal to the length of blobs
	if len(ids) == 0 || len(ids) != len(blobs) {
		return ResultSubmitBatch{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: fmt.Sprintf("failed to submit data: unexpected len(ids): %d, len(blobs): %d", len(ids), len(blobs)),
			},
		}
	}

	return ResultSubmitBatch{
		BaseResult: BaseResult{
			Code:           StatusSuccess,
			DAHeight:       binary.LittleEndian.Uint64(ids[0]),
			BlobSize:       blobSize,
			SubmittedCount: uint64(len(ids)),
		},
	}
}

// RetrieveBatch retrieves block data from DA.
func (dac *DAClient) RetrieveBatch(ctx context.Context, dataLayerHeight uint64) ResultRetrieveBatch {
	idsResult, err := dac.DA.GetIDs(ctx, dataLayerHeight, dac.Namespace)
	if err != nil {
		return ResultRetrieveBatch{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get IDs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}
	ids := idsResult.IDs

	// If no block data are found, return a non-blocking error.
	if len(ids) == 0 {
		return ResultRetrieveBatch{
			BaseResult: BaseResult{
				Code:     StatusNotFound,
				Message:  (&ErrBlobNotFound{}).Error(),
				DAHeight: dataLayerHeight,
			},
		}
	}

	ctx, cancel := context.WithTimeout(ctx, dac.RetrieveTimeout)
	defer cancel()
	blobs, err := dac.DA.Get(ctx, ids, dac.Namespace)
	if err != nil {
		return ResultRetrieveBatch{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get blobs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	data := make([]*types.Data, len(blobs))
	for i, blob := range blobs {
		var d pb.Data
		err = proto.Unmarshal(blob, &d)
		if err != nil {
			dac.logger.Error("failed to unmarshal block data", "daHeight", dataLayerHeight, "position", i, "error", err)
			continue
		}
		data[i] = new(types.Data)
		err := data[i].FromProto(&d)
		if err != nil {
			return ResultRetrieveBatch{
				BaseResult: BaseResult{
					Code:    StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return ResultRetrieveBatch{
		BaseResult: BaseResult{
			Code:     StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Data: data,
	}
}
