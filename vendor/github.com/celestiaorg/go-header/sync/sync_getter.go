package sync

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/celestiaorg/go-header"
)

// syncGetter is a Getter wrapper that ensure only one Head call happens at the time
type syncGetter[H header.Header[H]] struct {
	getterLk   sync.RWMutex
	isGetterLk atomic.Bool
	header.Getter[H]
}

// Lock locks the getter for single user.
// Reports 'true' if the lock was held by the current routine.
// Does not require unlocking on 'false'.
func (sg *syncGetter[H]) Lock() bool {
	// the lock construction here ensures only one routine is freed at a time
	// while others wait via Rlock
	acquiredLock := sg.getterLk.TryLock()
	if !acquiredLock {
		sg.getterLk.RLock()
		defer sg.getterLk.RUnlock()
		return false
	}
	sg.isGetterLk.Store(acquiredLock)
	return acquiredLock
}

// Unlock unlocks the getter.
func (sg *syncGetter[H]) Unlock() {
	sg.checkLock("Unlock without preceding Lock on syncGetter")
	sg.getterLk.Unlock()
	sg.isGetterLk.Store(false)
}

// Head must be called with held Lock.
func (sg *syncGetter[H]) Head(ctx context.Context, opts ...header.HeadOption[H]) (H, error) {
	sg.checkLock("Head without preceding Lock on syncGetter")
	return sg.Getter.Head(ctx, opts...)
}

// checkLock ensures api safety
func (sg *syncGetter[H]) checkLock(msg string) {
	if !sg.isGetterLk.Load() {
		panic(msg)
	}
}
