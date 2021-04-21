package store

import "github.com/lazyledger/optimint/types"

type BlockStore interface {
	Height() uint64

	SaveBlock(block *types.Block)

	LoadBlock(height uint64) *types.Block
	LoadBlockByHash(hash [32]byte) *types.Block
}
