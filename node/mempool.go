package node

import (
	"fmt"
	"math"

	"github.com/libp2p/go-libp2p-core/peer"
	tmsync "github.com/tendermint/tendermint/libs/sync"
)

const (
	maxActiveIDs = math.MaxUint16
)

type mempoolIDs struct {
	mtx       tmsync.RWMutex
	peerMap   map[peer.ID]uint16
	nextID    uint16              // assumes that a node will never have over 65536 active peers
	activeIDs map[uint16]struct{} // used to check if a given peerID key is used, the value doesn't matter
}

// Reserve searches for the next unused ID and assigns it to the
// peer.
func (ids *mempoolIDs) ReserveForPeer(peer peer.ID) {
	ids.mtx.Lock()
	defer ids.mtx.Unlock()

	curID := ids.nextPeerID()
	ids.peerMap[peer] = curID
	ids.activeIDs[curID] = struct{}{}
}

// nextPeerID returns the next unused peer ID to use.
// This assumes that ids's mutex is already locked.
func (ids *mempoolIDs) nextPeerID() uint16 {
	if len(ids.activeIDs) == maxActiveIDs {
		panic(fmt.Sprintf("node has maximum %d active IDs and wanted to get one more", maxActiveIDs))
	}

	_, idExists := ids.activeIDs[ids.nextID]
	for idExists {
		ids.nextID++
		_, idExists = ids.activeIDs[ids.nextID]
	}
	curID := ids.nextID
	ids.nextID++
	return curID
}

// Reclaim returns the ID reserved for the peer back to unused pool.
func (ids *mempoolIDs) Reclaim(peer peer.ID) {
	ids.mtx.Lock()
	defer ids.mtx.Unlock()

	removedID, ok := ids.peerMap[peer]
	if ok {
		delete(ids.activeIDs, removedID)
		delete(ids.peerMap, peer)
	}
}

// GetForPeer returns an ID for the peer. ID is generated if required.
func (ids *mempoolIDs) GetForPeer(peer peer.ID) uint16 {
	ids.mtx.Lock()
	defer ids.mtx.Unlock()

	id, ok := ids.peerMap[peer]
	if !ok {
		id = ids.nextPeerID()
		ids.peerMap[peer] = id
		ids.activeIDs[id] = struct{}{}
	}

	return id
}

func newMempoolIDs() *mempoolIDs {
	return &mempoolIDs{
		peerMap:   make(map[peer.ID]uint16),
		activeIDs: map[uint16]struct{}{0: {}},
		nextID:    1, // reserve unknownPeerID(0) for mempoolReactor.BroadcastTx
	}
}
