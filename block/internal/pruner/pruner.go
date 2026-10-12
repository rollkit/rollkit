package pruner

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/rs/zerolog"

	coreexecutor "github.com/evstack/ev-node/core/execution"
	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/store"
)

// Pruner periodically removes old state and execution metadata entries.
type Pruner struct {
	store      store.Store
	execPruner coreexecutor.ExecPruner
	cfg        config.PruningConfig
	blockTime  time.Duration
	daEnabled  bool
	logger     zerolog.Logger

	// Lifecycle
	ctx    context.Context
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// New creates a new Pruner instance.
func New(
	logger zerolog.Logger,
	store store.Store,
	execPruner coreexecutor.ExecPruner,
	cfg config.PruningConfig,
	blockTime time.Duration,
	daAddress string,
) *Pruner {
	return &Pruner{
		store:      store,
		execPruner: execPruner,
		cfg:        cfg,
		blockTime:  blockTime,
		daEnabled:  daAddress != "", // DA is enabled if address is provided
		logger:     logger.With().Str("component", "pruner").Logger(),
	}
}

// Start begins the pruning loop.
func (p *Pruner) Start(ctx context.Context) error {
	if p.cancel != nil {
		return errors.New("pruner already started")
	}
	if !p.cfg.IsPruningEnabled() {
		p.logger.Info().Msg("pruning is disabled, not starting pruner")
		return nil
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	// Start pruner loop
	p.wg.Go(p.pruneLoop)

	p.logger.Info().Msg("pruner started")
	return nil
}

// Stop stops the pruning loop.
func (p *Pruner) Stop() error {
	if !p.cfg.IsPruningEnabled() {
		return nil
	}

	if p.cancel != nil {
		p.cancel()
	}

	p.wg.Wait()

	p.logger.Info().Msg("pruner stopped")
	return nil
}

func (p *Pruner) pruneLoop() {
	ticker := time.NewTicker(p.cfg.Interval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			switch p.cfg.Mode {
			case config.PruningModeMetadata:
				if err := p.pruneMetadata(); err != nil {
					p.logger.Error().Err(err).Msg("failed to prune blocks metadata")
				}
			case config.PruningModeAll:
				if err := p.pruneBlocks(); err != nil {
					p.logger.Error().Err(err).Msg("failed to prune blocks")
				}
			}
		case <-p.ctx.Done():
			return
		}
	}
}

// pruneBlocks prunes blocks and their metadatas.
func (p *Pruner) pruneBlocks() error {
	storeHeight, err := p.store.Height(p.ctx)
	if err != nil {
		return fmt.Errorf("failed to get store height for pruning: %w", err)
	}

	upperBound := storeHeight

	// If DA is enabled, only prune blocks that are DA included
	if p.daEnabled {
		var currentDAIncluded uint64
		currentDAIncludedBz, err := p.store.GetMetadata(p.ctx, store.DAIncludedHeightKey)
		if err == nil && len(currentDAIncludedBz) == 8 {
			currentDAIncluded = binary.LittleEndian.Uint64(currentDAIncludedBz)
		} else {
			p.logger.Debug().Msg("skipping pruning: DA is enabled but DA included height is not available yet")
			return nil
		}

		// Never prune blocks that are not DA included
		upperBound = min(storeHeight, currentDAIncluded)
	}

	if upperBound <= p.cfg.KeepRecent {
		// Not enough fully included blocks to prune
		return nil
	}

	targetHeight := upperBound - p.cfg.KeepRecent

	lastPruned, err := p.getLastPrunedBlockHeight(p.ctx)
	if err != nil {
		return fmt.Errorf("failed to get last pruned block height: %w", err)
	}

	// Bound the work done per tick; a backlog drains over several ticks.
	blocksEnd := max(min(lastPruned+p.pruneBudget(), targetHeight), lastPruned)
	if err := p.pruneInChunks(lastPruned, blocksEnd, p.store.PruneBlocks); err != nil {
		p.logger.Error().Err(err).Uint64("target_height", blocksEnd).Msg("failed to prune old block data")
		return err
	}

	if blocksEnd == 0 {
		return nil
	}

	// PruneBlocks leaves per-height state snapshots in place. Keep the state at
	// blocksEnd: replaying the first retained block (blocksEnd+1) needs it.
	if _, err := p.pruneStates(blocksEnd - 1); err != nil {
		return err
	}

	if p.execPruner != nil {
		if err := p.execPruner.PruneExec(p.ctx, blocksEnd); err != nil && !errors.Is(err, ds.ErrNotFound) {
			return err
		}
	}

	p.logger.Debug().Uint64("pruned_up_to_height", blocksEnd).Bool("da_enabled", p.daEnabled).Msg("pruned blocks up to height")
	return nil
}

// maxPruneChunk bounds the number of heights pruned in a single store batch,
// keeping individual write transactions small.
const maxPruneChunk uint64 = 10_000

// pruneBudget returns the maximum number of heights to prune per tick.
// It is 4x the blocks produced during one pruning interval, so a backlog drains
// at ~3x the block production rate and pruning never falls behind the chain.
// Example: 100ms blocks and a 15m interval produce 9,000 blocks per interval,
// giving a budget of 36,000 heights (net catch-up of 27,000 per interval).
func (p *Pruner) pruneBudget() uint64 {
	blocksPerInterval := uint64(p.cfg.Interval.Duration / p.blockTime)
	return max(blocksPerInterval*4, maxPruneChunk)
}

// pruneInChunks calls prune for successive heights in (from, to], at most
// maxPruneChunk heights per call. The loop is bounded by (to-from)/maxPruneChunk.
func (p *Pruner) pruneInChunks(from, to uint64, prune func(context.Context, uint64) error) error {
	for from < to {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		end := min(from+maxPruneChunk, to)
		if err := prune(p.ctx, end); err != nil {
			return err
		}
		from = end
	}
	return nil
}

// pruneMetadata prunes old state and execution metadata entries based on the configured retention depth.
// It does not prune old blocks, as those are handled by the pruning logic.
// Pruning old state does not lose history but limits the ability to recover (replay or rollback) to the last HEAD-N blocks, where N is the retention depth.
func (p *Pruner) pruneMetadata() error {
	height, err := p.store.Height(p.ctx)
	if err != nil {
		return err
	}

	// Blocks in (target, height] stay replayable; the oldest one, target+1,
	// needs the state at target, so only states below target are pruned.
	if height <= p.cfg.KeepRecent+1 {
		return nil
	}
	target := height - p.cfg.KeepRecent

	statesEnd, err := p.pruneStates(target - 1)
	if err != nil || statesEnd == 0 {
		return err
	}

	if p.execPruner != nil {
		if err := p.execPruner.PruneExec(p.ctx, statesEnd); err != nil && !errors.Is(err, ds.ErrNotFound) {
			return err
		}
	}

	return nil
}

// pruneStates deletes per-height state snapshots in (lastPrunedState, upTo],
// bounded by pruneBudget per call. It returns the last pruned state height.
func (p *Pruner) pruneStates(upTo uint64) (uint64, error) {
	lastPrunedState, err := p.getLastPrunedStateHeight(p.ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get last pruned state height: %w", err)
	}

	if upTo <= lastPrunedState {
		return lastPrunedState, nil
	}

	statesEnd := min(lastPrunedState+p.pruneBudget(), upTo)
	if err := p.pruneInChunks(lastPrunedState, statesEnd, p.store.PruneStates); err != nil {
		return 0, fmt.Errorf("failed to prune state snapshots: %w", err)
	}

	p.logger.Debug().Uint64("pruned_to", statesEnd).Msg("pruned state snapshots up to height")
	return statesEnd, nil
}

// getLastPrunedBlockHeight returns the height of the last block that was pruned using PruneBlocks.
func (p *Pruner) getLastPrunedBlockHeight(ctx context.Context) (uint64, error) {
	lastPrunedBlockHeightBz, err := p.store.GetMetadata(ctx, store.LastPrunedBlockHeightKey)
	if errors.Is(err, ds.ErrNotFound) {
		// If not found, it means we haven't pruned any blocks yet, so we return 0.
		return 0, nil
	}

	if err != nil || len(lastPrunedBlockHeightBz) != 8 {
		return 0, fmt.Errorf("failed to get last pruned block height or invalid format: %w", err)
	}

	lastPrunedBlockHeight := binary.LittleEndian.Uint64(lastPrunedBlockHeightBz)
	if lastPrunedBlockHeight == 0 {
		return 0, fmt.Errorf("invalid last pruned block height")
	}

	return lastPrunedBlockHeight, nil
}

// getLastPrunedStateHeight returns the height of the last state that was pruned using PruneStates.
func (p *Pruner) getLastPrunedStateHeight(ctx context.Context) (uint64, error) {
	lastPrunedStateHeightBz, err := p.store.GetMetadata(ctx, store.LastPrunedStateHeightKey)
	if errors.Is(err, ds.ErrNotFound) {
		// If not found, it means we haven't pruned any state yet, so we return 0.
		return 0, nil
	}

	if err != nil || len(lastPrunedStateHeightBz) != 8 {
		return 0, fmt.Errorf("failed to get last pruned block height or invalid format: %w", err)
	}

	lastPrunedStateHeight := binary.LittleEndian.Uint64(lastPrunedStateHeightBz)
	if lastPrunedStateHeight == 0 {
		return 0, fmt.Errorf("invalid last pruned block height")
	}

	return lastPrunedStateHeight, nil
}
