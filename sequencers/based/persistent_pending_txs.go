package based

import (
	"context"
	"encoding/json"
	"time"

	datastore "github.com/ipfs/go-datastore"
)

// TxsWithTimestamp is a struct that holds transactions and their IDs along with a timestamp.
type TxsWithTimestamp struct {
	Txs       [][]byte
	IDs       [][]byte
	Timestamp time.Time
}

// PersistentPendingTxs is a persistent queue for pending transactions.
type PersistentPendingTxs struct {
	store datastore.Batching
	list  []TxsWithTimestamp
}

// NewPersistentPendingTxs creates a new PersistentPendingTxs instance.
func NewPersistentPendingTxs(store datastore.Batching) (*PersistentPendingTxs, error) {
	pt := &PersistentPendingTxs{store: store, list: []TxsWithTimestamp{}}
	if err := pt.Load(); err != nil && err != datastore.ErrNotFound {
		return nil, err
	}
	return pt, nil
}

// Push adds transactions to the pending queue with a timestamp.
func (pt *PersistentPendingTxs) Push(txs [][]byte, ids [][]byte, timestamp time.Time) error {
	pt.list = append(pt.list, TxsWithTimestamp{Txs: txs, IDs: ids, Timestamp: timestamp})
	return pt.Save()
}

// PopUpToMaxBytes pops transactions from the queue until the total size is less than maxBytes.
func (pt *PersistentPendingTxs) PopUpToMaxBytes(maxBytes uint64) ([][]byte, [][]byte, uint64, time.Time) {
	var (
		poppedTxs [][]byte
		ids       [][]byte
		totalSize uint64
		timestamp time.Time = time.Now()
	)

	for len(pt.list) > 0 {
		first := pt.list[0]
		timestamp = first.Timestamp
		for i, tx := range first.Txs {
			txSize := uint64(len(tx))
			if totalSize+txSize > maxBytes {
				pt.list[0] = TxsWithTimestamp{
					Txs:       first.Txs[i:],
					IDs:       first.IDs[i:],
					Timestamp: first.Timestamp,
				}
				pt.Save()
				return poppedTxs, ids, totalSize, timestamp
			}
			poppedTxs = append(poppedTxs, tx)
			ids = append(ids, first.IDs[i])
			totalSize += txSize
		}
		pt.list = pt.list[1:]
	}
	pt.Save()
	return poppedTxs, ids, totalSize, timestamp
}

// Save saves the pending transactions to the datastore.
func (pt *PersistentPendingTxs) Save() error {
	data, err := json.Marshal(pt.list)
	if err != nil {
		return err
	}
	return pt.store.Put(context.Background(), datastore.NewKey(dsPendingTxsKey), data)
}

// Load loads the pending transactions from the datastore.
func (pt *PersistentPendingTxs) Load() error {
	data, err := pt.store.Get(context.Background(), datastore.NewKey(dsPendingTxsKey))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &pt.list)
}
