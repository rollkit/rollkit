package kv

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/libs/pubsub/query/syntax"
	"github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/state"
	"github.com/rollkit/rollkit/state/indexer"
	"github.com/rollkit/rollkit/state/txindex"
	"github.com/rollkit/rollkit/store"
)

const (
	tagKeySeparator = "/"
)

var _ txindex.TxIndexer = (*TxIndex)(nil)

// TxIndex is the simplest possible indexer, backed by key-value storage (levelDB).
type TxIndex struct {
	store ds.TxnDatastore

	ctx context.Context
}

// NewTxIndex creates new KV indexer.
func NewTxIndex(ctx context.Context, store ds.TxnDatastore) *TxIndex {
	return &TxIndex{
		store: store,
		ctx:   ctx,
	}
}

// Get gets transaction from the TxIndex storage and returns it or nil if the
// transaction is not found.
func (txi *TxIndex) Get(hash []byte) (*abci.TxResult, error) {
	if len(hash) == 0 {
		return nil, txindex.ErrorEmptyHash
	}

	rawBytes, err := txi.store.Get(txi.ctx, ds.NewKey(hex.EncodeToString(hash)))
	if err != nil {
		if errors.Is(err, ds.ErrNotFound) {
			return nil, nil
		}
		panic(err)
	}
	if rawBytes == nil {
		return nil, nil
	}

	txResult := new(abci.TxResult)
	err = proto.Unmarshal(rawBytes, txResult)
	if err != nil {
		return nil, fmt.Errorf("error reading TxResult: %w", err)
	}

	return txResult, nil
}

// AddBatch indexes a batch of transactions using the given list of events. Each
// key that indexed from the tx's events is a composite of the event type and
// the respective attribute's key delimited by a "." (eg. "account.number").
// Any event with an empty type is not indexed.
func (txi *TxIndex) AddBatch(b *txindex.Batch) error {
	storeBatch, err := txi.store.NewTransaction(txi.ctx, false)
	if err != nil {
		return fmt.Errorf("failed to create a new batch for transaction: %w", err)
	}
	defer storeBatch.Discard(txi.ctx)

	for _, result := range b.Ops {
		hash := types.Tx(result.Tx).Hash()

		// index tx by events
		err := txi.indexEvents(result, hash, storeBatch)
		if err != nil {
			return err
		}

		// index by height (always)
		err = storeBatch.Put(txi.ctx, ds.NewKey(keyForHeight(result)), hash)
		if err != nil {
			return err
		}

		rawBytes, err := proto.Marshal(result)
		if err != nil {
			return err
		}
		// index by hash (always)
		err = storeBatch.Put(txi.ctx, ds.NewKey(hex.EncodeToString(hash)), rawBytes)
		if err != nil {
			return err
		}
	}

	return storeBatch.Commit(txi.ctx)
}

// Index indexes a single transaction using the given list of events. Each key
// that indexed from the tx's events is a composite of the event type and the
// respective attribute's key delimited by a "." (eg. "account.number").
// Any event with an empty type is not indexed.
func (txi *TxIndex) Index(result *abci.TxResult) error {
	b, err := txi.store.NewTransaction(txi.ctx, false)
	if err != nil {
		return fmt.Errorf("failed to create a new batch for transaction: %w", err)
	}
	defer b.Discard(txi.ctx)

	hash := types.Tx(result.Tx).Hash()

	// index tx by events
	err = txi.indexEvents(result, hash, b)
	if err != nil {
		return err
	}

	// index by height (always)
	err = b.Put(txi.ctx, ds.NewKey(keyForHeight(result)), hash)
	if err != nil {
		return err
	}

	rawBytes, err := proto.Marshal(result)
	if err != nil {
		return err
	}
	// index by hash (always)
	err = b.Put(txi.ctx, ds.NewKey(hex.EncodeToString(hash)), rawBytes)
	if err != nil {
		return err
	}

	return b.Commit(txi.ctx)
}

func (txi *TxIndex) indexEvents(result *abci.TxResult, hash []byte, store ds.Txn) error {
	for _, event := range result.Result.Events {
		// only index events with a non-empty type
		if len(event.Type) == 0 {
			continue
		}

		for _, attr := range event.Attributes {
			if len(attr.Key) == 0 {
				continue
			}

			// index if `index: true` is set
			compositeTag := event.Type + "." + attr.Key
			if attr.GetIndex() {
				err := store.Put(txi.ctx, ds.NewKey(keyForEvent(compositeTag, attr.Value, result)), hash)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Search performs a search using the given query.
//
// It breaks the query into conditions (like "tx.height > 5"). For each
// condition, it queries the DB index. One special use cases here: (1) if
// "tx.hash" is found, it returns tx result for it (2) for range queries it is
// better for the client to provide both lower and upper bounds, so we are not
// performing a full scan. Results from querying indexes are then intersected
// and returned to the caller, in no particular order.
//
// Search will exit early and return any result fetched so far,
// when a message is received on the context chan.
func (txi *TxIndex) Search(ctx context.Context, q *query.Query) ([]*abci.TxResult, error) {
	select {
	case <-ctx.Done():
		return make([]*abci.TxResult, 0), nil

	default:
	}

	var hashesInitialized bool
	filteredHashes := make(map[string][]byte)

	// get a list of conditions (like "tx.height > 5")
	conditions := q.Syntax()

	// if there is a hash condition, return the result immediately
	hash, ok, err := lookForHash(conditions)
	if err != nil {
		return nil, fmt.Errorf("error during searching for a hash in the query: %w", err)
	} else if ok {
		res, err := txi.Get(hash)
		switch {
		case err != nil:
			return []*abci.TxResult{}, fmt.Errorf("error while retrieving the result: %w", err)
		case res == nil:
			return []*abci.TxResult{}, nil
		default:
			return []*abci.TxResult{res}, nil
		}
	}

	// conditions to skip because they're handled before "everything else"
	skipIndexes := make([]int, 0)
	var heightInfo HeightInfo

	// If we are not matching events and tx.height = 3 occurs more than once, the later value will
	// overwrite the first one.
	conditions, heightInfo = dedupHeight(conditions)

	if !heightInfo.onlyHeightEq {
		skipIndexes = append(skipIndexes, heightInfo.heightEqIdx)
	}

	// extract ranges
	// if both upper and lower bounds exist, it's better to get them in order not
	// no iterate over kvs that are not within range.
	ranges, rangeIndexes, heightRange := indexer.LookForRangesWithHeight(conditions)
	heightInfo.heightRange = heightRange
	if len(ranges) > 0 {
		skipIndexes = append(skipIndexes, rangeIndexes...)

		for _, qr := range ranges {
			// If we have a query range over height and want to still look for
			// specific event values we do not want to simply return all
			// transactios in this height range. We remember the height range info
			// and pass it on to match() to take into account when processing events.
			if qr.Key == types.TxHeightKey && !heightInfo.onlyHeightRange {
				continue
			}
			if !hashesInitialized {
				filteredHashes = txi.matchRange(ctx, qr, startKey(qr.Key), filteredHashes, true, heightInfo)
				hashesInitialized = true

				// Ignore any remaining conditions if the first condition resulted
				// in no matches (assuming implicit AND operand).
				if len(filteredHashes) == 0 {
					break
				}
			} else {
				filteredHashes = txi.matchRange(ctx, qr, startKey(qr.Key), filteredHashes, false, heightInfo)
			}
		}
	}

	// for all other conditions
	for i, c := range conditions {
		if intInSlice(i, skipIndexes) {
			continue
		}

		if !hashesInitialized {
			filteredHashes = txi.match(ctx, c, startKeyForCondition(c, heightInfo.height), filteredHashes, true)
			hashesInitialized = true

			// Ignore any remaining conditions if the first condition resulted
			// in no matches (assuming implicit AND operand).
			if len(filteredHashes) == 0 {
				break
			}
		} else {
			filteredHashes = txi.match(ctx, c, startKeyForCondition(c, heightInfo.height), filteredHashes, false)
		}
	}

	results := make([]*abci.TxResult, 0, len(filteredHashes))
	resultMap := make(map[string]struct{})
RESULTS_LOOP:
	for _, h := range filteredHashes {

		res, err := txi.Get(h)
		if err != nil {
			return nil, fmt.Errorf("failed to get Tx{%X}: %w", h, err)
		}
		hashString := string(h)
		if _, ok := resultMap[hashString]; !ok {
			resultMap[hashString] = struct{}{}
			results = append(results, res)
		}
		// Potentially exit early.
		select {
		case <-ctx.Done():
			break RESULTS_LOOP
		default:
		}
	}

	return results, nil
}

func lookForHash(conditions []syntax.Condition) (hash []byte, ok bool, err error) {
	for _, c := range conditions {
		if c.Tag == types.TxHashKey {
			decoded, err := hex.DecodeString(c.Arg.Value())
			return decoded, true, err
		}
	}
	return
}

// match returns all matching txs by hash that meet a given condition and start
// key. An already filtered result (filteredHashes) is provided such that any
// non-intersecting matches are removed.
//
// NOTE: filteredHashes may be empty if no previous condition has matched.
func (txi *TxIndex) match(
	ctx context.Context,
	c syntax.Condition,
	startKeyBz string,
	filteredHashes map[string][]byte,
	firstRun bool,
) map[string][]byte {
	// A previous match was attempted but resulted in no matches, so we return
	// no matches (assuming AND operand).
	if !firstRun && len(filteredHashes) == 0 {
		return filteredHashes
	}

	tmpHashes := make(map[string][]byte)

	switch {
	case c.Op == syntax.TEq:
		results, err := store.PrefixEntries(ctx, txi.store, startKeyBz)
		if err != nil {
			panic(err)
		}

		for result := range results.Next() {
			cont := true

			tmpHashes[string(result.Entry.Value)] = result.Entry.Value

			// Potentially exit early.
			select {
			case <-ctx.Done():
				cont = false
			default:
			}

			if !cont {
				break
			}
		}

	case c.Op == syntax.TExists:
		// XXX: can't use startKeyBz here because c.Operand is nil
		// (e.g. "account.owner/<nil>/" won't match w/ a single row)
		results, err := store.PrefixEntries(ctx, txi.store, startKey(c.Tag))
		if err != nil {
			panic(err)
		}

		for result := range results.Next() {
			cont := true

			tmpHashes[string(result.Entry.Value)] = result.Entry.Value

			// Potentially exit early.
			select {
			case <-ctx.Done():
				cont = false
			default:
			}

			if !cont {
				break
			}
		}

	case c.Op == syntax.TContains:
		// XXX: startKey does not apply here.
		// For example, if startKey = "account.owner/an/" and search query = "account.owner CONTAINS an"
		// we can't iterate with prefix "account.owner/an/" because we might miss keys like "account.owner/Ulan/"
		results, err := store.PrefixEntries(ctx, txi.store, startKey(c.Tag))
		if err != nil {
			panic(err)
		}

		for result := range results.Next() {
			cont := true

			if !isTagKey([]byte(result.Entry.Key)) {
				continue
			}

			if strings.Contains(extractValueFromKey([]byte(result.Entry.Key)), c.Arg.Value()) {
				tmpHashes[string(result.Entry.Value)] = result.Entry.Value
			}

			// Potentially exit early.
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
		panic("other operators should be handled already")
	}

	if len(tmpHashes) == 0 || firstRun {
		// Either:
		//
		// 1. Regardless if a previous match was attempted, which may have had
		// results, but no match was found for the current condition, then we
		// return no matches (assuming AND operand).
		//
		// 2. A previous match was not attempted, so we return all results.
		return tmpHashes
	}

	// Remove/reduce matches in filteredHashes that were not found in this
	// match (tmpHashes).
	for k := range filteredHashes {
		cont := true

		if tmpHashes[k] == nil {
			delete(filteredHashes, k)

			// Potentially exit early.
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

	return filteredHashes
}

// matchRange returns all matching txs by hash that meet a given queryRange and
// start key. An already filtered result (filteredHashes) is provided such that
// any non-intersecting matches are removed.
//
// NOTE: filteredHashes may be empty if no previous condition has matched.
func (txi *TxIndex) matchRange(
	ctx context.Context,
	qr indexer.QueryRange,
	startKey string,
	filteredHashes map[string][]byte,
	firstRun bool,
	heightInfo HeightInfo,
) map[string][]byte {
	// A previous match was attempted but resulted in no matches, so we return
	// no matches (assuming AND operand).
	if !firstRun && len(filteredHashes) == 0 {
		return filteredHashes
	}

	tmpHashes := make(map[string][]byte)

	results, err := store.PrefixEntries(ctx, txi.store, startKey)
	if err != nil {
		panic(err)
	}

LOOP:
	for result := range results.Next() {
		cont := true

		if !isTagKey([]byte(result.Entry.Key)) {
			continue
		}

		if _, ok := qr.AnyBound().(*big.Float); ok {
			v := new(big.Int)
			v, ok := v.SetString(extractValueFromKey([]byte(result.Entry.Key)), 10)
			var vF *big.Float
			if !ok {
				vF, _, err = big.ParseFloat(extractValueFromKey([]byte(result.Entry.Key)), 10, 125, big.ToNearestEven)
				if err != nil {
					continue LOOP
				}

			}
			if qr.Key != types.TxHeightKey {
				keyHeight, err := extractHeightFromKey([]byte(result.Entry.Key))
				if err != nil {
					// txi.log.Error("failure to parse height from key:", err)
					continue
				}
				withinBounds, err := checkHeightConditions(heightInfo, keyHeight)
				if err != nil {
					// txi.log.Error("failure checking for height bounds:", err)
					continue
				}
				if !withinBounds {
					continue
				}
			}
			var withinBounds bool
			var err error
			if !ok {
				withinBounds, err = state.CheckBounds(qr, vF)
			} else {
				withinBounds, err = state.CheckBounds(qr, v)
			}
			if err != nil {

			} else {
				if withinBounds {
					tmpHashes[string(result.Entry.Value)] = result.Entry.Value
				}
			}

			// XXX: passing time in a ABCI Events is not yet implemented
			// case time.Time:
			// 	v := strconv.ParseInt(extractValueFromKey(it.Key()), 10, 64)
			// 	if v == r.upperBound {
			// 		break
			// 	}
		}

		// Potentially exit early.
		select {
		case <-ctx.Done():
			cont = false
		default:
		}

		if !cont {
			break
		}
	}

	if len(tmpHashes) == 0 || firstRun {
		// Either:
		//
		// 1. Regardless if a previous match was attempted, which may have had
		// results, but no match was found for the current condition, then we
		// return no matches (assuming AND operand).
		//
		// 2. A previous match was not attempted, so we return all results.
		return tmpHashes
	}

	// Remove/reduce matches in filteredHashes that were not found in this
	// match (tmpHashes).
	for k := range filteredHashes {
		cont := true

		if tmpHashes[k] == nil {
			delete(filteredHashes, k)

			// Potentially exit early.
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

	return filteredHashes
}

// Keys

func isTagKey(key []byte) bool {
	return strings.Count(string(key), tagKeySeparator) == 4
}

func extractValueFromKey(key []byte) string {
	parts := strings.SplitN(string(key), tagKeySeparator, 4)
	return parts[2]
}

func extractHeightFromKey(key []byte) (int64, error) {
	parts := strings.SplitN(string(key), tagKeySeparator, -1)

	return strconv.ParseInt(parts[len(parts)-2], 10, 64)
}

func keyForEvent(key string, value string, result *abci.TxResult) string {
	return fmt.Sprintf("%s/%s/%d/%d",
		key,
		value,
		result.Height,
		result.Index,
	)
}

func keyForHeight(result *abci.TxResult) string {
	return fmt.Sprintf("%s/%d/%d/%d",
		types.TxHeightKey,
		result.Height,
		result.Height,
		result.Index,
	)
}

func startKeyForCondition(c syntax.Condition, height int64) string {
	if height > 0 {
		return startKey(c.Tag, c.Arg.Value(), strconv.FormatInt(height, 10))
	}
	return startKey(c.Tag, c.Arg.Value())
}

func startKey(fields ...string) string {
	return store.GenerateKey(fields)
}
