package syncing

import (
	"context"
	crand "crypto/rand"
	"crypto/sha512"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/block/internal/cache"
	"github.com/evstack/ev-node/block/internal/common"
	"github.com/evstack/ev-node/block/internal/da"
	"github.com/evstack/ev-node/core/execution"
	"github.com/evstack/ev-node/pkg/config"
	datypes "github.com/evstack/ev-node/pkg/da/types"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/raft"
	signerpkg "github.com/evstack/ev-node/pkg/signer"
	"github.com/evstack/ev-node/pkg/signer/noop"
	"github.com/evstack/ev-node/pkg/store"
	testmocks "github.com/evstack/ev-node/test/mocks"
	extmocks "github.com/evstack/ev-node/test/mocks/external"
	"github.com/evstack/ev-node/types"
)

// stubRaftNode is a minimal RaftNode stub that records SetApplyCallback calls.
type stubRaftNode struct {
	mu        sync.Mutex
	callbacks []chan<- raft.RaftApplyMsg
}

type countingForcedInclusionRetriever struct {
	stopCalls int
}

func (*countingForcedInclusionRetriever) RetrieveForcedIncludedTxs(context.Context, uint64) (*da.ForcedInclusionEvent, error) {
	return nil, nil
}

func (*countingForcedInclusionRetriever) Start(context.Context) {}

func (r *countingForcedInclusionRetriever) Stop() {
	r.stopCalls++
}

type countingDAFollower struct {
	stopCalls int
}

func (*countingDAFollower) Start(context.Context) error { return nil }

func (f *countingDAFollower) Stop() {
	f.stopCalls++
}

func (*countingDAFollower) HasReachedHead() bool { return false }

func (*countingDAFollower) QueuePriorityHeight(uint64) {}

func (s *stubRaftNode) IsLeader() bool                                        { return false }
func (s *stubRaftNode) HasQuorum() bool                                       { return false }
func (s *stubRaftNode) GetState() *raft.RaftBlockState                        { return nil }
func (s *stubRaftNode) Broadcast(context.Context, *raft.RaftBlockState) error { return nil }
func (s *stubRaftNode) SetApplyCallback(ch chan<- raft.RaftApplyMsg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = append(s.callbacks, ch)
}

func (s *stubRaftNode) recordedCallbacks() []chan<- raft.RaftApplyMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]chan<- raft.RaftApplyMsg, len(s.callbacks))
	copy(out, s.callbacks)
	return out
}

// helper to create a signer, pubkey and address for tests
func buildSyncTestSigner(tb testing.TB) (addr []byte, pub crypto.PubKey, signer signerpkg.Signer) {
	tb.Helper()
	priv, _, err := crypto.GenerateEd25519Key(crand.Reader)
	require.NoError(tb, err)
	n, err := noop.NewNoopSigner(priv)
	require.NoError(tb, err)
	a, err := n.GetAddress()
	require.NoError(tb, err)
	p, err := n.GetPublic()
	require.NoError(tb, err)
	return a, p, n
}

// makeSignedHeaderBytes builds a valid SignedHeader and returns its binary encoding and the object
func makeSignedHeaderBytes(
	tb testing.TB,
	chainID string,
	height uint64,
	proposer []byte,
	pub crypto.PubKey,
	signer signerpkg.Signer,
	appHash []byte,
	data *types.Data,
	lastHeaderHash []byte,
) ([]byte, *types.SignedHeader) {
	time := uint64(time.Now().UnixNano())
	dataHash := common.DataHashForEmptyTxs
	if data != nil {
		time = uint64(data.Time().UnixNano())
		dataHash = data.DACommitment()
	}

	hdr := &types.SignedHeader{
		Header: types.Header{
			BaseHeader:      types.BaseHeader{ChainID: chainID, Height: height, Time: time},
			AppHash:         appHash,
			DataHash:        dataHash,
			ProposerAddress: proposer,
			LastHeaderHash:  lastHeaderHash,
		},
		Signer: types.Signer{PubKey: pub, Address: proposer},
	}
	bz, err := types.DefaultAggregatorNodeSignatureBytesProvider(&hdr.Header)
	require.NoError(tb, err)
	sig, err := signer.Sign(tb.Context(), bz)
	require.NoError(tb, err)
	hdr.Signature = sig
	bin, err := hdr.MarshalBinary()
	require.NoError(tb, err)
	return bin, hdr
}

func setupMockDAClient(tb testing.TB) (da.Client, chan datypes.SubscriptionEvent) {
	mockClient := testmocks.NewMockClient(tb)
	eventCh := make(chan datypes.SubscriptionEvent, 1)
	mockClient.EXPECT().SupportsSubscribe().Return(true).Maybe()
	mockClient.EXPECT().Subscribe(mock.Anything, mock.Anything, mock.Anything).Return((<-chan datypes.SubscriptionEvent)(eventCh), nil).Maybe()
	return mockClient, eventCh
}

func makeData(chainID string, height uint64, txs int) *types.Data {
	d := &types.Data{
		Metadata: &types.Metadata{
			ChainID: chainID,
			Height:  height,
			Time:    uint64(time.Now().Add(time.Duration(height) * time.Second).UnixNano())},
	}
	if txs > 0 {
		d.Txs = make(types.Txs, txs)
		for i := range txs {
			d.Txs[i] = types.Tx([]byte{byte(height), byte(i)})
		}
	}
	return d
}

func TestSyncer_validateBlock_DataHashMismatch(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)

	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second)}
	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, s.initializeState())
	// Create header and data with correct hash
	data := makeData(gen.ChainID, 1, 2) // non-empty
	_, header := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, nil, data, nil)

	err = s.ValidateBlock(t.Context(), s.getLastState(), data, header)
	require.NoError(t, err)

	// Create header and data with mismatched hash
	data = makeData(gen.ChainID, 1, 2) // non-empty
	_, header = makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, nil, nil, nil)
	err = s.ValidateBlock(t.Context(), s.getLastState(), data, header)
	require.Error(t, err)

	// Create header and empty data
	data = makeData(gen.ChainID, 1, 0) // empty
	_, header = makeSignedHeaderBytes(t, gen.ChainID, 2, addr, pub, signer, nil, nil, nil)
	err = s.ValidateBlock(t.Context(), s.getLastState(), data, header)
	require.Error(t, err)
}

func TestSyncer_ValidateBlock_UsesStateNextProposer(t *testing.T) {
	addr, _, _ := buildSyncTestSigner(t)
	badAddr, badPub, badSigner := buildSyncTestSigner(t)

	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second)}
	data := makeData(gen.ChainID, 1, 1)
	_, header := makeSignedHeaderBytes(t, gen.ChainID, 1, badAddr, badPub, badSigner, []byte("app0"), data, nil)

	s := &Syncer{logger: zerolog.Nop()}
	state := types.State{
		ChainID:             gen.ChainID,
		InitialHeight:       gen.InitialHeight,
		LastBlockHeight:     gen.InitialHeight - 1,
		LastBlockTime:       gen.StartTime,
		AppHash:             []byte("app0"),
		NextProposerAddress: addr,
	}

	err := s.ValidateBlock(t.Context(), state, data, header)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected proposer")
}

func TestSyncer_ExpectedProposerForHeight_OnlyNextHeight(t *testing.T) {
	addr, _, _ := buildSyncTestSigner(t)
	s := &Syncer{lastState: &atomic.Pointer[types.State]{}}
	s.SetLastState(types.State{
		LastBlockHeight:     4,
		NextProposerAddress: addr,
	})

	got, ok := s.expectedProposerForHeight(5)
	require.True(t, ok)
	require.Equal(t, addr, got)

	_, ok = s.expectedProposerForHeight(6)
	require.False(t, ok)
}

func TestSyncer_TrySyncNextBlock_ClassifiesExternalValidationFailures(t *testing.T) {
	expectedAddr, expectedPub, expectedSigner := buildSyncTestSigner(t)
	wrongAddr, wrongPub, wrongSigner := buildSyncTestSigner(t)

	now := time.Now()
	baseState := types.State{
		ChainID:             "tchain",
		InitialHeight:       1,
		LastBlockHeight:     1,
		LastBlockTime:       now,
		LastHeaderHash:      []byte("last-header-hash"),
		AppHash:             []byte("app0"),
		NextProposerAddress: expectedAddr,
	}

	makeSyncer := func(tb testing.TB) *Syncer {
		tb.Helper()
		ds := dssync.MutexWrap(datastore.NewMapDatastore())
		st := store.New(ds)
		cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
		require.NoError(tb, err)

		return &Syncer{
			cache:   cm,
			logger:  zerolog.Nop(),
			options: common.DefaultBlockOptions(),
		}
	}

	makeEvent := func(tb testing.TB, chainID string, proposer []byte, pub crypto.PubKey, signer signerpkg.Signer, appHash []byte, data *types.Data) common.DAHeightEvent {
		tb.Helper()
		_, header := makeSignedHeaderBytes(tb, chainID, 2, proposer, pub, signer, appHash, data, baseState.LastHeaderHash)
		return common.DAHeightEvent{
			Header: header,
			Data:   data,
			Source: common.SourceDA,
		}
	}

	tests := map[string]struct {
		event       func(testing.TB) common.DAHeightEvent
		wantState   bool
		wantInvalid bool
	}{
		"wrong proposer with bad app hash is an invalid external block": {
			event: func(tb testing.TB) common.DAHeightEvent {
				data := makeData(baseState.ChainID, 2, 1)
				data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				return makeEvent(tb, baseState.ChainID, wrongAddr, wrongPub, wrongSigner, []byte("forged-app"), data)
			},
			wantInvalid: true,
		},
		"wrong chain ID is an invalid external block": {
			event: func(tb testing.TB) common.DAHeightEvent {
				data := makeData("other-chain", 2, 1)
				data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				return makeEvent(tb, "other-chain", expectedAddr, expectedPub, expectedSigner, baseState.AppHash, data)
			},
			wantInvalid: true,
		},
		"data hash mismatch is an invalid external block": {
			event: func(tb testing.TB) common.DAHeightEvent {
				headerData := makeData(baseState.ChainID, 2, 1)
				headerData.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				event := makeEvent(tb, baseState.ChainID, expectedAddr, expectedPub, expectedSigner, baseState.AppHash, headerData)
				event.Data = makeData(baseState.ChainID, 2, 2)
				event.Data.Metadata.Time = headerData.Metadata.Time
				return event
			},
			wantInvalid: true,
		},
		"header data metadata mismatch is an invalid external block": {
			event: func(tb testing.TB) common.DAHeightEvent {
				headerData := makeData(baseState.ChainID, 2, 1)
				headerData.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				event := makeEvent(tb, baseState.ChainID, expectedAddr, expectedPub, expectedSigner, baseState.AppHash, headerData)
				event.Data = &types.Data{
					Metadata: &types.Metadata{
						ChainID: baseState.ChainID,
						Height:  3,
						Time:    headerData.Metadata.Time,
					},
					Txs: headerData.Txs,
				}
				return event
			},
			wantInvalid: true,
		},
		"expected proposer with bad app hash is invalid state": {
			event: func(tb testing.TB) common.DAHeightEvent {
				data := makeData(baseState.ChainID, 2, 1)
				data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				return makeEvent(tb, baseState.ChainID, expectedAddr, expectedPub, expectedSigner, []byte("wrong-app"), data)
			},
			wantState: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			event := tc.event(t)
			err := makeSyncer(t).trySyncNextBlockWithState(t.Context(), &event, baseState)
			require.Error(t, err)
			assert.Equal(t, tc.wantInvalid, errors.Is(err, errInvalidBlock), "invalid block classification")
			assert.Equal(t, tc.wantState, errors.Is(err, errInvalidState), "invalid state classification")
		})
	}
}

func TestSyncer_ValidateBlock_RejectsSignerAddressNotDerivedFromPubKey(t *testing.T) {
	expectedAddr, _, _ := buildSyncTestSigner(t)
	_, attackerPub, attackerSigner := buildSyncTestSigner(t)

	now := time.Now()
	state := types.State{
		ChainID:             "tchain",
		InitialHeight:       1,
		LastBlockHeight:     1,
		LastBlockTime:       now,
		LastHeaderHash:      []byte("last-header-hash"),
		AppHash:             []byte("app0"),
		NextProposerAddress: expectedAddr,
	}

	data := makeData(state.ChainID, 2, 1)
	data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
	_, header := makeSignedHeaderBytes(t, state.ChainID, 2, expectedAddr, attackerPub, attackerSigner, state.AppHash, data, state.LastHeaderHash)

	s := &Syncer{
		logger:  zerolog.Nop(),
		options: common.DefaultBlockOptions(),
	}

	err := s.ValidateBlock(t.Context(), state, data, header)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signer address")
}

func TestSyncer_ValidateBlock_ClassifiesFault(t *testing.T) {
	expectedAddr, expectedPub, expectedSigner := buildSyncTestSigner(t)
	wrongAddr, wrongPub, wrongSigner := buildSyncTestSigner(t)

	now := time.Now()
	baseState := types.State{
		ChainID:             "tchain",
		InitialHeight:       1,
		LastBlockHeight:     1,
		LastBlockTime:       now,
		LastHeaderHash:      []byte("last-header-hash"),
		AppHash:             []byte("app0"),
		NextProposerAddress: expectedAddr,
	}

	makeHeader := func(tb testing.TB, chainID string, proposer []byte, pub crypto.PubKey, signer signerpkg.Signer, appHash []byte, data *types.Data) *types.SignedHeader {
		tb.Helper()
		_, header := makeSignedHeaderBytes(tb, chainID, 2, proposer, pub, signer, appHash, data, baseState.LastHeaderHash)
		return header
	}

	tests := map[string]struct {
		setup     func(testing.TB) (*types.SignedHeader, *types.Data)
		wantFault ValidationFault
	}{
		"wrong proposer -> header fault": {
			setup: func(tb testing.TB) (*types.SignedHeader, *types.Data) {
				data := makeData(baseState.ChainID, 2, 1)
				data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				return makeHeader(tb, baseState.ChainID, wrongAddr, wrongPub, wrongSigner, baseState.AppHash, data), data
			},
			wantFault: FaultHeader,
		},
		"wrong chain id -> header fault": {
			setup: func(tb testing.TB) (*types.SignedHeader, *types.Data) {
				data := makeData("other-chain", 2, 1)
				data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				return makeHeader(tb, "other-chain", expectedAddr, expectedPub, expectedSigner, baseState.AppHash, data), data
			},
			wantFault: FaultHeader,
		},
		"data hash mismatch -> data fault": {
			setup: func(tb testing.TB) (*types.SignedHeader, *types.Data) {
				headerData := makeData(baseState.ChainID, 2, 1)
				headerData.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				header := makeHeader(tb, baseState.ChainID, expectedAddr, expectedPub, expectedSigner, baseState.AppHash, headerData)
				attached := makeData(baseState.ChainID, 2, 2)
				attached.Metadata.Time = headerData.Metadata.Time
				return header, attached
			},
			wantFault: FaultData,
		},
		"header data metadata mismatch -> data fault": {
			setup: func(tb testing.TB) (*types.SignedHeader, *types.Data) {
				headerData := makeData(baseState.ChainID, 2, 1)
				headerData.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				header := makeHeader(tb, baseState.ChainID, expectedAddr, expectedPub, expectedSigner, baseState.AppHash, headerData)
				attached := &types.Data{
					Metadata: &types.Metadata{
						ChainID: baseState.ChainID,
						Height:  3,
						Time:    headerData.Metadata.Time,
					},
					Txs: headerData.Txs,
				}
				return header, attached
			},
			wantFault: FaultData,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			header, data := tc.setup(t)
			s := &Syncer{logger: zerolog.Nop(), options: common.DefaultBlockOptions()}
			err := s.ValidateBlock(t.Context(), baseState, data, header)
			require.Error(t, err)

			var vErr *BlockValidationError
			require.ErrorAs(t, err, &vErr, "expected *BlockValidationError")
			assert.Equal(t, tc.wantFault, vErr.Fault, "fault classification")
		})
	}
}

func TestSyncer_TrySyncNextBlock_SelectiveCacheCleanup(t *testing.T) {
	expectedAddr, expectedPub, expectedSigner := buildSyncTestSigner(t)
	wrongAddr, wrongPub, wrongSigner := buildSyncTestSigner(t)

	now := time.Now()
	baseState := types.State{
		ChainID:             "tchain",
		InitialHeight:       1,
		LastBlockHeight:     1,
		LastBlockTime:       now,
		LastHeaderHash:      []byte("last-header-hash"),
		AppHash:             []byte("app0"),
		NextProposerAddress: expectedAddr,
	}

	makeSyncer := func(tb testing.TB) (*Syncer, cache.Manager) {
		tb.Helper()
		ds := dssync.MutexWrap(datastore.NewMapDatastore())
		st := store.New(ds)
		cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
		require.NoError(tb, err)
		return &Syncer{cache: cm, logger: zerolog.Nop(), options: common.DefaultBlockOptions()}, cm
	}

	tests := map[string]struct {
		event             func(testing.TB) common.DAHeightEvent
		wantHeaderInCache bool
		wantDataInCache   bool
	}{
		"header fault keeps data in cache": {
			event: func(tb testing.TB) common.DAHeightEvent {
				data := makeData(baseState.ChainID, 2, 1)
				data.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				_, header := makeSignedHeaderBytes(tb, baseState.ChainID, 2, wrongAddr, wrongPub, wrongSigner, baseState.AppHash, data, baseState.LastHeaderHash)
				return common.DAHeightEvent{Header: header, Data: data, Source: common.SourceDA}
			},
			wantHeaderInCache: false,
			wantDataInCache:   true,
		},
		"data fault keeps header in cache": {
			event: func(tb testing.TB) common.DAHeightEvent {
				headerData := makeData(baseState.ChainID, 2, 1)
				headerData.Metadata.Time = uint64(now.Add(time.Second).UnixNano())
				_, header := makeSignedHeaderBytes(tb, baseState.ChainID, 2, expectedAddr, expectedPub, expectedSigner, baseState.AppHash, headerData, baseState.LastHeaderHash)
				attached := makeData(baseState.ChainID, 2, 2)
				attached.Metadata.Time = headerData.Metadata.Time
				return common.DAHeightEvent{Header: header, Data: attached, Source: common.SourceDA}
			},
			wantHeaderInCache: true,
			wantDataInCache:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s, cm := makeSyncer(t)
			event := tc.event(t)
			headerHash := event.Header.Hash().String()
			dataHash := event.Data.DACommitment().String()

			// Seed the cache so we can observe what gets removed.
			cm.SetHeaderDAIncluded(headerHash, 1, event.Header.Height())
			cm.SetDataDAIncluded(dataHash, 1, event.Header.Height())

			err := s.trySyncNextBlockWithState(t.Context(), &event, baseState)
			require.Error(t, err)

			_, headerStillIncluded := cm.GetHeaderDAIncludedByHash(headerHash)
			_, dataStillIncluded := cm.GetDataDAIncludedByHash(dataHash)
			assert.Equal(t, tc.wantHeaderInCache, headerStillIncluded, "header cache presence")
			assert.Equal(t, tc.wantDataInCache, dataStillIncluded, "data cache presence")
		})
	}
}

func TestSyncer_ApplyBlockPersistsExecutionNextProposer(t *testing.T) {
	addr, _, _ := buildSyncTestSigner(t)
	execNext := []byte("execution-next-proposer")

	mockExec := testmocks.NewMockExecutor(t)
	data := makeData("tchain", 1, 1)
	header := types.Header{
		BaseHeader:      types.BaseHeader{ChainID: "tchain", Height: 1, Time: uint64(time.Now().UnixNano())},
		ProposerAddress: addr,
	}
	currentState := types.State{AppHash: []byte("app0"), NextProposerAddress: addr}

	mockExec.EXPECT().ExecuteTxs(mock.Anything, mock.Anything, uint64(1), mock.Anything, currentState.AppHash).
		Return(execution.ExecuteResult{
			UpdatedStateRoot:    []byte("app1"),
			NextProposerAddress: execNext,
		}, nil).Once()

	s := &Syncer{
		exec:   mockExec,
		ctx:    t.Context(),
		logger: zerolog.Nop(),
	}

	newState, err := s.ApplyBlock(t.Context(), header, data, currentState)
	require.NoError(t, err)
	require.Equal(t, execNext, newState.NextProposerAddress)
}

func TestProcessHeightEvent_SyncsAndUpdatesState(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)

	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	errChan := make(chan error, 1)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		errChan,
		nil,
	)

	require.NoError(t, s.initializeState())
	// set a context for internal loops that expect it
	s.ctx = t.Context()
	// Create signed header & data for height 1
	lastState := s.getLastState()
	data := makeData(gen.ChainID, 1, 0)
	_, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, lastState.AppHash, data, nil)
	daRetriever := &daRetriever{pendingData: map[uint64]*types.Data{1: data}}
	s.daRetriever = daRetriever

	// Expect ExecuteTxs call for height 1
	mockExec.EXPECT().ExecuteTxs(mock.Anything, mock.Anything, uint64(1), mock.Anything, lastState.AppHash).
		Return([]byte("app1"), nil).Once()

	evt := common.DAHeightEvent{Header: hdr, Data: data, Source: common.SourceDA, DaHeight: 1}
	s.processHeightEvent(t.Context(), &evt)

	requireEmptyChan(t, errChan)
	require.NotContains(t, daRetriever.pendingData, uint64(1), "accepted DA data should be removed from the retriever pending data")
	h, err := st.Height(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), h)
	st1, err := st.GetState(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(1), st1.LastBlockHeight)
}

func TestProcessHeightEvent_AlreadyProcessedDADataDoesNotAccumulate(t *testing.T) {
	const alreadyProcessedBlocks = 8

	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	batch, err := st.NewBatch(t.Context())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(alreadyProcessedBlocks))
	require.NoError(t, batch.Commit())

	addr, pub, signer := buildSyncTestSigner(t)
	gen := genesis.Genesis{
		ChainID:         "tchain",
		InitialHeight:   1,
		StartTime:       time.Now().Add(-time.Second),
		ProposerAddress: addr,
	}
	daRetriever := newTestDARetriever(t, nil, config.DefaultConfig(), gen)
	s := &Syncer{
		store:       st,
		daRetriever: daRetriever,
		logger:      zerolog.Nop(),
	}

	futureHeight := uint64(alreadyProcessedBlocks + 1)
	futureDataBin, _ := makeSignedDataBytes(t, gen.ChainID, futureHeight, addr, pub, signer, 8)
	require.Empty(t, daRetriever.processBlobs(t.Context(), [][]byte{futureDataBin}, futureHeight))
	require.Contains(t, daRetriever.pendingData, futureHeight)

	for height := uint64(1); height <= alreadyProcessedBlocks; height++ {
		dataBin, data := makeSignedDataBytes(t, gen.ChainID, height, addr, pub, signer, 8)
		headerBin, _ := makeSignedHeaderBytes(t, gen.ChainID, height, addr, pub, signer, nil, &data.Data, nil)

		events := daRetriever.processBlobs(t.Context(), [][]byte{headerBin, dataBin}, height)
		require.Len(t, events, 1)

		s.processHeightEvent(t.Context(), &events[0])
		require.NotContains(t, daRetriever.pendingData, height, "processed DA data must be removed immediately")
	}

	require.Len(t, daRetriever.pendingData, 1)
	require.Contains(t, daRetriever.pendingData, futureHeight, "DA data above the current height must remain pending")
}

func TestProcessHeightEvent_UnexpectedProposerFromDAIsNotCriticalStateError(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	expectedAddr, _, _ := buildSyncTestSigner(t)
	wrongAddr, wrongPub, wrongSigner := buildSyncTestSigner(t)

	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: expectedAddr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	errChan := make(chan error, 1)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		errChan,
		nil,
	)

	require.NoError(t, s.initializeState())
	s.ctx = t.Context()

	lastState := s.getLastState()
	data := makeData(gen.ChainID, 1, 0)
	_, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, wrongAddr, wrongPub, wrongSigner, lastState.AppHash, data, nil)

	evt := common.DAHeightEvent{Header: hdr, Data: data, Source: common.SourceDA, DaHeight: 1}
	s.processHeightEvent(t.Context(), &evt)

	requireEmptyChan(t, errChan)
	assert.False(t, s.hasCriticalError.Load(), "unexpected proposer from DA should be treated as an invalid external block")

	h, err := st.Height(t.Context())
	require.NoError(t, err)
	assert.Equal(t, uint64(0), h)
}

func TestSequentialBlockSync(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)
	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	errChan := make(chan error, 1)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		errChan,
		nil,
	)

	require.NoError(t, s.initializeState())
	s.ctx = t.Context()

	// Sync two consecutive blocks via processHeightEvent so ExecuteTxs is called and state stored
	st0 := s.getLastState()
	data1 := makeData(gen.ChainID, 1, 1) // non-empty
	_, hdr1 := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, st0.AppHash, data1, st0.LastHeaderHash)
	// Expect ExecuteTxs call for height 1
	mockExec.EXPECT().ExecuteTxs(mock.Anything, mock.Anything, uint64(1), mock.Anything, st0.AppHash).
		Return([]byte("app1"), nil).Once()
	evt1 := common.DAHeightEvent{Header: hdr1, Data: data1, DaHeight: 10}
	s.processHeightEvent(t.Context(), &evt1)

	st1, _ := st.GetState(t.Context())
	data2 := makeData(gen.ChainID, 2, 0) // empty data
	_, hdr2 := makeSignedHeaderBytes(t, gen.ChainID, 2, addr, pub, signer, st1.AppHash, data2, st1.LastHeaderHash)
	// Expect ExecuteTxs call for height 2
	mockExec.EXPECT().ExecuteTxs(mock.Anything, mock.Anything, uint64(2), mock.Anything, st1.AppHash).
		Return([]byte("app2"), nil).Once()
	evt2 := common.DAHeightEvent{Header: hdr2, Data: data2, DaHeight: 11}
	s.processHeightEvent(t.Context(), &evt2)

	// Mark DA inclusion in cache (as DA retrieval would)
	cm.SetDataDAIncluded(data1.DACommitment().String(), 10, 1)
	cm.SetDataDAIncluded(data2.DACommitment().String(), 11, 2) // empty data still needs cache entry
	cm.SetHeaderDAIncluded(hdr1.Header.Hash().String(), 10, 1)
	cm.SetHeaderDAIncluded(hdr2.Header.Hash().String(), 11, 2)

	// Verify both blocks were synced correctly
	finalState, _ := st.GetState(t.Context())
	assert.Equal(t, uint64(2), finalState.LastBlockHeight)

	// Verify DA inclusion markers are set
	_, ok := cm.GetHeaderDAIncludedByHash(hdr1.Hash().String())
	assert.True(t, ok)
	_, ok = cm.GetHeaderDAIncludedByHash(hdr2.Hash().String())
	assert.True(t, ok)
	_, ok = cm.GetDataDAIncludedByHash(data1.DACommitment().String())
	assert.True(t, ok)
	_, ok = cm.GetDataDAIncludedByHash(data2.DACommitment().String())
	assert.True(t, ok)
	requireEmptyChan(t, errChan)
}

func TestSyncer_RecoverFromRaft_BootstrapsStateWhenUninitialized(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)
	gen := genesis.Genesis{
		ChainID:         "1234",
		InitialHeight:   1,
		StartTime:       time.Now().Add(-time.Second),
		ProposerAddress: addr,
	}

	mockExec := testmocks.NewMockExecutor(t)
	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		config.DefaultConfig(),
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)

	// lastState intentionally not initialized to simulate recovery-before-start path.
	data := makeData(gen.ChainID, 1, 0)
	headerBz, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, []byte("app0"), data, nil)
	dataBz, err := data.MarshalBinary()
	require.NoError(t, err)

	mockExec.EXPECT().
		ExecuteTxs(mock.Anything, mock.Anything, uint64(1), mock.Anything, mock.Anything).
		Return([]byte("app1"), nil).
		Once()

	err = s.RecoverFromRaft(t.Context(), &raft.RaftBlockState{
		Height: 1,
		Hash:   hdr.Hash(),
		Header: headerBz,
		Data:   dataBz,
	})
	require.NoError(t, err)

	state, err := st.GetState(t.Context())
	require.NoError(t, err)
	require.Equal(t, gen.ChainID, state.ChainID)
	require.Equal(t, uint64(1), state.LastBlockHeight)
}

func TestSyncer_RecoverFromRaft_KeepsStrictValidationAfterStateExists(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)
	gen := genesis.Genesis{
		ChainID:         "1234",
		InitialHeight:   1,
		StartTime:       time.Now().Add(-time.Second),
		ProposerAddress: addr,
	}

	mockExec := testmocks.NewMockExecutor(t)
	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		config.DefaultConfig(),
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)

	// Non-empty state must remain strictly validated.
	s.SetLastState(types.State{
		ChainID:         "wrong-chain",
		InitialHeight:   1,
		LastBlockHeight: 0,
	})

	data := makeData(gen.ChainID, 1, 0)
	headerBz, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, []byte("app0"), data, nil)
	dataBz, err := data.MarshalBinary()
	require.NoError(t, err)

	err = s.RecoverFromRaft(t.Context(), &raft.RaftBlockState{
		Height: 1,
		Hash:   hdr.Hash(),
		Header: headerBz,
		Data:   dataBz,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid chain ID")
}

// TestSyncer_RecoverFromRaft_LocalAheadOfStaleSnapshot tests Bug A: when the node
// restarts and the EVM is ahead of the raft FSM snapshot (stale snapshot due to
// timing or log compaction), RecoverFromRaft should skip recovery if the local
// block at raftState.Height has a matching hash, rather than crashing.
func TestSyncer_RecoverFromRaft_LocalAheadOfStaleSnapshot(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)
	gen := genesis.Genesis{
		ChainID:         "1234",
		InitialHeight:   1,
		StartTime:       time.Now().Add(-time.Second),
		ProposerAddress: addr,
	}

	mockExec := testmocks.NewMockExecutor(t)
	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		config.DefaultConfig(),
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)

	// Build block at height 1 and persist it (simulates EVM block persisted before SIGTERM).
	data1 := makeData(gen.ChainID, 1, 0)
	headerBz1, hdr1 := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, []byte("app1"), data1, nil)
	dataBz1, err := data1.MarshalBinary()
	require.NoError(t, err)

	batch, err := st.NewBatch(t.Context())
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockDataFromBytes(hdr1, headerBz1, dataBz1, &hdr1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.UpdateState(types.State{
		ChainID:         gen.ChainID,
		InitialHeight:   1,
		LastBlockHeight: 1,
		LastHeaderHash:  hdr1.Hash(),
	}))
	require.NoError(t, batch.Commit())

	// Simulate EVM at height 1, raft snapshot stale at height 0 — but there is no
	// block 0 to check, so use height 1 EVM vs stale snapshot at height 0.
	// More realistic: EVM at height 2, raft snapshot at height 1.
	// Build a second block and advance the store state to height 2.
	data2 := makeData(gen.ChainID, 2, 0)
	headerBz2, hdr2 := makeSignedHeaderBytes(t, gen.ChainID, 2, addr, pub, signer, []byte("app2"), data2, hdr1.Hash())
	dataBz2, err := data2.MarshalBinary()
	require.NoError(t, err)

	batch2, err := st.NewBatch(t.Context())
	require.NoError(t, err)
	require.NoError(t, batch2.SaveBlockDataFromBytes(hdr2, headerBz2, dataBz2, &hdr2.Signature))
	require.NoError(t, batch2.SetHeight(2))
	require.NoError(t, batch2.UpdateState(types.State{
		ChainID:         gen.ChainID,
		InitialHeight:   1,
		LastBlockHeight: 2,
		LastHeaderHash:  hdr2.Hash(),
	}))
	require.NoError(t, batch2.Commit())

	// Set lastState to height 2 (EVM is at 2).
	s.SetLastState(types.State{
		ChainID:         gen.ChainID,
		InitialHeight:   1,
		LastBlockHeight: 2,
		LastHeaderHash:  hdr2.Hash(),
	})

	t.Run("matching hash skips recovery", func(t *testing.T) {
		// raft snapshot is stale at height 1 (EVM is at 2); hash matches local block 1.
		err := s.RecoverFromRaft(t.Context(), &raft.RaftBlockState{
			Height: 1,
			Hash:   hdr1.Hash(),
			Header: headerBz1,
			Data:   dataBz1,
		})
		require.NoError(t, err, "local ahead of stale raft snapshot with matching hash should not error")
	})

	t.Run("diverged hash returns error", func(t *testing.T) {
		wrongHash := make([]byte, len(hdr1.Hash()))
		copy(wrongHash, hdr1.Hash())
		wrongHash[0] ^= 0xFF // flip a byte to produce a different hash

		err := s.RecoverFromRaft(t.Context(), &raft.RaftBlockState{
			Height: 1,
			Hash:   wrongHash,
			Header: headerBz1,
			Data:   dataBz1,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "diverged from raft")
	})

	t.Run("get header fails returns error", func(t *testing.T) {
		// lastState is at height 2; raft snapshot at height 0.
		// No block is stored at height 0, so GetHeader fails.
		err := s.RecoverFromRaft(t.Context(), &raft.RaftBlockState{
			Height: 0,
			Hash:   make([]byte, 32),
			Header: headerBz1,
			Data:   dataBz1,
		})
		require.Error(t, err)
		require.ErrorContains(t, err, "cannot verify hash")
	})
}

func TestSyncer_Stop_CallsRaftRetrieverStop(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	raftNode := &stubRaftNode{}
	s := NewSyncer(
		st,
		nil,
		nil,
		cm,
		common.NopMetrics(),
		config.DefaultConfig(),
		genesis.Genesis{},
		nil,
		nil,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		raftNode,
	)

	require.NotNil(t, s.raftRetriever, "raftRetriever should be set when raftNode is provided")

	// Manually set cancel so Stop() doesn't bail out early (simulates having been started).
	ctx, cancel := context.WithCancel(t.Context())
	s.ctx = ctx
	s.cancel = cancel
	s.lifecycleState = syncerLifecycleStarted

	require.NoError(t, s.Stop(t.Context()))

	// raftRetriever.Stop clears the apply callback (sets it to nil).
	// The stub records each SetApplyCallback call; the last one should be nil.
	callbacks := raftNode.recordedCallbacks()
	require.NotEmpty(t, callbacks, "expected at least one callback registration")
	assert.Nil(t, callbacks[len(callbacks)-1], "last callback should be nil after Stop")
}

func TestSyncer_Stop_IsIdempotentAfterStartFailure(t *testing.T) {
	mockStore := testmocks.NewMockStore(t)
	mockStore.EXPECT().GetState(mock.Anything).Return(types.State{DAHeight: 1}, nil).Once()

	raftNode := &stubRaftNode{}
	fiRetriever := &countingForcedInclusionRetriever{}
	daFollower := &countingDAFollower{}
	s := NewSyncer(
		mockStore,
		nil,
		nil,
		nil,
		common.NopMetrics(),
		config.DefaultConfig(),
		genesis.Genesis{DAStartHeight: 2},
		nil,
		nil,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		raftNode,
	)
	s.fiRetriever = fiRetriever
	s.daFollower = daFollower

	err := s.Start(t.Context())
	require.ErrorContains(t, err, "DA height (1) is lower than DA start height (2)")
	require.NoError(t, s.Stop(t.Context()))
	require.NoError(t, s.Stop(t.Context()))

	assert.Nil(t, s.cancel)
	assert.Equal(t, syncerLifecycleStopped, s.lifecycleState)
	assert.Equal(t, 1, fiRetriever.stopCalls, "forced inclusion retriever should only be stopped once")
	assert.Equal(t, 1, daFollower.stopCalls, "DA follower should only be stopped once")
	_, open := <-s.heightInCh
	assert.False(t, open, "height input channel should be closed")

	callbacks := raftNode.recordedCallbacks()
	require.Len(t, callbacks, 1, "raft retriever should only be stopped once")
	assert.Nil(t, callbacks[0])
}

func TestSyncer_processPendingEvents(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	// current height 1
	batch, err := st.NewBatch(t.Context())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	s := &Syncer{
		store:      st,
		cache:      cm,
		ctx:        t.Context(),
		heightInCh: make(chan common.DAHeightEvent, 2),
		logger:     zerolog.Nop(),
	}

	// create two pending events, one for height 2 (> current) and one for height 1 (<= current)
	evt1 := &common.DAHeightEvent{Header: &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 1}}}, Data: &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 1}}}
	evt2 := &common.DAHeightEvent{Header: &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 2}}}, Data: &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 2}}}
	cm.SetPendingEvent(1, evt1)
	cm.SetPendingEvent(2, evt2)

	s.processPendingEvents(s.ctx)

	// should have forwarded height 2 and removed both
	select {
	case got := <-s.heightInCh:
		assert.Equal(t, uint64(2), got.Header.Height())
	default:
		t.Fatal("expected a forwarded pending event")
	}

	// Verify the event was removed by trying to get it again
	remaining := cm.GetNextPendingEvent(2)
	assert.Nil(t, remaining)
}

func TestSyncLoopPersistState(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cfg := config.DefaultConfig()
	t.Setenv("HOME", t.TempDir())
	cfg.RootDir = t.TempDir()

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	const myDAHeightOffset = uint64(1)
	const numBlocks = uint64(5)

	addr, pub, signer := buildSyncTestSigner(t)
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr, DAStartHeight: myDAHeightOffset}

	dummyExec := execution.NewDummyExecutor()

	// Create mock stores for P2P
	mockHeaderStore := extmocks.NewMockStore[*types.SignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()

	mockDataStore := extmocks.NewMockStore[*types.Data](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	mockP2PHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockP2PHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockP2PDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockP2PDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	errorCh := make(chan error, 1)
	syncerInst1 := NewSyncer(
		st,
		dummyExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockP2PHeaderStore,
		mockP2PDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		errorCh,
		nil,
	)
	require.NoError(t, syncerInst1.initializeState())

	ctx, cancel := context.WithCancel(t.Context())
	syncerInst1.ctx = ctx
	daRtrMock, p2pHndlMock := NewMockDARetriever(t), newMockp2pHandler(t)
	p2pHndlMock.On("ProcessHeight", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	p2pHndlMock.On("SetProcessedHeight", mock.Anything).Return().Maybe()

	syncerInst1.daRetriever, syncerInst1.p2pHandler = daRtrMock, p2pHndlMock
	syncerInst1.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     daRtrMock,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(syncerInst1.PipeEvent),
		Namespace:     []byte("ns"),
		StartDAHeight: syncerInst1.daRetrieverHeight.Load(),
	})

	// with n da blobs fetched
	var prevHeaderHash, prevAppHash []byte
	prevAppHash, _ = execution.NewDummyExecutor().InitChain(t.Context(), gen.StartTime, gen.DAStartHeight, gen.ChainID)
	for i := range numBlocks {
		chainHeight, daHeight := gen.InitialHeight+i, i+myDAHeightOffset
		blockTime := gen.StartTime.Add(time.Duration(chainHeight+1) * time.Second)
		emptyData := &types.Data{
			Metadata: &types.Metadata{
				ChainID: gen.ChainID,
				Height:  chainHeight,
				Time:    uint64(blockTime.UnixNano()),
			},
		}
		_, sigHeader := makeSignedHeaderBytes(t, gen.ChainID, chainHeight, addr, pub, signer, prevAppHash, emptyData, prevHeaderHash)
		evts := []common.DAHeightEvent{{
			Header:   sigHeader,
			Data:     emptyData,
			DaHeight: daHeight,
		}}
		daRtrMock.On("RetrieveFromDA", mock.Anything, daHeight).Return(evts, nil)
		prevHeaderHash = sigHeader.Hash()
		hasher := sha512.New()
		hasher.Write(prevAppHash)
		prevAppHash = hasher.Sum(nil)
	}

	// stop at next height
	myFutureDAHeight := myDAHeightOffset + numBlocks
	daRtrMock.On("RetrieveFromDA", mock.Anything, myFutureDAHeight).
		Run(func(_ mock.Arguments) {
			// wait for consumer to catch up
			require.Eventually(t, func() bool {
				return len(syncerInst1.heightInCh) == 0
			}, 1*time.Second, 10*time.Millisecond)
			cancel()
		}).
		Return(nil, datypes.ErrHeightFromFuture)

	go syncerInst1.processLoop(ctx)

	// Create and start a DAFollower so DA retrieval actually happens.
	daClient, eventCh := setupMockDAClient(t)
	follower1 := NewDAFollower(DAFollowerConfig{
		Client:        daClient,
		Retriever:     daRtrMock,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(syncerInst1.PipeEvent),
		Namespace:     []byte("ns"),
		StartDAHeight: syncerInst1.daRetrieverHeight.Load(),
		DABlockTime:   cfg.DA.BlockTime.Duration,
	}).(*daFollower)
	require.NoError(t, follower1.Start(ctx))
	eventCh <- datypes.SubscriptionEvent{Height: myFutureDAHeight}
	syncerInst1.startSyncWorkers(ctx)
	syncerInst1.wg.Wait()
	follower1.Stop()
	requireEmptyChan(t, errorCh)

	t.Log("sync workers on instance1 completed")
	require.Equal(t, myFutureDAHeight, follower1.subscriber.LocalDAHeight())

	// wait for all events consumed
	require.NoError(t, cm.SaveToStore())
	t.Log("processLoop on instance1 completed")

	// then
	daRtrMock.AssertExpectations(t)
	p2pHndlMock.AssertExpectations(t)
	require.Len(t, syncerInst1.heightInCh, 0)

	// and all processed - verify no events remain at heights we tested
	for i := range numBlocks {
		blockHeight := gen.InitialHeight + i + 1
		event := syncerInst1.cache.GetNextPendingEvent(blockHeight)
		require.Nil(t, event, "event at height %d should have been removed", blockHeight)
	}
	// and when new instance is up on restart
	cm, err = cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)
	require.NoError(t, cm.RestoreFromStore())

	syncerInst2 := NewSyncer(
		st,
		dummyExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockP2PHeaderStore,
		mockP2PDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, syncerInst2.initializeState())

	ctx, cancel = context.WithCancel(t.Context())
	t.Cleanup(cancel)
	syncerInst2.ctx = ctx
	daRtrMock, p2pHndlMock = NewMockDARetriever(t), newMockp2pHandler(t)
	p2pHndlMock.On("ProcessHeight", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	p2pHndlMock.On("SetProcessedHeight", mock.Anything).Return().Maybe()

	syncerInst2.daRetriever, syncerInst2.p2pHandler = daRtrMock, p2pHndlMock
	syncerInst2.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     daRtrMock,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(syncerInst2.PipeEvent),
		Namespace:     []byte("ns"),
		StartDAHeight: syncerInst2.daRetrieverHeight.Load(),
	})

	daRtrMock.On("RetrieveFromDA", mock.Anything, mock.Anything).
		Run(func(arg mock.Arguments) {
			cancel()
			// retrieve last one again
			assert.Equal(t, syncerInst2.daRetrieverHeight.Load(), arg.Get(1).(uint64))
		}).
		Return(nil, nil)

	// when it starts, it should fetch from the last height it stopped at
	t.Log("sync workers on instance2 started")

	// Create a follower for instance 2.
	daClient2, eventCh2 := setupMockDAClient(t)
	follower2 := NewDAFollower(DAFollowerConfig{
		Client:        daClient2,
		Retriever:     daRtrMock,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(syncerInst2.PipeEvent),
		Namespace:     []byte("ns"),
		StartDAHeight: syncerInst2.daRetrieverHeight.Load(),
		DABlockTime:   cfg.DA.BlockTime.Duration,
	}).(*daFollower)
	follower2.Start(ctx)
	eventCh2 <- datypes.SubscriptionEvent{Height: syncerInst2.daRetrieverHeight.Load() + 1}
	syncerInst2.startSyncWorkers(ctx)
	syncerInst2.wg.Wait()

	t.Log("sync workers exited")
}

func TestSyncer_executeTxsWithRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupMock     func(*testmocks.MockExecutor)
		expectSuccess bool
		expectHash    []byte
		expectError   string
	}{
		{
			name: "success on first attempt",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("ExecuteTxs", mock.Anything, mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return([]byte("new-hash"), nil).Once()
			},
			expectSuccess: true,
			expectHash:    []byte("new-hash"),
		},
		{
			name: "success on second attempt",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("ExecuteTxs", mock.Anything, mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return([]byte(nil), errors.New("temporary failure")).Once()
				exec.On("ExecuteTxs", mock.Anything, mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return([]byte("new-hash"), nil).Once()
			},
			expectSuccess: true,
			expectHash:    []byte("new-hash"),
		},
		{
			name: "success on third attempt",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("ExecuteTxs", mock.Anything, mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return([]byte(nil), errors.New("temporary failure")).Times(2)
				exec.On("ExecuteTxs", mock.Anything, mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return([]byte("new-hash"), nil).Once()
			},
			expectSuccess: true,
			expectHash:    []byte("new-hash"),
		},
		{
			name: "failure after max retries",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("ExecuteTxs", mock.Anything, mock.Anything, uint64(100), mock.Anything, mock.Anything).
					Return([]byte(nil), errors.New("persistent failure")).Times(common.MaxRetriesBeforeHalt)
			},
			expectSuccess: false,
			expectError:   "failed to execute transactions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx := t.Context()
				exec := testmocks.NewMockExecutor(t)
				tt.setupMock(exec)

				s := &Syncer{
					exec:   exec,
					ctx:    ctx,
					logger: zerolog.Nop(),
				}

				rawTxs := [][]byte{[]byte("tx1"), []byte("tx2")}
				header := types.Header{
					BaseHeader: types.BaseHeader{Height: 100, Time: uint64(time.Now().UnixNano())},
				}
				currentState := types.State{AppHash: []byte("current-hash")}

				result, err := s.executeTxsWithRetry(ctx, rawTxs, header, currentState)

				if tt.expectSuccess {
					require.NoError(t, err)
					assert.Equal(t, tt.expectHash, result.UpdatedStateRoot)
				} else {
					require.Error(t, err)
					if tt.expectError != "" {
						assert.Contains(t, err.Error(), tt.expectError)
					}
				}

				exec.AssertExpectations(t)
			})
		})
	}
}

func TestSyncer_InitializeState_CallsReplayer(t *testing.T) {
	// This test verifies that initializeState() invokes Replayer.
	// The detailed replay logic is tested in block/internal/common/replay_test.go

	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	// Create testmocks
	mockStore := testmocks.NewMockStore(t)
	mockExec := testmocks.NewMockHeightAwareExecutor(t)

	// Setup genesis
	gen := genesis.Genesis{
		ChainID:       "test-chain",
		InitialHeight: 1,
		StartTime:     time.Now().UTC(),
		DAStartHeight: 0,
	}

	// Setup state in store
	storeHeight := uint64(10)
	mockStore.EXPECT().GetState(mock.Anything).Return(
		types.State{
			ChainID:         gen.ChainID,
			InitialHeight:   gen.InitialHeight,
			LastBlockHeight: storeHeight,
			LastBlockTime:   time.Now().UTC(),
			DAHeight:        5,
			AppHash:         []byte("app-hash"),
		},
		nil,
	)

	// Mock Batch operations
	mockBatch := testmocks.NewMockBatch(t)
	mockBatch.Test(t)
	mockStore.EXPECT().NewBatch(mock.Anything).Return(mockBatch, nil)
	mockBatch.On("SetHeight", storeHeight).Return(nil)
	mockBatch.On("UpdateState", mock.Anything).Return(nil)
	mockBatch.On("Commit").Return(nil)

	// Setup execution layer to be in sync
	mockExec.On("GetLatestHeight", mock.Anything).Return(storeHeight, nil)

	syncer := &Syncer{
		store:             mockStore,
		exec:              mockExec,
		genesis:           gen,
		lastState:         &atomic.Pointer[types.State]{},
		daRetrieverHeight: &atomic.Uint64{},
		logger:            zerolog.Nop(),
		ctx:               t.Context(),
		cache:             cm,
	}

	// Initialize state - this should call Replayer
	err = syncer.initializeState()
	require.NoError(t, err)

	// Verify state was initialized correctly
	state := syncer.getLastState()
	assert.Equal(t, storeHeight, state.LastBlockHeight)
	assert.Equal(t, gen.ChainID, state.ChainID)

	// Verify that GetLatestHeight was called (proves Replayer was invoked)
	mockExec.AssertCalled(t, "GetLatestHeight", mock.Anything)
}

func requireEmptyChan(t *testing.T, errorCh chan error) {
	t.Helper()
	select {
	case err := <-errorCh:
		t.Fatalf("sync workers failed: %v", err)
	default:
	}
}

func TestProcessHeightEvent_TriggersAsyncDARetrieval(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, _, _ := buildSyncTestSigner(t)
	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	// Use a mock DA client that reports a latest height above the hint
	mockDAClient := testmocks.NewMockClient(t)
	mockDAClient.EXPECT().GetLatestDAHeight(mock.Anything).Return(uint64(200), nil).Maybe()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	s := NewSyncer(
		st,
		mockExec,
		mockDAClient,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, s.initializeState())
	s.ctx = context.Background()

	// Create a real daRetriever to test priority queue
	s.daRetriever = NewDARetriever(nil, cm, gen, zerolog.Nop())
	s.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     s.daRetriever,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(func(_ context.Context, _ common.DAHeightEvent) error { return nil }),
		Namespace:     []byte("ns"),
		StartDAHeight: 0,
	})

	// Create event with DA height hint
	evt := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 2}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 2}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{100, 100},
	}

	// Set the store height to 1 so the event can be processed as "next".
	batch, err := st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	s.processHeightEvent(t.Context(), &evt)

	// Verify that the priority height was queued in the daRetriever
	priorityHeight := s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(100), priorityHeight)
}

func TestProcessHeightEvent_RejectsUnreasonableDAHint(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, _, _ := buildSyncTestSigner(t)
	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	// Mock DA client reports latest DA height of 100
	mockDAClient := testmocks.NewMockClient(t)
	mockDAClient.EXPECT().GetLatestDAHeight(mock.Anything).Return(uint64(100), nil).Maybe()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	s := NewSyncer(
		st,
		mockExec,
		mockDAClient,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, s.initializeState())
	s.ctx = context.Background()
	s.daRetriever = NewDARetriever(nil, cm, gen, zerolog.Nop())
	s.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     s.daRetriever,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(func(_ context.Context, _ common.DAHeightEvent) error { return nil }),
		Namespace:     []byte("ns"),
		StartDAHeight: 0,
	})

	// Set store height to 1 so event at height 2 is "next"
	batch, err := st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Send a malicious P2P hint with math.MaxUint64
	evt := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 2}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 2}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{math.MaxUint64, math.MaxUint64},
	}

	s.processHeightEvent(t.Context(), &evt)

	// Verify that NO priority height was queued — the hint was rejected
	priorityHeight := s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(0), priorityHeight, "unreasonable DA hint should be rejected")
}

func TestProcessHeightEvent_AcceptsValidDAHint(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, _, _ := buildSyncTestSigner(t)
	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	// Mock DA client reports latest DA height of 100
	mockDAClient := testmocks.NewMockClient(t)
	mockDAClient.EXPECT().GetLatestDAHeight(mock.Anything).Return(uint64(100), nil).Maybe()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	s := NewSyncer(
		st,
		mockExec,
		mockDAClient,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, s.initializeState())
	s.ctx = context.Background()
	s.daRetriever = NewDARetriever(nil, cm, gen, zerolog.Nop())
	s.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     s.daRetriever,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(func(_ context.Context, _ common.DAHeightEvent) error { return nil }),
		Namespace:     []byte("ns"),
		StartDAHeight: 0,
	})

	// Set store height to 1 so event at height 2 is "next"
	batch, err := st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Send a valid P2P hint at height 50, which is below the latest DA height of 100
	evt := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 2}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 2}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{50, 50},
	}

	s.processHeightEvent(t.Context(), &evt)

	// Verify that the priority height was queued — the hint is valid
	priorityHeight := s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(50), priorityHeight, "valid DA hint should be queued")
}

// TestProcessHeightEvent_SkipsDAHintWhenAlreadyDAIncluded verifies that when the
// DA-inclusion cache already has an entry for the block height carried by a P2P
// event, the DA height hint is NOT queued for priority retrieval.
func TestProcessHeightEvent_SkipsDAHintWhenAlreadyDAIncluded(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, _, _ := buildSyncTestSigner(t)
	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	mockDAClient := testmocks.NewMockClient(t)

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	s := NewSyncer(
		st,
		mockExec,
		mockDAClient,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, s.initializeState())
	s.ctx = context.Background()
	s.daRetriever = NewDARetriever(nil, cm, gen, zerolog.Nop())
	s.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     s.daRetriever,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(func(_ context.Context, _ common.DAHeightEvent) error { return nil }),
		Namespace:     []byte("ns"),
		StartDAHeight: 0,
	})

	// Set the store height to 1 so the event at height 2 is "next".
	batch, err := st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Simulate already DA-included header and data at height 2.
	cm.SetHeaderDAIncluded("somehash-hdr2", 42, 2)
	cm.SetDataDAIncluded("somehash-data2", 42, 2)

	// Both hints point to DA height 100. They should be skipped due to cache hits.
	evt := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: gen.ChainID, Height: 2}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: gen.ChainID, Height: 2}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{100, 100},
	}
	s.processHeightEvent(t.Context(), &evt)

	priorityHeight := s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(0), priorityHeight,
		"DA hint must not be queued when header and data are already DA-included in cache")

	// Partial case: only header is DA-included at height 3, data is not.
	cm2, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)
	s.cache = cm2
	cm2.SetHeaderDAIncluded("somehash-hdr3", 55, 3)

	batch, err = st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(2))
	require.NoError(t, batch.Commit())

	evt3 := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: gen.ChainID, Height: 3}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: gen.ChainID, Height: 3}, Txs: types.Txs{types.Tx("tx2")}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{150, 151},
	}
	s.processHeightEvent(t.Context(), &evt3)

	priorityHeight = s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(151), priorityHeight,
		"data hint must be queued when only the header is already DA-included")
	assert.Equal(t, uint64(0), s.daFollower.(*daFollower).popPriorityHeight(),
		"no further hints should be queued")
}

func TestProcessHeightEvent_SkipsDAHintWhenBelowRetrieverCursor(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, _, _ := buildSyncTestSigner(t)
	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	// Mock DA client reports latest DA height of 100
	mockDAClient := testmocks.NewMockClient(t)
	mockDAClient.EXPECT().GetLatestDAHeight(mock.Anything).Return(uint64(100), nil).Maybe()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	s := NewSyncer(
		st,
		mockExec,
		mockDAClient,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		make(chan error, 1),
		nil,
	)
	require.NoError(t, s.initializeState())
	s.ctx = context.Background()

	// Create a real daRetriever to test priority queue
	s.daRetriever = NewDARetriever(nil, cm, gen, zerolog.Nop())
	s.daFollower = NewDAFollower(DAFollowerConfig{
		Retriever:     s.daRetriever,
		Logger:        zerolog.Nop(),
		EventSink:     common.EventSinkFunc(func(_ context.Context, _ common.DAHeightEvent) error { return nil }),
		Namespace:     []byte("ns"),
		StartDAHeight: 0,
	})

	// Set DA retriever height to 150 - simulating we've already fetched past height 100
	s.daRetrieverHeight.Store(150)

	// Set the store height to 1 so the event can be processed
	batch, err := st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Create event with DA height hint that is BELOW the current daRetrieverHeight
	evt := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 2}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 2}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{100, 100}, // Both hints are below 150
	}

	s.processHeightEvent(t.Context(), &evt)

	// Verify that no priority height was queued since we've already fetched past it
	priorityHeight := s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(0), priorityHeight, "should not queue DA hint that is below current daRetrieverHeight")

	// Now test with a hint that is ABOVE the current daRetrieverHeight
	evt2 := common.DAHeightEvent{
		Header:        &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: "c", Height: 3}}},
		Data:          &types.Data{Metadata: &types.Metadata{ChainID: "c", Height: 3}},
		Source:        common.SourceP2P,
		DaHeightHints: [2]uint64{200, 200}, // Both hints are above 150
	}

	// Set the store height to 2 so the event can be processed
	batch, err = st.NewBatch(context.Background())
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(2))
	require.NoError(t, batch.Commit())

	s.processHeightEvent(t.Context(), &evt2)

	// Verify that the priority height WAS queued since it's above daRetrieverHeight
	priorityHeight = s.daFollower.(*daFollower).popPriorityHeight()
	assert.Equal(t, uint64(200), priorityHeight, "should queue DA hint that is above current daRetrieverHeight")
}

// TestProcessHeightEvent_ExecutionFailure_DoesNotReschedule verifies that when
// ExecuteTxs fails after all retries (execution client unavailable), the event
// is NOT re-queued as pending.
func TestProcessHeightEvent_ExecutionFailure_DoesNotReschedule(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ds := dssync.MutexWrap(datastore.NewMapDatastore())
		st := store.New(ds)

		cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
		require.NoError(t, err)

		addr, pub, signer := buildSyncTestSigner(t)

		cfg := config.DefaultConfig()
		gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

		mockExec := testmocks.NewMockExecutor(t)
		mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

		// ExecuteTxs fails on all attempts — simulates unavailable execution client
		mockExec.EXPECT().ExecuteTxs(mock.Anything, mock.Anything, uint64(1), mock.Anything, mock.Anything).
			Return([]byte(nil), errors.New("connection refused")).Times(common.MaxRetriesBeforeHalt)

		mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
		mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
		mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
		mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

		errChan := make(chan error, 1)
		s := NewSyncer(
			st,
			mockExec,
			nil,
			cm,
			common.NopMetrics(),
			cfg,
			gen,
			mockHeaderStore,
			mockDataStore,
			zerolog.Nop(),
			common.DefaultBlockOptions(),
			errChan,
			nil,
		)

		require.NoError(t, s.initializeState())
		s.ctx = t.Context()

		lastState := s.getLastState()
		data := makeData(gen.ChainID, 1, 0)
		_, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, lastState.AppHash, data, nil)

		evt := common.DAHeightEvent{Header: hdr, Data: data, DaHeight: 1}
		s.processHeightEvent(t.Context(), &evt)

		// A critical error must have been sent
		select {
		case critErr := <-errChan:
			assert.ErrorContains(t, critErr, "failed to execute transactions")
		default:
			t.Fatal("expected a critical error on errorCh, got none")
		}

		// The hasCriticalError flag must be set
		assert.True(t, s.hasCriticalError.Load(), "hasCriticalError should be true after execution failure")

		// The event must NOT have been re-queued as pending
		pending := cm.GetNextPendingEvent(1)
		assert.Nil(t, pending, "event should not be re-queued as pending after execution client failure")
	})
}

// TestSyncer_Stop_SkipsDrainOnCriticalError verifies that Syncer.Stop skips the
// drain loop when hasCriticalError is set.
func TestSyncer_Stop_SkipsDrainOnCriticalError(t *testing.T) {
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	st := store.New(ds)

	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)

	addr, pub, signer := buildSyncTestSigner(t)

	cfg := config.DefaultConfig()
	gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

	// The executor should NOT be called during Stop's drain.
	mockExec := testmocks.NewMockExecutor(t)
	mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()

	mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
	mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
	mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
	mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

	errChan := make(chan error, 1)
	s := NewSyncer(
		st,
		mockExec,
		nil,
		cm,
		common.NopMetrics(),
		cfg,
		gen,
		mockHeaderStore,
		mockDataStore,
		zerolog.Nop(),
		common.DefaultBlockOptions(),
		errChan,
		nil,
	)

	require.NoError(t, s.initializeState())

	ctx, cancel := context.WithCancel(t.Context())
	s.ctx = ctx
	s.cancel = cancel
	s.lifecycleState = syncerLifecycleStarted

	// Enqueue events into heightInCh that would trigger ExecuteTxs if drained
	lastState := s.getLastState()
	data := makeData(gen.ChainID, 1, 0)
	_, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, lastState.AppHash, data, nil)
	evt := common.DAHeightEvent{Header: hdr, Data: data, DaHeight: 1}
	s.heightInCh <- evt

	// Simulate that a critical error was already reported (execution client died)
	s.hasCriticalError.Store(true)

	// Start a no-op goroutine tracked by the WaitGroup so Stop() doesn't block on wg.Wait()
	s.wg.Go(func() {})

	// Stop must complete quickly — no drain, no ExecuteTxs calls
	done := make(chan struct{})
	go func() {
		_ = s.Stop(t.Context())
		close(done)
	}()

	select {
	case <-done:
		// Stop returned promptly — drain was correctly skipped
	case <-time.After(2 * time.Second):
		t.Fatal("Syncer.Stop() hung — drain was not skipped despite critical error")
	}

	// ExecuteTxs should never have been called (only InitChain was expected)
	mockExec.AssertExpectations(t)
}

// TestSyncer_Stop_DrainWorksWithoutCriticalError is a sanity check confirming
// that the drain still runs (including ExecuteTxs) when there is no critical error.
func TestSyncer_Stop_DrainWorksWithoutCriticalError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ds := dssync.MutexWrap(datastore.NewMapDatastore())
		st := store.New(ds)

		cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
		require.NoError(t, err)

		addr, pub, signer := buildSyncTestSigner(t)

		cfg := config.DefaultConfig()
		gen := genesis.Genesis{ChainID: "tchain", InitialHeight: 1, StartTime: time.Now().Add(-time.Second), ProposerAddress: addr}

		mockExec := testmocks.NewMockExecutor(t)
		mockExec.EXPECT().InitChain(mock.Anything, mock.Anything, uint64(1), "tchain").Return([]byte("app0"), nil).Once()
		// The drain must call ExecuteTxs for a height-1 event that has not been processed yet.
		mockExec.EXPECT().ExecuteTxs(mock.Anything, mock.Anything, uint64(1), mock.Anything, mock.Anything).
			Return([]byte("app1"), nil).Once()

		mockHeaderStore := extmocks.NewMockStore[*types.P2PSignedHeader](t)
		mockHeaderStore.EXPECT().Height().Return(uint64(0)).Maybe()
		mockDataStore := extmocks.NewMockStore[*types.P2PData](t)
		mockDataStore.EXPECT().Height().Return(uint64(0)).Maybe()

		errChan := make(chan error, 1)
		s := NewSyncer(
			st,
			mockExec,
			nil,
			cm,
			common.NopMetrics(),
			cfg,
			gen,
			mockHeaderStore,
			mockDataStore,
			zerolog.Nop(),
			common.DefaultBlockOptions(),
			errChan,
			nil,
		)

		require.NoError(t, s.initializeState())

		ctx, cancel := context.WithCancel(t.Context())
		s.ctx = ctx
		s.cancel = cancel
		s.lifecycleState = syncerLifecycleStarted

		// Build a valid height-1 event that will actually reach ExecuteTxs during drain
		lastState := s.getLastState()
		data := makeData(gen.ChainID, 1, 0)
		_, hdr := makeSignedHeaderBytes(t, gen.ChainID, 1, addr, pub, signer, lastState.AppHash, data, nil)
		evt := common.DAHeightEvent{Header: hdr, Data: data, DaHeight: 1}
		s.heightInCh <- evt

		// hasCriticalError is false (default) — drain should process events including ExecuteTxs
		s.wg.Go(func() {})

		_ = s.Stop(t.Context())

		// Verify ExecuteTxs was actually called during drain
		mockExec.AssertExpectations(t)
	})
}
