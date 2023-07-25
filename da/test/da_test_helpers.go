package test

import (
	"time"

	"github.com/rollkit/rollkit/types"
)

const mockDaBlockTime = 100 * time.Millisecond

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.SignedHeader.Header.AppHash = types.GetRandomBytes(32)

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = types.GetRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = types.GetRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	return block
}
