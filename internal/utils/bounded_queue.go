package utils

import (
	"errors"
	"sync/atomic"
)

var ErrQueueClosed = errors.New("queue closed")

// BoundedQueue is a FIFO queue backed entirely by Go channels.
// Push blocks when the queue is full.
// Pop blocks when the queue is empty.
// After Close, Push returns ErrQueueClosed, and Pop drains remaining items then returns ok=false.
type BoundedQueue[T any] struct {
	ch        chan T
	done      chan struct{}
	closeOnce chan struct{} // cap-1 gate so Close is idempotent
	_done     atomic.Bool
}

func NewBoundedQueue[T any](initialCap, maxSize int) *BoundedQueue[T] {
	if maxSize <= 0 {
		return &BoundedQueue[T]{
			ch:        make(chan T),
			done:      make(chan struct{}),
			closeOnce: make(chan struct{}, 1),
			_done:     atomic.Bool{},
		}
	}
	return &BoundedQueue[T]{
		ch:        make(chan T, maxSize),
		done:      make(chan struct{}),
		closeOnce: make(chan struct{}, 1),
		_done:     atomic.Bool{},
	}
}

func (q *BoundedQueue[T]) Len() int {
	return len(q.ch)
}

func (q *BoundedQueue[T]) Close() {
	// select {
	// case q.closeOnce <- struct{}{}:
	// 	close(q.done)
	// default:
	// }
	q._done.Store(true)
	close(q.done)
}

func (q *BoundedQueue[T]) Push(v T) error {
	if q._done.Load() {
		return ErrQueueClosed
	}
	// select {
	// case <-q.done:
	// 	return ErrQueueClosed
	// default:
	// }
	// select {
	// case q.ch <- v:
	// 	return nil
	// case <-q.done:
	// 	return ErrQueueClosed
	// }
	q.ch <- v
	return nil
}

func (q *BoundedQueue[T]) Pop() (T, bool) {
	// select {
	// case v := <-q.ch:
	// 	return v, true
	// case <-q.done:
	// 	select {
	// 	case v := <-q.ch:
	// 		return v, true
	// 	default:
	// 		var zero T
	// 		return zero, false
	// 	}
	// }
	if q._done.Load() {
		var zero T
		return zero, false
	}
	return <-q.ch, true
}
