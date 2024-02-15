package p2p

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerIDStore is a utility for persisting peer IDs of good peers to a datastore.
type PeerIDStore interface {
	// Put stores the given peer IDs.
	Put(ctx context.Context, peers []peer.ID) error
	// Load loads the peer IDs from the store.
	Load(ctx context.Context) ([]peer.ID, error)
}
