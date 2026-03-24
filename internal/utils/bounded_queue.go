package utils

import "errors"

var ErrQueueClosed = errors.New("queue closed")

// BoundedQueue is a FIFO queue backed entirely by Go channels.
// Push blocks when the queue is full.
// Pop blocks when the queue is empty.
// After Close, Push returns ErrQueueClosed, and Pop drains remaining items then returns ok=false.
type BoundedQueue[T any] struct {
	ch        chan T
	done      chan struct{}
	closeOnce chan struct{} // cap-1 gate so Close is idempotent
}

func NewBoundedQueue[T any](initialCap, maxSize int) *BoundedQueue[T] {
	if maxSize <= 0 {
		panic("maxSize must be > 0")
	}
	return &BoundedQueue[T]{
		ch:        make(chan T, maxSize),
		done:      make(chan struct{}),
		closeOnce: make(chan struct{}, 1),
	}
}

func (q *BoundedQueue[T]) Len() int {
	return len(q.ch)
}

func (q *BoundedQueue[T]) Close() {
	select {
	case q.closeOnce <- struct{}{}:
		close(q.done)
	default:
	}
}

func (q *BoundedQueue[T]) Push(v T) error {
	select {
	case <-q.done:
		return ErrQueueClosed
	default:
	}
	select {
	case q.ch <- v:
		return nil
	case <-q.done:
		return ErrQueueClosed
	}
}

func (q *BoundedQueue[T]) Pop() (T, bool) {
	select {
	case v := <-q.ch:
		return v, true
	case <-q.done:
		select {
		case v := <-q.ch:
			return v, true
		default:
			var zero T
			return zero, false
		}
	}
}
