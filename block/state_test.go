package block

import (
	"context"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

func TestUpdateState(t *testing.T) {
	// Create a test manager
	logger := log.NewTestLogger(t)
	kvStore, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	testStore := store.New(kvStore)

	m := &Manager{
		store:        testStore,
		logger:       logger,
		lastStateMtx: new(sync.RWMutex),
		metrics:      NopMetrics(),
	}

	// Test updating state
	ctx := context.Background()
	testTime := time.Now().UTC().Truncate(time.Second)
	testState := types.State{
		ChainID:         "test-chain",
		LastBlockHeight: 5,
		LastBlockTime:   testTime,
	}

	err = m.updateState(ctx, testState)
	require.NoError(t, err)

	// Verify state was updated
	require.Equal(t, testState.ChainID, m.lastState.ChainID)
	require.Equal(t, testState.LastBlockHeight, m.lastState.LastBlockHeight)
	require.Equal(t, testTime, m.lastState.LastBlockTime)

	// Verify state can be retrieved from store
	storedState, err := testStore.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, testState.ChainID, storedState.ChainID)
	require.Equal(t, testState.LastBlockHeight, storedState.LastBlockHeight)
	require.Equal(t, testTime, storedState.LastBlockTime)
}

func TestGetLastBlockTime(t *testing.T) {
	// Create a test manager
	m := &Manager{
		lastStateMtx: new(sync.RWMutex),
	}

	// Set a test block time
	testTime := time.Now().UTC().Truncate(time.Second)
	m.lastState = types.State{
		LastBlockTime: testTime,
	}

	// Test retrieving the last block time
	blockTime := m.getLastBlockTime()
	require.Equal(t, testTime, blockTime)
}

func TestSetLastState(t *testing.T) {
	// Create a test manager
	m := &Manager{
		lastStateMtx: new(sync.RWMutex),
	}

	// Test setting the last state
	testTime := time.Now().UTC().Truncate(time.Second)
	testState := types.State{
		ChainID:         "test-chain",
		LastBlockHeight: 10,
		LastBlockTime:   testTime,
	}

	m.SetLastState(testState)
	require.Equal(t, testState.ChainID, m.lastState.ChainID)
	require.Equal(t, testState.LastBlockHeight, m.lastState.LastBlockHeight)
	require.Equal(t, testTime, m.lastState.LastBlockTime)
}

func TestNextState(t *testing.T) {
	// Create a test manager
	m := &Manager{}

	// Create current state
	currentState := types.State{
		ChainID:         "test-chain",
		InitialHeight:   1,
		LastBlockHeight: 5,
	}

	// Create header for next block
	headerTime := time.Now().UTC().Truncate(time.Second)

	// Create a header first
	header := types.Header{
		BaseHeader: types.BaseHeader{
			Height:  6,
			Time:    uint64(headerTime.UnixNano()),
			ChainID: "test-chain",
		},
		ProposerAddress: []byte("proposer-address"),
	}

	// Then create the signed header
	signedHeader := &types.SignedHeader{
		Header: header,
	}

	// Test generating next state
	stateRoot := []byte("state-root-hash")
	nextState, err := m.nextState(currentState, signedHeader, stateRoot)
	require.NoError(t, err)

	// Verify next state values
	require.Equal(t, currentState.ChainID, nextState.ChainID)
	require.Equal(t, currentState.InitialHeight, nextState.InitialHeight)
	require.Equal(t, signedHeader.Height(), nextState.LastBlockHeight)
	require.WithinDuration(t, headerTime, nextState.LastBlockTime, time.Microsecond)
	require.Equal(t, stateRoot, nextState.AppHash)
}

func TestDAIncludedHeight(t *testing.T) {
	// Create test components
	logger := log.NewTestLogger(t)
	kvStore, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	testStore := store.New(kvStore)

	// Create manager
	m := &Manager{
		store:  testStore,
		logger: logger,
	}

	ctx := context.Background()

	// Test initial value
	require.Equal(t, uint64(0), m.GetDAIncludedHeight())

	// Test setting a new height
	err = m.setDAIncludedHeight(ctx, 5)
	require.NoError(t, err)
	require.Equal(t, uint64(5), m.GetDAIncludedHeight())

	// Test setting a lower height (shouldn't change)
	err = m.setDAIncludedHeight(ctx, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(5), m.GetDAIncludedHeight())

	// Test setting a higher height
	err = m.setDAIncludedHeight(ctx, 10)
	require.NoError(t, err)
	require.Equal(t, uint64(10), m.GetDAIncludedHeight())
}
