package segmenttree

import (
	"os"
	"testing"

	"github.com/cockroachdb/pebble"
)

func TestPebbleDB(t *testing.T) {
	dbPath := "test-pebble.db"
	defer os.RemoveAll(dbPath)

	db, err := NewPebbleDB(dbPath, &pebble.Options{})
	if err != nil {
		t.Fatalf("Failed to create PebbleDB: %v", err)
	}
	defer db.Close()

	testDBOperations(t, db)
}

func TestSqliteDB(t *testing.T) {
	dbPath := "test-sqlite.db"
	defer RemoveSqliteDB(dbPath)

	db, err := NewSqliteDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create SqliteDB: %v", err)
	}
	defer db.Close()

	testDBOperations(t, db)
}

func testDBOperations(t *testing.T, db DB) {
	// Test basic Set and Get
	key := []byte("test-key")
	value := []byte("test-value")

	err := db.Set(key, value, false)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	retrieved, err := db.Get(key)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}

	if string(retrieved) != string(value) {
		t.Errorf("Expected %s, got %s", value, retrieved)
	}

	// Test non-existent key
	_, err = db.Get([]byte("non-existent-key"))
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	// Test batch operations
	batch := db.NewBatch()
	batch.Set([]byte("key1"), []byte("value1"), false)
	batch.Set([]byte("key2"), []byte("value2"), false)
	batch.Set([]byte("key3"), []byte("value3"), false)

	err = batch.Commit(false)
	if err != nil {
		t.Fatalf("Failed to commit batch: %v", err)
	}

	// Verify batch writes
	val1, err := db.Get([]byte("key1"))
	if err != nil || string(val1) != "value1" {
		t.Errorf("Batch write failed for key1")
	}

	val2, err := db.Get([]byte("key2"))
	if err != nil || string(val2) != "value2" {
		t.Errorf("Batch write failed for key2")
	}

	val3, err := db.Get([]byte("key3"))
	if err != nil || string(val3) != "value3" {
		t.Errorf("Batch write failed for key3")
	}

	// Test overwrite
	err = db.Set(key, []byte("updated-value"), false)
	if err != nil {
		t.Fatalf("Failed to overwrite value: %v", err)
	}

	updated, err := db.Get(key)
	if err != nil {
		t.Fatalf("Failed to get updated value: %v", err)
	}

	if string(updated) != "updated-value" {
		t.Errorf("Expected updated-value, got %s", updated)
	}
}

func BenchmarkPebbleDBWrite(b *testing.B) {
	dbPath := "bench-pebble.db"
	defer os.RemoveAll(dbPath)

	db, err := NewPebbleDB(dbPath, &pebble.Options{})
	if err != nil {
		b.Fatalf("Failed to create PebbleDB: %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte("key-" + string(rune(i)))
		value := []byte("value-" + string(rune(i)))
		db.Set(key, value, false)
	}
}

func BenchmarkSqliteDBWrite(b *testing.B) {
	dbPath := "bench-sqlite.db"
	defer RemoveSqliteDB(dbPath)

	db, err := NewSqliteDB(dbPath)
	if err != nil {
		b.Fatalf("Failed to create SqliteDB: %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte("key-" + string(rune(i)))
		value := []byte("value-" + string(rune(i)))
		db.Set(key, value, false)
	}
}

func BenchmarkPebbleDBRead(b *testing.B) {
	dbPath := "bench-pebble-read.db"
	defer os.RemoveAll(dbPath)

	db, err := NewPebbleDB(dbPath, &pebble.Options{})
	if err != nil {
		b.Fatalf("Failed to create PebbleDB: %v", err)
	}
	defer db.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := []byte("key-" + string(rune(i)))
		value := []byte("value-" + string(rune(i)))
		db.Set(key, value, false)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte("key-" + string(rune(i%1000)))
		db.Get(key)
	}
}

func BenchmarkSqliteDBRead(b *testing.B) {
	dbPath := "bench-sqlite-read.db"
	defer RemoveSqliteDB(dbPath)

	db, err := NewSqliteDB(dbPath)
	if err != nil {
		b.Fatalf("Failed to create SqliteDB: %v", err)
	}
	defer db.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := []byte("key-" + string(rune(i)))
		value := []byte("value-" + string(rune(i)))
		db.Set(key, value, false)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte("key-" + string(rune(i%1000)))
		db.Get(key)
	}
}

