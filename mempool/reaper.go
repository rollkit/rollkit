package mempool

import (
	"context"
	"fmt"
	"time"

	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
)

const ReapInterval time.Duration = 1 * time.Second

type CListMempoolReaper struct {
	mempool    Mempool
	stopCh     chan struct{}
	grpcClient *seqGRPC.Client
	ctx        context.Context
	rollupId   []byte
}

// NewCListMempool initializes the mempool and sets up the gRPC client.
func NewCListMempoolReaper(mempool Mempool, rollupId []byte, seqClient *seqGRPC.Client) (*CListMempoolReaper, error) {
	return &CListMempoolReaper{
		mempool:    mempool,
		stopCh:     make(chan struct{}),
		grpcClient: seqClient,
		ctx:        context.Background(),
		rollupId:   rollupId,
	}, nil
}

// StartReaper starts the reaper goroutine.
func (reaper *CListMempoolReaper) StartReaper(grpcAddressSequencerMiddleware string) error {
	go func() {
		ticker := time.NewTicker(ReapInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				reaper.reap()
			case <-reaper.stopCh:
				return
			}
		}
	}()
	return nil
}

// StopReaper stops the reaper goroutine.
func (reaper *CListMempoolReaper) StopReaper() {
	close(reaper.stopCh)
}

// reap removes all transactions from the mempool and sends them to the gRPC server.
func (reaper *CListMempoolReaper) reap() {
	txs := reaper.mempool.ReapMaxTxs(-1)
	for _, tx := range txs {
		if err := reaper.grpcClient.SubmitRollupTransaction(reaper.ctx, reaper.rollupId, tx); err != nil {
			panic(fmt.Errorf("submitting reaped tx failed with error: %w", err))
		}
	}
}
