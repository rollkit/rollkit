package block

import (
	"context"
	"sort"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

const (
	numBlocks  = 5
	testHeight = 3
)

func TestPendingBlocks(t *testing.T) {
	cases := []struct {
		name                    string
		init                    func(context.Context, *testing.T, *PendingHeaders, string)
		exec                    func(context.Context, *testing.T, *PendingHeaders)
		expectedBlocksAfterInit int
		expectedBlocksAfterExec int
	}{
		{name: "empty store",
			init:                    func(context.Context, *testing.T, *PendingHeaders, string) {},
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
				pb.lastSubmittedHeight.Store(pb.store.Height(ctx))
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

			tc.init(ctx, t, pb, "TestPendingBlocks")
			checkRequirements(ctx, t, pb, tc.expectedBlocksAfterInit)

			tc.exec(ctx, t, pb)
			checkRequirements(ctx, t, pb, tc.expectedBlocksAfterExec)
		})
	}
}

func newPendingBlocks(t *testing.T) *PendingHeaders {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	pendingBlocks, err := NewPendingHeaders(store.New(kv), log.NewTestLogger(t))
	require.NoError(t, err)
	return pendingBlocks
}

func fillWithBlockData(ctx context.Context, t *testing.T, pb *PendingHeaders, chainID string) {
	for i := uint64(1); i <= numBlocks; i++ {
		h, d := types.GetRandomBlock(i, 0, chainID)
		require.NoError(t, pb.store.SaveBlockData(ctx, h, d, &types.Signature{}))
		err := pb.store.SetHeight(ctx, i)
		require.NoError(t, err)
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
