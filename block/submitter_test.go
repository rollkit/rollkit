package block

import (
	"context"
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// newTestManagerWithDA creates a Manager instance with a mocked DA layer for testing.
func newTestManagerWithDA(t *testing.T, da *mocks.DA) (m *Manager) {
	logger := log.NewNopLogger()
	nodeConf := config.DefaultConfig

	return &Manager{
		da:            da,
		logger:        logger,
		config:        nodeConf,
		gasPrice:      1.0,
		gasMultiplier: 2.0,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
	}
}

// TestSubmitBatchToDA_Success verifies that submitBatchToDA succeeds when the DA layer accepts the batch.
func TestSubmitBatchToDA_Success(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	batch := coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}

	// Simulate DA success
	da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	err := m.submitBatchToDA(context.Background(), batch)
	assert.NoError(t, err)
}

// TestSubmitBatchToDA_Failure verifies that submitBatchToDA returns an error for various DA failures.
func TestSubmitBatchToDA_Failure(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)

	batch := coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}

	// Table-driven test for different DA error scenarios
	testCases := []struct {
		name    string
		daError error
	}{
		{"AlreadyInMempool", coreda.ErrTxAlreadyInMempool},
		{"TimedOut", coreda.ErrTxTimedOut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock expectations for each error scenario
			da.ExpectedCalls = nil
			da.On("GasMultiplier", mock.Anything).Return(2.0, nil)
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, tc.daError)

			// Expect an error from submitBatchToDA
			err := m.submitBatchToDA(context.Background(), batch)
			assert.Error(t, err, "expected error")
		})
	}
}

func TestSubmitHeadersToDA_Success(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)
	// Prepare a mock PendingHeaders with test data
	m.pendingHeaders = newPendingBlocks(t)

	// Fill the pending headers with mock block data
	fillWithBlockData(context.Background(), t, m.pendingHeaders, "Test Submitting Headers")

	// Simulate DA layer successfully accepting the header submission
	da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]coreda.ID{[]byte("id")}, nil)

	// Call submitHeadersToDA and expect no error
	err := m.submitHeadersToDA(context.Background())
	assert.NoError(t, err)
}

// TestSubmitHeadersToDA_Failure verifies that submitHeadersToDA returns an error for various DA failures.
func TestSubmitHeadersToDA_Failure(t *testing.T) {
	da := &mocks.DA{}
	m := newTestManagerWithDA(t, da)
	// Prepare a mock PendingHeaders with test data
	m.pendingHeaders = newPendingBlocks(t)

	// Table-driven test for different DA error scenarios
	testCases := []struct {
		name    string
		daError error
	}{
		{"AlreadyInMempool", coreda.ErrTxAlreadyInMempool},
		{"TimedOut", coreda.ErrTxTimedOut},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Fill the pending headers with mock block data for each subtest
			fillWithBlockData(context.Background(), t, m.pendingHeaders, "Test Submitting Headers")
			// Reset mock expectations for each error scenario
			da.ExpectedCalls = nil
			// Simulate DA layer returning a specific error
			da.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return(nil, tc.daError)

			// Call submitHeadersToDA and expect an error
			err := m.submitHeadersToDA(context.Background())
			assert.Error(t, err, "expected error for DA error: %v", tc.daError)
			assert.Contains(t, err.Error(), "failed to submit all headers to DA layer")
		})
	}
}
