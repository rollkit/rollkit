package kv

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/orderedcode"
	ds "github.com/ipfs/go-datastore"
	badger3 "github.com/ipfs/go-ds-badger3"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/pubsub/query"
	"github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/rollmint/state/indexer"
	"github.com/celestiaorg/rollmint/store"
)

var _ indexer.BlockIndexer = (*BlockerIndexer)(nil)

// BlockerIndexer implements a block indexer, indexing BeginBlock and EndBlock
// events with an underlying KV store. Block events are indexed by their height,
// such that matching search criteria returns the respective block height(s).
type BlockerIndexer struct {
	store ds.Datastore

	ctx context.Context
}

func New(ctx context.Context, store ds.Datastore) *BlockerIndexer {
	return &BlockerIndexer{
		store: store,
		ctx:   ctx,
	}
}

// Has returns true if the given height has been indexed. An error is returned
// upon database query failure.
func (idx *BlockerIndexer) Has(height int64) (bool, error) {
	key, err := heightKey(height)
	if err != nil {
		return false, fmt.Errorf("failed to create block height index key: %w", err)
	}

	_, err = idx.store.Get(idx.ctx, ds.NewKey(string(key)))
	if err == ds.ErrNotFound {
		return false, nil
	}
	return err == nil, err
}

// Index indexes BeginBlock and EndBlock events for a given block by its height.
// The following is indexed:
//
// primary key: encode(block.height | height) => encode(height)
// BeginBlock events: encode(eventType.eventAttr|eventValue|height|begin_block) => encode(height)
// EndBlock events: encode(eventType.eventAttr|eventValue|height|end_block) => encode(height)
func (idx *BlockerIndexer) Index(bh types.EventDataNewBlockHeader) error {
	badgerDS, ok := idx.store.(*badger3.Datastore)
	if !ok {
		errors.New("failed to retrieve the ds.Datastore implementation")
	}
	fmt.Println("so far good")
	batch, err := badgerDS.NewTransaction(idx.ctx, false)
	fmt.Println("so far not so good")
	if err != nil {
		return fmt.Errorf("failed to create a new batch for transaction: %w", err)
	}
	defer batch.Discard(idx.ctx)

	height := bh.Header.Height

	// 1. index by height
	key, err := heightKey(height)
	if err != nil {
		return fmt.Errorf("failed to create block height index key: %w", err)
	}
	if err := batch.Put(idx.ctx, ds.NewKey(string(key)), int64ToBytes(height)); err != nil {
		return err
	}

	// 2. index BeginBlock events
	if err := idx.indexEvents(batch, bh.ResultBeginBlock.Events, "begin_block", height); err != nil {
		return fmt.Errorf("failed to index BeginBlock events: %w", err)
	}

	// 3. index EndBlock events
	if err := idx.indexEvents(batch, bh.ResultEndBlock.Events, "end_block", height); err != nil {
		return fmt.Errorf("failed to index EndBlock events: %w", err)
	}

	return batch.Commit(idx.ctx)
}

// Search performs a query for block heights that match a given BeginBlock
// and Endblock event search criteria. The given query can match against zero,
// one or more block heights. In the case of height queries, i.e. block.height=H,
// if the height is indexed, that height alone will be returned. An error and
// nil slice is returned. Otherwise, a non-nil slice and nil error is returned.
func (idx *BlockerIndexer) Search(ctx context.Context, q *query.Query) ([]int64, error) {
	results := make([]int64, 0)
	select {
	case <-ctx.Done():
		return results, nil

	default:
	}

	conditions, err := q.Conditions()
	if err != nil {
		return nil, fmt.Errorf("failed to parse query conditions: %w", err)
	}

	// If there is an exact height query, return the result immediately
	// (if it exists).
	height, ok := lookForHeight(conditions)
	if ok {
		ok, err := idx.Has(height)
		if err != nil {
			return nil, err
		}

		if ok {
			return []int64{height}, nil
		}

		return results, nil
	}

	var heightsInitialized bool
	filteredHeights := make(map[string][]byte)

	// conditions to skip because they're handled before "everything else"
	skipIndexes := make([]int, 0)

	// Extract ranges. If both upper and lower bounds exist, it's better to get
	// them in order as to not iterate over kvs that are not within range.
	ranges, rangeIndexes := indexer.LookForRanges(conditions)
	if len(ranges) > 0 {
		skipIndexes = append(skipIndexes, rangeIndexes...)

		for _, qr := range ranges {
			prefix, err := orderedcode.Append(nil, qr.Key)
			if err != nil {
				return nil, fmt.Errorf("failed to create prefix key: %w", err)
			}

			if !heightsInitialized {
				filteredHeights, err = idx.matchRange(ctx, qr, prefix, filteredHeights, true)
				if err != nil {
					return nil, err
				}

				heightsInitialized = true

				// Ignore any remaining conditions if the first condition resulted in no
				// matches (assuming implicit AND operand).
				if len(filteredHeights) == 0 {
					break
				}
			} else {
				filteredHeights, err = idx.matchRange(ctx, qr, prefix, filteredHeights, false)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// for all other conditions
	for i, c := range conditions {
		if intInSlice(i, skipIndexes) {
			continue
		}

		startKey, err := orderedcode.Append(nil, c.CompositeKey, fmt.Sprintf("%v", c.Operand))
		if err != nil {
			return nil, err
		}

		if !heightsInitialized {
			filteredHeights, err = idx.match(ctx, c, startKey, filteredHeights, true)
			if err != nil {
				return nil, err
			}

			heightsInitialized = true

			// Ignore any remaining conditions if the first condition resulted in no
			// matches (assuming implicit AND operand).
			if len(filteredHeights) == 0 {
				break
			}
		} else {
			filteredHeights, err = idx.match(ctx, c, startKey, filteredHeights, false)
			if err != nil {
				return nil, err
			}
		}
	}

	// fetch matching heights
	results = make([]int64, 0, len(filteredHeights))
	for _, hBz := range filteredHeights {
		cont := true

		h := int64FromBytes(hBz)

		ok, err := idx.Has(h)
		if err != nil {
			return nil, err
		}
		if ok {
			results = append(results, h)
		}

		select {
		case <-ctx.Done():
			cont = false

		default:
		}
		if !cont {
			break
		}
	}

	sort.Slice(results, func(i, j int) bool { return results[i] < results[j] })

	return results, nil
}

// matchRange returns all matching block heights that match a given QueryRange
// and start key. An already filtered result (filteredHeights) is provided such
// that any non-intersecting matches are removed.
//
// NOTE: The provided filteredHeights may be empty if no previous condition has
// matched.
func (idx *BlockerIndexer) matchRange(
	ctx context.Context,
	qr indexer.QueryRange,
	startKey []byte,
	filteredHeights map[string][]byte,
	firstRun bool,
) (map[string][]byte, error) {

	// A previous match was attempted but resulted in no matches, so we return
	// no matches (assuming AND operand).
	if !firstRun && len(filteredHeights) == 0 {
		return filteredHeights, nil
	}

	tmpHeights := make(map[string][]byte)
	lowerBound := qr.LowerBoundValue()
	upperBound := qr.UpperBoundValue()

	entries, err := store.PrefixEntries(ctx, idx.store, string(startKey))
	if err != nil {
		return nil, err
	}

LOOP:
	for _, entry := range entries {
		cont := true

		var (
			eventValue string
			err        error
		)

		if qr.Key == types.BlockHeightKey {
			eventValue, err = parseValueFromPrimaryKey([]byte(entry.Key))
		} else {
			eventValue, err = parseValueFromEventKey([]byte(entry.Key))
		}

		if err != nil {
			continue
		}

		if _, ok := qr.AnyBound().(int64); ok {
			v, err := strconv.ParseInt(eventValue, 10, 64)
			if err != nil {
				continue LOOP
			}

			include := true
			if lowerBound != nil && v < lowerBound.(int64) {
				include = false
			}

			if upperBound != nil && v > upperBound.(int64) {
				include = false
			}

			if include {
				tmpHeights[string(entry.Value)] = entry.Value
			}
		}

		select {
		case <-ctx.Done():
			cont = false

		default:
		}

		if !cont {
			break
		}
	}

	if len(tmpHeights) == 0 || firstRun {
		// Either:
		//
		// 1. Regardless if a previous match was attempted, which may have had
		// results, but no match was found for the current condition, then we
		// return no matches (assuming AND operand).
		//
		// 2. A previous match was not attempted, so we return all results.
		return tmpHeights, nil
	}

	// Remove/reduce matches in filteredHashes that were not found in this
	// match (tmpHashes).
	for k := range filteredHeights {
		cont := true

		if tmpHeights[k] == nil {
			delete(filteredHeights, k)

			select {
			case <-ctx.Done():
				cont = false

			default:
			}
		}

		if !cont {
			break
		}
	}

	return filteredHeights, nil
}

// match returns all matching heights that meet a given query condition and start
// key. An already filtered result (filteredHeights) is provided such that any
// non-intersecting matches are removed.
//
// NOTE: The provided filteredHeights may be empty if no previous condition has
// matched.
func (idx *BlockerIndexer) match(
	ctx context.Context,
	c query.Condition,
	startKeyBz []byte,
	filteredHeights map[string][]byte,
	firstRun bool,
) (map[string][]byte, error) {

	// A previous match was attempted but resulted in no matches, so we return
	// no matches (assuming AND operand).
	if !firstRun && len(filteredHeights) == 0 {
		return filteredHeights, nil
	}

	tmpHeights := make(map[string][]byte)

	switch {
	case c.Op == query.OpEqual:
		entries, err := store.PrefixEntries(ctx, idx.store, string(startKeyBz))
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			tmpHeights[string(entry.Value)] = entry.Value

			if err := ctx.Err(); err != nil {
				break
			}
		}

	case c.Op == query.OpExists:
		prefix, err := orderedcode.Append(nil, c.CompositeKey)
		if err != nil {
			return nil, err
		}

		entries, err := store.PrefixEntries(ctx, idx.store, string(prefix))
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			cont := true

			tmpHeights[string(entry.Value)] = entry.Value

			select {
			case <-ctx.Done():
				cont = false

			default:
			}

			if !cont {
				break
			}
		}

	case c.Op == query.OpContains:
		prefix, err := orderedcode.Append(nil, c.CompositeKey)
		if err != nil {
			return nil, err
		}

		entries, err := store.PrefixEntries(ctx, idx.store, string(prefix))
		if err != nil {
			return nil, err
		}

		for _, entry := range entries {
			cont := true

			eventValue, err := parseValueFromEventKey([]byte(entry.Key))
			if err != nil {
				continue
			}

			if strings.Contains(eventValue, c.Operand.(string)) {
				tmpHeights[string(entry.Value)] = entry.Value
			}

			select {
			case <-ctx.Done():
				cont = false

			default:
			}

			if !cont {
				break
			}
		}

	default:
		return nil, errors.New("other operators should be handled already")
	}

	if len(tmpHeights) == 0 || firstRun {
		// Either:
		//
		// 1. Regardless if a previous match was attempted, which may have had
		// results, but no match was found for the current condition, then we
		// return no matches (assuming AND operand).
		//
		// 2. A previous match was not attempted, so we return all results.
		return tmpHeights, nil
	}

	// Remove/reduce matches in filteredHeights that were not found in this
	// match (tmpHeights).
	for k := range filteredHeights {
		cont := true

		if tmpHeights[k] == nil {
			delete(filteredHeights, k)

			select {
			case <-ctx.Done():
				cont = false

			default:
			}
		}

		if !cont {
			break
		}
	}

	return filteredHeights, nil
}

func (idx *BlockerIndexer) indexEvents(batch ds.Txn, events []abci.Event, typ string, height int64) error {
	heightBz := int64ToBytes(height)

	for _, event := range events {
		// only index events with a non-empty type
		if len(event.Type) == 0 {
			continue
		}

		for _, attr := range event.Attributes {
			if len(attr.Key) == 0 {
				continue
			}

			// index iff the event specified index:true and it's not a reserved event
			compositeKey := fmt.Sprintf("%s.%s", event.Type, string(attr.Key))
			if compositeKey == types.BlockHeightKey {
				return fmt.Errorf("event type and attribute key \"%s\" is reserved; please use a different key", compositeKey)
			}

			if attr.GetIndex() {
				key, err := eventKey(compositeKey, typ, string(attr.Value), height)
				if err != nil {
					return fmt.Errorf("failed to create block index key: %w", err)
				}

				if err := batch.Put(idx.ctx, ds.NewKey(string(key)), heightBz); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
