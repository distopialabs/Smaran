package utils

import (
	"errors"
)

// Queue
type Queue[T any] []T

func NewQueue[T any]() Queue[T] {
	return make(Queue[T], 0)
}

func (q *Queue[T]) Enqueue(value T) {
	*q = append(*q, value)
}

func (q *Queue[T]) Dequeue() (T, error) {
	var zero T
	if len(*q) == 0 {
		return zero, errors.New("queue is empty")
	}
	val := (*q)[0]
	(*q)[0] = zero
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
