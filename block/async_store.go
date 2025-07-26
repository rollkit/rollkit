package block

import (
	"context"
	"time"

	"cosmossdk.io/log"
	"github.com/rollkit/rollkit/pkg/store"
)

const DefaultFlushInterval = 10 * time.Second

// AsyncPruner is a service that periodically prunes block data in the background.
type AsyncPruner struct {
	ps store.PruningStore

	flushInterval time.Duration
	logger        log.Logger
}

func NewAsyncPruner(pruningStore store.PruningStore, flushInterval time.Duration, logger log.Logger) *AsyncPruner {
	return &AsyncPruner{
		ps: pruningStore,

		flushInterval: flushInterval,
		logger:        logger,
	}
}

// Start starts the async pruner that periodically prunes block data.
func (s *AsyncPruner) Start(ctx context.Context) {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	s.logger.Info("AsyncPruner started", "interval", s.flushInterval)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("AsyncPruner stopped")
			return
		case <-ticker.C:
			err := s.ps.PruneBlockData(ctx)
			if err != nil {
				s.logger.Error("Failed to prune block data", "error", err)
			}
		}
	}
}
