package main

import (
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/benchmark"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/segmenttree"
	"github.com/nepal80m/samurai/internal/utils"
)

// generateCommitmentsBenchmark runs the commit generation in benchmark mode
// It runs for a fixed duration, collecting detailed metrics
func generateCommitmentsBenchmark(cfg *config.Config, caches []*segmenttree.Cache, dbs []*segmenttree.PebbleDB) {
	fmt.Printf("\n🚀 Starting Benchmark Mode\n")
	fmt.Printf("   Duration: %d seconds\n", cfg.Benchmark.DurationSecs)
	fmt.Printf("   Workers: %d\n", cfg.Workers.CommitWorkerCount)

	// Initialize metrics collector (always enabled for latency/throughput)
	metrics, err := benchmark.NewMetricsCollector(cfg.Benchmark.OutputDir)
	if err != nil {
		panic(fmt.Errorf("failed to create metrics collector: %w", err))
	}
	defer metrics.Close()

	// Initialize DB metrics collector (optional)
	var dbMetrics *benchmark.DBMetricsCollector
	if cfg.Benchmark.CollectDBMetrics {
		dbMetrics, err = benchmark.NewDBMetricsCollector(cfg.Benchmark.OutputDir)
		if err != nil {
			panic(fmt.Errorf("failed to create DB metrics collector: %w", err))
		}
		defer dbMetrics.Close()

		// Convert dbs to PebbleMetricsProvider interface slice
		dbProviders := make([]benchmark.PebbleMetricsProvider, len(dbs))
		for i, db := range dbs {
			dbProviders[i] = db
		}
		// Start periodic DB metrics collection (every 1 second)
		dbMetrics.StartPeriodicCollection(dbProviders, time.Second)
	}

	// Initialize pipeline metrics collector (optional)
	var pipelineMetrics *benchmark.PipelineMetricsCollector
	if cfg.Benchmark.CollectPipelineSizes {
		pipelineMetrics, err = benchmark.NewPipelineMetricsCollector(cfg.Benchmark.OutputDir)
		if err != nil {
			panic(fmt.Errorf("failed to create pipeline metrics collector: %w", err))
		}
		defer pipelineMetrics.Close()
	}

	// Initialize cache metrics collector (optional)
	var cacheMetrics *benchmark.CacheMetricsCollector
	if cfg.Benchmark.CollectCacheMetrics {
		cacheMetrics, err = benchmark.NewCacheMetricsCollector(cfg.Benchmark.OutputDir)
		if err != nil {
			panic(fmt.Errorf("failed to create cache metrics collector: %w", err))
		}
		defer cacheMetrics.Close()

		// Convert caches to CacheMetricsProvider interface slice
		cacheProviders := make([]benchmark.CacheMetricsProvider, len(caches))
		for i, cache := range caches {
			cacheProviders[i] = cache
		}
		// Start periodic cache metrics collection (every 1 second)
		cacheMetrics.StartPeriodicCollection(cacheProviders, time.Second)
	}

	// Channels
	blockInfoCh := make(chan blockInfo, cfg.Queue.BlockInfoChannelSize)
	updateTaskChs := make([]chan updateTask, cfg.Workers.CommitWorkerCount)
	for i := range cfg.Workers.CommitWorkerCount {
		updateTaskChs[i] = make(chan updateTask, cfg.Workers.CommitWorkerChannelSize)
	}

	// Control flag for stopping block fetcher
	var stopFetching atomic.Bool

	// Start benchmark timer
	benchStart := time.Now()
	benchDuration := time.Duration(cfg.Benchmark.DurationSecs) * time.Second

	go logChannelSizeBench(blockInfoCh, updateTaskChs)

	// Start block fetcher (runs for benchDuration)
	go spawnBlockFetcherBenchmark(
		cfg.Blocks.StartingBlockNumber,
		cfg.Blocks.EndingBlockNumber,
		blockInfoCh,
		&stopFetching,
		metrics,
	)

	// Stop fetcher after benchmark duration
	go func() {
		time.Sleep(benchDuration)
		fmt.Printf("\n⏱️  Benchmark duration reached (%v), stopping block fetcher...\n", benchDuration)
		stopFetching.Store(true)
	}()

	// Feed update tasks (with timing)
	go feedUpdateTasksBenchmark(cfg, blockInfoCh, updateTaskChs, &stopFetching, pipelineMetrics)

	// Start workers
	wg := sync.WaitGroup{}
	var accountsSeen sync.Map
	for i := range cfg.Workers.CommitWorkerCount {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range updateTaskChs[workerID] {
				segmenttree.CreateOrUpdateAccountInfo(
					task.Account,
					task.Balance,
					task.BlockNumber,
					caches[workerID],
					&accountsSeen,
				)
				// Record completion
				metrics.RecordUpdateCompleted(workerID, task.BlockNumber, task.EnqueuedAt)
			}
		}(i)
	}

	wg.Wait()

	totalDuration := time.Since(benchStart)
	fmt.Printf("\n✅ Benchmark completed in %v\n", totalDuration.Round(time.Millisecond))
}

// spawnBlockFetcherBenchmark fetches blocks until stopped or dataset exhausted
func spawnBlockFetcherBenchmark(
	startingBlockNumber uint64,
	endingBlockNumber uint64,
	blockInfoCh chan blockInfo,
	stopFetching *atomic.Bool,
	metrics *benchmark.MetricsCollector,
) {
	defer close(blockInfoCh)

	r := dataset.NewDatasetReader(dataset.DATASET_DIR, dataset.SEGMENT_SIZE)
	defer r.Close()

	blocksSubmitted := 0
	for bn := startingBlockNumber; bn <= endingBlockNumber; bn++ {
		if stopFetching.Load() {
			fmt.Printf("⏱️ Block fetcher stopped by timer after %d blocks\n", blocksSubmitted)
			return
		}

		entries, err := r.GetBlock(uint32(bn))
		if err != nil {
			// Dataset exhausted or block not available - stop gracefully
			fmt.Printf("⚠️Block fetcher stopped: dataset exhausted at block %d (%d blocks submitted): %v\n", bn, blocksSubmitted, err)
			return
		}

		blockInfoCh <- blockInfo{
			Number:  bn,
			Entries: entries,
		}
		// Record block submission after sending to channel
		metrics.RecordBlockSubmitted(bn, len(entries))
		blocksSubmitted++

		if blocksSubmitted%10000 == 0 {
			fmt.Printf("📦 Submitted %d blocks to pipeline\n", blocksSubmitted)
		}
	}

	fmt.Printf("📦 Block fetcher completed full range (%d blocks)\n", blocksSubmitted)
}

// feedUpdateTasksBenchmark distributes updates to workers with timing information
func feedUpdateTasksBenchmark(
	cfg *config.Config,
	blockInfoCh chan blockInfo,
	updateTaskChs []chan updateTask,
	stopFeeding *atomic.Bool,
	pipelineMetrics *benchmark.PipelineMetricsCollector,
) {
	updateTaskQueues := make([]utils.Queue[updateTask], cfg.Workers.CommitWorkerCount)
	for i := range cfg.Workers.CommitWorkerCount {
		updateTaskQueues[i] = utils.NewQueue[updateTask]()
	}

	// Start pipeline metrics collection if enabled
	if pipelineMetrics != nil {
		queueSizes := func(shardID int) int {
			return updateTaskQueues[shardID].Size()
		}
		channelSizes := func(shardID int) (int, int) {
			return len(updateTaskChs[shardID]), cap(updateTaskChs[shardID])
		}
		pipelineMetrics.StartPeriodicCollection(
			cfg.Workers.CommitWorkerCount,
			queueSizes,
			channelSizes,
			100*time.Millisecond, // Sample every 100ms
		)
	}

	blockInfoChClosed := false
	allUpdateTaskQueuesEmpty := false
	for !(blockInfoChClosed && allUpdateTaskQueuesEmpty) {

		// Check if benchmark timer expired - stop feeding from queues
		if stopFeeding.Load() {
			// Count remaining items in queues that won't be processed
			var totalDropped int
			for i := range cfg.Workers.CommitWorkerCount {
				totalDropped += updateTaskQueues[i].Size()
			}
			fmt.Printf("⏱️ Update feeder stopped by timer. Dropped %d queued updates.\n", totalDropped)
			break
		}

		// check if all updateTaskCh are full
		allUpdateTaskChsFull := true
		for i := range cfg.Workers.CommitWorkerCount {
			if len(updateTaskChs[i]) < cfg.Workers.CommitWorkerChannelSize {
				allUpdateTaskChsFull = false
				break
			}
		}
		if allUpdateTaskChsFull {
			sleepTime := time.Millisecond * 500
			// fmt.Println("All updateTaskChs are full, skipping the fetch of new blocks and sleeping for", sleepTime)
			// TODO: decide whether to sleep for a longer time or not
			time.Sleep(sleepTime)
			continue
		}

		// Check if any queue has hit the limit
		anyQueueAtLimit := false
		for i := range cfg.Workers.CommitWorkerCount {
			if updateTaskQueues[i].Size() >= cfg.Workers.CommitWorkerQueueSize {
				anyQueueAtLimit = true
				// fmt.Printf("⚠️ Queue %d has hit the limit: %d tasks\n", i, updateTaskQueues[i].Size())
				break
			}
		}

		anyWorkDone := false
		// Try to read from blockInfoCh (non-blocking) only if no queue is at limit
		if !blockInfoChClosed && !anyQueueAtLimit {
			select {
			case blk, ok := <-blockInfoCh:
				if !ok {
					blockInfoChClosed = true
				} else {
					anyWorkDone = true

					// Enqueue all entries from this block
					for _, entry := range blk.Entries {
						chIdx := utils.AddressToShardIndex(entry.Address, cfg.Workers.CommitWorkerCount)
						updateTaskQueues[chIdx].Enqueue(updateTask{
							BlockNumber: blk.Number,
							Account:     common.BytesToAddress(entry.Address[:]),
							Balance:     new(big.Int).SetBytes(entry.Balance),
							// EnqueuedAt will be set when actually sent to channel
						})
					}
				}
			default:
				// No data available right now
			}
		}

		// Drain queues to channels (non-blocking)
		allUpdateTaskQueuesEmpty = true

		for i := range cfg.Workers.CommitWorkerCount {
			// Drain as many items as possible from queue[i] to channel[i]
			for !updateTaskQueues[i].IsEmpty() {
				allUpdateTaskQueuesEmpty = false

				task, err := updateTaskQueues[i].Peek()
				if err != nil {
					panic(fmt.Errorf("failed to peek update task: %w", err))
				}

				// Set enqueue timestamp right before sending
				task.EnqueuedAt = time.Now().UnixNano()

				// Try non-blocking send
				select {
				case updateTaskChs[i] <- task:
					_, err := updateTaskQueues[i].Dequeue()
					if err != nil {
						panic(fmt.Errorf("failed to dequeue update task: %w", err))
					}
					anyWorkDone = true
				default:
					// Channel is full, move to next queue
					goto nextQueue
				}
			}
		nextQueue:
		}

		// Prevent busy-wait: sleep briefly if no work was done this iteration
		if !anyWorkDone {
			time.Sleep(time.Microsecond * 100)
		}
	}

	fmt.Println("📤 Closing worker channels, letting in-flight updates complete...")
	for i := range cfg.Workers.CommitWorkerCount {
		close(updateTaskChs[i])
	}
	fmt.Println("📤 All worker channels closed")
}
func logChannelSizeBench(blockInfoCh chan blockInfo, updateTaskChs []chan updateTask) {
	// keep logging the size of the channel every 5 seconds until the channel is closed
	for {
		time.Sleep(1 * time.Millisecond)
		// remaining := cap(blockInfoCh) - len(blockInfoCh)
		// // if remaining > 5 {
		// // 	fmt.Printf("BlockInfoCh: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		// // }
		// if remaining > 0 && remaining < 5 {
		// 	fmt.Printf("⚠️ BlockInfoCh is almost full: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		// }
		// if remaining <= 0 {
		// 	fmt.Printf("🚨 BlockInfoCh is full: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		// }

		// remaining = cap(orderedBlockInfoCh) - len(orderedBlockInfoCh)
		// if remaining > 5 {
		// 	fmt.Printf("OrderedBlockInfoCh: %d/%d\n", len(orderedBlockInfoCh), cap(orderedBlockInfoCh))
		// }
		// if remaining > 0 && remaining < 5 {
		// 	fmt.Printf("⚠️ OrderedBlockInfoCh is almost full, %d/%d\n", len(orderedBlockInfoCh), cap(orderedBlockInfoCh))
		// }
		// if remaining <= 0 {
		// 	fmt.Printf("🚨 OrderedBlockInfoCh is full: %d/%d\n", len(orderedBlockInfoCh), cap(orderedBlockInfoCh))
		// }
		for i, updateTaskCh := range updateTaskChs {
			if len(updateTaskCh) < 10 {
				fmt.Printf("🚨 UpdateTaskCh %d is almost empty: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			}
			// remaining := cap(updateTaskCh) - len(updateTaskCh)
			// if remaining > 5 {
			// 	fmt.Printf("UpdateTaskCh %d: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			// }
			// if remaining > 0 && remaining < 5 {
			// 	fmt.Printf("⚠️ UpdateTaskCh %d is almost full: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			// }
			// if remaining <= 0 {
			// 	fmt.Printf("🚨 UpdateTaskCh %d is full: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			// }
		}

	}
}
