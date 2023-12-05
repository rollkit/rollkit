package da

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/proto"

	goDA "github.com/rollkit/go-da"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// ErrBlobNotFound is used to indicate that the blob was not found.
var ErrBlobNotFound = errors.New("blob: not found")

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
	DA     goDA.DA
	Logger log.Logger
}

// SubmitBlocks submits blocks to DA.
func (dac *DAClient) SubmitBlocks(ctx context.Context, blocks []*types.Block) ResultSubmitBlocks {
	var blobs [][]byte
	var blobSize uint64
	maxBlobSize := dac.DA.Config()
	for i := range blocks {
		blob, err := blocks[i].MarshalBinary()
		if err != nil {
			return ResultSubmitBlocks{
				BaseResult: BaseResult{
					Code:    StatusError,
					Message: "failed to serialize block",
				},
			}
		}
		blobSize += uint64(len(blob))
		if blobSize > maxBlobSize {
			dac.Logger.Info("skipping blocks over max blob size limit", "i", i, "blobSize", blobSize, "maxBlobSize", maxBlobSize)
			break
		}
		blobs[i] = blob
	}
	ids, _, err := dac.DA.Submit(blobs)
	if err != nil {
		return ResultSubmitBlocks{
			BaseResult: BaseResult{
				Code:    StatusError,
				Message: "failed to submit blocks: " + err.Error(),
			},
		}
	}

	return ResultSubmitBlocks{
		BaseResult: BaseResult{
			Code:     StatusSuccess,
			DAHeight: binary.LittleEndian.Uint64(ids[0]),
		},
	}
}

// RetrieveBlocks retrieves blocks from DA.
func (dac *DAClient) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) ResultRetrieveBlocks {
	ids, err := dac.DA.GetIDs(dataLayerHeight)
	if err != nil {
		return ResultRetrieveBlocks{
			BaseResult: BaseResult{
				Code:     StatusError,
				Message:  fmt.Sprintf("failed to get IDs: %s", err.Error()),
				DAHeight: dataLayerHeight,
			},
		}
	}
	// ids can be nil if there are no blocks at the requested height.
	if ids == nil {
		return ResultRetrieveBlocks{
			BaseResult: BaseResult{
				Code:    StatusNotFound,
				Message: ErrBlobNotFound.Error(),
			},
		}
	}

	blobs, err := dac.DA.Get(ids)
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
