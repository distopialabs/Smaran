package store

import (
	"bytes"

	"github.com/syndtr/goleveldb/leveldb"
	leveldberr "github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/iterator"
)

type leveldbStore struct {
	db *leveldb.DB
}

func openLevelDB(path string) (KVStore, error) {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &leveldbStore{db: db}, nil
}

func (s *leveldbStore) Get(key []byte) ([]byte, error) {
	val, err := s.db.Get(key, nil)
	if err != nil {
		if err == leveldberr.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return val, nil
}

func (s *leveldbStore) Put(key, value []byte) error {
	return s.db.Put(key, value, nil)
}

func (s *leveldbStore) Delete(key []byte) error {
	return s.db.Delete(key, nil)
}

func (s *leveldbStore) NewBatch() Batch {
	return &leveldbBatch{batch: new(leveldb.Batch), db: s.db}
}

func (s *leveldbStore) NewIterator() Iterator {
	return &leveldbIterator{iter: s.db.NewIterator(nil, nil)}
}

func (s *leveldbStore) Sync() error {
	return nil
}

func (s *leveldbStore) Close() error {
	return s.db.Close()
}

type leveldbBatch struct {
	batch *leveldb.Batch
	db    *leveldb.DB
}

func (b *leveldbBatch) Put(key, value []byte) error {
	b.batch.Put(key, value)
	return nil
}

func (b *leveldbBatch) Delete(key []byte) error {
	b.batch.Delete(key)
	return nil
}

func (b *leveldbBatch) Commit() error {
	return b.db.Write(b.batch, nil)
}

// --- LevelDB Iterator ---

type leveldbIterator struct {
	iter iterator.Iterator
}

// SeekLT positions at the largest key strictly less than the given key.
// LevelDB only has Seek (≥), so we seek then step back.
func (it *leveldbIterator) SeekLT(key []byte) bool {
	if it.iter.Seek(key) {
		// Iterator is at first key >= target
		if bytes.Equal(it.iter.Key(), key) {
			// Exact match — go back one for strictly less than
			return it.iter.Prev()
		}
		// Iterator is past our key, go back one
		return it.iter.Prev()
	}
	// No key >= target exists, so the last key in the DB is < target
	return it.iter.Last()
}

func (it *leveldbIterator) Valid() bool {
	return it.iter.Valid()
}

func (it *leveldbIterator) Key() []byte {
	return it.iter.Key()
}

func (it *leveldbIterator) Value() []byte {
	return it.iter.Value()
}

func (it *leveldbIterator) Close() error {
	it.iter.Release()
	return it.iter.Error()
}
