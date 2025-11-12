package segmenttree

import (
	"database/sql"
	"fmt"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// SqliteDB wraps a SQLite database to implement the DB interface
type SqliteDB struct {
	db *sql.DB
	mu sync.RWMutex

	// Transaction batching for better write performance
	txMu        sync.Mutex
	tx          *sql.Tx
	txStmt      *sql.Stmt
	txCount     int
	txBatchSize int

	// Write buffer to allow reads to see uncommitted writes
	writeBuffer map[string][]byte
}

// NewSqliteDB creates a new SqliteDB wrapper
func NewSqliteDB(path string) (*SqliteDB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Optimize SQLite for performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",        // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous=NORMAL",      // Faster writes (NORMAL instead of FULL)
		"PRAGMA cache_size=-262144",      // 256MB cache (negative means KB)
		"PRAGMA temp_store=MEMORY",       // Store temp tables in memory
		"PRAGMA mmap_size=2147483648",    // 2GB memory-mapped I/O
		"PRAGMA page_size=4096",          // 4KB page size
		"PRAGMA busy_timeout=5000",       // 5 second busy timeout
		"PRAGMA wal_autocheckpoint=1000", // Checkpoint every 1000 pages
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma %s: %w", pragma, err)
		}
	}

	// Create the key-value table
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS kv (
		key BLOB PRIMARY KEY,
		value BLOB NOT NULL
	) WITHOUT ROWID;
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	// Create index on key (already primary key, but explicit for clarity)
	// The WITHOUT ROWID optimization uses the primary key as the clustering index

	sqliteDB := &SqliteDB{
		db:          db,
		txBatchSize: 1000, // Commit every 1000 writes
		writeBuffer: make(map[string][]byte, 1000),
	}

	// Start the first transaction
	if err := sqliteDB.beginTx(); err != nil {
		db.Close()
		return nil, err
	}

	return sqliteDB, nil
}

// beginTx starts a new transaction for batching writes
func (s *SqliteDB) beginTx() error {
	var err error
	s.tx, err = s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	s.txStmt, err = s.tx.Prepare("INSERT OR REPLACE INTO kv (key, value) VALUES (?, ?)")
	if err != nil {
		s.tx.Rollback()
		return fmt.Errorf("failed to prepare statement: %w", err)
	}

	s.txCount = 0
	return nil
}

// commitTx commits the current transaction and starts a new one
func (s *SqliteDB) commitTx() error {
	if s.txStmt != nil {
		s.txStmt.Close()
		s.txStmt = nil
	}

	if s.tx != nil {
		if err := s.tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		s.tx = nil
	}

	// Clear the write buffer after commit
	s.writeBuffer = make(map[string][]byte, s.txBatchSize)

	return s.beginTx()
}

// RemoveSqliteDB removes the SQLite database file and its WAL files
func RemoveSqliteDB(path string) error {
	// Remove main database file
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("failed to remove database file: %w", err)
	}
	// Remove WAL file
	walPath := path + "-wal"
	if err := os.RemoveAll(walPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove WAL file: %w", err)
	}
	// Remove SHM file
	shmPath := path + "-shm"
	if err := os.RemoveAll(shmPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove SHM file: %w", err)
	}
	return nil
}

// Get retrieves a value for a given key
func (s *SqliteDB) Get(key []byte) ([]byte, error) {
	keyStr := string(key)

	// First check the write buffer for uncommitted writes
	s.txMu.Lock()
	if val, ok := s.writeBuffer[keyStr]; ok {
		s.txMu.Unlock()
		// Return a copy to avoid external modifications
		result := make([]byte, len(val))
		copy(result, val)
		return result, nil
	}
	s.txMu.Unlock()

	// Not in write buffer, query the database
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value []byte
	err := s.db.QueryRow("SELECT value FROM kv WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get value: %w", err)
	}
	return value, nil
}

// Set stores a key-value pair using batched transactions for performance
func (s *SqliteDB) Set(key []byte, value []byte, sync bool) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()

	// Use the prepared statement in the current transaction
	if s.txStmt == nil {
		return fmt.Errorf("transaction statement not initialized")
	}

	_, err := s.txStmt.Exec(key, value)
	if err != nil {
		return fmt.Errorf("failed to set value: %w", err)
	}

	// Add to write buffer so Get() can see it
	keyStr := string(key)
	valCopy := make([]byte, len(value))
	copy(valCopy, value)
	s.writeBuffer[keyStr] = valCopy

	s.txCount++

	// Commit the transaction if we've reached the batch size or sync is requested
	if sync || s.txCount >= s.txBatchSize {
		if err := s.commitTx(); err != nil {
			return err
		}
	}

	return nil
}

// Close closes the database
func (s *SqliteDB) Close() error {
	// Commit any pending transaction
	s.txMu.Lock()
	if s.txStmt != nil {
		s.txStmt.Close()
		s.txStmt = nil
	}
	if s.tx != nil {
		s.tx.Commit()
		s.tx = nil
	}
	// Clear write buffer
	s.writeBuffer = nil
	s.txMu.Unlock()

	// Checkpoint and close the WAL
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return s.db.Close()
}

// NewBatch creates a new write batch
func (s *SqliteDB) NewBatch() Batch {
	return &SqliteBatch{
		sqliteDB: s,
		entries:  make([]batchEntry, 0, 100),
	}
}

type batchEntry struct {
	key   []byte
	value []byte
}

// SqliteBatch implements batched writes for SQLite
type SqliteBatch struct {
	sqliteDB *SqliteDB
	entries  []batchEntry
}

// Set adds a key-value pair to the batch
func (b *SqliteBatch) Set(key []byte, value []byte, sync bool) {
	// Note: sync parameter is ignored for individual batch entries
	// It will be used during Commit
	b.entries = append(b.entries, batchEntry{
		key:   append([]byte(nil), key...),   // Copy key
		value: append([]byte(nil), value...), // Copy value
	})
}

// Commit writes all batched operations using the SqliteDB's transaction system
func (b *SqliteBatch) Commit(sync bool) error {
	if len(b.entries) == 0 {
		return nil
	}

	// Use the parent SqliteDB's Set() to leverage transaction batching
	for _, entry := range b.entries {
		if err := b.sqliteDB.Set(entry.key, entry.value, false); err != nil {
			return fmt.Errorf("failed to set batch entry: %w", err)
		}
	}

	// If sync is requested, flush the transaction
	if sync {
		b.sqliteDB.txMu.Lock()
		if b.sqliteDB.txCount > 0 {
			if err := b.sqliteDB.commitTx(); err != nil {
				b.sqliteDB.txMu.Unlock()
				return err
			}
		}
		b.sqliteDB.txMu.Unlock()
	}

	b.entries = nil
	return nil
}

// Close closes the batch without committing
func (b *SqliteBatch) Close() error {
	b.entries = nil
	return nil
}
