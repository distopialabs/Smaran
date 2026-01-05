package benchmark

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// UpdateMetric records timing data for a single update operation
type UpdateMetric struct {
	BlockNumber   uint64
	LatencyNs     int64 // Time from enqueue to completion
	CompletedAtNs int64 // Absolute timestamp when completed
}

// BlockMetric records timing data for a complete block
type BlockMetric struct {
	BlockNumber   uint64
	SubmittedAtNs int64 // When block was added to blockInfoCh
	CompletedAtNs int64 // When last update of this block finished
	UpdateCount   int   // Number of updates in this block
}

// blockTracker tracks pending updates for a block
type blockTracker struct {
	submittedAtNs int64
	updateCount   int
	remaining     atomic.Int64
}

// MetricsCollector collects benchmark metrics with minimal overhead
type MetricsCollector struct {
	updateCh chan UpdateMetric
	blockCh  chan BlockMetric

	// Block tracking: blockNumber -> *blockTracker
	blockTrackers sync.Map

	// Output files
	updateFile *os.File
	blockFile  *os.File
	updateBuf  *bufio.Writer
	blockBuf   *bufio.Writer

	// Synchronization
	wg     sync.WaitGroup
	closed atomic.Bool

	// Stats for summary
	totalUpdates atomic.Int64
	totalBlocks  atomic.Int64
	startTime    time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(outputDir string) (*MetricsCollector, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	// Create update metrics file
	updatePath := filepath.Join(outputDir, fmt.Sprintf("bench_updates_%s.csv", timestamp))
	updateFile, err := os.Create(updatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create update metrics file: %w", err)
	}

	// Create block metrics file
	blockPath := filepath.Join(outputDir, fmt.Sprintf("bench_blocks_%s.csv", timestamp))
	blockFile, err := os.Create(blockPath)
	if err != nil {
		updateFile.Close()
		return nil, fmt.Errorf("failed to create block metrics file: %w", err)
	}

	mc := &MetricsCollector{
		updateCh:   make(chan UpdateMetric, 100000), // Large buffer for low contention
		blockCh:    make(chan BlockMetric, 10000),
		updateFile: updateFile,
		blockFile:  blockFile,
		updateBuf:  bufio.NewWriterSize(updateFile, 1<<20), // 1MB buffer
		blockBuf:   bufio.NewWriterSize(blockFile, 1<<18),  // 256KB buffer
		startTime:  time.Now(),
	}

	// Write CSV headers
	mc.updateBuf.WriteString("block_number,latency_ns,completed_at_ns\n")
	mc.blockBuf.WriteString("block_number,submitted_at_ns,completed_at_ns,update_count\n")

	// Start background writers
	mc.wg.Add(2)
	go mc.updateWriter()
	go mc.blockWriter()

	fmt.Printf("📊 Benchmark metrics will be written to:\n")
	fmt.Printf("   Updates: %s\n", updatePath)
	fmt.Printf("   Blocks:  %s\n", blockPath)

	return mc, nil
}

// updateWriter writes update metrics to file
func (mc *MetricsCollector) updateWriter() {
	defer mc.wg.Done()
	for m := range mc.updateCh {
		fmt.Fprintf(mc.updateBuf, "%d,%d,%d\n", m.BlockNumber, m.LatencyNs, m.CompletedAtNs)
		mc.totalUpdates.Add(1)
	}
}

// blockWriter writes block metrics to file
func (mc *MetricsCollector) blockWriter() {
	defer mc.wg.Done()
	for m := range mc.blockCh {
		fmt.Fprintf(mc.blockBuf, "%d,%d,%d,%d\n", m.BlockNumber, m.SubmittedAtNs, m.CompletedAtNs, m.UpdateCount)
		mc.totalBlocks.Add(1)
	}
}

// RecordBlockSubmitted records when a block is submitted to the pipeline
func (mc *MetricsCollector) RecordBlockSubmitted(blockNumber uint64, updateCount int) {
	if mc.closed.Load() {
		return
	}
	tracker := &blockTracker{
		submittedAtNs: time.Now().UnixNano(),
		updateCount:   updateCount,
	}
	tracker.remaining.Store(int64(updateCount))
	mc.blockTrackers.Store(blockNumber, tracker)
}

// RecordUpdateCompleted records when an update completes
// Returns true if this was the last update for the block
func (mc *MetricsCollector) RecordUpdateCompleted(blockNumber uint64, enqueuedAtNs int64) {
	if mc.closed.Load() {
		return
	}

	completedAt := time.Now().UnixNano()
	latency := completedAt - enqueuedAtNs

	// Record update metric
	select {
	case mc.updateCh <- UpdateMetric{
		BlockNumber:   blockNumber,
		LatencyNs:     latency,
		CompletedAtNs: completedAt,
	}:
	default:
		// Drop metric if channel is full (shouldn't happen with large buffer)
	}

	// Check if this completes the block
	if v, ok := mc.blockTrackers.Load(blockNumber); ok {
		tracker := v.(*blockTracker)
		if tracker.remaining.Add(-1) == 0 {
			// This was the last update for this block
			select {
			case mc.blockCh <- BlockMetric{
				BlockNumber:   blockNumber,
				SubmittedAtNs: tracker.submittedAtNs,
				CompletedAtNs: completedAt,
				UpdateCount:   tracker.updateCount,
			}:
			default:
			}
			mc.blockTrackers.Delete(blockNumber)
		}
	}
}

// Close flushes and closes the metrics collector
func (mc *MetricsCollector) Close() error {
	if mc.closed.Swap(true) {
		return nil // Already closed
	}

	// Close channels to signal writers to stop
	close(mc.updateCh)
	close(mc.blockCh)

	// Wait for writers to finish
	mc.wg.Wait()

	// Flush and close files
	mc.updateBuf.Flush()
	mc.blockBuf.Flush()
	mc.updateFile.Close()
	mc.blockFile.Close()

	// Print summary
	duration := time.Since(mc.startTime)
	totalUpdates := mc.totalUpdates.Load()
	totalBlocks := mc.totalBlocks.Load()

	fmt.Printf("\n📊 Benchmark Summary:\n")
	fmt.Printf("   Duration: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("   Total Updates: %d (%.2f updates/sec)\n", totalUpdates, float64(totalUpdates)/duration.Seconds())
	fmt.Printf("   Total Blocks: %d (%.2f blocks/sec)\n", totalBlocks, float64(totalBlocks)/duration.Seconds())

	return nil
}
