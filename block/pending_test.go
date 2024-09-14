package block

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

const (
	numBlocks  = 5
	testHeight = 3
)

func TestPendingBlocks(t *testing.T) {
	cases := []struct {
		name                    string
		init                    func(context.Context, *testing.T, *PendingHeaders)
		exec                    func(context.Context, *testing.T, *PendingHeaders)
		expectedBlocksAfterInit int
		expectedBlocksAfterExec int
	}{
		{name: "empty store",
			init:                    func(context.Context, *testing.T, *PendingHeaders) {},
			exec:                    func(context.Context, *testing.T, *PendingHeaders) {},
			expectedBlocksAfterInit: 0,
			expectedBlocksAfterExec: 0,
		},
		{
			name: "mock successful DA submission of some blocks by manually setting last submitted height",
			init: fillWithBlockData,
			exec: func(ctx context.Context, t *testing.T, pb *PendingHeaders) {
				pb.lastSubmittedHeight.Store(testHeight)
			},
			expectedBlocksAfterInit: numBlocks,
			expectedBlocksAfterExec: numBlocks - testHeight,
		},
		{
			name: "mock successful DA submission of all blocks by manually setting last submitted height",
			init: fillWithBlockData,
			exec: func(ctx context.Context, t *testing.T, pb *PendingHeaders) {
				pb.lastSubmittedHeight.Store(numBlocks)
			},
			expectedBlocksAfterInit: numBlocks,
			expectedBlocksAfterExec: 0,
		},
		{
			name: "mock successful DA submission of all blocks by setting last submitted height using store",
			init: fillWithBlockData,
			exec: func(ctx context.Context, t *testing.T, pb *PendingHeaders) {
				pb.lastSubmittedHeight.Store(pb.store.Height())
			},
			expectedBlocksAfterInit: numBlocks,
			expectedBlocksAfterExec: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// use timeout to ensure tests will end
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			pb := newPendingBlocks(t)

			tc.init(ctx, t, pb)
			checkRequirements(ctx, t, pb, tc.expectedBlocksAfterInit)

			tc.exec(ctx, t, pb)
			checkRequirements(ctx, t, pb, tc.expectedBlocksAfterExec)
		})
	}
}

func newPendingBlocks(t *testing.T) *PendingHeaders {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	pendingBlocks, err := NewPendingHeaders(store.New(kv), test.NewLogger(t))
	require.NoError(t, err)
	return pendingBlocks
}

func fillWithBlockData(ctx context.Context, t *testing.T, pb *PendingHeaders) {
	for i := uint64(1); i <= numBlocks; i++ {
		h, d := types.GetRandomBlock(i, 0)
		require.NoError(t, pb.store.SaveBlockData(ctx, h, d, &types.Signature{}))
		pb.store.SetHeight(ctx, i)
	}
}

func checkRequirements(ctx context.Context, t *testing.T, pb *PendingHeaders, nBlocks int) {
	require.Equal(t, pb.isEmpty(), nBlocks == 0)
	blocks, err := pb.getPendingHeaders(ctx)
	require.NoError(t, err)
	require.Len(t, blocks, nBlocks)
	require.Equal(t, uint64(len(blocks)), pb.numPendingHeaders())
	require.True(t, sort.SliceIsSorted(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	}))
}
