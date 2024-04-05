package da

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"

	goDA "github.com/rollkit/go-da"
	"github.com/rollkit/rollkit/third_party/log"
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
	// SubmittedCount is the number of successfully submitted blocks.
	SubmittedCount uint64
}

// ResultSubmitBlocks contains information returned from DA layer after blocks submission.
type ResultSubmitBlocks struct {
	BaseResult
	// Not sure if this needs to be bubbled up to other
	// parts of Rollkit.
	// Hash hash.Hash
}

// ResultRetrieveBlocks contains batch of blocks returned from DA layer client.
type ResultRetrieveBlocks struct {
	BaseResult
	// Block is the full block retrieved from Data Availability Layer.
	// If Code is not equal to StatusSuccess, it has to be nil.
	Blocks []*types.Block
}

// DAClient is a new DA implementation.
type DAClient struct {
	DA              goDA.DA
	GasPrice        float64
	GasMultiplier   float64
	Namespace       goDA.Namespace
	SubmitTimeout   time.Duration
	RetrieveTimeout time.Duration
	Logger          log.Logger
}

// NewDAClient returns a new DA client.
func NewDAClient(da goDA.DA, gasPrice, gasMultiplier float64, ns goDA.Namespace, logger log.Logger) *DAClient {
	return &DAClient{
		DA:              da,
		GasPrice:        gasPrice,
		GasMultiplier:   gasMultiplier,
		Namespace:       ns,
		SubmitTimeout:   defaultSubmitTimeout,
		RetrieveTimeout: defaultRetrieveTimeout,
		Logger:          logger,
	}
}

// SubmitBlocks submits blocks to DA.
func (dac *DAClient) SubmitBlocks(ctx context.Context, blocks []*types.Block, maxBlobSize uint64, gasPrice float64) ResultSubmitBlocks {
	var (
		blobs    [][]byte
		blobSize uint64
		message  string
	)
	for i := range blocks {
		blob, err := blocks[i].MarshalBinary()
		if err != nil {
			message = fmt.Sprint("failed to serialize block", err)
			dac.Logger.Info(message)
			break
		}
		if blobSize+uint64(len(blob)) > maxBlobSize {
			message = fmt.Sprint(ErrBlobSizeOverLimit.Error(), "blob size limit reached", "maxBlobSize", maxBlobSize, "index", i, "blobSize", blobSize, "len(blob)", len(blob))
			dac.Logger.Info(message)
			break
		}
		blobSize += uint64(len(blob))
		blobs = append(blobs, blob)
	}
	if len(blobs) == 0 {
		return ResultSubmitBlocks{
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
		return ResultSubmitBlocks{
			BaseResult: BaseResult{
				Code:    status,
				Message: "failed to submit blocks: " + err.Error(),
			},
		}
	}

	if len(ids) == 0 {
		return ResultSubmitBlocks{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: "failed to submit blocks: unexpected len(ids): 0",
			},
		}
	}

	return ResultSubmitBlocks{
		BaseResult: BaseResult{
			Code:           StatusSuccess,
			DAHeight:       binary.LittleEndian.Uint64(ids[0]),
			SubmittedCount: uint64(len(ids)),
		},
	}
}

// RetrieveBlocks retrieves blocks from DA.
func (dac *DAClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) ResultRetrieveBlocks {
	ids, err := dac.DA.GetIDs(ctx, dataLayerHeight, dac.Namespace)
	if err != nil {
		return ResultRetrieveBlocks{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get IDs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	// If no blocks are found, return a non-blocking error.
	if len(ids) == 0 {
		return ResultRetrieveBlocks{
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
		return ResultRetrieveBlocks{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get blobs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}

	blocks := make([]*types.Block, len(blobs))
	for i, blob := range blobs {
		var block pb.Block
		err = proto.Unmarshal(blob, &block)
		if err != nil {
			dac.Logger.Error("failed to unmarshal block", "daHeight", dataLayerHeight, "position", i, "error", err)
			continue
		}
		blocks[i] = new(types.Block)
		err := blocks[i].FromProto(&block)
		if err != nil {
			return ResultRetrieveBlocks{
				BaseResult: BaseResult{
					Code:    StatusError,
					Message: err.Error(),
				},
			}
		}
	}

	return ResultRetrieveBlocks{
		BaseResult: BaseResult{
			Code:     StatusSuccess,
			DAHeight: dataLayerHeight,
		},
		Blocks: blocks,
	}
}
