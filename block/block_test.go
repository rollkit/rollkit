package block

import (
	"context"
	"sync"
	"testing"
	"time"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/require"
)

func TestCreateBlock(t *testing.T) {
	// Setup a minimal Genesis object
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Create Manager with minimal required fields
	appHash := []byte("test-app-hash")
	m := &Manager{
		genesis: genesisData,
		lastState: types.State{
			ChainID:         "test-chain",
			Version:         types.Version{Block: 1, App: 2},
			LastBlockHeight: 5,
			AppHash:         appHash,
		},
		lastStateMtx: new(sync.RWMutex),
	}

	// Setup test parameters
	ctx := context.Background()
	height := uint64(6)
	sig := types.Signature([]byte("test-signature"))
	lastSignature := &sig
	lastHeaderHash := types.Hash([]byte("test-header-hash"))

	// Create a batch with some transactions
	txs := [][]byte{
		[]byte("tx1"),
		[]byte("tx2"),
		[]byte("tx3"),
	}
	batchData := &BatchData{
		Batch: &coresequencer.Batch{
			Transactions: txs,
		},
		Time: time.Now().UTC(),
		Data: [][]byte{[]byte("batch-data")},
	}

	// Call createBlock
	header, data, err := m.createBlock(ctx, height, lastSignature, lastHeaderHash, batchData)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, header)
	require.NotNil(t, data)

	// Check header fields
	require.Equal(t, m.lastState.Version.Block, header.Version.Block)
	require.Equal(t, m.lastState.Version.App, header.Version.App)
	require.Equal(t, m.lastState.ChainID, header.ChainID())
	require.Equal(t, height, header.Height())
	require.Equal(t, uint64(batchData.Time.UnixNano()), header.BaseHeader.Time)
	require.Equal(t, lastHeaderHash, header.LastHeaderHash)
	require.ElementsMatch(t, appHash, header.AppHash)
	require.Equal(t, genesisData.ProposerAddress(), header.ProposerAddress)

	// Check data fields
	require.Equal(t, len(txs), len(data.Txs))
	for i, tx := range txs {
		require.Equal(t, tx, []byte(data.Txs[i]))
	}

	// Check signature
	require.Equal(t, *lastSignature, header.Signature)
}

func TestCreateBlockWithEmptyBatch(t *testing.T) {
	// Setup a minimal Genesis object
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Create Manager with minimal required fields
	m := &Manager{
		genesis: genesisData,
		lastState: types.State{
			ChainID:         "test-chain",
			Version:         types.Version{Block: 1, App: 2},
			LastBlockHeight: 5,
			AppHash:         []byte("test-app-hash"),
		},
		lastStateMtx: new(sync.RWMutex),
	}

	// Setup test parameters
	ctx := context.Background()
	height := uint64(6)
	sig := types.Signature([]byte("test-signature"))
	lastSignature := &sig
	lastHeaderHash := types.Hash([]byte("test-header-hash"))

	// Create an empty batch
	batchData := &BatchData{
		Batch: &coresequencer.Batch{
			Transactions: [][]byte{},
		},
		Time: time.Now().UTC(),
		Data: [][]byte{},
	}

	// Call createBlock
	header, data, err := m.createBlock(ctx, height, lastSignature, lastHeaderHash, batchData)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, header)
	require.NotNil(t, data)

	// Check data fields - should be empty
	require.Empty(t, data.Txs)
}

func TestApplyBlock(t *testing.T) {
	// Setup test context
	ctx := context.Background()

	// Create a dummy executor
	dummyExec := coreexecutor.NewDummyExecutor()

	// Initialize initial state
	initialState := types.State{
		ChainID:         "test-chain",
		Version:         types.Version{Block: 1, App: 2},
		LastBlockHeight: 5,
		AppHash:         []byte("previous-app-hash"),
	}

	// Create a Manager with the executor and initial state
	m := &Manager{
		exec:         dummyExec,
		lastState:    initialState,
		lastStateMtx: new(sync.RWMutex),
	}

	// Create a test header
	blockTime := time.Now().UTC()
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: 1,
				App:   2,
			},
			BaseHeader: types.BaseHeader{
				ChainID: "test-chain",
				Height:  6, // next height
				Time:    uint64(blockTime.UnixNano()),
			},
			LastHeaderHash:  types.Hash([]byte("last-header-hash")),
			AppHash:         []byte("app-hash"),
			ProposerAddress: []byte("proposer-address"),
		},
	}

	// Create some test transactions
	txs := [][]byte{
		[]byte("tx1"),
		[]byte("tx2"),
		[]byte("tx3"),
	}

	// Create test block data with the transactions
	data := &types.Data{
		Txs: make(types.Txs, len(txs)),
	}
	for i, tx := range txs {
		data.Txs[i] = types.Tx(tx)
	}

	// Call applyBlock
	newState, err := m.applyBlock(ctx, header, data)

	// Verify results
	require.NoError(t, err)
	require.NotEmpty(t, newState)

	// Check state fields
	require.Equal(t, initialState.ChainID, newState.ChainID)
	require.Equal(t, initialState.Version, newState.Version)
	require.Equal(t, header.Height(), newState.LastBlockHeight)
	require.Equal(t, header.Time(), newState.LastBlockTime)

	// Check that state root is different (since our dummy executor changes it)
	require.NotEqual(t, initialState.AppHash, newState.AppHash)
}

func TestApplyBlockWithEmptyTransactions(t *testing.T) {
	// Setup test context
	ctx := context.Background()

	// Create a dummy executor
	dummyExec := coreexecutor.NewDummyExecutor()

	// Initialize initial state
	initialState := types.State{
		ChainID:         "test-chain",
		Version:         types.Version{Block: 1, App: 2},
		LastBlockHeight: 5,
		AppHash:         []byte("previous-app-hash"),
	}

	// Create a Manager with the executor and initial state
	m := &Manager{
		exec:         dummyExec,
		lastState:    initialState,
		lastStateMtx: new(sync.RWMutex),
	}

	// Create a test header
	blockTime := time.Now().UTC()
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: 1,
				App:   2,
			},
			BaseHeader: types.BaseHeader{
				ChainID: "test-chain",
				Height:  6, // next height
				Time:    uint64(blockTime.UnixNano()),
			},
			LastHeaderHash:  types.Hash([]byte("last-header-hash")),
			AppHash:         []byte("app-hash"),
			ProposerAddress: []byte("proposer-address"),
		},
	}

	// Create empty block data
	data := &types.Data{
		Txs: types.Txs{},
	}

	// Call applyBlock
	newState, err := m.applyBlock(ctx, header, data)

	// Verify results
	require.NoError(t, err)
	require.NotEmpty(t, newState)

	// Check state fields
	require.Equal(t, initialState.ChainID, newState.ChainID)
	require.Equal(t, initialState.Version, newState.Version)
	require.Equal(t, header.Height(), newState.LastBlockHeight)
	require.Equal(t, header.Time(), newState.LastBlockTime)

	// Check that state root is different (since our dummy executor changes it)
	require.NotEqual(t, initialState.AppHash, newState.AppHash)
}
