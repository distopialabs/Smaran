package proof

import (
	"errors"
	"math"
)

func Pow(base, exponent int) int {
	return int(math.Pow(float64(base), float64(exponent)))
}

// Queue
type Queue[T any] []T

func (q *Queue[T]) Enqueue(value T) {
	*q = append(*q, value)
}

func (q *Queue[T]) Dequeue() (T, error) {
	var zero T
	if len(*q) == 0 {
		return zero, errors.New("queue is empty")
	}
	val := (*q)[0]
	*q = (*q)[1:]
	return val, nil
}

func (q *Queue[T]) Peek() (T, error) {
	var zero T
	if len(*q) == 0 {
		return zero, errors.New("queue is empty")
	}
	return (*q)[0], nil
}

func (q *Queue[T]) IsEmpty() bool {
	return len(*q) == 0
}

func (q *Queue[T]) Size() int {
	return len(*q)
}
