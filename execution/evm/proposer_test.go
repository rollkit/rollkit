package evm

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type proposerEthRPCClient struct {
	headerByNumberFn   func(ctx context.Context, number *big.Int) (*types.Header, error)
	getTxsFn           func(ctx context.Context) ([]string, error)
	getNextProposerFn  func(ctx context.Context, number *big.Int) (common.Hash, error)
	nextProposerBlocks []*big.Int
}

func (m *proposerEthRPCClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	if m.headerByNumberFn != nil {
		return m.headerByNumberFn(ctx, number)
	}
	return &types.Header{GasLimit: 30_000_000}, nil
}

func (m *proposerEthRPCClient) GetTxs(ctx context.Context) ([]string, error) {
	if m.getTxsFn != nil {
		return m.getTxsFn(ctx)
	}
	return nil, nil
}

func (m *proposerEthRPCClient) GetNextProposer(ctx context.Context, number *big.Int) (common.Hash, error) {
	m.nextProposerBlocks = append(m.nextProposerBlocks, number)
	if m.getNextProposerFn != nil {
		return m.getNextProposerFn(ctx, number)
	}
	return common.Hash{}, nil
}

func TestGetExecutionInfoDoesNotFetchNextProposer(t *testing.T) {
	ethClient := &proposerEthRPCClient{
		getNextProposerFn: func(ctx context.Context, number *big.Int) (common.Hash, error) {
			t.Fatalf("GetExecutionInfo must not fetch proposer without an explicit height")
			return common.Hash{}, nil
		},
	}
	client := &EngineClient{ethClient: ethClient}

	info, err := client.GetExecutionInfo(t.Context())

	require.NoError(t, err)
	require.Equal(t, uint64(30_000_000), info.MaxGas)
	require.Empty(t, ethClient.nextProposerBlocks)
}

func TestExecuteTxsReturnsNextProposerWhenChanged(t *testing.T) {
	timestamp := time.Unix(1_700_000_000, 0)
	stateRoot := common.HexToHash("0x01")
	prevProposer := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	nextProposer := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	store := NewEVMStore(dssync.MutexWrap(ds.NewMapDatastore()))
	require.NoError(t, store.SaveExecMeta(t.Context(), &ExecMeta{
		Height:    2,
		StateRoot: stateRoot.Bytes(),
		Timestamp: timestamp.Unix(),
		Stage:     ExecStagePromoted,
	}))

	ethClient := &proposerEthRPCClient{
		headerByNumberFn: func(ctx context.Context, number *big.Int) (*types.Header, error) {
			require.Equal(t, int64(2), number.Int64())
			return &types.Header{
				Number:   big.NewInt(2),
				Time:     uint64(timestamp.Unix()),
				Root:     stateRoot,
				GasLimit: 30_000_000,
			}, nil
		},
		getNextProposerFn: func(ctx context.Context, number *big.Int) (common.Hash, error) {
			switch number.Uint64() {
			case 1:
				return prevProposer, nil
			case 2:
				return nextProposer, nil
			default:
				t.Fatalf("unexpected proposer block %v", number)
				return common.Hash{}, nil
			}
		},
	}
	client := &EngineClient{
		engineClient:              proposerEngineRPCClient{},
		ethClient:                 ethClient,
		store:                     store,
		currentSafeBlockHash:      common.HexToHash("0x10"),
		currentFinalizedBlockHash: common.HexToHash("0x10"),
		forkchoiceInitialized:     true,
		logger:                    zerolog.Nop(),
	}

	result, err := client.ExecuteTxs(t.Context(), nil, 2, timestamp, nil)

	require.NoError(t, err)
	require.Equal(t, stateRoot.Bytes(), result.UpdatedStateRoot)
	require.Equal(t, nextProposer.Bytes(), result.NextProposerAddress)
	require.Len(t, ethClient.nextProposerBlocks, 2)
}

type proposerEngineRPCClient struct{}

func (proposerEngineRPCClient) ForkchoiceUpdated(context.Context, engine.ForkchoiceStateV1, map[string]any) (*engine.ForkChoiceResponse, error) {
	return &engine.ForkChoiceResponse{
		PayloadStatus: engine.PayloadStatusV1{Status: engine.VALID},
	}, nil
}

func (proposerEngineRPCClient) GetPayload(context.Context, engine.PayloadID) (*EnginePayloadEnvelope, error) {
	return nil, nil
}

func (proposerEngineRPCClient) NewPayload(context.Context, *EnginePayloadEnvelope, []string, string, [][]byte) (*engine.PayloadStatusV1, error) {
	return nil, nil
}
