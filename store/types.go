package store

import "github.com/lazyledger/optimint/types"

type Store interface {
	Height() uint64

	SaveBlock(block *types.Block, commit *types.Commit) error

	LoadBlock(height uint64) (*types.Block, error)
	LoadBlockByHash(hash [32]byte) (*types.Block, error)

	LoadCommit(height uint64) (*types.Commit, error)
	LoadCommitByHash(hash [32]byte) (*types.Commit, error)
}
