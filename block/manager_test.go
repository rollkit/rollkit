package block

import (
	"context"
	"testing"

	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
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

// Happy case, all blocks A, B, C are submitted on first round
func TestSubmitBlocksToDAHappy(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	logger := test.NewFileLoggerCustom(t, test.TempLogFileName(t, t.Name()))

	// Create a minimalistic block manager
	m := &Manager{
		dalc:          &da.DAClient{DA: goDATest.NewDummyDA(), GasPrice: -1, Logger: logger},
		blockCache:    NewBlockCache(),
		pendingBlocks: NewPendingBlocks(),
		logger:        logger,
	}

	// Prepare blocks A, B, C to add to manager's pendingBlocks
	numTxs, numBlocks := 5, 3
	blocks := make([]*types.Block, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blocks[i] = types.GetRandomBlock(uint64(i+1), numTxs)
		m.pendingBlocks.addPendingBlock(blocks[i])
	}

	numAttempts, err := m.submitBlocksToDA(ctx)
	require.NoError(err)
	require.Equal(numAttempts, uint64(1))

	// Blocks A and B are submitted first round because including c triggers size limit. C is then submitted on second round.
	limit, err := m.dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

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

		if uint64(len(blob1)+len(blob2)) < limit && uint64(len(blob1)+len(blob2)+len(blob3)) > limit {
			m.pendingBlocks.addPendingBlock(block1)
			m.pendingBlocks.addPendingBlock(block2)
			m.pendingBlocks.addPendingBlock(block3)
			break
		}
	}
	numAttempts, err = m.submitBlocksToDA(ctx)
	require.NoError(err)
	require.Equal(numAttempts, uint64(2))

	// A and B are submitted successful but C is too big on its own, so C never gets submitted
	for i := 0; i < numBlocks-1; i++ {
		blocks[i] = types.GetRandomBlock(uint64(i+1), numTxs)
		m.pendingBlocks.addPendingBlock(blocks[i])
	}
	for numTxs := 0; ; numTxs += 100 {
		block3 = types.GetRandomBlock(3, numTxs)
		blob3, err := block3.MarshalBinary()
		require.NoError(err)

		if uint64(len(blob3)) > limit {
			m.pendingBlocks.addPendingBlock(block3)
			break
		}
	}
	numAttempts, err = m.submitBlocksToDA(ctx)
	require.NotNil(err)
	require.Equal(numAttempts, uint64(maxSubmitAttempts))

}
