package evm

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEnsureForkchoiceInitializedRestoresExecutionLayerState(t *testing.T) {
	head := forkchoiceTestHeader(8, 8)
	safe := forkchoiceTestHeader(6, 6)
	finalized := forkchoiceTestHeader(4, 4)
	ethRPC := newForkchoiceTestEthRPC(head, safe, finalized)
	client := newForkchoiceTestClient(ethRPC, &forkchoiceTestEngineRPC{})

	require.NoError(t, client.ensureForkchoiceInitialized(t.Context()))
	require.Equal(t, head.Hash(), client.currentHeadBlockHash)
	require.Equal(t, uint64(8), client.currentHeadHeight)
	require.Equal(t, safe.Hash(), client.currentSafeBlockHash)
	require.Equal(t, finalized.Hash(), client.currentFinalizedBlockHash)
	require.Equal(t, head.Hash(), client.blockHashCache[8])
	require.Equal(t, safe.Hash(), client.blockHashCache[6])
	require.Equal(t, finalized.Hash(), client.blockHashCache[4])
	require.Equal(t, 1, ethRPC.callsFor(rpc.LatestBlockNumber.Int64()))
	require.Equal(t, 1, ethRPC.callsFor(rpc.SafeBlockNumber.Int64()))
	require.Equal(t, 1, ethRPC.callsFor(rpc.FinalizedBlockNumber.Int64()))
}

func TestExecuteTxsFirstForkchoiceUsesRestoredSafeAndFinalized(t *testing.T) {
	head := forkchoiceTestHeader(5, 5)
	safe := forkchoiceTestHeader(4, 4)
	finalized := forkchoiceTestHeader(3, 3)
	next := forkchoiceTestHeader(6, 6)
	ethRPC := newForkchoiceTestEthRPC(head, safe, finalized)
	ethRPC.headers[5] = head
	engineRPC := &forkchoiceTestEngineRPC{
		payload: &EnginePayloadEnvelope{ExecutionPayload: &engine.ExecutableData{
			Number:    6,
			Timestamp: 1_700_000_006,
			BlockHash: next.Hash(),
			StateRoot: common.HexToHash("0x600"),
		}},
	}
	client := newForkchoiceTestClient(ethRPC, engineRPC)

	_, err := client.ExecuteTxs(t.Context(), nil, 6, time.Unix(1_700_000_006, 0), head.Root.Bytes())
	require.NoError(t, err)

	states := engineRPC.forkchoiceStates()
	require.Len(t, states, 2)
	require.Equal(t, head.Hash(), states[0].HeadBlockHash)
	require.Equal(t, safe.Hash(), states[0].SafeBlockHash)
	require.Equal(t, finalized.Hash(), states[0].FinalizedBlockHash)
}

func TestSetFinalFirstOperationPreservesRestoredHead(t *testing.T) {
	head := forkchoiceTestHeader(8, 8)
	safe := forkchoiceTestHeader(6, 6)
	finalized := forkchoiceTestHeader(4, 4)
	ethRPC := newForkchoiceTestEthRPC(head, safe, finalized)
	ethRPC.headers[6] = safe
	engineRPC := &forkchoiceTestEngineRPC{}
	client := newForkchoiceTestClient(ethRPC, engineRPC)

	require.NoError(t, client.SetFinal(t.Context(), 6))

	states := engineRPC.forkchoiceStates()
	require.Len(t, states, 1)
	require.Equal(t, head.Hash(), states[0].HeadBlockHash)
	require.Equal(t, safe.Hash(), states[0].SafeBlockHash)
	require.Equal(t, safe.Hash(), states[0].FinalizedBlockHash)
}

func TestForkchoiceInitializationConcurrentAndRetryable(t *testing.T) {
	t.Run("concurrent callers restore once", func(t *testing.T) {
		head := forkchoiceTestHeader(8, 8)
		ethRPC := newForkchoiceTestEthRPC(head, forkchoiceTestHeader(6, 6), forkchoiceTestHeader(4, 4))
		client := newForkchoiceTestClient(ethRPC, &forkchoiceTestEngineRPC{})

		var wg sync.WaitGroup
		errs := make(chan error, 12)
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- client.ensureForkchoiceInitialized(t.Context())
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err)
		}
		require.Equal(t, 3, ethRPC.totalCalls())
	})

	t.Run("failed restore can be retried", func(t *testing.T) {
		head := forkchoiceTestHeader(8, 8)
		ethRPC := newForkchoiceTestEthRPC(head, forkchoiceTestHeader(6, 6), forkchoiceTestHeader(4, 4))
		ethRPC.failures[rpc.SafeBlockNumber.Int64()] = 1
		client := newForkchoiceTestClient(ethRPC, &forkchoiceTestEngineRPC{})

		require.Error(t, client.ensureForkchoiceInitialized(t.Context()))
		require.False(t, client.forkchoiceInitialized)
		require.NoError(t, client.ensureForkchoiceInitialized(t.Context()))
		require.True(t, client.forkchoiceInitialized)
		require.Equal(t, 2, ethRPC.callsFor(rpc.SafeBlockNumber.Int64()))
	})
}

func TestInvalidRestoredForkchoicePreventsUpdate(t *testing.T) {
	tests := map[string]struct {
		safe      *types.Header
		finalized *types.Header
		failTag   rpc.BlockNumber
	}{
		"safe above head": {
			safe:      forkchoiceTestHeader(9, 9),
			finalized: forkchoiceTestHeader(4, 4),
		},
		"finalized above safe": {
			safe:      forkchoiceTestHeader(6, 6),
			finalized: forkchoiceTestHeader(7, 7),
		},
		"safe unavailable": {
			safe:      forkchoiceTestHeader(6, 6),
			finalized: forkchoiceTestHeader(4, 4),
			failTag:   rpc.SafeBlockNumber,
		},
		"finalized unavailable": {
			safe:      forkchoiceTestHeader(6, 6),
			finalized: forkchoiceTestHeader(4, 4),
			failTag:   rpc.FinalizedBlockNumber,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ethRPC := newForkchoiceTestEthRPC(forkchoiceTestHeader(8, 8), test.safe, test.finalized)
			if test.failTag != 0 {
				ethRPC.failures[test.failTag.Int64()] = 1
			}
			engineRPC := &forkchoiceTestEngineRPC{}
			client := newForkchoiceTestClient(ethRPC, engineRPC)

			err := client.SetSafe(t.Context(), common.HexToHash("0x1234"))
			require.Error(t, err)
			require.Empty(t, engineRPC.forkchoiceStates())
			require.False(t, client.forkchoiceInitialized)
		})
	}
}

func TestInitChainMarksGenesisForkchoiceInitialized(t *testing.T) {
	genesis := forkchoiceTestHeader(0, 1)
	ethRPC := newForkchoiceTestEthRPC(genesis, nil, nil)
	ethRPC.headers[0] = genesis
	engineRPC := &forkchoiceTestEngineRPC{}
	client := newForkchoiceTestClient(ethRPC, engineRPC)
	client.genesisHash = genesis.Hash()

	_, err := client.InitChain(t.Context(), time.Unix(0, 0), 1, "test")
	require.NoError(t, err)
	require.True(t, client.forkchoiceInitialized)

	states := engineRPC.forkchoiceStates()
	require.Len(t, states, 1)
	require.Equal(t, genesis.Hash(), states[0].HeadBlockHash)
	require.Equal(t, genesis.Hash(), states[0].SafeBlockHash)
	require.Equal(t, genesis.Hash(), states[0].FinalizedBlockHash)
}

func forkchoiceTestHeader(height uint64, marker byte) *types.Header {
	return &types.Header{
		Number:   new(big.Int).SetUint64(height),
		Root:     common.BytesToHash([]byte{marker, 0xaa}),
		GasLimit: 30_000_000,
		Time:     1_700_000_000 + height,
		Extra:    []byte{marker},
	}
}

type forkchoiceTestEthRPC struct {
	mu       sync.Mutex
	headers  map[int64]*types.Header
	failures map[int64]int
	calls    map[int64]int
}

func newForkchoiceTestEthRPC(head, safe, finalized *types.Header) *forkchoiceTestEthRPC {
	return &forkchoiceTestEthRPC{
		headers: map[int64]*types.Header{
			rpc.LatestBlockNumber.Int64():    head,
			rpc.SafeBlockNumber.Int64():      safe,
			rpc.FinalizedBlockNumber.Int64(): finalized,
		},
		failures: make(map[int64]int),
		calls:    make(map[int64]int),
	}
}

func (m *forkchoiceTestEthRPC) HeaderByNumber(_ context.Context, number *big.Int) (*types.Header, error) {
	key := number.Int64()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls[key]++
	if m.failures[key] > 0 {
		m.failures[key]--
		return nil, errors.New("temporary header failure")
	}
	header := m.headers[key]
	if header == nil {
		return nil, errors.New("header not found")
	}
	return header, nil
}

func (*forkchoiceTestEthRPC) GetTxs(context.Context) ([]string, error) { return nil, nil }

func (*forkchoiceTestEthRPC) GetNextProposer(context.Context, *big.Int) (common.Hash, error) {
	return common.Hash{}, nil
}

func (m *forkchoiceTestEthRPC) callsFor(key int64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls[key]
}

func (m *forkchoiceTestEthRPC) totalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, calls := range m.calls {
		total += calls
	}
	return total
}

type forkchoiceTestEngineRPC struct {
	mu      sync.Mutex
	states  []engine.ForkchoiceStateV1
	payload *EnginePayloadEnvelope
}

func (m *forkchoiceTestEngineRPC) ForkchoiceUpdated(_ context.Context, state engine.ForkchoiceStateV1, attrs map[string]any) (*engine.ForkChoiceResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = append(m.states, state)
	response := &engine.ForkChoiceResponse{PayloadStatus: engine.PayloadStatusV1{Status: engine.VALID}}
	if attrs != nil {
		payloadID := engine.PayloadID{1}
		response.PayloadID = &payloadID
	}
	return response, nil
}

func (m *forkchoiceTestEngineRPC) GetPayload(context.Context, engine.PayloadID) (*EnginePayloadEnvelope, error) {
	if m.payload == nil {
		return nil, errors.New("payload not configured")
	}
	return m.payload, nil
}

func (*forkchoiceTestEngineRPC) NewPayload(context.Context, *EnginePayloadEnvelope, []string, string, [][]byte) (*engine.PayloadStatusV1, error) {
	return &engine.PayloadStatusV1{Status: engine.VALID}, nil
}

func (m *forkchoiceTestEngineRPC) forkchoiceStates() []engine.ForkchoiceStateV1 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]engine.ForkchoiceStateV1(nil), m.states...)
}

func newForkchoiceTestClient(ethRPC EthRPCClient, engineRPC EngineRPCClient) *EngineClient {
	return &EngineClient{
		ethClient:                 ethRPC,
		engineClient:              engineRPC,
		store:                     NewEVMStore(dssync.MutexWrap(ds.NewMapDatastore())),
		currentHeadBlockHash:      common.HexToHash("0x01"),
		currentSafeBlockHash:      common.HexToHash("0x01"),
		currentFinalizedBlockHash: common.HexToHash("0x01"),
		blockHashCache:            make(map[uint64]common.Hash),
		logger:                    zerolog.Nop(),
	}
}
