package newda

import (
	"context"

	ds "github.com/ipfs/go-datastore"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"

	newda "github.com/rollkit/go-da"
)

type NewDA struct {
	da newda.DA
}

func (n *NewDA) Init(namespaceID types.NamespaceID, config []byte, kvStore ds.Datastore, logger log.Logger) error {
	return nil
}

func (n *NewDA) Start() error {
	return nil
}

func (n *NewDA) Stop() error {
	return nil
}

func (n *NewDA) SubmitBlocks(ctx context.Context, blocks []*types.Block) da.ResultSubmitBlocks {
	blobs := make([][]byte, len(blocks))
	for i := range blocks {
		blob, err := blocks[i].MarshalBinary()
		if err != nil {
			return da.ResultSubmitBlocks{
				BaseResult: da.BaseResult{
					Code:    da.StatusError,
					Message: "failed to serialize block",
				},
			}
		}
		blobs[i] = blob
	}
	_, _, err := n.da.Submit(blobs)
	if err != nil {
		return da.ResultSubmitBlocks{
			BaseResult: da.BaseResult{
				Code:    da.StatusError,
				Message: "failed to submit blocks: " + err.Error(),
			},
		}
	}

	return da.ResultSubmitBlocks{
		BaseResult: da.BaseResult{
			Code: da.StatusSuccess,
		},
	}
}

func (n *NewDA) RetrieveBlocks(ctx context.Context, dataLayerHeight uint64) da.ResultRetrieveBlocks {
	//TODO implement me
	panic("implement me")
}
