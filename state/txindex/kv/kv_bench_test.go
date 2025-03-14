package kv

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/libs/pubsub/query"
	"github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/store"
)

func BenchmarkTxSearch(b *testing.B) {
	dbDir := b.TempDir()

	db, err := store.NewDefaultKVStore(dbDir, "db", "benchmark_tx_search_test")
	if err != nil {
		b.Errorf("failed to create database: %s", err)
	}

	indexer := NewTxIndex(context.Background(), db)

	for i := 0; i < 35000; i++ {
		events := []abci.Event{
			{
				Type: "transfer",
				Attributes: []abci.EventAttribute{
					{Key: string("address"), Value: string(fmt.Sprintf("address_%d", i%100)), Index: true},
					{Key: string("amount"), Value: string("50"), Index: true},
				},
			},
		}

		txBz := make([]byte, 8)
		if _, err := rand.Read(txBz); err != nil {
			b.Errorf("failed produce random bytes: %s", err)
		}

		txResult := &abci.TxResult{
			Height: int64(i),
			Index:  0,
			Tx:     types.Tx(string(txBz)),
			Result: abci.ExecTxResult{
				Data:   []byte{0},
				Code:   abci.CodeTypeOK,
				Log:    "",
				Events: events,
			},
		}

		if err := indexer.Index(txResult); err != nil {
			b.Errorf("failed to index tx: %s", err)
		}
	}

	txQuery := query.MustCompile("transfer.address = 'address_43' AND transfer.amount = 50")

	b.ResetTimer()

	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		if _, err := indexer.Search(ctx, txQuery); err != nil {
			b.Errorf("failed to query for txs: %s", err)
		}
	}
}
