package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestSetFinal_UnavailableBlock(t *testing.T) {
	specs := map[string]struct {
		head        *big.Int
		headErr     error
		expectErr   bool
		expectFCUpd bool
	}{
		"skip_when_below_head": {
			head:        big.NewInt(1_000),
			expectErr:   false,
			expectFCUpd: false,
		},
		"error_when_above_head": {
			head:      big.NewInt(50),
			expectErr: true,
		},
		"error_when_head_unavailable": {
			headErr:   errors.New("connection refused"),
			expectErr: true,
		},
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			engineRPC := &mockSetFinalEngineRPCClient{}
			client := &EngineClient{
				engineClient: engineRPC,
				ethClient: mockSetFinalEthRPCClient{
					head:    spec.head,
					headErr: spec.headErr,
				},
				logger: zerolog.Nop(),
			}

			err := client.SetFinal(t.Context(), 100)

			if spec.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, spec.expectFCUpd, engineRPC.forkchoiceCalls > 0)
		})
	}
}

// mockSetFinalEthRPCClient fails every historical lookup, as if the EL had
// pruned it, and serves the configured head for latest-block queries.
type mockSetFinalEthRPCClient struct {
	head    *big.Int
	headErr error
}

func (m mockSetFinalEthRPCClient) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	if number != nil {
		return nil, errors.New("block pruned")
	}
	if m.headErr != nil {
		return nil, m.headErr
	}
	return &types.Header{Number: m.head}, nil
}

func (mockSetFinalEthRPCClient) GetTxs(_ context.Context) ([]string, error) {
	return nil, errors.New("unexpected GetTxs call")
}

func (mockSetFinalEthRPCClient) GetNextProposer(_ context.Context, _ *big.Int) (common.Hash, error) {
	return common.Hash{}, errors.New("unexpected GetNextProposer call")
}

type mockSetFinalEngineRPCClient struct {
	forkchoiceCalls int
}

func (m *mockSetFinalEngineRPCClient) ForkchoiceUpdated(_ context.Context, _ engine.ForkchoiceStateV1, _ map[string]any) (*engine.ForkChoiceResponse, error) {
	m.forkchoiceCalls++
	return nil, errors.New("unexpected ForkchoiceUpdated call")
}

func (m *mockSetFinalEngineRPCClient) GetPayload(_ context.Context, _ engine.PayloadID) (*EnginePayloadEnvelope, error) {
	return nil, errors.New("unexpected GetPayload call")
}

func (m *mockSetFinalEngineRPCClient) NewPayload(_ context.Context, _ *EnginePayloadEnvelope, _ []string, _ string, _ [][]byte) (*engine.PayloadStatusV1, error) {
	return nil, errors.New("unexpected NewPayload call")
}
