package node

import (
	"sync"
)

// Used by header and block exchange service for keeping track of
// the status of the syncer in them.
type SyncerStatus struct {
	started bool
	m       sync.Mutex
}

func (syncerStatus *SyncerStatus) setStarted() {
	syncerStatus.m.Lock()
	defer syncerStatus.m.Unlock()
	syncerStatus.started = true
}

func (syncerStatus *SyncerStatus) getStarted() bool {
	syncerStatus.m.Lock()
	defer syncerStatus.m.Unlock()
	return syncerStatus.started
}
