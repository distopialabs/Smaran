# SQLite Database Backend Migration Summary

## Overview

Successfully implemented SQLite as an alternative database backend to Pebble for the Samurai project. The codebase now supports both backends through a unified interface, making it easy to compare performance characteristics.

## Changes Made

### 1. Database Abstraction Layer

Created a common `DB` interface in `internal/segmenttree/db_interface.go`:
- `Get(key []byte) ([]byte, error)` - Retrieve values
- `Set(key []byte, value []byte, sync bool) error` - Store values
- `Close() error` - Close database
- `NewBatch() Batch` - Create batched writes

### 2. Implementation Files

**New Files Created:**
- `internal/segmenttree/db_interface.go` - Common interface
- `internal/segmenttree/db_pebble.go` - Pebble wrapper
- `internal/segmenttree/db_sqlite.go` - SQLite implementation
- `internal/segmenttree/db_test.go` - Tests for both backends
- `DATABASE_BACKEND.md` - Usage documentation
- `MIGRATION_SUMMARY.md` - This file

**Files Modified:**
- `internal/segmenttree/cache.go` - Use DB interface
- `internal/segmenttree/cachedb.go` - Use DB interface
- `internal/segmenttree/db_wrapper.go` - Use DB interface, commented out Pebble-specific iterator function
- `internal/segmenttree/segmenttree.go` - Use DB interface
- `internal/segmenttree/update.go` - Use DB interface
- `internal/proof/proof.go` - Use DB interface
- `internal/proof/rebuildproof.go` - Use DB interface
- `cmd/samurai/commit.go` - Backend selection logic
- `cmd/samurai/proof.go` - Use DB wrapper

### 3. SQLite Optimizations

The SQLite implementation includes aggressive performance optimizations:

```sql
PRAGMA journal_mode=WAL              -- Write-Ahead Logging
PRAGMA synchronous=NORMAL            -- Faster writes
PRAGMA cache_size=-262144            -- 256MB cache
PRAGMA temp_store=MEMORY             -- Memory temp tables
PRAGMA mmap_size=2147483648          -- 2GB memory-mapped I/O
PRAGMA page_size=4096                -- 4KB pages
PRAGMA busy_timeout=5000             -- 5 second timeout
PRAGMA wal_autocheckpoint=1000       -- Auto-checkpoint
```

The table uses `WITHOUT ROWID` optimization for better B-tree performance.

## How to Use

### Switching Backends

Edit `cmd/samurai/commit.go` around line 43:

```go
// For SQLite:
backend := SqliteBackend

// For Pebble:
backend := PebbleBackend
```

Then rebuild and run:
```bash
go build ./cmd/samurai
./samurai
```

### Database Files

- **Pebble**: `samurai-with-cache-pebble.db/` (directory)
- **SQLite**: `samurai-with-cache-sqlite.db` (file + WAL files)

## Performance Testing

### Quick Benchmark Results

Simple key-value operations (synthetic benchmark):

```
BenchmarkPebbleDBWrite-64    1000000      1,076 ns/op
BenchmarkSqliteDBWrite-64       1077  1,114,854 ns/op

BenchmarkPebbleDBRead-64     1732629        632 ns/op
BenchmarkSqliteDBRead-64        4339    242,849 ns/op
```

**Note:** These are synthetic benchmarks. Real-world performance depends on:
- Access patterns (sequential vs. random)
- Cache hit rates
- Read/write ratios
- Concurrent operations
- Data sizes

### Running Your Own Performance Test

1. **Test with Pebble:**
   ```bash
   # Edit commit.go to set: backend := PebbleBackend
   go build ./cmd/samurai
   time ./samurai
   ```

2. **Test with SQLite:**
   ```bash
   # Edit commit.go to set: backend := SqliteBackend
   go build ./cmd/samurai
   time ./samurai
   ```

3. **Compare metrics:**
   - Total execution time
   - Memory usage (`/usr/bin/time -v ./samurai`)
   - Disk usage (`du -sh samurai-*`)
   - Look for "time:" log messages in output

## Technical Details

### Error Handling

Both implementations normalize errors to `ErrNotFound` for missing keys, ensuring consistent error handling across backends.

### Batch Operations

Both backends support batched writes for efficiency:
- **Pebble**: Native batch support
- **SQLite**: Implemented via transactions with prepared statements

### Thread Safety

- **Pebble**: Thread-safe by design
- **SQLite**: Protected with `sync.RWMutex` for concurrent access

### Closing and Cleanup

Both implementations properly:
- Close database connections
- Clean up resources
- Checkpoint WAL (SQLite)
- Close all open iterators/handles

## Testing

Run tests:
```bash
go test -v ./internal/segmenttree -run "TestPebbleDB|TestSqliteDB"
```

Run benchmarks:
```bash
go test -bench=. ./internal/segmenttree -benchtime=1s
```

## Potential Performance Scenarios

### SQLite May Be Better For:
- High cache hit rates (in-memory advantages)
- Read-heavy workloads with the in-memory cache
- Workloads that benefit from SQLite's query planner
- Scenarios where OS page cache integration helps

### Pebble May Be Better For:
- Write-heavy workloads (LSM-tree advantage)
- Very large datasets that exceed memory
- Sequential write patterns
- Workloads with large batches

## Known Limitations

1. **Pebble-Specific Function**: `GetCurrentLXBatchTreeAndCommitments` was commented out as it used Pebble-specific iterator APIs. The code already had an alternative implementation using separate Get calls.

2. **CGO Requirement**: SQLite requires CGO to be enabled (`CGO_ENABLED=1`).

3. **Platform Support**: SQLite driver may have platform-specific considerations.

## Dependencies Added

```
go get github.com/mattn/go-sqlite3
```

Already included in `go.mod` after running `go mod tidy`.

## Next Steps

1. **Run your actual workload** with both backends
2. **Measure specific metrics** important to your use case:
   - Latency percentiles (p50, p95, p99)
   - Throughput (operations/second)
   - Memory usage
   - CPU utilization
3. **Profile if needed** using Go's pprof:
   ```bash
   go build -o samurai ./cmd/samurai
   ./samurai -cpuprofile=cpu.prof
   go tool pprof cpu.prof
   ```

## Conclusion

The migration provides a flexible foundation for comparing database backends. The abstraction layer makes it trivial to switch between implementations or even add new ones in the future. Performance characteristics will depend heavily on your specific workload patterns.

---

**Questions or Issues?** Check `DATABASE_BACKEND.md` for troubleshooting and detailed configuration options.

