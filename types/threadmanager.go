package types

import "sync"

type ThreadManager struct {
	wg sync.WaitGroup
}

func NewThreadManager() *ThreadManager {
	return &ThreadManager{}
}

func (tm *ThreadManager) Wait() {
	tm.wg.Wait()
}

func (tm *ThreadManager) Go(f func()) {
	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		f()
	}()
}
