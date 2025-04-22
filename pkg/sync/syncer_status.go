package sync

import "sync/atomic"

// SyncerStatus is used by header and block exchange service for keeping track
// of the status of the syncer in them.
type SyncerStatus struct {
	started atomic.Bool
}

func (syncerStatus *SyncerStatus) isStarted() bool {
	return syncerStatus.started.Load()
}
