package benchutil

import (
	"strconv"
	"sync/atomic"
	"time"
)

// UpdateMetricsCollector collects per-update timing from commit workers using
// atomic counters and periodically writes time-windowed aggregates to a CSV.
//
// For Samurai, Record is called once per update with the actual KZG compute
// duration. For Merkle/Verkle, RecordN is called once per block with the
// block processing time and update count (amortised).
type UpdateMetricsCollector struct {
	updates   atomic.Uint64
	computeNs atomic.Int64

	csv      *BenchCSVWriter
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// UpdateMetricsCSVHeader is the header for the update-level metrics CSV.
var UpdateMetricsCSVHeader = []string{
	"window_end_ns",
	"updates_completed",
	"sum_compute_ns",
}

// NewUpdateMetricsCollector creates a collector that writes time-windowed rows
// to csvPath. Call Run to start the sampling goroutine.
func NewUpdateMetricsCollector(csvPath string, interval time.Duration) (*UpdateMetricsCollector, error) {
	csv, err := NewBenchCSVWriter(csvPath, UpdateMetricsCSVHeader)
	if err != nil {
		return nil, err
	}
	return &UpdateMetricsCollector{
		csv:      csv,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}, nil
}

// Record adds a single update completion with the given compute duration.
// Safe to call concurrently from multiple goroutines.
func (m *UpdateMetricsCollector) Record(computeNs int64) {
	m.updates.Add(1)
	m.computeNs.Add(computeNs)
}

// RecordN adds n update completions with a total compute duration.
// Used by Merkle/Verkle where updates are processed atomically per block.
func (m *UpdateMetricsCollector) RecordN(n uint64, computeNs int64) {
	m.updates.Add(n)
	m.computeNs.Add(computeNs)
}

// Run starts the periodic sampling goroutine. It blocks until Stop is called.
func (m *UpdateMetricsCollector) Run() {
	defer close(m.doneCh)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	var prevUpdates uint64
	var prevComputeNs int64

	for {
		select {
		case <-ticker.C:
			now := time.Now().UnixNano()
			curUpdates := m.updates.Load()
			curComputeNs := m.computeNs.Load()

			deltaUpdates := curUpdates - prevUpdates
			deltaComputeNs := curComputeNs - prevComputeNs

			if deltaUpdates > 0 {
				_ = m.csv.WriteRow(
					strconv.FormatInt(now, 10),
					strconv.FormatUint(deltaUpdates, 10),
					strconv.FormatInt(deltaComputeNs, 10),
				)
			}

			prevUpdates = curUpdates
			prevComputeNs = curComputeNs

		case <-m.stopCh:
			// Write final partial window.
			now := time.Now().UnixNano()
			curUpdates := m.updates.Load()
			curComputeNs := m.computeNs.Load()

			deltaUpdates := curUpdates - prevUpdates
			deltaComputeNs := curComputeNs - prevComputeNs

			if deltaUpdates > 0 {
				_ = m.csv.WriteRow(
					strconv.FormatInt(now, 10),
					strconv.FormatUint(deltaUpdates, 10),
					strconv.FormatInt(deltaComputeNs, 10),
				)
			}

			m.csv.Close()
			return
		}
	}
}

// Stop signals the sampling goroutine to flush and exit, then waits for it.
func (m *UpdateMetricsCollector) Stop() {
	close(m.stopCh)
	<-m.doneCh
}
