package submitting

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
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
	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/rpc/server"
	"github.com/evstack/ev-node/pkg/signer"
	"github.com/evstack/ev-node/pkg/store"
	testmocks "github.com/evstack/ev-node/test/mocks"
	"github.com/evstack/ev-node/types"
)

func TestSubmitter_setFinalWithRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupMock      func(*testmocks.MockExecutor)
		expectSuccess  bool
		expectAttempts int
		expectError    string
	}{
		{
			name: "success on first attempt",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("SetFinal", mock.Anything, uint64(100)).Return(nil).Once()
			},
			expectSuccess:  true,
			expectAttempts: 1,
		},
		{
			name: "success on second attempt",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("SetFinal", mock.Anything, uint64(100)).Return(errors.New("temporary failure")).Once()
				exec.On("SetFinal", mock.Anything, uint64(100)).Return(nil).Once()
			},
			expectSuccess:  true,
			expectAttempts: 2,
		},
		{
			name: "success on third attempt",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("SetFinal", mock.Anything, uint64(100)).Return(errors.New("temporary failure")).Times(2)
				exec.On("SetFinal", mock.Anything, uint64(100)).Return(nil).Once()
			},
			expectSuccess:  true,
			expectAttempts: 3,
		},
		{
			name: "failure after max retries",
			setupMock: func(exec *testmocks.MockExecutor) {
				exec.On("SetFinal", mock.Anything, uint64(100)).Return(errors.New("persistent failure")).Times(common.MaxRetriesBeforeHalt)
			},
			expectSuccess: false,
			expectError:   "failed to set final height after 3 attempts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				exec := testmocks.NewMockExecutor(t)
				tt.setupMock(exec)

				s := &Submitter{
					exec:   exec,
					ctx:    ctx,
					logger: zerolog.Nop(),
				}

				err := s.setFinalWithRetry(100)

				if tt.expectSuccess {
					require.NoError(t, err)
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

func TestSubmitter_IsHeightDAIncluded(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	cm, st := newTestCacheAndStore(t)
	batch, err := st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SetHeight(5))
	require.NoError(t, batch.Commit())

	daIncludedHeight := &atomic.Uint64{}
	daIncludedHeight.Store(2) // heights 1 and 2 are already DA included and cache cleared

	s := &Submitter{store: st, cache: cm, logger: zerolog.Nop(), daIncludedHeight: daIncludedHeight}
	s.ctx = ctx

	h1, d1 := newHeaderAndData("chain", 3, true)
	h2, d2 := newHeaderAndData("chain", 4, true)
	_, d3 := newHeaderAndData("chain", 2, true) // already DA included, cache was cleared

	cm.SetHeaderDAIncluded(h1.Hash().String(), 100, 3)
	cm.SetDataDAIncluded(d1.DACommitment().String(), 100, 3)
	cm.SetHeaderDAIncluded(h2.Hash().String(), 101, 4)
	// no data for h2
	// no cache entries for h3/d3 - they were cleared after DA inclusion processing

	specs := map[string]struct {
		height uint64
		data   *types.Data
		exp    bool
		expErr bool
	}{
		"below store height and cached":          {height: 3, data: d1, exp: true},
		"above store height":                     {height: 6, data: d2, exp: false},
		"data missing":                           {height: 4, data: d2, exp: false},
		"at daIncludedHeight - cache cleared":    {height: 2, data: d3, exp: true},
		"below daIncludedHeight - cache cleared": {height: 1, data: d3, exp: true},
	}

	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			included, err := s.IsHeightDAIncluded(spec.height, spec.data)
			if spec.expErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, spec.exp, included)
		})
	}
}

func TestSubmitter_setSequencerHeightToDAHeight(t *testing.T) {
	ctx := t.Context()
	cm, _ := newTestCacheAndStore(t)

	// Use a mock store to validate metadata writes
	mockStore := testmocks.NewMockStore(t)

	cfg := config.DefaultConfig()
	cfg.DA.Namespace = "test-ns"
	cfg.DA.DataNamespace = "test-data-ns"
	metrics := common.NopMetrics()
	daClient := testmocks.NewMockClient(t)
	// Namespace getters may be called implicitly; allow optional returns
	daClient.On("GetHeaderNamespace").Return([]byte(cfg.DA.Namespace)).Maybe()
	daClient.On("GetDataNamespace").Return([]byte(cfg.DA.DataNamespace)).Maybe()
	daClient.On("GetForcedInclusionNamespace").Return([]byte(nil)).Maybe()
	daClient.On("HasForcedInclusionNamespace").Return(false).Maybe()
	daSub := NewDASubmitter(daClient, cfg, genesis.Genesis{}, common.BlockOptions{}, metrics, zerolog.Nop(), nil, nil)
	s := NewSubmitter(mockStore, nil, cm, metrics, cfg, genesis.Genesis{}, daSub, nil, nil, zerolog.Nop(), nil)
	s.ctx = ctx

	h, d := newHeaderAndData("chain", 1, true)

	// set DA included heights in cache
	cm.SetHeaderDAIncluded(h.Hash().String(), 100, 1)
	cm.SetDataDAIncluded(d.DACommitment().String(), 90, 1)

	headerKey := store.GetHeightToDAHeightHeaderKey(1)
	dataKey := store.GetHeightToDAHeightDataKey(1)

	hBz := make([]byte, 8)
	binary.LittleEndian.PutUint64(hBz, 100)
	dBz := make([]byte, 8)
	binary.LittleEndian.PutUint64(dBz, 90)
	gBz := make([]byte, 8)
	binary.LittleEndian.PutUint64(gBz, 90) // min(header=100, data=90)

	mockStore.On("SetMetadata", mock.Anything, headerKey, hBz).Return(nil).Once()
	mockStore.On("SetMetadata", mock.Anything, dataKey, dBz).Return(nil).Once()
	mockStore.On("SetMetadata", mock.Anything, store.GenesisDAHeightKey, gBz).Return(nil).Once()

	require.NoError(t, s.setNodeHeightToDAHeight(ctx, 1, d, true))
}

func TestSubmitter_setNodeHeightToDAHeight_Errors(t *testing.T) {
	ctx := t.Context()
	cm, st := newTestCacheAndStore(t)

	s := &Submitter{store: st, cache: cm, logger: zerolog.Nop()}

	h, d := newHeaderAndData("chain", 1, true)

	// No cache entries -> expect error on missing header
	_, ok := cm.GetHeaderDAIncludedByHash(h.Hash().String())
	assert.False(t, ok)
	assert.Error(t, s.setNodeHeightToDAHeight(ctx, 1, d, false))

	// Add header, missing data
	cm.SetHeaderDAIncluded(h.Hash().String(), 10, 1)
	assert.Error(t, s.setNodeHeightToDAHeight(ctx, 1, d, false))
}

func TestSubmitter_initializeDAIncludedHeight(t *testing.T) {
	ctx := t.Context()
	_, st := newTestCacheAndStore(t)

	// write DAIncludedHeightKey
	bz := make([]byte, 8)
	binary.LittleEndian.PutUint64(bz, 7)
	require.NoError(t, st.SetMetadata(ctx, store.DAIncludedHeightKey, bz))

	s := &Submitter{store: st, daIncludedHeight: &atomic.Uint64{}, logger: zerolog.Nop()}
	require.NoError(t, s.initializeDAIncludedHeight(ctx))
	assert.Equal(t, uint64(7), s.GetDAIncludedHeight())
}

func TestSubmitter_processDAInclusionLoop_advances(t *testing.T) {
	ctx := t.Context()

	// Clean up any existing visualization server
	defer server.SetDAVisualizationServer(nil)
	server.SetDAVisualizationServer(nil)

	cm, st := newTestCacheAndStore(t)

	// small block time to tick quickly
	cfg := config.DefaultConfig()
	cfg.DA.BlockTime.Duration = 5 * time.Millisecond
	cfg.RPC.EnableDAVisualization = false // Ensure visualization is disabled
	metrics := common.PrometheusMetrics("test")

	exec := testmocks.NewMockExecutor(t)
	exec.On("SetFinal", mock.Anything, uint64(1)).Return(nil).Once()
	exec.On("SetFinal", mock.Anything, uint64(2)).Return(nil).Once()

	daClient := testmocks.NewMockClient(t)
	daClient.On("GetHeaderNamespace").Return([]byte(cfg.DA.Namespace)).Maybe()
	daClient.On("GetDataNamespace").Return([]byte(cfg.DA.DataNamespace)).Maybe()
	daClient.On("GetForcedInclusionNamespace").Return([]byte(nil)).Maybe()
	daClient.On("HasForcedInclusionNamespace").Return(false).Maybe()
	daSub := NewDASubmitter(daClient, cfg, genesis.Genesis{}, common.BlockOptions{}, metrics, zerolog.Nop(), nil, nil)
	s := NewSubmitter(st, exec, cm, metrics, cfg, genesis.Genesis{}, daSub, nil, nil, zerolog.Nop(), nil)

	// prepare two consecutive blocks in store with DA included in cache
	h1, d1 := newHeaderAndData("chain", 1, true)
	h2, d2 := newHeaderAndData("chain", 2, true)
	require.NotEqual(t, h1.Hash(), h2.Hash())
	require.NotEqual(t, d1.DACommitment(), d2.DACommitment())

	sig := types.Signature([]byte("sig"))

	// Save block 1
	batch1, err := st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch1.SaveBlockData(h1, d1, &sig))
	require.NoError(t, batch1.SetHeight(1))
	require.NoError(t, batch1.Commit())

	// Save block 2
	batch2, err := st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch2.SaveBlockData(h2, d2, &sig))
	require.NoError(t, batch2.SetHeight(2))
	require.NoError(t, batch2.Commit())

	cm.SetHeaderDAIncluded(h1.Hash().String(), 100, 1)
	cm.SetDataDAIncluded(d1.DACommitment().String(), 100, 1)
	cm.SetHeaderDAIncluded(h2.Hash().String(), 101, 2)
	cm.SetDataDAIncluded(d2.DACommitment().String(), 101, 2)

	require.NoError(t, s.initializeDAIncludedHeight(ctx))
	require.Equal(t, uint64(0), s.GetDAIncludedHeight())

	// when
	require.NoError(t, s.Start(ctx))

	require.Eventually(t, func() bool {
		return s.GetDAIncludedHeight() == 2
	}, 1*time.Second, 10*time.Millisecond)
	require.NoError(t, s.Stop())

	// verify metadata mapping persisted in store
	for i := range 2 {
		hBz, err := st.GetMetadata(ctx, store.GetHeightToDAHeightHeaderKey(uint64(i+1)))
		require.NoError(t, err)
		assert.Equal(t, uint64(100+i), binary.LittleEndian.Uint64(hBz))

		dBz, err := st.GetMetadata(ctx, store.GetHeightToDAHeightDataKey(uint64(i+1)))
		require.NoError(t, err)
		assert.Equal(t, uint64(100+i), binary.LittleEndian.Uint64(dBz))
	}

	localHeightBz, err := s.store.GetMetadata(ctx, store.DAIncludedHeightKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), binary.LittleEndian.Uint64(localHeightBz))

}

// TestSubmitter_processDAInclusionLoop_setFinalFailureDoesNotHalt ensures that
// exhausting SetFinal retries holds the DA included height and retries on the
// next tick instead of reporting a critical error that stops the node.
func TestSubmitter_processDAInclusionLoop_setFinalFailureDoesNotHalt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cm, st := newTestCacheAndStore(t)

		cfg := config.DefaultConfig()
		cfg.DA.BlockTime.Duration = 5 * time.Millisecond

		exec := testmocks.NewMockExecutor(t)
		exec.On("SetFinal", mock.Anything, uint64(1)).Return(errors.New("block pruned")).Times(common.MaxRetriesBeforeHalt)
		exec.On("SetFinal", mock.Anything, uint64(1)).Return(nil).Once()
		exec.On("SetFinal", mock.Anything, uint64(2)).Return(nil).Once()

		errCh := make(chan error, 1)
		ctx, cancel := context.WithCancel(t.Context())
		s := &Submitter{
			store:            st,
			exec:             exec,
			cache:            cm,
			metrics:          common.NopMetrics(),
			config:           cfg,
			daIncludedHeight: &atomic.Uint64{},
			errorCh:          errCh,
			logger:           zerolog.Nop(),
			ctx:              ctx,
		}

		sig := types.Signature([]byte("sig"))
		for height := uint64(1); height <= 2; height++ {
			h, d := newHeaderAndData("chain", height, true)
			batch, err := st.NewBatch(ctx)
			require.NoError(t, err)
			require.NoError(t, batch.SaveBlockData(h, d, &sig))
			require.NoError(t, batch.SetHeight(height))
			require.NoError(t, batch.Commit())
			cm.SetHeaderDAIncluded(h.Hash().String(), 100+height, height)
			cm.SetDataDAIncluded(d.DACommitment().String(), 100+height, height)
		}

		done := make(chan struct{})
		go func() {
			s.processDAInclusionLoop()
			close(done)
		}()

		require.Eventually(t, func() bool {
			return s.GetDAIncludedHeight() == 2
		}, time.Minute, cfg.DA.BlockTime.Duration)

		cancel()
		<-done

		select {
		case err := <-errCh:
			t.Fatalf("unexpected critical error: %v", err)
		default:
		}
	})
}

// helper to create a minimal header and data for tests
func newHeaderAndData(chainID string, height uint64, nonEmpty bool) (*types.SignedHeader, *types.Data) {
	now := time.Now()
	h := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{ChainID: chainID, Height: height, Time: uint64(now.UnixNano())}, ProposerAddress: []byte{1}}}
	d := &types.Data{Metadata: &types.Metadata{ChainID: chainID, Height: height, Time: uint64(now.UnixNano())}}
	if nonEmpty {
		d.Txs = types.Txs{types.Tx(fmt.Sprintf("any-unique-tx-%d-%d", height, now.UnixNano()))}
	}
	return h, d
}

func newTestCacheAndStore(t *testing.T) (cache.Manager, store.Store) {
	st := store.New(dssync.MutexWrap(datastore.NewMapDatastore()))
	cm, err := cache.NewManager(config.DefaultConfig(), st, zerolog.Nop())
	require.NoError(t, err)
	return cm, st
}

// TestSubmitter_daSubmissionLoop ensures that when there are pending headers/data,
// the submitter invokes the DA submitter methods periodically.
func TestSubmitter_daSubmissionLoop(t *testing.T) {
	ctx := t.Context()

	cm, st := newTestCacheAndStore(t)

	// Set a small block time so the ticker fires quickly
	cfg := config.DefaultConfig()
	cfg.DA.BlockTime.Duration = 5 * time.Millisecond
	// Use immediate batching strategy so submissions happen right away
	cfg.DA.BatchingStrategy = "immediate"
	metrics := common.NopMetrics()

	// Prepare fake DA submitter capturing calls
	fakeDA := &fakeDASubmitter{
		chHdr:  make(chan struct{}, 1),
		chData: make(chan struct{}, 1),
	}

	// Provide a non-nil executor; it won't be used because DA inclusion won't advance
	exec := testmocks.NewMockExecutor(t)

	batchingStrategy, err := NewBatchingStrategy(cfg.DA)
	require.NoError(t, err)

	s := &Submitter{
		store:            st,
		exec:             exec,
		cache:            cm,
		metrics:          metrics,
		config:           cfg,
		genesis:          genesis.Genesis{},
		daSubmitter:      fakeDA,
		signer:           &fakeSigner{},
		daIncludedHeight: &atomic.Uint64{},
		batchingStrategy: batchingStrategy,
		logger:           zerolog.Nop(),
	}

	// Set last submit times far in past so strategy allows submission
	pastTime := time.Now().Add(-time.Hour).UnixNano()
	s.lastHeaderSubmit.Store(pastTime)
	s.lastDataSubmit.Store(pastTime)

	// Make there be pending headers and data by setting store height > last submitted
	h1, d1 := newHeaderAndData("test-chain", 1, true)
	h2, d2 := newHeaderAndData("test-chain", 2, true)

	// Store the blocks
	batch, err := st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &types.Signature{}))
	require.NoError(t, batch.SaveBlockData(h2, d2, &types.Signature{}))
	require.NoError(t, batch.SetHeight(2))
	require.NoError(t, batch.Commit())

	// Start and wait for calls
	require.NoError(t, s.Start(ctx))
	t.Cleanup(func() { _ = s.Stop() })

	// Both should be invoked eventually
	require.Eventually(t, func() bool {
		select {
		case <-fakeDA.chHdr:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	require.Eventually(t, func() bool {
		select {
		case <-fakeDA.chData:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
}

// fakeDASubmitter is a lightweight test double for the DASubmitter used by the loop.
type fakeDASubmitter struct {
	chHdr  chan struct{}
	chData chan struct{}
}

func (f *fakeDASubmitter) SubmitHeaders(ctx context.Context, _ []*types.SignedHeader, _ [][]byte, _ cache.Manager, _ signer.Signer) error {
	select {
	case f.chHdr <- struct{}{}:
	default:
	}
	return nil
}

func (f *fakeDASubmitter) SubmitData(ctx context.Context, _ []*types.SignedData, _ [][]byte, _ cache.Manager, _ signer.Signer) error {
	select {
	case f.chData <- struct{}{}:
	default:
	}
	return nil
}

// fakeSigner implements signer.Signer with deterministic behavior for tests.
type fakeSigner struct{}

func (f *fakeSigner) Sign(_ context.Context, msg []byte) ([]byte, error) {
	return append([]byte(nil), msg...), nil
}
func (f *fakeSigner) GetPublic() (crypto.PubKey, error) { return nil, nil }
func (f *fakeSigner) GetAddress() ([]byte, error)       { return []byte("addr"), nil }

func TestSubmitter_CacheClearedOnHeightInclusion(t *testing.T) {
	ctx := t.Context()

	cm, st := newTestCacheAndStore(t)

	cfg := config.DefaultConfig()
	cfg.DA.BlockTime.Duration = 5 * time.Millisecond
	metrics := common.NopMetrics()

	exec := testmocks.NewMockExecutor(t)
	exec.On("SetFinal", mock.Anything, uint64(1)).Return(nil).Once()
	exec.On("SetFinal", mock.Anything, uint64(2)).Return(nil).Once()

	daClient := testmocks.NewMockClient(t)
	daClient.On("GetHeaderNamespace").Return([]byte(cfg.DA.Namespace)).Maybe()
	daClient.On("GetDataNamespace").Return([]byte(cfg.DA.DataNamespace)).Maybe()
	daClient.On("GetForcedInclusionNamespace").Return([]byte(nil)).Maybe()
	daClient.On("HasForcedInclusionNamespace").Return(false).Maybe()
	daSub := NewDASubmitter(daClient, cfg, genesis.Genesis{}, common.BlockOptions{}, metrics, zerolog.Nop(), nil, nil)
	s := NewSubmitter(st, exec, cm, metrics, cfg, genesis.Genesis{}, daSub, nil, nil, zerolog.Nop(), nil)

	// Create test blocks
	h1, d1 := newHeaderAndData("chain", 1, true)
	h2, d2 := newHeaderAndData("chain", 2, true)
	h3, d3 := newHeaderAndData("chain", 3, true)

	sig := types.Signature([]byte("sig"))

	// Save blocks to store
	blocks := []struct {
		header *types.SignedHeader
		data   *types.Data
		height uint64
	}{
		{h1, d1, 1},
		{h2, d2, 2},
		{h3, d3, 3},
	}

	for _, block := range blocks {
		batch, err := st.NewBatch(ctx)
		require.NoError(t, err)
		require.NoError(t, batch.SaveBlockData(block.header, block.data, &sig))
		require.NoError(t, batch.SetHeight(block.height))
		require.NoError(t, batch.Commit())
	}

	// Set up cache with headers and data seen for all heights
	cm.SetHeaderSeen(h1.Hash().String(), 1)
	cm.SetDataSeen(d1.DACommitment().String(), 1)
	cm.SetHeaderSeen(h2.Hash().String(), 2)
	cm.SetDataSeen(d2.DACommitment().String(), 2)
	cm.SetHeaderSeen(h3.Hash().String(), 3)
	cm.SetDataSeen(d3.DACommitment().String(), 3)

	// Verify items are seen in cache before processing
	assert.True(t, cm.IsHeaderSeen(h1.Hash().String()))
	assert.True(t, cm.IsDataSeen(d1.DACommitment().String()))
	assert.True(t, cm.IsHeaderSeen(h2.Hash().String()))
	assert.True(t, cm.IsDataSeen(d2.DACommitment().String()))
	assert.True(t, cm.IsHeaderSeen(h3.Hash().String()))
	assert.True(t, cm.IsDataSeen(d3.DACommitment().String()))

	// Set DA inclusion for heights 1 and 2 only (height 3 will remain unprocessed)
	cm.SetHeaderDAIncluded(h1.Hash().String(), 100, 1)
	cm.SetDataDAIncluded(d1.DACommitment().String(), 100, 1)
	cm.SetHeaderDAIncluded(h2.Hash().String(), 101, 2)
	cm.SetDataDAIncluded(d2.DACommitment().String(), 101, 2)

	require.NoError(t, s.initializeDAIncludedHeight(ctx))
	require.Equal(t, uint64(0), s.GetDAIncludedHeight())

	// Start submitter to process DA inclusions
	require.NoError(t, s.Start(ctx))

	// Wait for heights 1 and 2 to be processed
	require.Eventually(t, func() bool {
		return s.GetDAIncludedHeight() == 2
	}, 1*time.Second, 10*time.Millisecond)

	require.NoError(t, s.Stop())

	// Verify cache is cleared for processed heights (1 and 2)
	assert.False(t, cm.IsHeaderSeen(h1.Hash().String()), "height 1 header should be cleared from cache")
	assert.False(t, cm.IsDataSeen(d1.DACommitment().String()), "height 1 data should be cleared from cache")
	assert.False(t, cm.IsHeaderSeen(h2.Hash().String()), "height 2 header should be cleared from cache")
	assert.False(t, cm.IsDataSeen(d2.DACommitment().String()), "height 2 data should be cleared from cache")

	// Verify DA inclusion status is removed for processed heights (cleaned up after finalization)
	_, h1DAIncluded := cm.GetHeaderDAIncludedByHash(h1.Hash().String())
	_, d1DAIncluded := cm.GetDataDAIncludedByHash(d1.DACommitment().String())
	_, h2DAIncluded := cm.GetHeaderDAIncludedByHash(h2.Hash().String())
	_, d2DAIncluded := cm.GetDataDAIncludedByHash(d2.DACommitment().String())
	assert.False(t, h1DAIncluded, "height 1 header DA inclusion status should be removed after finalization")
	assert.False(t, d1DAIncluded, "height 1 data DA inclusion status should be removed after finalization")
	assert.False(t, h2DAIncluded, "height 2 header DA inclusion status should be removed after finalization")
	assert.False(t, d2DAIncluded, "height 2 data DA inclusion status should be removed after finalization")

	// Verify unprocessed height 3 cache remains intact
	assert.True(t, cm.IsHeaderSeen(h3.Hash().String()), "height 3 header should remain in cache")
	assert.True(t, cm.IsDataSeen(d3.DACommitment().String()), "height 3 data should remain in cache")

	// Verify height 3 has no DA inclusion status since it wasn't processed
	_, h3DAIncluded := cm.GetHeaderDAIncludedByHash(h3.Hash().String())
	_, d3DAIncluded := cm.GetDataDAIncludedByHash(d3.DACommitment().String())
	assert.False(t, h3DAIncluded, "height 3 header should not have DA inclusion status")
	assert.False(t, d3DAIncluded, "height 3 data should not have DA inclusion status")
}

// TestSubmitter_IsHeightDAIncluded_AfterRestart proves that IsHeightDAIncluded
// returns true for in-flight blocks immediately after a restart, before the DA
// retriever has had a chance to re-fire SetHeaderDAIncluded with the real
// content hash.
//
// Scenario:
//  1. Node runs normally: heights 1–3 are DA-included, height 3 is in-flight
//     (submitted to DA but not yet finalized). SetHeaderDAIncluded writes both
//     the real-hash entry AND the snapshot key.
//  2. Node restarts: a fresh Manager is constructed on the same store.
//     RestoreFromStore reads the snapshot and installs placeholder entries
//     keyed by height (not by content hash).
//  3. processDAInclusionLoop calls IsHeightDAIncluded(3, h3, d3) BEFORE the
//     DA retriever has re-fired SetHeaderDAIncluded("realHash3", …).
//     Without the height-based fallback this would return false and stall.
//  4. With the fallback it finds the placeholder, returns true, and the loop
//     can advance.
func TestSubmitter_IsHeightDAIncluded_AfterRestart(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// ── Step 1: pre-restart state ─────────────────────────────────────────────
	// Build a store with three blocks and a cache that has height 3 in-flight.
	ds1 := dssync.MutexWrap(datastore.NewMapDatastore())
	st1 := store.New(ds1)
	cm1, err := cache.NewManager(config.DefaultConfig(), st1, zerolog.Nop())
	require.NoError(t, err)

	h1, d1 := newHeaderAndData("chain", 1, true)
	h2, d2 := newHeaderAndData("chain", 2, true)
	h3, d3 := newHeaderAndData("chain", 3, true)

	sig := types.Signature([]byte("sig"))
	for _, blk := range []struct {
		h   *types.SignedHeader
		d   *types.Data
		hgt uint64
	}{
		{h1, d1, 1},
		{h2, d2, 2},
		{h3, d3, 3},
	} {
		batch, err := st1.NewBatch(ctx)
		require.NoError(t, err)
		require.NoError(t, batch.SaveBlockData(blk.h, blk.d, &sig))
		require.NoError(t, batch.SetHeight(blk.hgt))
		require.NoError(t, batch.Commit())
	}

	// Heights 1 and 2 are fully finalized; height 3 is in-flight.
	cm1.SetHeaderDAIncluded(h1.Hash().String(), 10, 1)
	cm1.SetDataDAIncluded(d1.DACommitment().String(), 10, 1)
	cm1.SetHeaderDAIncluded(h2.Hash().String(), 11, 2)
	cm1.SetDataDAIncluded(d2.DACommitment().String(), 11, 2)
	cm1.SetHeaderDAIncluded(h3.Hash().String(), 12, 3) // in-flight
	cm1.SetDataDAIncluded(d3.DACommitment().String(), 12, 3)

	// Persist the snapshot (already done by setDAIncluded on every mutation,
	// but call SaveToStore explicitly to be clear about what survives restart).
	require.NoError(t, cm1.SaveToStore())

	// Persist DAIncludedHeight = 2 (heights 1 & 2 finalized, 3 is in-flight).
	daIncBz := make([]byte, 8)
	binary.LittleEndian.PutUint64(daIncBz, 2)
	require.NoError(t, st1.SetMetadata(ctx, store.DAIncludedHeightKey, daIncBz))

	// ── Step 2: simulate restart ──────────────────────────────────────────────
	// Build a fresh Manager on the SAME underlying datastore. This exercises
	// the RestoreFromStore → snapshot-decode path. The DA retriever has NOT
	// yet re-fired SetHeaderDAIncluded with the real hashes.
	cm2, err := cache.NewManager(config.DefaultConfig(), st1, zerolog.Nop())
	require.NoError(t, err)

	daIncludedHeight := &atomic.Uint64{}
	daIncludedHeight.Store(2) // matches what was persisted above

	s := &Submitter{
		store:            st1,
		cache:            cm2,
		logger:           zerolog.Nop(),
		daIncludedHeight: daIncludedHeight,
		ctx:              ctx,
	}

	// ── Step 3: check IsHeightDAIncluded BEFORE DA retriever re-fires ─────────
	// Height 3 is in-flight: above daIncludedHeight (2) so we can't short-circuit.
	// The real hashes are NOT in cm2 yet — only the snapshot placeholders are.
	_, realHeaderFound := cm2.GetHeaderDAIncludedByHash(h3.Hash().String())
	assert.False(t, realHeaderFound, "real hash must not be present before DA retriever re-fires")

	included, err := s.IsHeightDAIncluded(3, d3)
	require.NoError(t, err)
	assert.True(t, included,
		"IsHeightDAIncluded must return true for in-flight height using snapshot placeholder, "+
			"before DA retriever re-fires SetHeaderDAIncluded")

	// ── Step 4: after DA retriever re-fires, real hash lookup also works ──────
	cm2.SetHeaderDAIncluded(h3.Hash().String(), 12, 3)
	cm2.SetDataDAIncluded(d3.DACommitment().String(), 12, 3)

	_, realHeaderFound = cm2.GetHeaderDAIncludedByHash(h3.Hash().String())
	assert.True(t, realHeaderFound, "real hash must be present after DA retriever re-fires")

	included, err = s.IsHeightDAIncluded(3, d3)
	require.NoError(t, err)
	assert.True(t, included, "IsHeightDAIncluded must still return true after real hash is written")
}

// TestSubmitter_setNodeHeightToDAHeight_AfterRestart proves that
// setNodeHeightToDAHeight succeeds for an in-flight block immediately after a
// restart, before the DA retriever has re-fired SetHeaderDAIncluded with the
// real content hash.
//
// The bug this guards against:
//   - Snapshot encodes [blockHeight → daHeight] pairs, not real content hashes.
//   - On restart, RestoreFromStore installs placeholder keys (indexed by height).
//   - setNodeHeightToDAHeight uses GetHeaderDAIncludedByHeight which resolves
//     the placeholder entry directly, so it succeeds before the DA retriever
//     re-fires SetHeaderDAIncluded with the real content hash.
//     metadata and DAIncludedHeight never advances.
//   - With GetHeaderDAIncludedByHeight / GetDataDAIncludedByHeight the lookup
//     succeeds via the placeholder and the metadata is written correctly.
func TestSubmitter_setNodeHeightToDAHeight_AfterRestart(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// ── Step 1: pre-restart — write snapshot with height 3 in-flight ─────────
	ds1 := dssync.MutexWrap(datastore.NewMapDatastore())
	st1 := store.New(ds1)
	cm1, err := cache.NewManager(config.DefaultConfig(), st1, zerolog.Nop())
	require.NoError(t, err)

	h3, d3 := newHeaderAndData("chain", 3, true)

	sig := types.Signature([]byte("sig"))
	batch, err := st1.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h3, d3, &sig))
	require.NoError(t, batch.SetHeight(3))
	require.NoError(t, batch.Commit())

	// Mark height 3 as DA-included in the pre-restart cache, then flush (shutdown).
	cm1.SetHeaderDAIncluded(h3.Hash().String(), 12, 3)
	cm1.SetDataDAIncluded(d3.DACommitment().String(), 12, 3)
	require.NoError(t, cm1.SaveToStore())

	// Persist DAIncludedHeight = 2 (height 3 is in-flight, not yet finalized).
	daIncBz := make([]byte, 8)
	binary.LittleEndian.PutUint64(daIncBz, 2)
	require.NoError(t, st1.SetMetadata(ctx, store.DAIncludedHeightKey, daIncBz))

	// ── Step 2: simulate restart — fresh Manager, no DA retriever re-fire ─────
	cm2, err := cache.NewManager(config.DefaultConfig(), st1, zerolog.Nop())
	require.NoError(t, err)

	// Confirm the real content hashes are NOT present after restore.
	_, realHdrFound := cm2.GetHeaderDAIncludedByHash(h3.Hash().String())
	_, realDataFound := cm2.GetDataDAIncludedByHash(d3.DACommitment().String())
	require.False(t, realHdrFound, "real header hash must not be in cache before DA retriever re-fires")
	require.False(t, realDataFound, "real data hash must not be in cache before DA retriever re-fires")

	daIncludedHeight := &atomic.Uint64{}
	daIncludedHeight.Store(2)

	s := &Submitter{
		store:            st1,
		cache:            cm2,
		logger:           zerolog.Nop(),
		daIncludedHeight: daIncludedHeight,
		ctx:              ctx,
	}

	// ── Step 3: call setNodeHeightToDAHeight — must succeed via fallback ──────
	// Before this fix, GetHeaderDAIncludedByHash(realHash) would miss and the function
	// would return an error, stalling processDAInclusionLoop.
	err = s.setNodeHeightToDAHeight(ctx, 3, d3, false)
	require.NoError(t, err,
		"setNodeHeightToDAHeight must succeed via height-based lookup before DA retriever re-fires")

	// ── Step 4: verify the HeightToDAHeight metadata was written correctly ─────
	headerDABz, err := st1.GetMetadata(ctx, store.GetHeightToDAHeightHeaderKey(3))
	require.NoError(t, err)
	require.Len(t, headerDABz, 8)
	assert.Equal(t, uint64(12), binary.LittleEndian.Uint64(headerDABz),
		"header HeightToDAHeight metadata must reflect the DA height from the snapshot")

	dataDABz, err := st1.GetMetadata(ctx, store.GetHeightToDAHeightDataKey(3))
	require.NoError(t, err)
	require.Len(t, dataDABz, 8)
	assert.Equal(t, uint64(12), binary.LittleEndian.Uint64(dataDABz),
		"data HeightToDAHeight metadata must reflect the DA height from the snapshot")

	// ── Step 5: after DA retriever re-fires, the real-hash path still works ───
	cm2.SetHeaderDAIncluded(h3.Hash().String(), 12, 3)
	cm2.SetDataDAIncluded(d3.DACommitment().String(), 12, 3)

	err = s.setNodeHeightToDAHeight(ctx, 3, d3, false)
	require.NoError(t, err, "setNodeHeightToDAHeight must still work once real hashes are populated")
}
