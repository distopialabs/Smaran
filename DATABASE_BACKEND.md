# Database Backend Configuration

This document describes how to switch between Pebble and SQLite database backends for improved performance testing.

## Overview

The codebase now supports two database backends:
- **Pebble**: Original LSM-tree based key-value store (default previously)
- **SQLite**: Alternative relational database with optimized settings

## How to Switch Backends

### In `cmd/samurai/commit.go`

Find the `generateCommitmentsV2` function and locate this line:

```go
backend := SqliteBackend
```

Change it to one of:
- `backend := PebbleBackend` - Use Pebble database
- `backend := SqliteBackend` - Use SQLite database (current)

### Database Locations

Each backend uses a separate database file:
- **Pebble**: `samurai-with-cache-pebble.db/`
- **SQLite**: `samurai-with-cache-sqlite.db`

The database is automatically removed and recreated on each run.

## SQLite Optimizations

The SQLite implementation includes several performance optimizations:

1. **Write-Ahead Logging (WAL)**: Enables better concurrency
2. **Synchronous Mode**: Set to NORMAL for faster writes
3. **Large Cache**: 256MB cache size for frequently accessed data
4. **Memory-Mapped I/O**: 2GB mmap for improved read performance
5. **WITHOUT ROWID**: Table optimization for better performance
6. **Prepared Statements**: Used in batch operations

## Performance Testing

To compare performance between backends:

1. Build the project:
   ```bash
   go build ./cmd/samurai
   ```

2. Run with Pebble:
   - Edit `commit.go` to set `backend := PebbleBackend`
   - Rebuild and run
   - Note the execution time

3. Run with SQLite:
   - Edit `commit.go` to set `backend := SqliteBackend`
   - Rebuild and run
   - Note the execution time

4. Compare metrics:
   - Total execution time
   - Read latency (check logs)
   - Memory usage
   - Database size on disk

## Implementation Details

### Database Interface

A common `DB` interface abstracts database operations:

```go
type DB interface {
    Get(key []byte) ([]byte, error)
    Set(key []byte, value []byte, sync bool) error
    Close() error
    NewBatch() Batch
}
```

### Implementations

- **PebbleDB** (`internal/segmenttree/db_pebble.go`): Wraps Pebble
- **SqliteDB** (`internal/segmenttree/db_sqlite.go`): Wraps SQLite

All database operations in the codebase use this interface, making it easy to swap backends.

## Files Modified

- `internal/segmenttree/db_interface.go` - Common DB interface
- `internal/segmenttree/db_pebble.go` - Pebble wrapper
- `internal/segmenttree/db_sqlite.go` - SQLite implementation
- `internal/segmenttree/db_wrapper.go` - Updated to use DB interface
- `internal/segmenttree/cache.go` - Updated cache to use DB interface
- `internal/segmenttree/cachedb.go` - Updated to use DB interface
- `internal/segmenttree/update.go` - Updated to use DB interface
- `internal/segmenttree/segmenttree.go` - Updated to use DB interface
- `internal/proof/proof.go` - Updated to use DB interface
- `internal/proof/rebuildproof.go` - Updated to use DB interface
- `cmd/samurai/commit.go` - Added backend selection
- `cmd/samurai/proof.go` - Updated to use DB wrapper

## Troubleshooting

### SQLite Dependency

If you get import errors for `github.com/mattn/go-sqlite3`, run:

```bash
go get github.com/mattn/go-sqlite3
go mod tidy
```

### CGO Requirement

SQLite requires CGO to be enabled. If you get build errors, ensure:

```bash
export CGO_ENABLED=1
go build ./cmd/samurai
```

### Database Lock Errors

If you see "database is locked" errors:
- Ensure only one process is accessing the database
- Check that previous runs have fully closed connections
- Remove the `.db-wal` and `.db-shm` files manually if needed

## Performance Expectations

SQLite may provide better read latency in scenarios with:
- High read-to-write ratios
- Frequent random access patterns
- Need for concurrent reads

Pebble may perform better with:
- Write-heavy workloads
- Sequential access patterns
- Need for very high write throughput

Your actual results will depend on the specific workload and system configuration.

