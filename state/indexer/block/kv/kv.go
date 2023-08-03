package kv

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	ds "github.com/ipfs/go-datastore"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/state/indexer"
	"github.com/rollkit/rollkit/store"
)

var _ indexer.BlockIndexer = (*BlockerIndexer)(nil)

// BlockerIndexer implements a block indexer, indexing BeginBlock and EndBlock
// events with an underlying KV store. Block events are indexed by their height,
// such that matching search criteria returns the respective block height(s).
type BlockerIndexer struct {
	store ds.TxnDatastore

	ctx context.Context
}

func New(ctx context.Context, store ds.TxnDatastore) *BlockerIndexer {
	return &BlockerIndexer{
		store: store,
		ctx:   ctx,
	}
}

// Has returns true if the given height has been indexed. An error is returned
// upon database query failure.
func (idx *BlockerIndexer) Has(height int64) (bool, error) {
	_, err := idx.store.Get(idx.ctx, ds.NewKey(heightKey(height)))
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
	batch, err := idx.store.NewTransaction(idx.ctx, false)
	if err != nil {
		return fmt.Errorf("failed to create a new batch for transaction: %w", err)
	}
	defer batch.Discard(idx.ctx)

	height := bh.Header.Height

	// 1. index by height
	if err := batch.Put(idx.ctx, ds.NewKey(heightKey(height)), int64ToBytes(height)); err != nil {
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
			if !heightsInitialized {
				filteredHeights, err = idx.matchRange(ctx, qr, ds.NewKey(qr.Key).String(), filteredHeights, true)
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
				filteredHeights, err = idx.matchRange(ctx, qr, ds.NewKey(qr.Key).String(), filteredHeights, false)
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

		startKey := store.GenerateKey([]interface{}{c.CompositeKey, c.Operand})

		if !heightsInitialized {
			filteredHeights, err = idx.match(ctx, c, ds.NewKey(startKey).String(), filteredHeights, true)
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
			filteredHeights, err = idx.match(ctx, c, ds.NewKey(string(startKey)).String(), filteredHeights, false)
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
	startKey string,
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

	results, err := store.PrefixEntries(ctx, idx.store, startKey)
	if err != nil {
		return nil, err
	}

	for result := range results.Next() {
		cont := true

		var (
			v    int64
			vStr string
			err  error
		)

		if qr.Key == types.BlockHeightKey {
			v, err = parseValueFromPrimaryKey(result.Entry.Key)
		} else {
			vStr = parseValueFromEventKey(result.Entry.Key)
			v, err = strconv.ParseInt(vStr, 10, 64)
		}
		if err != nil {
			continue
		}

		if _, ok := qr.AnyBound().(*big.Int); ok {
			include := true
			if lowerBound != nil && v < lowerBound.(*big.Int).Int64() {
				include = false
			}

			if upperBound != nil && v > upperBound.(*big.Int).Int64() {
				include = false
			}

			if include {
				tmpHeights[string(result.Entry.Value)] = result.Entry.Value
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
	startKeyBz string,
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
		results, err := store.PrefixEntries(ctx, idx.store, startKeyBz)
		if err != nil {
			return nil, err
		}

		for result := range results.Next() {
			tmpHeights[string(result.Entry.Value)] = result.Entry.Value

			if err := ctx.Err(); err != nil {
				break
			}
		}

	case c.Op == query.OpExists:

		results, err := store.PrefixEntries(ctx, idx.store, ds.NewKey(c.CompositeKey).String())
		if err != nil {
			return nil, err
		}

		for result := range results.Next() {
			cont := true

			tmpHeights[string(result.Entry.Value)] = result.Entry.Value

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

		results, err := store.PrefixEntries(ctx, idx.store, ds.NewKey(c.CompositeKey).String())
		if err != nil {
			return nil, err
		}

		for result := range results.Next() {
			cont := true

			eventValue := parseValueFromEventKey(result.Entry.Key)

			if strings.Contains(eventValue, c.Operand.(string)) {
				tmpHeights[string(result.Entry.Value)] = result.Entry.Value
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
				key := eventKey(compositeKey, typ, string(attr.Value), height)

				if err := batch.Put(idx.ctx, ds.NewKey(key), heightBz); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
