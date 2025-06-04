package block

import (
	"context"
	"time"

	"github.com/rollkit/rollkit/pkg/store"
)

const DefaultFlushInterval = 1 * time.Second

// AsyncPruner is a service that periodically prunes block data in the background.
type AsyncPruner struct {
	ps store.PruningStore

	flushInterval time.Duration
}

func NewAsyncPruner(pruningStore store.PruningStore, flushInterval time.Duration) *AsyncPruner {
	if flushInterval <= 0 {
		flushInterval = DefaultFlushInterval
	}

	return &AsyncPruner{
		ps: pruningStore,

		flushInterval: flushInterval,
	}
}

// Start starts the async pruner that periodically prunes block data.
func (s *AsyncPruner) Start(ctx context.Context) {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Currently PruneBlockData only returns nil.
			_ = s.ps.PruneBlockData(ctx)
		}
	}
}
