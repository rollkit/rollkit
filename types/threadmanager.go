package types

import "sync"

// ThreadManager is a simple wrapper around sync.WaitGroup to make it easier to
// manage threads
type ThreadManager struct {
	wg sync.WaitGroup
}

// NewThreadManager creates a new ThreadManager
func NewThreadManager() *ThreadManager {
	return &ThreadManager{}
}

// Wait blocks until all goroutines have called Done on the WaitGroup
func (tm *ThreadManager) Wait() {
	tm.wg.Wait()
}

// Go launches a goroutine and adds it to the WaitGroup
func (tm *ThreadManager) Go(f func()) {
	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		f()
	}()
}
