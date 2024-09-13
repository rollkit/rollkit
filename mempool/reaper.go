package mempool

import (
	"context"
	"sync"
	"time"

	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/rollkit/go-sequencing/proxy/grpc"
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
	grpcClient *grpc.Client
	ctx        context.Context
	rollupId   []byte
	submitted  map[cmtypes.TxKey]struct{}
	mu         sync.RWMutex // Add a mutex to protect the submitted map
	logger     log.Logger
}

// NewCListMempoolReaper initializes the mempool and sets up the gRPC client.
func NewCListMempoolReaper(mempool Mempool, rollupId []byte, seqClient *grpc.Client, logger log.Logger) *CListMempoolReaper {
	return &CListMempoolReaper{
		mempool:    mempool,
		stopCh:     make(chan struct{}),
		grpcClient: seqClient,
		ctx:        context.Background(),
		rollupId:   rollupId,
		submitted:  make(map[cmtypes.TxKey]struct{}),
		logger:     logger,
	}
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

// UpdateCommitedTxs removes the committed transactions from the submitted map.
func (r *CListMempoolReaper) UpdateCommitedTxs(txs []cmtypes.Tx) {
	r.mu.Lock() // Lock the mutex before modifying the map
	defer r.mu.Unlock()
	for _, tx := range txs {
		delete(r.submitted, tx.Key())
	}
}

// StopReaper stops the reaper goroutine.
func (r *CListMempoolReaper) StopReaper() {
	close(r.stopCh)
}

// reap removes all transactions from the mempool and sends them to the gRPC server.
func (r *CListMempoolReaper) reap() {
	txs := r.mempool.ReapMaxTxs(-1)
	for _, tx := range txs {
		r.mu.RLock() // Read lock before checking the map
		_, ok := r.submitted[tx.Key()]
		r.mu.RUnlock() // Unlock after checking

		if ok {
			continue
		}
		if err := r.retrySubmitTransaction(tx, MaxRetries, RetryDelay); err != nil {
			r.logger.Error("Error submitting transaction", "tx key", tx.Key(), "error", err)
			continue
		}
		r.logger.Info("Reaper submitted transaction successfully", "tx key", tx.Key())

		r.mu.Lock() // Lock the mutex before writing to the map
		r.submitted[tx.Key()] = struct{}{}
		r.mu.Unlock() // Unlock after modifying the map
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
