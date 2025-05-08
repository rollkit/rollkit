package based_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	datastore "github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/node"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	remote_signer "github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/sequencers/based"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// MockDAAddress is the address used by the mock gRPC service
	MockDAAddress = "grpc://localhost:7990"

	// MockDANamespace is a sample namespace used by the mock DA client
	MockDANamespace = "00000000000000000000000000000000000000000000000000deadbeef"

	// MockBasedDANamespace is a sample namespace used by the mock based DA client
	MockBasedDANamespace = "00000000000000000000000000000000000000000000000000deadbeaf"
)

func newTestSequencer(t *testing.T) *based.Sequencer {
	dummyDA := coreda.NewDummyDA(100_000_000, 1.0, 1.5)
	store := ds.NewMapDatastore()
	seq, err := based.NewSequencer(log.NewNopLogger(), dummyDA, []byte("rollup1"), 0, 2, store)
	assert.NoError(t, err)
	return seq
}

func TestSequencer_SubmitRollupBatchTxs_Valid(t *testing.T) {
	sequencer := newTestSequencer(t)

	batch := &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}
	resp, err := sequencer.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: []byte("rollup1"),
		Batch:    batch,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestSequencer_SubmitRollupBatchTxs_InvalidRollup(t *testing.T) {
	sequencer := newTestSequencer(t)

	batch := &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}
	resp, err := sequencer.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: []byte("invalid"),
		Batch:    batch,
	})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSequencer_GetNextBatch_OnlyPendingQueue(t *testing.T) {
	sequencer := newTestSequencer(t)

	timestamp := time.Now()
	sequencer.AddToPendingTxs([][]byte{[]byte("tx1")}, [][]byte{[]byte("id1")}, timestamp)

	resp, err := sequencer.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: []byte("rollup1")})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(resp.Batch.Transactions))
	assert.Equal(t, timestamp.Unix(), resp.Timestamp.Unix())
}

func TestSequencer_GetNextBatch_FromDALayer(t *testing.T) {
	sequencer := newTestSequencer(t)
	ctx := context.Background()

	blobs := []coreda.Blob{[]byte("tx2"), []byte("tx3")}
	_, err := sequencer.DA.Submit(ctx, blobs, 1.0, []byte("ns"))
	assert.NoError(t, err)

	resp, err := sequencer.GetNextBatch(ctx, coresequencer.GetNextBatchRequest{
		RollupId: []byte("rollup1"),
	})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Batch.Transactions), 1)
	assert.GreaterOrEqual(t, len(resp.BatchData), 1)
}

func TestSequencer_GetNextBatch_InvalidRollup(t *testing.T) {
	sequencer := newTestSequencer(t)

	resp, err := sequencer.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{
		RollupId: []byte("invalid"),
	})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSequencer_GetNextBatch_ExceedsMaxDrift(t *testing.T) {
	dummyDA := coreda.NewDummyDA(100_000_000, 1.0, 1.5)
	store := ds.NewMapDatastore()
	sequencer, err := based.NewSequencer(log.NewNopLogger(), dummyDA, []byte("rollup1"), 0, 0, store)
	assert.NoError(t, err)

	ctx := context.Background()
	_, err = dummyDA.Submit(ctx, []coreda.Blob{[]byte("tx4")}, 1.0, []byte("ns"))
	assert.NoError(t, err)

	resp, err := sequencer.GetNextBatch(ctx, coresequencer.GetNextBatchRequest{
		RollupId: []byte("rollup1"),
	})
	assert.NoError(t, err)
	if resp != nil {
		assert.LessOrEqual(t, len(resp.Batch.Transactions), 1)
	}
}

func TestSequencer_VerifyBatch_Success(t *testing.T) {
	sequencer := newTestSequencer(t)

	ctx := context.Background()
	ids, err := sequencer.DA.Submit(ctx, []coreda.Blob{[]byte("tx1")}, 1.0, []byte("ns"))
	assert.NoError(t, err)

	resp, err := sequencer.VerifyBatch(ctx, coresequencer.VerifyBatchRequest{
		RollupId:  []byte("rollup1"),
		BatchData: ids,
	})
	assert.NoError(t, err)
	assert.True(t, resp.Status)
}

func TestSequencer_VerifyBatch_InvalidRollup(t *testing.T) {
	sequencer := newTestSequencer(t)

	ctx := context.Background()
	resp, err := sequencer.VerifyBatch(ctx, coresequencer.VerifyBatchRequest{
		RollupId:  []byte("invalid"),
		BatchData: [][]byte{[]byte("someID")},
	})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSequencer_VerifyBatch_InvalidProof(t *testing.T) {
	sequencer := newTestSequencer(t)

	ctx := context.Background()
	resp, err := sequencer.VerifyBatch(ctx, coresequencer.VerifyBatchRequest{
		RollupId:  []byte("rollup1"),
		BatchData: [][]byte{[]byte("invalid")},
	})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func getTestConfig(t *testing.T, n int) rollkitconfig.Config {
	// Use a higher base port to reduce chances of conflicts with system services
	startPort := 40000 + n*100 // Spread port ranges further apart
	return rollkitconfig.Config{
		RootDir: t.TempDir(),
		Node: rollkitconfig.NodeConfig{
			Aggregator:        true,
			BlockTime:         rollkitconfig.DurationWrapper{Duration: 500 * time.Millisecond},
			LazyBlockInterval: rollkitconfig.DurationWrapper{Duration: 5 * time.Second},
		},
		DA: rollkitconfig.DAConfig{
			Address:   MockDAAddress,
			Namespace: MockDANamespace,
		},
		P2P: rollkitconfig.P2PConfig{
			ListenAddress: fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", startPort+n),
		},
		RPC: rollkitconfig.RPCConfig{
			Address: fmt.Sprintf("tcp://127.0.0.1:%d", startPort+n+1),
		},
	}
}

// createTestComponents creates test components for node initialization
func createTestComponents(t *testing.T) (coreexecutor.Executor, coresequencer.Sequencer, coreda.DA, *p2p.Client, datastore.Batching) {
	executor := coreexecutor.NewDummyExecutor()
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)

	// Create genesis and keys for P2P client
	_, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	nodeKey := &key.NodeKey{
		PrivKey: genesisValidatorKey,
		PubKey:  genesisValidatorKey.GetPublic(),
	}
	p2pClient, err := p2p.NewClient(rollkitconfig.DefaultConfig, nodeKey, dssync.MutexWrap(datastore.NewMapDatastore()), log.NewNopLogger(), p2p.NopMetrics())
	require.NoError(t, err)
	require.NotNil(t, p2pClient)
	ds := dssync.MutexWrap(datastore.NewMapDatastore())

	return executor, sequencer, dummyDA, p2pClient, ds
}

func startNodeInBackground(t *testing.T, ctx context.Context, runningWg *sync.WaitGroup, errCh chan error, node *node.FullNode) {
	runningWg.Add(1)
	go func() {
		defer runningWg.Done()
		err := node.Run(ctx)
		select {
		case errCh <- err:
		default:
			t.Logf("Error channel full, discarding error: %v", err)
		}
	}()
}

func TestTwoProposerNodes(t *testing.T) {
	var runningWg sync.WaitGroup
	var errCh = make(chan error, 1)
	require := require.New(t)
	ctx, _ := context.WithCancel(context.Background())

	// Setup two proposer nodes with independent configurations
	config1 := getTestConfig(t, 1)
	config1.Node.Aggregator = true
	config2 := getTestConfig(t, 2)
	config2.Node.Aggregator = true

	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	remoteSigner1, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(err)
	remoteSigner2, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(err)

	nodeKey1, err := key.GenerateNodeKey()
	require.NoError(err)
	nodeKey2, err := key.GenerateNodeKey()
	require.NoError(err)

	executor1, sequencer1, dac1, p2pClient1, ds1 := createTestComponents(t)
	executor2, sequencer2, dac2, p2pClient2, ds2 := createTestComponents(t)

	logger1 := log.NewTestLogger(t)
	logger2 := log.NewTestLogger(t)

	basedDA := coreda.NewDummyDA(100_000, 0, 0)

	sequencer1, err = based.NewSequencer(
		logger1,
		basedDA,
		[]byte("test-chain"),
		0,
		1,
		ds1,
	)
	require.NoError(err)

	sequencer2, err = based.NewSequencer(
		logger2,
		basedDA,
		[]byte("test-chain"),
		0,
		1,
		ds2,
	)
	require.NoError(err)

	node1, err := node.NewNode(
		ctx,
		config1,
		executor1,
		sequencer1,
		dac1,
		remoteSigner1,
		*nodeKey1,
		p2pClient1,
		genesis,
		ds1,
		node.DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logger1,
	)
	require.NoError(err)

	node2, err := node.NewNode(
		ctx,
		config2,
		executor2,
		sequencer2,
		dac2,
		remoteSigner2,
		*nodeKey2,
		p2pClient2,
		genesis,
		ds2,
		node.DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logger2,
	)
	require.NoError(err)

	// Start both nodes
	startNodeInBackground(t, ctx, &runningWg, errCh, node1.(*node.FullNode))
	startNodeInBackground(t, ctx, &runningWg, errCh, node2.(*node.FullNode))

	dummyExecutor := executor1.(*coreexecutor.DummyExecutor)
	dummyExecutor.InjectTx([]byte("dummy transaction"))

	// Wait for both nodes to produce blocks
	time.Sleep(2 * time.Second)

	// Verify that both nodes produce the same block
	header1, _, err := node1.(*node.FullNode).Store.GetBlockData(ctx, 1)
	require.NoError(err)
	header2, _, err := node2.(*node.FullNode).Store.GetBlockData(ctx, 1)
	require.NoError(err)

	require.Equal(header1.Header, header2.Header, "Both nodes should produce the same block content")
}

func TestTwoProposerNodes_ContinuousInjection(t *testing.T) {
	var runningWg sync.WaitGroup
	var errCh = make(chan error, 1)
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup two proposer nodes
	config1 := getTestConfig(t, 1)
	config1.Node.Aggregator = true
	config2 := getTestConfig(t, 2)
	config2.Node.Aggregator = true

	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	remoteSigner1, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(err)
	remoteSigner2, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(err)

	nodeKey1, err := key.GenerateNodeKey()
	require.NoError(err)
	nodeKey2, err := key.GenerateNodeKey()
	require.NoError(err)

	executor1, sequencer1, dac1, p2pClient1, ds1 := createTestComponents(t)
	executor2, sequencer2, dac2, p2pClient2, ds2 := createTestComponents(t)

	logger1 := log.NewTestLogger(t)
	logger2 := log.NewTestLogger(t)

	basedDA := coreda.NewDummyDA(100_000, 0, 0)

	sequencer1, err = based.NewSequencer(
		logger1,
		basedDA,
		[]byte("test-chain"),
		0,
		1,
		ds1,
	)
	require.NoError(err)

	sequencer2, err = based.NewSequencer(
		logger2,
		basedDA,
		[]byte("test-chain"),
		0,
		1,
		ds2,
	)
	require.NoError(err)

	node1, err := node.NewNode(
		ctx,
		config1,
		executor1,
		sequencer1,
		dac1,
		remoteSigner1,
		*nodeKey1,
		p2pClient1,
		genesis,
		ds1,
		node.DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logger1,
	)
	require.NoError(err)

	node2, err := node.NewNode(
		ctx,
		config2,
		executor2,
		sequencer2,
		dac2,
		remoteSigner2,
		*nodeKey2,
		p2pClient2,
		genesis,
		ds2,
		node.DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logger2,
	)
	require.NoError(err)

	fullNode1 := node1.(*node.FullNode)
	fullNode2 := node2.(*node.FullNode)

	startNodeInBackground(t, ctx, &runningWg, errCh, fullNode1)
	startNodeInBackground(t, ctx, &runningWg, errCh, fullNode2)

	dummyExecutor := executor1.(*coreexecutor.DummyExecutor)

	// Inject transactions continuously in the background
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				txData := fmt.Sprintf("dummy tx %d", time.Now().UnixNano())
				dummyExecutor.InjectTx([]byte(txData))
			}
		}
	}()

	const targetHeight = 5

	// Wait until both nodes have produced up to `targetHeight`
	waitForHeight := func(node *node.FullNode, height uint64) {
		for {
			latestHeight, _ := node.Store.Height(ctx)
			if latestHeight >= height {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}

	waitForHeight(fullNode1, targetHeight)
	waitForHeight(fullNode2, targetHeight)

	// Compare block headers up to targetHeight
	for h := uint64(1); h <= targetHeight; h++ {
		header1, _, err := fullNode1.Store.GetBlockData(ctx, h)
		require.NoError(err, "failed to get block data from node1 at height %d", h)

		header2, _, err := fullNode2.Store.GetBlockData(ctx, h)
		require.NoError(err, "failed to get block data from node2 at height %d", h)

		require.Equal(header1.Header, header2.Header, "Mismatch at height %d", h)
	}
}
