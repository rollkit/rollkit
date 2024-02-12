package block

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goDA "github.com/rollkit/go-da"
	goDATest "github.com/rollkit/go-da/test"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

// Returns a minimalistic block manager
func getManager(t *testing.T, backend goDA.DA, logger log.Logger) *Manager {
	return &Manager{
		dalc:       &da.DAClient{DA: backend, GasPrice: -1, GasMultiplier: -1, Logger: logger},
		blockCache: NewBlockCache(),
		logger:     logger,
	}
}

// getBlockBiggerThan generates a block with the given height bigger than the specified limit.
func getBlockBiggerThan(blockHeight, limit uint64) (*types.Block, error) {
	for numTxs := 0; ; numTxs += 100 {
		block := types.GetRandomBlock(blockHeight, numTxs)
		blob, err := block.MarshalBinary()
		if err != nil {
			return nil, err
		}

		if uint64(len(blob)) > limit {
			return block, nil
		}
	}
}

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

func TestSubmitBlocksToMockDA(t *testing.T) {
	ctx := context.Background()

	mockDA := &mock.MockDA{}
	logger := test.NewLogger(t)
	m := getManager(t, mockDA, logger)
	m.conf.DABlockTime = time.Millisecond

	t.Run("handle_mempool_errors_gracefully", func(t *testing.T) {
		var blobs [][]byte
		block := types.GetRandomBlock(1, 5)
		blob, err := block.MarshalBinary()

		require.NoError(t, err)
		blobs = append(blobs, blob)
		// Set up the mock to throw timeout waiting for tx to be included once
		// tx already in mempool 10 times
		// then submit successfully
		mockDA.On("MaxBlobSize").Return(uint64(12345), nil)
		mockDA.
			On("Submit", blobs, float64(-1)).
			Return([][]byte{}, [][]byte{}, errors.New("timed out waiting for tx to be included in a block")).Once()
		mockDA.
			On("Submit", blobs, float64(-1)).
			Return([][]byte{}, [][]byte{}, errors.New("tx already in mempool")).Times(10)
		mockDA.
			On("Submit", blobs, float64(-1)).
			Return([][]byte{bytes.Repeat([]byte{0x00}, 8)}, [][]byte{[]byte("proof")}, nil)

		m.pendingBlocks = NewPendingBlocks()
		m.pendingBlocks.addPendingBlock(block)
		// should have slept at least 10 * DABlockTime to avoid resubmitting too fast
		before := time.Now().UnixMilli()
		err = m.submitBlocksToDA(ctx)
		after := time.Now().UnixMilli()
		assert.LessOrEqual(t, int64(10), after-before)
		require.NoError(t, err)
	})
}

func TestSubmitBlocksToDA(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	logger := test.NewFileLoggerCustom(t, test.TempLogFileName(t, t.Name()))
	m := getManager(t, goDATest.NewDummyDA(), logger)

	maxDABlobSizeLimit, err := m.dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

	testCases := []struct {
		name                        string
		blocks                      []*types.Block
		isErrExpected               bool
		expectedPendingBlocksLength int
	}{
		{
			name:                        "happy path, all blocks A, B, C combine to less than maxDABlobSize",
			blocks:                      []*types.Block{types.GetRandomBlock(1, 5), types.GetRandomBlock(2, 5), types.GetRandomBlock(3, 5)},
			isErrExpected:               false,
			expectedPendingBlocksLength: 0,
		},
		{
			name: "blocks A and B are submitted together without C because including C triggers blob size limit. C is submitted in a separate round",
			blocks: func() []*types.Block {
				// Find three blocks where two of them are under blob size limit
				// but adding the third one exceeds the blob size limit
				block1 := types.GetRandomBlock(1, 100)
				blob1, err := block1.MarshalBinary()
				require.NoError(err)

				block2 := types.GetRandomBlock(2, 100)
				blob2, err := block2.MarshalBinary()
				require.NoError(err)

				block3, err := getBlockBiggerThan(3, maxDABlobSizeLimit-uint64(len(blob1)+len(blob2)))
				require.NoError(err)

				return []*types.Block{block1, block2, block3}
			}(),
			isErrExpected:               false,
			expectedPendingBlocksLength: 0,
		},
		{
			name: "A and B are submitted successfully but C is too big on its own, so C never gets submitted",
			blocks: func() []*types.Block {
				numBlocks, numTxs := 3, 5
				blocks := make([]*types.Block, numBlocks)
				for i := 0; i < numBlocks-1; i++ {
					blocks[i] = types.GetRandomBlock(uint64(i+1), numTxs)
				}
				blocks[2], err = getBlockBiggerThan(3, maxDABlobSizeLimit)
				require.NoError(err)
				return blocks
			}(),
			isErrExpected:               true,
			expectedPendingBlocksLength: 1,
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
			assert.Equal(t, tc.expectedPendingBlocksLength, len(m.pendingBlocks.getPendingBlocks()))
		})
	}
}
