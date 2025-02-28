package da

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/log"
	"github.com/cosmos/gogoproto/proto"

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

var (
	// ErrBlobNotFound is used to indicate that the blob was not found.
	ErrBlobNotFound = errors.New("blob: not found")

	// ErrBlobSizeOverLimit is used to indicate that the blob size is over limit
	ErrBlobSizeOverLimit = errors.New("blob: over size limit")

	// ErrTxTimedout is the error message returned by the DA when mempool is congested
	ErrTxTimedout = errors.New("timed out waiting for tx to be included in a block")

	// ErrTxAlreadyInMempool is  the error message returned by the DA when tx is already in mempool
	ErrTxAlreadyInMempool = errors.New("tx already in mempool")

	// ErrTxIncorrectAccountSequence is the error message returned by the DA when tx has incorrect sequence
	ErrTxIncorrectAccountSequence = errors.New("incorrect account sequence")

	// ErrTxSizeTooBig is the error message returned by the DA when tx size is too big
	ErrTxSizeTooBig = errors.New("tx size is too big")

	//ErrTxTooLarge is the err message returned by the DA when tx size is too large
	ErrTxTooLarge = errors.New("tx too large")

	// ErrContextDeadline is the error message returned by the DA when context deadline exceeds
	ErrContextDeadline = errors.New("context deadline")
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
	// BlobSize is the size of the blob submitted.
	BlobSize uint64
	// SubmittedCount is the number of successfully submitted blocks.
	SubmittedCount uint64
}

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

// DAClient is a new DA implementation.
type DAClient struct {
	logger          log.Logger
	DA              coreda.DA
	GasPrice        int64
	GasMultiplier   int64
	Namespace       coreda.Namespace
	SubmitTimeout   time.Duration
	RetrieveTimeout time.Duration
}

// NewDAClient returns a new DA client.
func NewDAClient(da coreda.DA, gasPrice, gasMultiplier int64, ns coreda.Namespace, logger log.Logger) *DAClient {
	return &DAClient{
		logger:          logger,
		DA:              da,
		GasPrice:        gasPrice,
		GasMultiplier:   gasMultiplier,
		Namespace:       ns,
		SubmitTimeout:   defaultSubmitTimeout,
		RetrieveTimeout: defaultRetrieveTimeout,
	}
}

// SubmitBatch submits block data to DA.
func (dac *DAClient) SubmitBatch(ctx context.Context, data []*coresequencer.Batch, maxBlobSize uint64, gasPrice int64) ResultSubmitBatch {
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
			message = fmt.Sprint(ErrBlobSizeOverLimit.Error(), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
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
		case strings.Contains(err.Error(), ErrTxTimedout.Error()):
			status = StatusNotIncludedInBlock
		case strings.Contains(err.Error(), ErrTxAlreadyInMempool.Error()):
			status = StatusAlreadyInMempool
		case strings.Contains(err.Error(), ErrTxIncorrectAccountSequence.Error()):
			status = StatusAlreadyInMempool
		case strings.Contains(err.Error(), ErrTxSizeTooBig.Error()),
			strings.Contains(err.Error(), ErrTxTooLarge.Error()):
			status = StatusTooBig
		case strings.Contains(err.Error(), ErrContextDeadline.Error()):
			status = StatusContextDeadline
		}
		return ResultSubmitBatch{
			BaseResult: BaseResult{
				Code:    status,
				Message: "failed to submit block data: " + err.Error(),
			},
		}
	}

	if len(ids) == 0 {
		return ResultSubmitBatch{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: "failed to submit data: unexpected len(ids): 0",
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
				Message:  ErrBlobNotFound.Error(),
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
