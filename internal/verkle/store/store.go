// Package store provides an abstract KV store interface with pebble and
// leveldb backends, plus a Verkle node store and metadata helpers.
package store

import "errors"

// ErrNotFound is returned when a key is not found in the store.
var ErrNotFound = errors.New("store: key not found")

// KVStore is the interface for key-value storage backends.
type KVStore interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
	Delete(key []byte) error
	NewBatch() Batch
	NewIterator() Iterator
	Sync() error
	Close() error
}

// Batch accumulates writes for atomic commit.
type Batch interface {
	Put(key, value []byte) error
	Delete(key []byte) error
	Commit() error
}

// Iterator provides ordered key-value iteration with seek support.
type Iterator interface {
	// SeekLT positions the iterator at the largest key strictly less than the given key.
	SeekLT(key []byte) bool
	Valid() bool
	Key() []byte
	Value() []byte
	Close() error
}

// OpenKVStore opens a KV store with the given backend ("pebble" or "leveldb").
func OpenKVStore(backend, path string) (KVStore, error) {
	switch backend {
	case "pebble":
		return openPebble(path)
	case "leveldb":
		return openLevelDB(path)
	default:
		return nil, errors.New("unknown db-backend: " + backend + " (use 'pebble' or 'leveldb')")
	}
}
