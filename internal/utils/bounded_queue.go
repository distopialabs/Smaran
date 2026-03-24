package utils

import (
	"errors"
	"sync"
)

var ErrQueueClosed = errors.New("queue closed")

// BoundedQueue is a dynamically growing FIFO queue with a hard max size.
// Push blocks when the queue is full.
// Pop blocks when the queue is empty.
// After Close, Push returns ErrQueueClosed, and Pop drains remaining items then returns ok=false.
type BoundedQueue[T any] struct {
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond

	buf     []T
	head    int
	size    int
	maxSize int
	closed  bool
}

func NewBoundedQueue[T any](initialCap, maxSize int) *BoundedQueue[T] {
	if maxSize <= 0 {
		panic("maxSize must be > 0")
	}
	if initialCap <= 0 {
		initialCap = 16
	}
	if initialCap > maxSize {
		initialCap = maxSize
	}

	q := &BoundedQueue[T]{
		buf:     make([]T, initialCap),
		maxSize: maxSize,
	}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

func (q *BoundedQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

func (q *BoundedQueue[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

func (q *BoundedQueue[T]) Push(v T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for {
		if q.closed {
			return ErrQueueClosed
		}
		if q.size < len(q.buf) {
			break
		}
		if len(q.buf) < q.maxSize {
			q.grow()
			break
		}
		q.notFull.Wait()
	}

	tail := (q.head + q.size) % len(q.buf)
	q.buf[tail] = v
	q.size++
	q.notEmpty.Signal()
	return nil
}

func (q *BoundedQueue[T]) Pop() (T, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.size == 0 && !q.closed {
		q.notEmpty.Wait()
	}

	var zero T
	if q.size == 0 {
		return zero, false
	}

	v := q.buf[q.head]
	q.buf[q.head] = zero // help GC
	q.head = (q.head + 1) % len(q.buf)
	q.size--
	q.notFull.Signal()
	return v, true
}

func (q *BoundedQueue[T]) grow() {
	newCap := len(q.buf) * 2
	if newCap == 0 {
		newCap = 16
	}
	if newCap > q.maxSize {
		newCap = q.maxSize
	}

	newBuf := make([]T, newCap)
	for i := 0; i < q.size; i++ {
		newBuf[i] = q.buf[(q.head+i)%len(q.buf)]
	}
	q.buf = newBuf
	q.head = 0
}
