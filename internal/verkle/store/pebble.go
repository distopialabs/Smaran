package store

import (
	"github.com/cockroachdb/pebble"
)

type pebbleStore struct {
	db *pebble.DB
}

func openPebble(path string) (KVStore, error) {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, err
	}
	return &pebbleStore{db: db}, nil
}

func (s *pebbleStore) Get(key []byte) ([]byte, error) {
	val, closer, err := s.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer closer.Close()
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (s *pebbleStore) Put(key, value []byte) error {
	return s.db.Set(key, value, pebble.NoSync)
}

func (s *pebbleStore) Delete(key []byte) error {
	return s.db.Delete(key, pebble.Sync)
}

func (s *pebbleStore) NewBatch() Batch {
	return &pebbleBatch{batch: s.db.NewBatch()}
}

func (s *pebbleStore) NewIterator() Iterator {
	iter, _ := s.db.NewIter(nil)
	return &pebbleIterator{iter: iter}
}

func (s *pebbleStore) Sync() error {
	return s.db.Flush()
}

func (s *pebbleStore) Close() error {
	return s.db.Close()
}

type pebbleBatch struct {
	batch *pebble.Batch
}

func (b *pebbleBatch) Put(key, value []byte) error {
	return b.batch.Set(key, value, nil)
}

func (b *pebbleBatch) Delete(key []byte) error {
	return b.batch.Delete(key, nil)
}

func (b *pebbleBatch) Commit() error {
	return b.batch.Commit(pebble.NoSync)
}

// --- Pebble Iterator ---

type pebbleIterator struct {
	iter *pebble.Iterator
}

func (it *pebbleIterator) SeekLT(key []byte) bool {
	return it.iter.SeekLT(key)
}

func (it *pebbleIterator) Valid() bool {
	return it.iter.Valid()
}

func (it *pebbleIterator) Key() []byte {
	return it.iter.Key()
}

func (it *pebbleIterator) Value() []byte {
	return it.iter.Value()
}

func (it *pebbleIterator) Close() error {
	return it.iter.Close()
}
