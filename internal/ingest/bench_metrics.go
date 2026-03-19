package ingest

import (
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nepal80m/samurai/internal/utils"
)

// ---------------------------------------------------------------------------
// Atomic helpers
// ---------------------------------------------------------------------------

func atomicMinInt64(a *atomic.Int64, v int64) {
	for {
		old := a.Load()
		if v >= old {
			return
		}
		if a.CompareAndSwap(old, v) {
			return
		}
	}
}

func atomicMaxInt64(a *atomic.Int64, v int64) {
	for {
		old := a.Load()
		if v <= old {
			return
		}
		if a.CompareAndSwap(old, v) {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Per-block accumulator -- updated atomically by concurrent commit workers
// ---------------------------------------------------------------------------

// blockAccumulator aggregates per-update metrics for one block. Commitment
// workers call RecordCommitLatency / RecordQueueWait from different goroutines;
// all fields use atomics to avoid locking.
type blockAccumulator struct {
	commitCount  atomic.Uint64
	commitSumNs  atomic.Int64
	commitMinNs  atomic.Int64
	commitMaxNs  atomic.Int64
	queueWtCount atomic.Uint64
	queueWtSumNs atomic.Int64
	queueWtMinNs atomic.Int64
	queueWtMaxNs atomic.Int64
}

func newBlockAccumulator() *blockAccumulator {
	ba := &blockAccumulator{}
	ba.commitMinNs.Store(math.MaxInt64)
	ba.queueWtMinNs.Store(math.MaxInt64)
	return ba
}

// ---------------------------------------------------------------------------
// MetricsCollector
// ---------------------------------------------------------------------------

// MetricsCollector is the central benchmark recorder. It owns:
//   - global atomic counters (for periodic telemetry)
//   - a sync.Map of per-block accumulators (for per-update aggregation)
//   - a buffered CSV writer for per-block rows
//   - summary bookkeeping
type MetricsCollector struct {
	// Global atomic counters -- hot path, read by telemetry goroutine.
	RawUpdates       atomic.Uint64
	SelectedUpdates  atomic.Uint64
	DiscardedUpdates atomic.Uint64
	CommitCompleted  atomic.Uint64
	BlocksCommitted  atomic.Uint64

	// Per-block accumulators keyed by block number.
	blockAccums sync.Map // uint64 -> *blockAccumulator

	// Per-block CSV output (written only by the MPT worker, single-threaded).
	blockFile *os.File
	blockCSV  *csv.Writer

	// Telemetry goroutine control.
	telemetryStop chan struct{}
	telemetryWG   sync.WaitGroup

	// Summary fields.
	BenchStart time.Time
	BenchEnd   time.Time
	StartBlock uint64
	Duration   time.Duration
	NumUsers   int
	OutputDir  string
}

var blockCSVHeader = []string{
	"block_number",
	"raw_updates",
	"selected_updates",
	"discarded_updates",
	"emitted_at_ns",
	"mpt_start_at_ns",
	"completed_at_ns",
	"e2e_latency_ms",
	"mpt_phase_latency_ms",
	"wait_commitments_latency_ms",
	"commit_state_latency_ms",
	"flush_trie_latency_ms",
	"commit_latency_count",
	"commit_latency_sum_us",
	"commit_latency_min_us",
	"commit_latency_max_us",
	"commit_latency_avg_us",
	"queue_wait_count",
	"queue_wait_sum_us",
	"queue_wait_min_us",
	"queue_wait_max_us",
	"queue_wait_avg_us",
}

func NewMetricsCollector(outputDir string, startBlock uint64, duration time.Duration, numUsers int) (*MetricsCollector, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	blockPath := filepath.Join(outputDir, "blocks.csv")
	f, err := os.Create(blockPath)
	if err != nil {
		return nil, fmt.Errorf("create blocks.csv: %w", err)
	}

	w := csv.NewWriter(f)
	if err := w.Write(blockCSVHeader); err != nil {
		f.Close()
		return nil, fmt.Errorf("write CSV header: %w", err)
	}

	return &MetricsCollector{
		blockFile:     f,
		blockCSV:      w,
		telemetryStop: make(chan struct{}),
		StartBlock:    startBlock,
		Duration:      duration,
		NumUsers:      numUsers,
		OutputDir:     outputDir,
	}, nil
}

// ---------------------------------------------------------------------------
// Per-block accumulator access
// ---------------------------------------------------------------------------

func (mc *MetricsCollector) GetOrCreateAccumulator(blockNumber uint64) *blockAccumulator {
	if v, ok := mc.blockAccums.Load(blockNumber); ok {
		return v.(*blockAccumulator)
	}
	ba := newBlockAccumulator()
	actual, _ := mc.blockAccums.LoadOrStore(blockNumber, ba)
	return actual.(*blockAccumulator)
}

// RecordCommitLatency records a single commitment latency sample for a block.
func (mc *MetricsCollector) RecordCommitLatency(blockNumber uint64, durNs int64) {
	ba := mc.GetOrCreateAccumulator(blockNumber)
	ba.commitCount.Add(1)
	ba.commitSumNs.Add(durNs)
	atomicMinInt64(&ba.commitMinNs, durNs)
	atomicMaxInt64(&ba.commitMaxNs, durNs)
	mc.CommitCompleted.Add(1)
}

// RecordQueueWait records a single queue-wait sample for a block.
func (mc *MetricsCollector) RecordQueueWait(blockNumber uint64, durNs int64) {
	ba := mc.GetOrCreateAccumulator(blockNumber)
	ba.queueWtCount.Add(1)
	ba.queueWtSumNs.Add(durNs)
	atomicMinInt64(&ba.queueWtMinNs, durNs)
	atomicMaxInt64(&ba.queueWtMaxNs, durNs)
}

// ---------------------------------------------------------------------------
// Per-block CSV row (called from the single-threaded MPT worker)
// ---------------------------------------------------------------------------

// WriteBlockRow writes one row to blocks.csv and removes the block's
// accumulator from the map. Must be called from a single goroutine (MPT
// worker).
func (mc *MetricsCollector) WriteBlockRow(
	blockNumber, rawUpdates, selectedUpdates, discardedUpdates uint64,
	emittedAtNs, mptStartNs, completedAtNs int64,
	mptPhaseNs, waitCommitmentsNs int64,
	commitStateNs, flushTrieNs int64,
) {
	e2eNs := completedAtNs - emittedAtNs

	var (
		clCount                                      uint64
		clSumUs, clMinUs, clMaxUs, clAvgUs           float64
		qwCount                                      uint64
		qwSumUs, qwMinUs, qwMaxUs, qwAvgUs          float64
	)

	if v, ok := mc.blockAccums.LoadAndDelete(blockNumber); ok {
		ba := v.(*blockAccumulator)
		clCount = ba.commitCount.Load()
		if clCount > 0 {
			clSumUs = float64(ba.commitSumNs.Load()) / 1e3
			clMinUs = float64(ba.commitMinNs.Load()) / 1e3
			clMaxUs = float64(ba.commitMaxNs.Load()) / 1e3
			clAvgUs = clSumUs / float64(clCount)
		}
		qwCount = ba.queueWtCount.Load()
		if qwCount > 0 {
			qwSumUs = float64(ba.queueWtSumNs.Load()) / 1e3
			qwMinUs = float64(ba.queueWtMinNs.Load()) / 1e3
			qwMaxUs = float64(ba.queueWtMaxNs.Load()) / 1e3
			qwAvgUs = qwSumUs / float64(qwCount)
		}
	}

	row := []string{
		strconv.FormatUint(blockNumber, 10),
		strconv.FormatUint(rawUpdates, 10),
		strconv.FormatUint(selectedUpdates, 10),
		strconv.FormatUint(discardedUpdates, 10),
		strconv.FormatInt(emittedAtNs, 10),
		strconv.FormatInt(mptStartNs, 10),
		strconv.FormatInt(completedAtNs, 10),
		fmt.Sprintf("%.3f", float64(e2eNs)/1e6),
		fmt.Sprintf("%.3f", float64(mptPhaseNs)/1e6),
		fmt.Sprintf("%.3f", float64(waitCommitmentsNs)/1e6),
		fmt.Sprintf("%.3f", float64(commitStateNs)/1e6),
		fmt.Sprintf("%.3f", float64(flushTrieNs)/1e6),
		strconv.FormatUint(clCount, 10),
		fmt.Sprintf("%.3f", clSumUs),
		fmt.Sprintf("%.3f", clMinUs),
		fmt.Sprintf("%.3f", clMaxUs),
		fmt.Sprintf("%.3f", clAvgUs),
		strconv.FormatUint(qwCount, 10),
		fmt.Sprintf("%.3f", qwSumUs),
		fmt.Sprintf("%.3f", qwMinUs),
		fmt.Sprintf("%.3f", qwMaxUs),
		fmt.Sprintf("%.3f", qwAvgUs),
	}

	_ = mc.blockCSV.Write(row)

	mc.BlocksCommitted.Add(1)
}

// ---------------------------------------------------------------------------
// Periodic telemetry
// ---------------------------------------------------------------------------

// StartTelemetry launches a background goroutine that prints pipeline counters
// and queue depths to stderr at the given interval.
func (mc *MetricsCollector) StartTelemetry(
	interval time.Duration,
	queues []*utils.BoundedQueue[UpdateTask],
	commitCh <-chan mptUpdateCommitmentInfo,
) {
	mc.telemetryWG.Add(1)
	go func() {
		defer mc.telemetryWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-mc.telemetryStop:
				return
			case <-ticker.C:
				elapsed := time.Since(start).Seconds()
				depths := make([]int, len(queues))
				for i, q := range queues {
					depths[i] = q.Len()
				}

				totalQueueDepth := 0
				for _, d := range depths {
					totalQueueDepth += d
				}

				log.Printf("[bench %.1fs] raw=%d sel=%d disc=%d committed=%d blocks=%d totalQueueDepth=%d commitChDepth=%d",
					elapsed,
					mc.RawUpdates.Load(),
					mc.SelectedUpdates.Load(),
					mc.DiscardedUpdates.Load(),
					mc.CommitCompleted.Load(),
					mc.BlocksCommitted.Load(),
					totalQueueDepth,
					len(commitCh),
				)
			}
		}
	}()
}

// StopTelemetry signals the telemetry goroutine to exit and waits for it.
func (mc *MetricsCollector) StopTelemetry() {
	close(mc.telemetryStop)
	mc.telemetryWG.Wait()
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

// WriteSummary writes a key-value summary CSV and flushes the block CSV.
func (mc *MetricsCollector) WriteSummary() error {
	mc.blockCSV.Flush()
	if err := mc.blockCSV.Error(); err != nil {
		return fmt.Errorf("flush blocks.csv: %w", err)
	}

	wallSec := mc.BenchEnd.Sub(mc.BenchStart).Seconds()
	totalBlocks := mc.BlocksCommitted.Load()
	totalSel := mc.SelectedUpdates.Load()
	totalCommit := mc.CommitCompleted.Load()

	var e2eUPS, e2eBPS, commitUPS float64
	if wallSec > 0 {
		e2eUPS = float64(totalSel) / wallSec
		e2eBPS = float64(totalBlocks) / wallSec
		commitUPS = float64(totalCommit) / wallSec
	}

	summaryPath := filepath.Join(mc.OutputDir, "summary.csv")
	f, err := os.Create(summaryPath)
	if err != nil {
		return fmt.Errorf("create summary.csv: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	rows := [][]string{
		{"key", "value"},
		{"start_block", strconv.FormatUint(mc.StartBlock, 10)},
		{"configured_duration_sec", strconv.FormatFloat(mc.Duration.Seconds(), 'f', 1, 64)},
		{"num_users", strconv.Itoa(mc.NumUsers)},
		{"bench_start_ns", strconv.FormatInt(mc.BenchStart.UnixNano(), 10)},
		{"bench_end_ns", strconv.FormatInt(mc.BenchEnd.UnixNano(), 10)},
		{"wall_clock_sec", fmt.Sprintf("%.3f", wallSec)},
		{"total_blocks", strconv.FormatUint(totalBlocks, 10)},
		{"total_raw_updates", strconv.FormatUint(mc.RawUpdates.Load(), 10)},
		{"total_selected_updates", strconv.FormatUint(totalSel, 10)},
		{"total_discarded_updates", strconv.FormatUint(mc.DiscardedUpdates.Load(), 10)},
		{"avg_e2e_updates_per_sec", fmt.Sprintf("%.2f", e2eUPS)},
		{"avg_e2e_blocks_per_sec", fmt.Sprintf("%.2f", e2eBPS)},
		{"avg_commit_updates_per_sec", fmt.Sprintf("%.2f", commitUPS)},
	}
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("write summary.csv: %w", err)
	}

	return nil
}

// Close flushes and closes the block CSV file.
func (mc *MetricsCollector) Close() {
	mc.blockCSV.Flush()
	mc.blockFile.Close()
}
