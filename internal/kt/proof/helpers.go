package proof

import (
	"errors"
	"math"
)

// Pow returns base raised to the exponent power.
func Pow(base, exponent int) int {
	return int(math.Pow(float64(base), float64(exponent)))
}

// Queue is a simple generic FIFO queue.
type Queue[T any] []T

// Enqueue adds a value to the end of the queue.
func (q *Queue[T]) Enqueue(value T) {
	*q = append(*q, value)
}

// Dequeue removes and returns the first value from the queue.
func (q *Queue[T]) Dequeue() (T, error) {
	var zero T
	if len(*q) == 0 {
		return zero, errors.New("queue is empty")
	}
	val := (*q)[0]
	*q = (*q)[1:]
	return val, nil
}

// Peek returns the first value without removing it.
func (q *Queue[T]) Peek() (T, error) {
	var zero T
	if len(*q) == 0 {
		return zero, errors.New("queue is empty")
	}
	return (*q)[0], nil
}

// IsEmpty returns true if the queue is empty.
func (q *Queue[T]) IsEmpty() bool {
	return len(*q) == 0
}

// Size returns the number of elements in the queue.
func (q *Queue[T]) Size() int {
	return len(*q)
}
