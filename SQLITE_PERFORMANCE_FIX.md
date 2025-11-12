# SQLite Performance Fix - Transaction Batching

## Problem Identified

The initial SQLite implementation was **extremely slow** (taking forever for just 100 blocks vs 8 seconds for Pebble) because:

1. Each `Set()` call was executed as a **separate transaction** in autocommit mode
2. Each account save makes **~9 individual Set() calls** (1 balance + 4 batch trees + 4 commitments + 1 historical balance)
3. For 100 blocks with multiple accounts, this resulted in **thousands of individual transactions**
4. Each transaction has fsync overhead, even with `PRAGMA synchronous=NORMAL`

## Solution: Automatic Transaction Batching

The fixed implementation uses **automatic transaction batching**:

```go
// Key changes in db_sqlite.go:
type SqliteDB struct {
    // ... existing fields ...
    
    // Transaction batching
    txMu   sync.Mutex
    tx     *sql.Tx           // Current transaction
    txStmt *sql.Stmt         // Prepared statement
    txCount int              // Number of writes in current tx
    txBatchSize int          // Commit after this many writes (default: 1000)
}
```

### How It Works

1. **On initialization**: Opens a long-running transaction with a prepared statement
2. **On each Set()**: Executes within the current transaction, incrementing the counter
3. **Auto-commit**: Commits and starts new transaction after 1000 writes OR when sync=true
4. **On Get()**: Flushes pending writes first, so reads always see latest data
5. **On Close()**: Commits any remaining writes

## Performance Impact

**Before (individual transactions):**
- ~10 transactions per account update
- 100 blocks × 10 accounts × 10 transactions = 10,000+ transactions
- Each transaction: prepare → execute → commit → fsync
- Result: **Extremely slow (minutes/hours)**

**After (batched transactions):**
- ~10 writes accumulate in one transaction
- Commits only every 1000 writes
- Result: **Should be much closer to Pebble performance**

## Testing

Run your workload now:
```bash
./samurai
```

You should see **dramatically improved performance** - likely within the same ballpark as Pebble.

## Tuning

If you want to adjust the batch size, edit `db_sqlite.go` around line 68:

```go
sqliteDB := &SqliteDB{
    db: db,
    txBatchSize: 1000,  // Increase for more batching, decrease for more frequent commits
}
```

**Trade-offs:**
- **Larger batch size**: Fewer commits = faster writes, but more data loss risk on crash
- **Smaller batch size**: More commits = safer, but slower

For your use case (commitment generation), 1000 is a good balance.

## Read Performance

Reads automatically flush pending writes, so you always see consistent data. The flush adds minimal overhead since:
- It only happens if there are pending writes
- Writes are already fast with batching
- Your workload is write-heavy anyway

## Comparison to Pebble

**Pebble:**
- LSM-tree optimized for high write throughput
- Individual writes are fast (no immediate fsync)
- Compaction happens in background

**SQLite with batching:**
- B-tree with transactional batching
- Similar write performance with proper batching
- WAL mode provides good concurrency
- May have better read performance with warm cache

Both should now perform similarly for your workload!

