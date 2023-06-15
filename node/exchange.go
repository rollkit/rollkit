package node

import (
	"sync"
)

type SyncerStatus struct {
	started bool
	m       sync.Mutex
}
