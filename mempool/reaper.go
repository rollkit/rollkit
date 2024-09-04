package mempool

import (
	"context"
	"time"

	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/cometbft/cometbft/libs/log"
	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
)

// ReapInterval is the interval at which the reaper checks the mempool for transactions to reap.
const (
	ReapInterval time.Duration = 1 * time.Second
	MaxRetries   int           = 3
	RetryDelay   time.Duration = 2 * time.Second
)

// CListMempoolReaper is a reaper that reaps transactions from the mempool and sends them to the gRPC server.
type CListMempoolReaper struct {
	mempool    Mempool
	stopCh     chan struct{}
	grpcClient *seqGRPC.Client
	ctx        context.Context
	rollupId   []byte
	submitted  map[cmtypes.TxKey]struct{}
	logger     log.Logger
}

// NewCListMempool initializes the mempool and sets up the gRPC client.
func NewCListMempoolReaper(mempool Mempool, rollupId []byte, seqClient *seqGRPC.Client, logger log.Logger) (*CListMempoolReaper, error) {
	return &CListMempoolReaper{
		mempool:    mempool,
		stopCh:     make(chan struct{}),
		grpcClient: seqClient,
		ctx:        context.Background(),
		rollupId:   rollupId,
		submitted:  make(map[cmtypes.TxKey]struct{}),
		logger:     logger,
	}, nil
}

// StartReaper starts the reaper goroutine.
func (r *CListMempoolReaper) StartReaper() error {
	go func() {
		ticker := time.NewTicker(ReapInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.reap()
			case <-r.stopCh:
				return
			}
		}
	}()
	return nil
}

// StopReaper stops the reaper goroutine.
func (r *CListMempoolReaper) StopReaper() {
	close(r.stopCh)
}

// reap removes all transactions from the mempool and sends them to the gRPC server.
func (r *CListMempoolReaper) reap() {
	txs := r.mempool.ReapMaxTxs(-1)
	for _, tx := range txs {
		if _, ok := r.submitted[tx.Key()]; ok {
			continue
		}
		if err := r.retrySubmitTransaction(tx, MaxRetries, RetryDelay); err != nil {
			r.logger.Error("Error submitting transaction", "tx key", tx.Key(), "error", err)
			continue
		}
		r.logger.Info("Reaper submitted transaction successfully", "tx key", tx.Key())
		r.submitted[tx.Key()] = struct{}{}
	}
}

func (reaper *CListMempoolReaper) retrySubmitTransaction(tx cmtypes.Tx, maxRetries int, delay time.Duration) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = reaper.grpcClient.SubmitRollupTransaction(reaper.ctx, reaper.rollupId, tx)
		if err == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return err
}
