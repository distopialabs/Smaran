package db

import (
	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
)

// PebbleDB wraps a Pebble database to implement the DB interface.
type PebbleDB struct {
	db *pebble.DB
}

// Metrics returns the current Pebble database metrics.
func (p *PebbleDB) Metrics() *pebble.Metrics {
	return p.db.Metrics()
}

// NewInMemoryPebbleDB creates a Pebble database backed entirely by RAM.
func NewInMemoryPebbleDB() (*PebbleDB, error) {
	db, err := pebble.Open("", &pebble.Options{FS: vfs.NewMem()})
	if err != nil {
		return nil, err
	}
	return &PebbleDB{db: db}, nil
}

// NewPebbleDB creates a new PebbleDB wrapper.
func NewPebbleDB(path string, opts *pebble.Options) (*PebbleDB, error) {
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}
	return &PebbleDB{db: db}, nil
}

// Get retrieves a value for a given key.
func (p *PebbleDB) Get(key []byte) ([]byte, error) {
	val, closer, err := p.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	// Make a copy of the value since it's only valid until closer.Close()
	result := make([]byte, len(val))
	copy(result, val)
	closer.Close()
	return result, nil
}

// Set stores a key-value pair.
func (p *PebbleDB) Set(key []byte, value []byte, sync bool) error {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	return p.db.Set(key, value, writeOpts)
}

// Delete removes a key from the database.
func (p *PebbleDB) Delete(key []byte, sync bool) error {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	return p.db.Delete(key, writeOpts)
}

// Close closes the database.
func (p *PebbleDB) Close() error {
	// Flush memtables to disk before closing (important with DisableWAL)
	if err := p.db.Flush(); err != nil {
		return err
	}
	return p.db.Close()
}

// NewBatch creates a new write batch.
func (p *PebbleDB) NewBatch() Batch {
	return &PebbleBatch{batch: p.db.NewBatch()}
}

// InnerDB returns the underlying Pebble database for advanced operations.
func (p *PebbleDB) InnerDB() *pebble.DB {
	return p.db
}

// PebbleBatch wraps a Pebble batch to implement the Batch interface.
type PebbleBatch struct {
	batch *pebble.Batch
}

// Set adds a key-value pair to the batch.
func (b *PebbleBatch) Set(key []byte, value []byte, sync bool) {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	b.batch.Set(key, value, writeOpts)
}

// Delete removes a key in the batch.
func (b *PebbleBatch) Delete(key []byte, sync bool) {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	b.batch.Delete(key, writeOpts)
}

// Commit writes all batched operations.
func (b *PebbleBatch) Commit(sync bool) error {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	return b.batch.Commit(writeOpts)
}

// Close closes the batch.
func (b *PebbleBatch) Close() error {
	return b.batch.Close()
}
