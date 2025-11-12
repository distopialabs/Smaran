package segmenttree

import (
	"github.com/cockroachdb/pebble"
)

// PebbleDB wraps a Pebble database to implement the DB interface
type PebbleDB struct {
	db *pebble.DB
}

// NewPebbleDB creates a new PebbleDB wrapper
func NewPebbleDB(path string, opts *pebble.Options) (*PebbleDB, error) {
	db, err := pebble.Open(path, opts)
	if err != nil {
		return nil, err
	}
	return &PebbleDB{db: db}, nil
}

// Get retrieves a value for a given key
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

// Set stores a key-value pair
func (p *PebbleDB) Set(key []byte, value []byte, sync bool) error {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	return p.db.Set(key, value, writeOpts)
}

// Close closes the database
func (p *PebbleDB) Close() error {
	return p.db.Close()
}

// NewBatch creates a new write batch
func (p *PebbleDB) NewBatch() Batch {
	return &PebbleBatch{batch: p.db.NewBatch()}
}

// PebbleBatch wraps a Pebble batch to implement the Batch interface
type PebbleBatch struct {
	batch *pebble.Batch
}

// Set adds a key-value pair to the batch
func (b *PebbleBatch) Set(key []byte, value []byte, sync bool) {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	b.batch.Set(key, value, writeOpts)
}

// Commit writes all batched operations
func (b *PebbleBatch) Commit(sync bool) error {
	writeOpts := pebble.NoSync
	if sync {
		writeOpts = pebble.Sync
	}
	return b.batch.Commit(writeOpts)
}

// Close closes the batch
func (b *PebbleBatch) Close() error {
	return b.batch.Close()
}
