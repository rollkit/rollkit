package store

import (
	"github.com/rollkit/rollkit/types"
)

func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				Signatures: make([][]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = types.GetRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = types.GetRandomBytes(32)
	}

	return block
}
