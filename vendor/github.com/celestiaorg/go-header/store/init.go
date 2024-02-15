package store

import (
	"context"
	"errors"

	"github.com/celestiaorg/go-header"
)

// Init ensures a Store is initialized. If it is not already initialized,
// it initializes the Store by requesting the header with the given hash.
func Init[H header.Header[H]](ctx context.Context, store header.Store[H], ex header.Exchange[H], hash header.Hash) error {
	_, err := store.Head(ctx)
	switch {
	default:
		return err
	case errors.Is(err, header.ErrNoHead):
		initial, err := ex.Get(ctx, hash)
		if err != nil {
			return err
		}

		return store.Init(ctx, initial)
	}
}
