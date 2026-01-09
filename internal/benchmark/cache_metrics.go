package benchmark

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	ristretto "github.com/dgraph-io/ristretto/v2"
)

// CacheMetric records cache metrics at a point in time for a single shard
type CacheMetric struct {
	TimestampNs int64
	ShardID     int

	// Hit/Miss metrics
	Hits   uint64
	Misses uint64

	// Cost metrics (approximates cache size usage)
	CostAdded   uint64
	CostEvicted uint64

	// Key metrics
	KeysAdded   uint64
	KeysEvicted uint64
	KeysUpdated uint64

	// Rejection metrics
	SetsRejected uint64
	SetsDropped  uint64
	GetsDropped  uint64
	GetsKept     uint64
}

// CacheMetricsCollector collects Ristretto cache metrics periodically
type CacheMetricsCollector struct {
	metricsCh chan CacheMetric

	// Output file
	cacheFile *os.File
	cacheBuf  *bufio.Writer

	// Synchronization
	wg     sync.WaitGroup
	stopCh chan struct{}
	closed atomic.Bool

	// Stats
	totalSamples atomic.Int64
	startTime    time.Time
}

// CacheMetricsProvider is an interface for types that can provide Ristretto metrics
type CacheMetricsProvider interface {
	Metrics() *ristretto.Metrics
}

// NewCacheMetricsCollector creates a new cache metrics collector
func NewCacheMetricsCollector(outputDir string) (*CacheMetricsCollector, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	// Create cache metrics file
	cachePath := filepath.Join(outputDir, fmt.Sprintf("bench_cache_metrics_%s.csv", timestamp))
	cacheFile, err := os.Create(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache metrics file: %w", err)
	}

	cmc := &CacheMetricsCollector{
		metricsCh: make(chan CacheMetric, 10000),
		cacheFile: cacheFile,
		cacheBuf:  bufio.NewWriterSize(cacheFile, 1<<18), // 256KB buffer
		stopCh:    make(chan struct{}),
		startTime: time.Now(),
	}

	// Write CSV header
	cmc.cacheBuf.WriteString("timestamp_ns,shard_id,hits,misses,cost_added,cost_evicted,keys_added,keys_evicted,keys_updated,sets_rejected,sets_dropped,gets_dropped,gets_kept\n")

	// Start background writer
	cmc.wg.Add(1)
	go cmc.cacheWriter()

	fmt.Printf("   Cache Metrics: %s\n", cachePath)

	return cmc, nil
}

// cacheWriter writes cache metrics to file
func (cmc *CacheMetricsCollector) cacheWriter() {
	defer cmc.wg.Done()
	for m := range cmc.metricsCh {
		fmt.Fprintf(cmc.cacheBuf, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n",
			m.TimestampNs,
			m.ShardID,
			m.Hits,
			m.Misses,
			m.CostAdded,
			m.CostEvicted,
			m.KeysAdded,
			m.KeysEvicted,
			m.KeysUpdated,
			m.SetsRejected,
			m.SetsDropped,
			m.GetsDropped,
			m.GetsKept,
		)
		cmc.totalSamples.Add(1)
	}
}

// CollectMetrics collects metrics from multiple caches, one row per shard
func (cmc *CacheMetricsCollector) CollectMetrics(caches []CacheMetricsProvider) {
	if cmc.closed.Load() {
		return
	}

	now := time.Now().UnixNano()

	for shardID, cache := range caches {
		m := cache.Metrics()
		if m == nil {
			continue
		}

		metric := CacheMetric{
			TimestampNs:  now,
			ShardID:      shardID,
			Hits:         m.Hits(),
			Misses:       m.Misses(),
			CostAdded:    m.CostAdded(),
			CostEvicted:  m.CostEvicted(),
			KeysAdded:    m.KeysAdded(),
			KeysEvicted:  m.KeysEvicted(),
			KeysUpdated:  m.KeysUpdated(),
			SetsRejected: m.SetsRejected(),
			SetsDropped:  m.SetsDropped(),
			GetsDropped:  m.GetsDropped(),
			GetsKept:     m.GetsKept(),
		}

		select {
		case cmc.metricsCh <- metric:
		default:
			// Drop if channel full
		}
	}
}

// StartPeriodicCollection starts collecting metrics at the specified interval
func (cmc *CacheMetricsCollector) StartPeriodicCollection(caches []CacheMetricsProvider, interval time.Duration) {
	cmc.wg.Add(1)
	go func() {
		defer cmc.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cmc.CollectMetrics(caches)
			case <-cmc.stopCh:
				return
			}
		}
	}()
}

// Close stops collection and flushes data
func (cmc *CacheMetricsCollector) Close() error {
	if cmc.closed.Swap(true) {
		return nil
	}

	// Signal periodic collector to stop
	close(cmc.stopCh)

	// Close channel to signal writer to stop
	close(cmc.metricsCh)

	// Wait for goroutines to finish
	cmc.wg.Wait()

	// Flush and close file
	cmc.cacheBuf.Flush()
	cmc.cacheFile.Close()

	fmt.Printf("   Cache Metrics samples collected: %d\n", cmc.totalSamples.Load())

	return nil
}
