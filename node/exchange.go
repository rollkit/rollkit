package node

import (
	"sync"
)

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
