package types

import (
	"testing"
	"time"
)

func TestThreadManager(t *testing.T) {
	tm := NewThreadManager()
	start := time.Now()

	// Launch a goroutine that waits for 10ms
	tm.Go(func() {
		time.Sleep(10 * time.Millisecond)
	})
	// Launch a goroutine that waits for 100ms
	tm.Go(func() {
		time.Sleep(100 * time.Millisecond)
	})

	// This will block until the goroutines have called Done on the
	// WaitGroup
	//
	// This also implicitly verifies that we are properly calling Done(),
	// otherwise this test would time out the test suite
	tm.Wait()
	if time.Since(start) < 100*time.Millisecond {
		t.Errorf("WaitGroup returned before all goroutines are done")
	}
}
