package evm

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestSetFinal_UnavailableBlock(t *testing.T) {
	prunedErr := &testRPCError{code: prunedHistoryErrorCode, msg: "pruned history unavailable"}

	specs := map[string]struct {
		lookupErr error
		head      *big.Int
		headErr   error
		expectErr bool
	}{
		"skip_pruned_below_head": {
			lookupErr: prunedErr,
			head:      big.NewInt(1_000),
		},
		"skip_not_found_below_head": {
			lookupErr: ethereum.NotFound,
			head:      big.NewInt(1_000),
		},
		"error_transient_lookup_below_head": {
			lookupErr: errors.New("i/o timeout"),
			head:      big.NewInt(1_000),
			expectErr: true,
		},
		"error_other_rpc_code_below_head": {
			lookupErr: &testRPCError{code: -32000, msg: "internal error"},
			head:      big.NewInt(1_000),
			expectErr: true,
		},
		"error_pruned_above_head": {
			lookupErr: prunedErr,
			head:      big.NewInt(50),
			expectErr: true,
		},
		"error_pruned_head_unavailable": {
			lookupErr: prunedErr,
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
					lookupErr: spec.lookupErr,
					head:      spec.head,
					headErr:   spec.headErr,
				},
				logger: zerolog.Nop(),
			}

			err := client.SetFinal(t.Context(), 100)

			if spec.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Zero(t, engineRPC.forkchoiceCalls)
		})
	}
}

// mockSetFinalEthRPCClient fails every numbered lookup with lookupErr and
// serves the configured head for latest-block queries.
type mockSetFinalEthRPCClient struct {
	lookupErr error
	head      *big.Int
	headErr   error
}

func (m mockSetFinalEthRPCClient) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	if number != nil {
		return nil, m.lookupErr
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
