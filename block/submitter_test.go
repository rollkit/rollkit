package block

import (
	"context"
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/libp2p/go-libp2p/core/crypto"
	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// newTestManagerWithDA creates a Manager instance with a mocked DA layer for testing.
func newTestManagerWithDA(t *testing.T, da *mocks.DA) (m *Manager) {
	logger := log.NewNopLogger()
	nodeConf := config.DefaultConfig

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	testSigner, err := noop.NewNoopSigner(privKey)
	require.NoError(t, err)

	return &Manager{
		da:            da,
		logger:        logger,
		config:        nodeConf,
		gasPrice:      1.0,
		gasMultiplier: 2.0,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		signer:        testSigner,
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

	pubKey, err := m.signer.GetPublic()
	require.NoError(t, err)
	addr, err := m.signer.GetAddress()
	require.NoError(t, err)

	signedData := types.SignedData{
		Data: types.Data{
			Txs: make(types.Txs, len(batch.Transactions)),
		},
		Signer: types.Signer{
			Address: addr,
			PubKey:  pubKey,
		},
	}

	signedData.Txs = make(types.Txs, len(batch.Transactions))
	for i, tx := range batch.Transactions {
		signedData.Txs[i] = types.Tx(tx)
	}

	signature, err := m.getDataSignature(&signedData.Data)
	require.NoError(t, err)
	signedData.Signature = signature

	err = m.submitBatchToDA(context.Background(), &signedData)
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

			pubKey, err := m.signer.GetPublic()
			require.NoError(t, err)
			addr, err := m.signer.GetAddress()
			require.NoError(t, err)

			signedData := types.SignedData{
				Data: types.Data{
					Txs: make(types.Txs, len(batch.Transactions)),
				},
				Signer: types.Signer{
					Address: addr,
					PubKey:  pubKey,
				},
			}

			signedData.Txs = make(types.Txs, len(batch.Transactions))
			for i, tx := range batch.Transactions {
				signedData.Txs[i] = types.Tx(tx)
			}

			signature, err := m.getDataSignature(&signedData.Data)
			require.NoError(t, err)
			signedData.Signature = signature

			// Expect an error from submitBatchToDA
			err = m.submitBatchToDA(context.Background(), &signedData)
			assert.Error(t, err, "expected error")
		})
	}
}
