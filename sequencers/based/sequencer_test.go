package based_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"

	"github.com/rollkit/rollkit/sequencers/based"
	"github.com/stretchr/testify/assert"
)

func newTestSequencer(t *testing.T) *based.Sequencer {
	dummyDA := coreda.NewDummyDA(100_000_000, 1.0, 1.5)
	store := ds.NewMapDatastore()
	seq, err := based.NewSequencer(log.NewNopLogger(), dummyDA, []byte("ns"), []byte("rollup1"), 0, 2, store)
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
	sequencer, err := based.NewSequencer(log.NewNopLogger(), dummyDA, []byte("ns"), []byte("rollup1"), 0, 0, store)
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
