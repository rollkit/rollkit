package queue

import (
	"sync"
)

// Queue is a generic queue with thread-safe operations
type Queue[T any] struct {
	queue    []T
	mu       sync.Mutex
	notifyCh chan struct{}
}

// New creates a new Queue
func New[T any]() *Queue[T] {
	return &Queue[T]{
		queue:    make([]T, 0),
		notifyCh: make(chan struct{}, 1),
	}
}

// Add adds a new item to the queue
func (q *Queue[T]) Add(item T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, item)
	select {
	case q.notifyCh <- struct{}{}: // Send notification if there's no pending notification
	default:
		// Do nothing if a notification is already pending
	}
}

// Next returns the next item in the queue
func (q *Queue[T]) Next() *T {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return nil
	}
	item := q.queue[0]
	q.queue = q.queue[1:]
	return &item
}

// NotifyCh returns the notification channel
func (q *Queue[T]) NotifyCh() <-chan struct{} {
	return q.notifyCh
}

// Notify returns a channel that is notified when the queue is updated
func (q *Queue[T]) Notify(item T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, item)
	select {
	case q.notifyCh <- struct{}{}: // Send notification if there's no pending notification
	default:
		// Do nothing if a notification is already pending
	}
}

// IsEmpty checks if the queue is empty
func (q *Queue[T]) IsEmpty() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue) == 0
}

// Len returns the current length of the queue
func (q *Queue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

// Clear empties the queue
func (q *Queue[T]) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = make([]T, 0)
}
