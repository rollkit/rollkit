package queue

import (
	"sync"
	"testing"
	"time"
)

func TestQueue(t *testing.T) {
	t.Run("Basic operations", func(t *testing.T) {
		queue := New[int]()

		// Check initial state
		if !queue.IsEmpty() {
			t.Fatal("New queue should be empty")
		}

		if queue.Len() != 0 {
			t.Fatal("New queue should have length 0")
		}

		// Add items
		queue.Add(1)
		queue.Add(2)
		queue.Add(3)

		if queue.IsEmpty() {
			t.Fatal("Queue should not be empty after adding items")
		}

		if queue.Len() != 3 {
			t.Fatalf("Queue length should be 3, got %d", queue.Len())
		}

		// Get items
		if *queue.Next() != 1 {
			t.Fatal("First item should be 1")
		}

		if *queue.Next() != 2 {
			t.Fatal("Second item should be 2")
		}

		if *queue.Next() != 3 {
			t.Fatal("Third item should be 3")
		}

		// Queue should be empty now
		if !queue.IsEmpty() {
			t.Fatal("Queue should be empty after getting all items")
		}

		// Next should return nil for empty queue
		if queue.Next() != nil {
			t.Fatal("Next should return nil for empty queue")
		}
	})

	t.Run("Clear operation", func(t *testing.T) {
		queue := New[string]()
		queue.Add("a")
		queue.Add("b")

		queue.Clear()

		if !queue.IsEmpty() {
			t.Fatal("Queue should be empty after Clear")
		}

		if queue.Len() != 0 {
			t.Fatal("Queue length should be 0 after Clear")
		}
	})

	t.Run("Notification channel", func(t *testing.T) {
		queue := New[int]()
		notified := false

		var wg sync.WaitGroup
		wg.Add(1)

		// Start a goroutine to listen for notifications
		go func() {
			defer wg.Done()
			select {
			case <-queue.NotifyCh():
				notified = true
			case <-time.After(time.Millisecond * 100):
				// Timeout
			}
		}()

		// Add an item to trigger notification
		time.Sleep(time.Millisecond * 10)
		queue.Add(1)

		wg.Wait()

		if !notified {
			t.Fatal("Notification channel should have been triggered")
		}
	})

	t.Run("Concurrent operations", func(t *testing.T) {
		queue := New[int]()
		const numGoroutines = 10
		const numItemsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Add items concurrently
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numItemsPerGoroutine; j++ {
					queue.Add(id*numItemsPerGoroutine + j)
				}
			}(i)
		}

		wg.Wait()

		// Check total number of items
		if queue.Len() != numGoroutines*numItemsPerGoroutine {
			t.Fatalf("Queue should have %d items, got %d", numGoroutines*numItemsPerGoroutine, queue.Len())
		}

		// Drain the queue
		count := 0
		for queue.Next() != nil {
			count++
		}

		if count != numGoroutines*numItemsPerGoroutine {
			t.Fatalf("Expected to drain %d items, got %d", numGoroutines*numItemsPerGoroutine, count)
		}
	})

	t.Run("Custom struct type", func(t *testing.T) {
		type TestStruct struct {
			ID   int
			Name string
			Time time.Time
		}

		queue := New[TestStruct]()

		now := time.Now()
		item1 := TestStruct{ID: 1, Name: "Item 1", Time: now}
		item2 := TestStruct{ID: 2, Name: "Item 2", Time: now.Add(time.Second)}

		queue.Add(item1)
		queue.Add(item2)

		result1 := queue.Next()
		if result1 == nil || result1.ID != 1 || result1.Name != "Item 1" {
			t.Fatal("Wrong item retrieved")
		}

		result2 := queue.Next()
		if result2 == nil || result2.ID != 2 || result2.Name != "Item 2" {
			t.Fatal("Wrong item retrieved")
		}
	})
}
