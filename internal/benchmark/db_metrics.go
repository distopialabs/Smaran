package benchmark

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
)

// DBMetric records database metrics at a point in time for a single shard
type DBMetric struct {
	TimestampNs int64
	ShardID     int

	// Compaction metrics
	CompactCount           int64
	CompactEstimatedDebt   uint64
	CompactInProgressBytes int64
	CompactNumInProgress   int64

	// Flush metrics
	FlushCount int64

	// MemTable metrics
	MemTableSize  uint64
	MemTableCount int64

	// Level metrics (L0 is most important for write stalls)
	L0NumFiles int64
	L0Size     int64

	// Total levels info
	TotalNumFiles int64
	TotalSize     int64

	// Write stall info
	WriteStallCount    int64
	WriteStallDuration time.Duration

	// Cache metrics
	BlockCacheSize   int64
	BlockCacheHits   int64
	BlockCacheMisses int64
}

// DBMetricsCollector collects Pebble database metrics periodically
type DBMetricsCollector struct {
	metricsCh chan DBMetric

	// Output file
	dbFile *os.File
	dbBuf  *bufio.Writer

	// Synchronization
	wg     sync.WaitGroup
	stopCh chan struct{}
	closed atomic.Bool

	// Stats
	totalSamples atomic.Int64
	startTime    time.Time
}

// NewDBMetricsCollector creates a new DB metrics collector
func NewDBMetricsCollector(outputDir string) (*DBMetricsCollector, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	// Create DB metrics file
	dbPath := filepath.Join(outputDir, fmt.Sprintf("bench_db_metrics_%s.csv", timestamp))
	dbFile, err := os.Create(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create DB metrics file: %w", err)
	}

	dmc := &DBMetricsCollector{
		metricsCh: make(chan DBMetric, 10000),
		dbFile:    dbFile,
		dbBuf:     bufio.NewWriterSize(dbFile, 1<<18), // 256KB buffer
		stopCh:    make(chan struct{}),
		startTime: time.Now(),
	}

	// Write CSV header
	dmc.dbBuf.WriteString("timestamp_ns,shard_id,compact_count,compact_estimated_debt,compact_in_progress_bytes,compact_num_in_progress,flush_count,memtable_size,memtable_count,l0_num_files,l0_size,total_num_files,total_size,write_stall_count,write_stall_duration_ns,block_cache_size,block_cache_hits,block_cache_misses\n")

	// Start background writer
	dmc.wg.Add(1)
	go dmc.dbWriter()

	fmt.Printf("   DB Metrics: %s\n", dbPath)

	return dmc, nil
}

// dbWriter writes DB metrics to file
func (dmc *DBMetricsCollector) dbWriter() {
	defer dmc.wg.Done()
	for m := range dmc.metricsCh {
		fmt.Fprintf(dmc.dbBuf, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
			m.TimestampNs,
			m.ShardID,
			m.CompactCount,
			m.CompactEstimatedDebt,
			m.CompactInProgressBytes,
			m.CompactNumInProgress,
			m.FlushCount,
			m.MemTableSize,
			m.MemTableCount,
			m.L0NumFiles,
			m.L0Size,
			m.TotalNumFiles,
			m.TotalSize,
			m.WriteStallCount,
			m.WriteStallDuration.Nanoseconds(),
			m.BlockCacheSize,
			m.BlockCacheHits,
			m.BlockCacheMisses,
		)
		dmc.totalSamples.Add(1)
	}
}

// PebbleMetricsProvider is an interface for types that can provide Pebble metrics
type PebbleMetricsProvider interface {
	Metrics() *pebble.Metrics
}

// CollectMetrics collects metrics from multiple Pebble databases, one row per shard
func (dmc *DBMetricsCollector) CollectMetrics(dbs []PebbleMetricsProvider) {
	if dmc.closed.Load() {
		return
	}

	now := time.Now().UnixNano()

	for shardID, db := range dbs {
		m := db.Metrics()
		if m == nil {
			continue
		}

		metric := DBMetric{
			TimestampNs:            now,
			ShardID:                shardID,
			CompactCount:           m.Compact.Count,
			CompactEstimatedDebt:   m.Compact.EstimatedDebt,
			CompactInProgressBytes: m.Compact.InProgressBytes,
			CompactNumInProgress:   int64(m.Compact.NumInProgress),
			FlushCount:             m.Flush.Count,
			MemTableSize:           m.MemTable.Size,
			MemTableCount:          m.MemTable.Count,
			BlockCacheSize:         m.BlockCache.Size,
			BlockCacheHits:         m.BlockCache.Hits,
			BlockCacheMisses:       m.BlockCache.Misses,
		}

		// Collect level metrics
		for i, level := range m.Levels {
			metric.TotalNumFiles += level.NumFiles
			metric.TotalSize += level.Size
			if i == 0 {
				metric.L0NumFiles = level.NumFiles
				metric.L0Size = level.Size
			}
		}

		select {
		case dmc.metricsCh <- metric:
		default:
			// Drop if channel full
		}
	}
}

// StartPeriodicCollection starts collecting metrics at the specified interval
func (dmc *DBMetricsCollector) StartPeriodicCollection(dbs []PebbleMetricsProvider, interval time.Duration) {
	dmc.wg.Add(1)
	go func() {
		defer dmc.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				dmc.CollectMetrics(dbs)
			case <-dmc.stopCh:
				return
			}
		}
	}()
}

// Close stops collection and flushes data
func (dmc *DBMetricsCollector) Close() error {
	if dmc.closed.Swap(true) {
		return nil
	}

	// Signal periodic collector to stop
	close(dmc.stopCh)

	// Close channel to signal writer to stop
	close(dmc.metricsCh)

	// Wait for goroutines to finish
	dmc.wg.Wait()

	// Flush and close file
	dmc.dbBuf.Flush()
	dmc.dbFile.Close()

	fmt.Printf("   DB Metrics samples collected: %d\n", dmc.totalSamples.Load())

	return nil
}
