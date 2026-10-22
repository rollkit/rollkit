package store

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/celestiaorg/go-header"

	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/types"
)

// errElapsedHeight is returned when the requested height was already stored.
var errElapsedHeight = errors.New("elapsed height")

const (
	// maxPendingCacheSize is the maximum number of items in the pending cache.
	// When this limit is reached, Append will block until items are pruned.
	maxPendingCacheSize = 10_000

	// pruneRetryInterval is how long to wait between prune attempts when cache is full.
	pruneRetryInterval = 50 * time.Millisecond
)

// StoreGetter abstracts the store access methods for different types (headers vs data).
type StoreGetter[H header.Header[H]] interface {
	// GetByHeight retrieves an item by its height.
	GetByHeight(ctx context.Context, height uint64) (H, error)
	// GetByHash retrieves an item by its hash.
	GetByHash(ctx context.Context, hash []byte) (H, error)
	// Height returns the current height of the store.
	Height(ctx context.Context) (uint64, error)
	// HasAt checks if an item exists at the given height.
	HasAt(ctx context.Context, height uint64) bool
	// GetDAHint retrieves the DA hint for a given height.
	GetDAHint(ctx context.Context, height uint64) (uint64, error)
	// SetDAHint stores the DA hint for a given height.
	SetDAHint(ctx context.Context, height uint64, daHint uint64) error
}

// EntityWithDAHint extends header.Header with DA hint methods.
// This interface is used by sync services and store adapters to track
// which DA height contains the data for a given block.
type EntityWithDAHint[H any] interface {
	header.Header[H]
	SetDAHint(daHeight uint64)
	DAHint() uint64
}

// heightSub provides a mechanism for waiting on a specific height to be stored.
// This is critical for go-header syncer which expects GetByHeight to block until
// the requested height is available.
type heightSub struct {
	height    atomic.Uint64
	heightMu  sync.Mutex
	heightChs map[uint64][]chan struct{}
}

func newHeightSub(initialHeight uint64) *heightSub {
	hs := &heightSub{
		heightChs: make(map[uint64][]chan struct{}),
	}
	hs.height.Store(initialHeight)
	return hs
}

// Height returns the current height.
func (hs *heightSub) Height() uint64 {
	return hs.height.Load()
}

// SetHeight updates the current height and notifies any waiters.
func (hs *heightSub) SetHeight(h uint64) {
	hs.height.Store(h)
	hs.notifyUpTo(h)
}

// Wait blocks until the given height is reached or context is canceled.
// Returns errElapsedHeight if the height was already reached.
func (hs *heightSub) Wait(ctx context.Context, height uint64) error {
	// Fast path: height already reached
	if hs.height.Load() >= height {
		return errElapsedHeight
	}

	hs.heightMu.Lock()
	// Double-check after acquiring lock
	if hs.height.Load() >= height {
		hs.heightMu.Unlock()
		return errElapsedHeight
	}

	// Create a channel to wait on
	ch := make(chan struct{})
	hs.heightChs[height] = append(hs.heightChs[height], ch)
	hs.heightMu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		hs.removeWaiter(height, ch)
		return ctx.Err()
	}
}

// removeWaiter unregisters a specific waiter from the requested height.
func (hs *heightSub) removeWaiter(height uint64, target chan struct{}) {
	hs.heightMu.Lock()
	defer hs.heightMu.Unlock()

	chs, ok := hs.heightChs[height]
	if !ok {
		return
	}

	for i, ch := range chs {
		if ch != target {
			continue
		}
		copy(chs[i:], chs[i+1:])
		chs[len(chs)-1] = nil
		chs = chs[:len(chs)-1]
		if len(chs) == 0 {
			delete(hs.heightChs, height)
		} else {
			hs.heightChs[height] = chs
		}
		return
	}
}

// notifyUpTo notifies all waiters for heights <= h.
func (hs *heightSub) notifyUpTo(h uint64) {
	hs.heightMu.Lock()
	defer hs.heightMu.Unlock()

	for height, chs := range hs.heightChs {
		if height <= h {
			for _, ch := range chs {
				close(ch)
			}
			delete(hs.heightChs, height)
		}
	}
}

// pendingCache holds all pending state under a single mutex.
// Items are stored here after P2P receipt until persisted to the store.
type pendingCache[H EntityWithDAHint[H]] struct {
	mu        sync.RWMutex
	items     map[uint64]H      // height -> item
	byHash    map[string]uint64 // hash -> height (for O(1) lookups)
	daHints   map[uint64]uint64 // height -> DA hint
	maxHeight uint64            // tracked for O(1) access
}

func newPendingCache[H EntityWithDAHint[H]]() *pendingCache[H] {
	return &pendingCache[H]{
		items:   make(map[uint64]H),
		byHash:  make(map[string]uint64),
		daHints: make(map[uint64]uint64),
	}
}

func (c *pendingCache[H]) len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *pendingCache[H]) add(item H) {
	c.mu.Lock()
	defer c.mu.Unlock()
	height := item.Height()
	c.items[height] = item
	c.byHash[string(item.Hash())] = height
	if hint := item.DAHint(); hint > 0 {
		c.daHints[height] = hint
	}
	if height > c.maxHeight {
		c.maxHeight = height
	}
}

func (c *pendingCache[H]) get(height uint64) (H, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.items[height]
	return item, ok
}

func (c *pendingCache[H]) getByHash(hash []byte) (H, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var zero H
	height, ok := c.byHash[string(hash)]
	if !ok {
		return zero, false
	}
	item, ok := c.items[height]
	return item, ok
}

func (c *pendingCache[H]) has(height uint64) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.items[height]
	return ok
}

func (c *pendingCache[H]) hasByHash(hash []byte) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.byHash[string(hash)]
	return ok
}

func (c *pendingCache[H]) getDAHint(height uint64) (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	hint, ok := c.daHints[height]
	return hint, ok
}

func (c *pendingCache[H]) setDAHint(height, hint uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.daHints[height] = hint
}

func (c *pendingCache[H]) delete(height uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, ok := c.items[height]; ok {
		delete(c.byHash, string(item.Hash()))
		delete(c.items, height)
		if height == c.maxHeight {
			c.recalcMaxHeight()
		}
	}
	delete(c.daHints, height)
}

func (c *pendingCache[H]) getMaxHeight() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.maxHeight
}

// head returns the highest item in the pending cache and its height.
// Returns zero value and 0 if pending cache is empty.
func (c *pendingCache[H]) head() (H, uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var zero H
	if c.maxHeight == 0 {
		return zero, 0
	}
	item, ok := c.items[c.maxHeight]
	if !ok {
		return zero, 0
	}
	return item, c.maxHeight
}

// pruneIf removes items where the predicate returns true.
// Returns the number of items pruned.
func (c *pendingCache[H]) pruneIf(pred func(height uint64) bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	pruned := 0
	needsMaxRecalc := false
	for height, item := range c.items {
		if pred(height) {
			delete(c.byHash, string(item.Hash()))
			delete(c.items, height)
			delete(c.daHints, height)
			pruned++
			if height == c.maxHeight {
				needsMaxRecalc = true
			}
		}
	}
	if needsMaxRecalc {
		c.recalcMaxHeight()
	}
	return pruned
}

// recalcMaxHeight recalculates maxHeight from items. Must be called with lock held.
func (c *pendingCache[H]) recalcMaxHeight() {
	c.maxHeight = 0
	for h := range c.items {
		if h > c.maxHeight {
			c.maxHeight = h
		}
	}
}

// StoreAdapter is a generic adapter that wraps Store to implement header.Store[H].
// This allows the ev-node store to be used directly by go-header's P2P infrastructure,
// eliminating the need for a separate go-header store and reducing data duplication.
//
// The adapter maintains an in-memory cache for items received via P2P (through Append).
// This cache allows the go-header syncer and P2P handler to access items before they
// are validated and persisted by the ev-node syncer. Once the ev-node syncer processes
// a block, it writes to the underlying store, and subsequent reads will come from the store.
type StoreAdapter[H EntityWithDAHint[H]] struct {
	getter               StoreGetter[H]
	store                Store
	genesisInitialHeight uint64

	// heightSub tracks the current height and allows waiting for specific heights.
	// This is required by go-header syncer for blocking GetByHeight.
	heightSub *heightSub

	// mu protects initialization state
	mu          sync.RWMutex
	initialized bool

	// pending holds items received via Append that haven't been written to the store yet.
	// Bounded by maxPendingCacheSize with backpressure when full.
	pending *pendingCache[H]

	// onDeleteFn is called when items are deleted (for rollback scenarios)
	onDeleteFn func(context.Context, uint64) error
}

// NewStoreAdapter creates a new StoreAdapter wrapping the given store getter and backing Store.
// The genesis is used to determine the initial height for efficient Tail lookups.
func NewStoreAdapter[H EntityWithDAHint[H]](getter StoreGetter[H], store Store, gen genesis.Genesis) *StoreAdapter[H] {
	// Get actual current height from store (0 if empty)
	var storeHeight uint64
	if h, err := getter.Height(context.Background()); err == nil {
		storeHeight = h
	}

	adapter := &StoreAdapter[H]{
		getter:               getter,
		store:                store,
		genesisInitialHeight: max(gen.InitialHeight, 1),
		pending:              newPendingCache[H](),
		heightSub:            newHeightSub(storeHeight),
	}

	// Mark as initialized if we have data
	if storeHeight > 0 {
		adapter.initialized = true
	}

	return adapter
}

// Start implements header.Store. It initializes the adapter if needed.
func (a *StoreAdapter[H]) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Refresh height from store
	h, err := a.getter.Height(ctx)
	if err != nil {
		return err
	}

	if h > 0 {
		a.heightSub.SetHeight(h)
		a.initialized = true
	}

	return nil
}

// Stop implements header.Store. No-op since the underlying store lifecycle
// is managed separately.
func (a *StoreAdapter[H]) Stop(ctx context.Context) error {
	return nil
}

// Head returns the highest item in the store.
func (a *StoreAdapter[H]) Head(ctx context.Context, _ ...header.HeadOption[H]) (H, error) {
	var zero H

	storeHeight, _ := a.getter.Height(ctx)
	pendingHead, pendingHeight := a.pending.head()

	// Prefer pending if it's higher than store
	if pendingHeight > storeHeight {
		a.heightSub.SetHeight(pendingHeight)
		return pendingHead, nil
	}

	// Try to get from store
	if storeHeight > 0 {
		a.heightSub.SetHeight(storeHeight)
		if item, err := a.getter.GetByHeight(ctx, storeHeight); err == nil {
			return item, nil
		}
	}

	// Fall back to pending if store failed
	if pendingHeight > 0 {
		a.heightSub.SetHeight(pendingHeight)
		return pendingHead, nil
	}

	return zero, header.ErrEmptyStore
}

// Tail returns the lowest item in the store.
// For ev-node, this is typically the genesis/initial height.
// If pruning has occurred, it walks up from initialHeight to find the first available item.
func (a *StoreAdapter[H]) Tail(ctx context.Context) (H, error) {
	var zero H

	height := a.heightSub.Height()
	if height == 0 {
		// Check store
		h, err := a.getter.Height(ctx)
		if err != nil || h == 0 {
			return zero, header.ErrEmptyStore
		}
		height = h
	}

	// Determine the first candidate tail height. By default, this is the
	// genesis initial height, but if pruning metadata is available we can
	// skip directly past fully-pruned ranges.
	startHeight := a.genesisInitialHeight
	if lastPrunedBlockHeightBz, err := a.store.GetMetadata(ctx, LastPrunedBlockHeightKey); err == nil && len(lastPrunedBlockHeightBz) == heightLength {
		if lastPruned, err := decodeHeight(lastPrunedBlockHeightBz); err == nil {
			if candidate := lastPruned + 1; candidate > startHeight {
				startHeight = candidate
			}
		}
	}

	item, err := a.getter.GetByHeight(ctx, startHeight)
	if err == nil {
		return item, nil
	}

	// Check pending for genesisInitialHeight
	if pendingItem, ok := a.pending.get(a.genesisInitialHeight); ok {
		return pendingItem, nil
	}

	// Walk up from startHeight to find the first available item
	for h := startHeight + 1; h <= height; h++ {
		item, err = a.getter.GetByHeight(ctx, h)
		if err == nil {
			return item, nil
		}
		if pendingItem, ok := a.pending.get(h); ok {
			return pendingItem, nil
		}
	}

	// should never happen
	return zero, header.ErrEmptyStore
}

// Get returns an item by its hash.
func (a *StoreAdapter[H]) Get(ctx context.Context, hash header.Hash) (H, error) {
	var zero H

	// First try the store
	item, err := a.getter.GetByHash(ctx, hash)
	if err == nil {
		a.applyDAHint(ctx, item)
		return item, nil
	}

	// Check pending items using hash index for O(1) lookup
	if pendingItem, ok := a.pending.getByHash(hash); ok {
		a.applyDAHint(ctx, pendingItem)
		return pendingItem, nil
	}

	return zero, header.ErrNotFound
}

// GetByHeight returns an item at the given height.
// If the height is not yet available, it blocks until it is or context is canceled.
func (a *StoreAdapter[H]) GetByHeight(ctx context.Context, height uint64) (H, error) {
	var zero H

	// Try to get the item first
	if item, err := a.getByHeightNoWait(ctx, height); err == nil {
		return item, nil
	}

	// If not found, wait for the height to be stored
	err := a.heightSub.Wait(ctx, height)
	if err != nil && !errors.Is(err, errElapsedHeight) {
		return zero, err
	}

	// Try again after waiting
	return a.getByHeightNoWait(ctx, height)
}

// getByHeightNoWait returns an item at the given height without blocking.
func (a *StoreAdapter[H]) getByHeightNoWait(ctx context.Context, height uint64) (H, error) {
	var zero H

	// First try the store
	item, err := a.getter.GetByHeight(ctx, height)
	if err == nil {
		a.applyDAHint(ctx, item)
		return item, nil
	}

	// Check pending items
	if pendingItem, ok := a.pending.get(height); ok {
		a.applyDAHint(ctx, pendingItem)
		return pendingItem, nil
	}

	return zero, header.ErrNotFound
}

// applyDAHint sets the DA hint on the item from cache or disk.
func (a *StoreAdapter[H]) applyDAHint(ctx context.Context, item H) {
	if item.IsZero() {
		return
	}

	height := item.Height()

	// Check pending cache first
	if hint, found := a.pending.getDAHint(height); found {
		item.SetDAHint(hint)
		return
	}

	// Try to load from disk
	if diskHint, err := a.getter.GetDAHint(ctx, height); err == nil && diskHint > 0 {
		a.pending.setDAHint(height, diskHint)
		item.SetDAHint(diskHint)
	}
}

// GetRangeByHeight returns items in the range [from.Height()+1, to).
// This follows go-header's convention where 'from' is the trusted item
// and we return items starting from the next height.
func (a *StoreAdapter[H]) GetRangeByHeight(ctx context.Context, from H, to uint64) ([]H, error) {
	if from.IsZero() {
		return nil, header.ErrNotFound
	}

	startHeight := from.Height() + 1
	if startHeight >= to {
		return nil, nil
	}

	return a.GetRange(ctx, startHeight, to)
}

// GetRange returns items in the range [from, to).
func (a *StoreAdapter[H]) GetRange(ctx context.Context, from, to uint64) ([]H, error) {
	if from >= to {
		return nil, nil
	}

	items := make([]H, 0, to-from)
	for height := from; height < to; height++ {
		item, err := a.GetByHeight(ctx, height)
		if err != nil {
			// Return what we have so far
			if len(items) > 0 {
				return items, nil
			}
			return nil, header.ErrNotFound
		}
		items = append(items, item)
	}

	return items, nil
}

// Has checks if an item with the given hash exists.
func (a *StoreAdapter[H]) Has(ctx context.Context, hash header.Hash) (bool, error) {
	// Check store first
	_, err := a.getter.GetByHash(ctx, hash)
	if err == nil {
		return true, nil
	}

	// Check pending items using hash index for O(1) lookup
	return a.pending.hasByHash(hash), nil
}

// HasAt checks if an item exists at the given height.
func (a *StoreAdapter[H]) HasAt(ctx context.Context, height uint64) bool {
	// Check store first
	if a.getter.HasAt(ctx, height) {
		return true
	}

	// Check pending items
	return a.pending.has(height)
}

// Height returns the current height of the store.
func (a *StoreAdapter[H]) Height() uint64 {
	// Check store first
	if h, err := a.getter.Height(context.Background()); err == nil && h > 0 {
		// Also check pending for higher heights
		maxPending := a.pending.getMaxHeight()
		if maxPending > h {
			a.heightSub.SetHeight(maxPending)
			return maxPending
		}
		a.heightSub.SetHeight(h)
		return h
	}

	// Fall back to cached height or check pending
	height := a.heightSub.Height()
	if height > 0 {
		return height
	}

	if maxPending := a.pending.getMaxHeight(); maxPending > height {
		return maxPending
	}
	return height
}

// Append stores items in the pending cache.
// These items are received via P2P and will be available for retrieval
// until the ev-node syncer processes and persists them to the store.
// If items have a DA hint set, it will be cached for later retrieval.
// If the cache is full, this will block until space is available or context is canceled.
func (a *StoreAdapter[H]) Append(ctx context.Context, items ...H) error {
	if len(items) == 0 {
		return nil
	}

	for _, item := range items {
		if item.IsZero() {
			continue
		}

		height := item.Height()

		// Check if already in store
		if a.getter.HasAt(ctx, height) {
			// Already persisted, skip adding to pending
			continue
		}

		// Wait for space in the cache if full (backpressure)
		if err := a.waitForSpace(ctx); err != nil {
			return err
		}

		// Add to pending cache (includes DA hint if present)
		a.pending.add(item)

		// Persist DA hint to disk
		if hint := item.DAHint(); hint > 0 {
			_ = a.getter.SetDAHint(ctx, height, hint)
		}

		// Update cached height and notify waiters
		if height > a.heightSub.Height() {
			a.heightSub.SetHeight(height)
		}
	}

	return nil
}

// waitForSpace blocks until there's space in the pending cache or context is canceled.
// It actively prunes persisted items to make room.
func (a *StoreAdapter[H]) waitForSpace(ctx context.Context) error {
	for {
		if a.pending.len() < maxPendingCacheSize {
			return nil
		}

		// Cache is full, try to prune
		a.prunePersisted(ctx)

		if a.pending.len() < maxPendingCacheSize {
			return nil
		}

		// Still full, wait a bit for executor to catch up
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pruneRetryInterval):
			// Retry
		}
	}
}

// prunePersisted removes items that have already been persisted to the underlying store.
func (a *StoreAdapter[H]) prunePersisted(ctx context.Context) {
	storeHeight, err := a.getter.Height(ctx)
	if err != nil || storeHeight == 0 {
		return
	}
	// Items at or below store height are definitely persisted
	a.pending.pruneIf(func(height uint64) bool {
		return height <= storeHeight
	})
}

// Init initializes the store with the first item.
// This is called by go-header when bootstrapping the store with a trusted item.
func (a *StoreAdapter[H]) Init(ctx context.Context, item H) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.initialized {
		return nil
	}

	if item.IsZero() {
		return nil
	}

	// Add to pending cache
	a.pending.add(item)

	a.heightSub.SetHeight(item.Height())
	a.initialized = true

	return nil
}

// Sync ensures all pending writes are flushed.
// No-op for the adapter as pending data is in-memory cache.
func (a *StoreAdapter[H]) Sync(ctx context.Context) error {
	return nil
}

// DeleteRange deletes all items in the range [from, to).
func (a *StoreAdapter[H]) DeleteRange(ctx context.Context, from, to uint64) error {
	// Remove from pending cache
	for height := from; height < to; height++ {
		a.pending.delete(height)
	}

	// Call onDeleteFn outside the lock
	if a.onDeleteFn != nil {
		for height := from; height < to; height++ {
			if err := a.onDeleteFn(ctx, height); err != nil {
				return err
			}
		}
	}

	// Update the cached height to reflect the highest item that still exists.
	// The head is only gone when the deleted range actually covered it: since the
	// range is half-open, deleting a non-head range leaves higher items behind and
	// must not lower the cached height. Recomputing instead of using `from - 1`
	// also handles pending caches with gaps and avoids the uint64 underflow that
	// `from - 1` causes when from == 0.
	newHeight := a.pending.getMaxHeight()
	if storeHeight, err := a.getter.Height(ctx); err == nil && storeHeight > newHeight {
		newHeight = storeHeight
	}
	if current := a.heightSub.Height(); newHeight < current {
		a.heightSub.SetHeight(newHeight)
	}

	return nil
}

// OnDelete registers a callback to be invoked when items are deleted.
func (a *StoreAdapter[H]) OnDelete(fn func(context.Context, uint64) error) {
	a.onDeleteFn = fn
}

// HeaderStoreGetter implements StoreGetter for *types.SignedHeader.
type HeaderStoreGetter struct {
	store Store
}

// NewHeaderStoreGetter creates a new HeaderStoreGetter.
func NewHeaderStoreGetter(store Store) *HeaderStoreGetter {
	return &HeaderStoreGetter{store: store}
}

// GetByHeight implements StoreGetter.
func (g *HeaderStoreGetter) GetByHeight(ctx context.Context, height uint64) (*types.P2PSignedHeader, error) {
	header, err := g.store.GetHeader(ctx, height)
	if err != nil {
		return nil, err
	}

	daHint, _ := g.GetDAHint(ctx, height)

	return &types.P2PSignedHeader{
		SignedHeader: header,
		DAHeightHint: daHint,
	}, nil
}

// GetDAHint implements StoreGetter.
func (g *HeaderStoreGetter) GetDAHint(ctx context.Context, height uint64) (uint64, error) {
	data, err := g.store.GetMetadata(ctx, GetHeightToDAHeightHeaderKey(height))
	if err != nil {
		return 0, err
	}
	if len(data) != 8 {
		return 0, fmt.Errorf("invalid da hint data length: %d", len(data))
	}
	return binary.LittleEndian.Uint64(data), nil
}

// SetDAHint implements StoreGetter.
func (g *HeaderStoreGetter) SetDAHint(ctx context.Context, height uint64, daHint uint64) error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, daHint)
	return g.store.SetMetadata(ctx, GetHeightToDAHeightHeaderKey(height), data)
}

// GetByHash implements StoreGetter.
func (g *HeaderStoreGetter) GetByHash(ctx context.Context, hash []byte) (*types.P2PSignedHeader, error) {
	hdr, _, err := g.store.GetBlockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	daHint, _ := g.GetDAHint(ctx, hdr.Height())

	return &types.P2PSignedHeader{
		SignedHeader: hdr,
		DAHeightHint: daHint,
	}, nil
}

// Height implements StoreGetter.
func (g *HeaderStoreGetter) Height(ctx context.Context) (uint64, error) {
	return g.store.Height(ctx)
}

// HasAt implements StoreGetter.
func (g *HeaderStoreGetter) HasAt(ctx context.Context, height uint64) bool {
	_, err := g.store.GetHeader(ctx, height)
	return err == nil
}

// DataStoreGetter implements StoreGetter for *types.Data.
type DataStoreGetter struct {
	store Store
}

// NewDataStoreGetter creates a new DataStoreGetter.
func NewDataStoreGetter(store Store) *DataStoreGetter {
	return &DataStoreGetter{store: store}
}

// GetByHeight implements StoreGetter.
func (g *DataStoreGetter) GetByHeight(ctx context.Context, height uint64) (*types.P2PData, error) {
	_, data, err := g.store.GetBlockData(ctx, height)
	if err != nil {
		return nil, err
	}

	daHint, _ := g.GetDAHint(ctx, height)

	return &types.P2PData{
		Data:         data,
		DAHeightHint: daHint,
	}, nil
}

// GetDAHint implements StoreGetter.
func (g *DataStoreGetter) GetDAHint(ctx context.Context, height uint64) (uint64, error) {
	data, err := g.store.GetMetadata(ctx, GetHeightToDAHeightDataKey(height))
	if err != nil {
		return 0, err
	}
	if len(data) != 8 {
		return 0, fmt.Errorf("invalid da hint data length: %d", len(data))
	}
	return binary.LittleEndian.Uint64(data), nil
}

// SetDAHint implements StoreGetter.
func (g *DataStoreGetter) SetDAHint(ctx context.Context, height uint64, daHint uint64) error {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data, daHint)
	return g.store.SetMetadata(ctx, GetHeightToDAHeightDataKey(height), data)
}

// GetByHash implements StoreGetter.
func (g *DataStoreGetter) GetByHash(ctx context.Context, hash []byte) (*types.P2PData, error) {
	_, data, err := g.store.GetBlockByHash(ctx, hash)
	if err != nil {
		return nil, err
	}

	daHint, _ := g.GetDAHint(ctx, data.Height())

	return &types.P2PData{
		Data:         data,
		DAHeightHint: daHint,
	}, nil
}

// Height implements StoreGetter.
func (g *DataStoreGetter) Height(ctx context.Context) (uint64, error) {
	return g.store.Height(ctx)
}

// HasAt implements StoreGetter.
func (g *DataStoreGetter) HasAt(ctx context.Context, height uint64) bool {
	_, _, err := g.store.GetBlockData(ctx, height)
	return err == nil
}

// Type aliases for convenience
type HeaderStoreAdapter = StoreAdapter[*types.P2PSignedHeader]
type DataStoreAdapter = StoreAdapter[*types.P2PData]

// NewHeaderStoreAdapter creates a new StoreAdapter for headers.
// The genesis is used to determine the initial height for efficient Tail lookups.
func NewHeaderStoreAdapter(store Store, gen genesis.Genesis) *HeaderStoreAdapter {
	return NewStoreAdapter(NewHeaderStoreGetter(store), store, gen)
}

// NewDataStoreAdapter creates a new StoreAdapter for data.
// The genesis is used to determine the initial height for efficient Tail lookups.
func NewDataStoreAdapter(store Store, gen genesis.Genesis) *DataStoreAdapter {
	return NewStoreAdapter(NewDataStoreGetter(store), store, gen)
}
