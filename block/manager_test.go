package block

import (
	"context"
	"testing"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

func TestInitialStateClean(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 1,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	es, _ := store.NewDefaultInMemoryKVStore()
	emptyStore := store.New(es)
	s, err := getInitialState(emptyStore, genesis)
	require.Equal(s.LastBlockHeight, uint64(genesis.InitialHeight-1))
	require.NoError(err)
	require.Equal(uint64(genesis.InitialHeight), s.InitialHeight)
}

func TestInitialStateStored(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 1,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "myChain",
		InitialHeight:   1,
		LastBlockHeight: 100,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	s, err := getInitialState(store, genesis)
	require.Equal(s.LastBlockHeight, uint64(100))
	require.NoError(err)
	require.Equal(s.InitialHeight, uint64(1))
}

func TestInitialStateUnexpectedHigherGenesis(t *testing.T) {
	require := require.New(t)
	genesisDoc, _ := types.GetGenesisWithPrivkey()
	genesis := &cmtypes.GenesisDoc{
		ChainID:       "myChain",
		InitialHeight: 2,
		Validators:    genesisDoc.Validators,
		AppHash:       []byte("app hash"),
	}
	sampleState := types.State{
		ChainID:         "myChain",
		InitialHeight:   1,
		LastBlockHeight: 0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	es, _ := store.NewDefaultInMemoryKVStore()
	store := store.New(es)
	err := store.UpdateState(ctx, sampleState)
	require.NoError(err)
	_, err = getInitialState(store, genesis)
	require.EqualError(err, "genesis.InitialHeight (2) is greater than last stored state's LastBlockHeight (0)")
}

func TestIsDAIncluded(t *testing.T) {
	require := require.New(t)

	// Create a minimalistic block manager
	m := &Manager{
		blockCache: NewBlockCache(),
	}
	hash := types.Hash([]byte("hash"))

	// IsDAIncluded should return false for unseen hash
	require.False(m.IsDAIncluded(hash))

	// Set the hash as DAIncluded and verify IsDAIncluded returns true
	m.blockCache.setDAIncluded(hash.String())
	require.True(m.IsDAIncluded(hash))
}

func TestSubmitBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	m := getManager(t)

	maxDABlobSizeLimit, err := m.dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

	testCases := []struct {
		name          string
		blocks        []*types.Block
		isErrExpected bool
	}{
		{
			name:          "happy path, all blocks A, B, C are submitted on first round",
			blocks:        []*types.Block{types.GetRandomBlock(1, 5), types.GetRandomBlock(2, 5), types.GetRandomBlock(3, 5)},
			isErrExpected: false,
		},
		{
			name: "blocks A and B are submitted first round because including c triggers blob size limit. C is submitted on second round",
			blocks: func() []*types.Block {
				// Find three blocks where two of them are under blob size limit
				// but adding the third one exceeds the blob size limit
				var block1, block2, block3 *types.Block
				for numTxs := 0; ; numTxs += 100 {
					block1 = types.GetRandomBlock(1, numTxs)
					blob1, err := block1.MarshalBinary()
					require.NoError(err)

					block2 = types.GetRandomBlock(2, numTxs)
					blob2, err := block2.MarshalBinary()
					require.NoError(err)

					block3 = types.GetRandomBlock(3, numTxs)
					blob3, err := block3.MarshalBinary()
					require.NoError(err)

					if uint64(len(blob1)+len(blob2)) < maxDABlobSizeLimit && uint64(len(blob1)+len(blob2)+len(blob3)) > maxDABlobSizeLimit {
						return []*types.Block{block1, block2, block3}
					}
				}
			}(),
			isErrExpected: false,
		},
		{
			name: "A and B are submitted successfully but C is too big on its own, so C never gets submitted",
			blocks: func() []*types.Block {
				numBlocks, numTxs := 3, 5
				blocks := make([]*types.Block, numBlocks)
				for i := 0; i < numBlocks-1; i++ {
					blocks[i] = types.GetRandomBlock(uint64(i+1), numTxs)
				}
				for numTxs := 0; ; numTxs += 100 {
					block3 := types.GetRandomBlock(3, numTxs)
					blob3, err := block3.MarshalBinary()
					require.NoError(err)

					if uint64(len(blob3)) > maxDABlobSizeLimit {
						blocks[2] = block3
						return blocks
					}
				}
			}(),
			isErrExpected: true,
		},
	}

	for _, tc := range testCases {
		m.pendingBlocks = NewPendingBlocks()
		t.Run(tc.name, func(t *testing.T) {
			for _, block := range tc.blocks {
				m.pendingBlocks.addPendingBlock(block)
			}
			err := m.submitBlocksToDA(ctx)
			assert.Equal(t, tc.isErrExpected, err != nil)
		})
	}
}
