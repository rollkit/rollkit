package based_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/sequencers/based"
	"github.com/stretchr/testify/assert"
)

func TestSequencer_SubmitRollupBatchTxs_Valid(t *testing.T) {
	daClient := da.NewDummyDA(100_000_000, 1.0, 1.5)
	rollupID := []byte("rollup1")
	sequencer, err := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), rollupID, 0, 2)
	assert.NoError(t, err)

	batch := &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}
	resp, err := sequencer.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: rollupID,
		Batch:    batch,
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestSequencer_SubmitRollupBatchTxs_InvalidRollup(t *testing.T) {
	daClient := da.NewDummyDA(100_000_000, 1.0, 1.5)
	sequencer, _ := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 2)

	batch := &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}
	resp, err := sequencer.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: []byte("invalid"),
		Batch:    batch,
	})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSequencer_GetNextBatch_OnlyPendingQueue(t *testing.T) {
	daClient := da.NewDummyDA(1_000_000, 1.0, 1.5)
	sequencer, _ := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 2)

	// Push pending txs
	timestamp := time.Now()
	sequencer.AddToPendingTxs([][]byte{[]byte("tx1")}, [][]byte{[]byte("id1")}, timestamp)

	resp, err := sequencer.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: []byte("rollup1")})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(resp.Batch.Transactions))
	assert.Equal(t, timestamp.Unix(), resp.Timestamp.Unix())
}

func TestSequencer_GetNextBatch_FromDALayer(t *testing.T) {
	daClient := da.NewDummyDA(1_000_000, 1.0, 1.5)
	sequencer, _ := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 5)

	ctx := context.Background()
	// Submit some blobs directly to the dummy DA
	blobs := []da.Blob{[]byte("tx2"), []byte("tx3")}
	_, err := daClient.Submit(ctx, blobs, 1.0, []byte("ns"), nil)
	assert.NoError(t, err)

	resp, err := sequencer.GetNextBatch(ctx, coresequencer.GetNextBatchRequest{
		RollupId:      []byte("rollup1"),
		LastBatchData: nil,
	})
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Batch.Transactions), 1)
	assert.GreaterOrEqual(t, len(resp.BatchData), 1)
}

func TestSequencer_GetNextBatch_InvalidRollup(t *testing.T) {
	daClient := da.NewDummyDA(1_000_000, 1.0, 1.5)
	sequencer, _ := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 5)

	resp, err := sequencer.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: []byte("invalid")})
	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSequencer_GetNextBatch_ExceedsMaxDrift(t *testing.T) {
	daClient := da.NewDummyDA(1_000_000, 1.0, 1.5)
	sequencer, _ := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 0) // no drift allowed

	ctx := context.Background()
	blobs := []da.Blob{[]byte("tx4")}
	_, err := daClient.Submit(ctx, blobs, 1.0, []byte("ns"), nil)
	assert.NoError(t, err)

	resp, err := sequencer.GetNextBatch(ctx, coresequencer.GetNextBatchRequest{
		RollupId:      []byte("rollup1"),
		LastBatchData: nil,
	})
	// Should return only pending txs or none because it cannot go beyond height drift 0
	assert.NoError(t, err)
	if resp != nil {
		assert.LessOrEqual(t, len(resp.Batch.Transactions), 1)
	}
}

func TestSequencer_VerifyBatch_Success(t *testing.T) {
	daClient := da.NewDummyDA(100_000_000, 1.0, 1.5)
	sequencer, err := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 2)
	assert.NoError(t, err)

	ctx := context.Background()
	blobs := []da.Blob{[]byte("tx1")}
	ids, err := daClient.Submit(ctx, blobs, 1.0, []byte("ns"), nil)
	assert.NoError(t, err)

	resp, err := sequencer.VerifyBatch(ctx, coresequencer.VerifyBatchRequest{
		RollupId:  []byte("rollup1"),
		BatchData: ids,
	})

	assert.NoError(t, err)
	assert.True(t, resp.Status)
}

func TestSequencer_VerifyBatch_InvalidRollup(t *testing.T) {
	daClient := da.NewDummyDA(100_000_000, 1.0, 1.5)
	sequencer, _ := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 2)

	ctx := context.Background()
	resp, err := sequencer.VerifyBatch(ctx, coresequencer.VerifyBatchRequest{
		RollupId:  []byte("invalid"),
		BatchData: [][]byte{[]byte("someID")},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
}

func TestSequencer_VerifyBatch_InvalidProof(t *testing.T) {
	daClient := da.NewDummyDA(100_000_000, 1.0, 1.5)
	sequencer, err := based.NewSequencer(log.NewNopLogger(), daClient, []byte("ns"), []byte("rollup1"), 0, 2)
	assert.NoError(t, err)

	ctx := context.Background()
	// Use random ID that has no proof in DummyDA
	batchID := []byte("invalid")
	resp, err := sequencer.VerifyBatch(ctx, coresequencer.VerifyBatchRequest{
		RollupId:  []byte("rollup1"),
		BatchData: [][]byte{batchID},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
}
