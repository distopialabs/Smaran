package segmenttree

import (
	"errors"
)

// Common errors
var (
	ErrNotFound = errors.New("key not found")
)

// DB is an interface for key-value database operations
type DB interface {
	// Get retrieves a value for a given key
	// Returns ErrNotFound if the key doesn't exist
	Get(key []byte) ([]byte, error)

	// Set stores a key-value pair
	Set(key []byte, value []byte, sync bool) error

	// Close closes the database
	Close() error

	// NewBatch creates a new write batch
	NewBatch() Batch
}

// Batch is an interface for batched writes
type Batch interface {
	// Set adds a key-value pair to the batch
	Set(key []byte, value []byte, sync bool)

	// Commit writes all batched operations
	Commit(sync bool) error

	// Close closes the batch
	Close() error
}
