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

// PipelineMetric records queue and channel sizes at a point in time for a single shard
type PipelineMetric struct {
	TimestampNs     int64
	ShardID         int
	QueueSize       int
	ChannelSize     int
	ChannelCapacity int
}

// PipelineMetricsCollector collects pipeline size metrics periodically
type PipelineMetricsCollector struct {
	metricsCh chan PipelineMetric

	// Output file
	pipelineFile *os.File
	pipelineBuf  *bufio.Writer

	// Synchronization
	wg     sync.WaitGroup
	stopCh chan struct{}
	closed atomic.Bool

	// Stats
	totalSamples atomic.Int64
	startTime    time.Time
}

// NewPipelineMetricsCollector creates a new pipeline metrics collector
func NewPipelineMetricsCollector(outputDir string) (*PipelineMetricsCollector, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")

	// Create pipeline metrics file
	pipelinePath := filepath.Join(outputDir, fmt.Sprintf("bench_pipeline_sizes_%s.csv", timestamp))
	pipelineFile, err := os.Create(pipelinePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline metrics file: %w", err)
	}

	pmc := &PipelineMetricsCollector{
		metricsCh:    make(chan PipelineMetric, 100000), // Large buffer
		pipelineFile: pipelineFile,
		pipelineBuf:  bufio.NewWriterSize(pipelineFile, 1<<18), // 256KB buffer
		stopCh:       make(chan struct{}),
		startTime:    time.Now(),
	}

	// Write CSV header
	pmc.pipelineBuf.WriteString("timestamp_ns,shard_id,queue_size,channel_size,channel_capacity\n")

	// Start background writer
	pmc.wg.Add(1)
	go pmc.pipelineWriter()

	fmt.Printf("   Pipeline Sizes: %s\n", pipelinePath)

	return pmc, nil
}

// pipelineWriter writes pipeline metrics to file
func (pmc *PipelineMetricsCollector) pipelineWriter() {
	defer pmc.wg.Done()
	for m := range pmc.metricsCh {
		fmt.Fprintf(pmc.pipelineBuf, "%d,%d,%d,%d,%d\n",
			m.TimestampNs,
			m.ShardID,
			m.QueueSize,
			m.ChannelSize,
			m.ChannelCapacity,
		)
		pmc.totalSamples.Add(1)
	}
}

// QueueSizeProvider is a function that returns the size of a queue for a given shard
type QueueSizeProvider func(shardID int) int

// ChannelSizeProvider is a function that returns the size and capacity of a channel for a given shard
type ChannelSizeProvider func(shardID int) (size int, capacity int)

// CollectMetrics collects metrics for all shards
func (pmc *PipelineMetricsCollector) CollectMetrics(numShards int, queueSizes QueueSizeProvider, channelSizes ChannelSizeProvider) {
	if pmc.closed.Load() {
		return
	}

	now := time.Now().UnixNano()

	for shardID := 0; shardID < numShards; shardID++ {
		queueSize := queueSizes(shardID)
		channelSize, channelCapacity := channelSizes(shardID)

		metric := PipelineMetric{
			TimestampNs:     now,
			ShardID:         shardID,
			QueueSize:       queueSize,
			ChannelSize:     channelSize,
			ChannelCapacity: channelCapacity,
		}

		select {
		case pmc.metricsCh <- metric:
		default:
			// Drop if channel full
		}
	}
}

// StartPeriodicCollection starts collecting metrics at the specified interval
func (pmc *PipelineMetricsCollector) StartPeriodicCollection(
	numShards int,
	queueSizes QueueSizeProvider,
	channelSizes ChannelSizeProvider,
	interval time.Duration,
) {
	pmc.wg.Add(1)
	go func() {
		defer pmc.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				pmc.CollectMetrics(numShards, queueSizes, channelSizes)
			case <-pmc.stopCh:
				return
			}
		}
	}()
}

// Close stops collection and flushes data
func (pmc *PipelineMetricsCollector) Close() error {
	if pmc.closed.Swap(true) {
		return nil
	}

	// Signal periodic collector to stop
	close(pmc.stopCh)

	// Close channel to signal writer to stop
	close(pmc.metricsCh)

	// Wait for goroutines to finish
	pmc.wg.Wait()

	// Flush and close file
	pmc.pipelineBuf.Flush()
	pmc.pipelineFile.Close()

	fmt.Printf("   Pipeline Sizes samples collected: %d\n", pmc.totalSamples.Load())

	return nil
}

