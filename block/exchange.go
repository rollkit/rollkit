package block

import (
	"sync"
)

// Used by header and block exchange service for keeping track of
// the status of the syncer in them.
type SyncerStatus struct {
	started bool
	m       sync.RWMutex
}

func (syncerStatus *SyncerStatus) isStarted() bool {
	syncerStatus.m.RLock()
	defer syncerStatus.m.RUnlock()
	return syncerStatus.started
}
