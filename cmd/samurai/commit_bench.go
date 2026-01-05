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
func generateCommitmentsBenchmark(cfg *config.Config, caches []*segmenttree.Cache) {
	fmt.Printf("\n🚀 Starting Benchmark Mode\n")
	fmt.Printf("   Duration: %d seconds\n", cfg.Benchmark.DurationSecs)
	fmt.Printf("   Workers: %d\n", cfg.Workers.CommitWorkerCount)

	// Initialize metrics collector
	metrics, err := benchmark.NewMetricsCollector(cfg.Benchmark.OutputDir)
	if err != nil {
		panic(fmt.Errorf("failed to create metrics collector: %w", err))
	}
	defer metrics.Close()

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

	// go logChannelSize(blockInfoCh, updateTaskChs)

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
	go feedUpdateTasksBenchmark(cfg, blockInfoCh, updateTaskChs, &stopFetching)

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
				metrics.RecordUpdateCompleted(task.BlockNumber, task.EnqueuedAt)
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
) {
	updateTaskQueues := make([]utils.Queue[updateTask], cfg.Workers.CommitWorkerCount)
	for i := range cfg.Workers.CommitWorkerCount {
		updateTaskQueues[i] = utils.NewQueue[updateTask]()
	}

	blockInfoChClosed := false
	for {
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

		// Check if any queue has hit the limit
		anyQueueAtLimit := false
		for i := range cfg.Workers.CommitWorkerCount {
			if updateTaskQueues[i].Size() >= cfg.Workers.CommitWorkerQueueSize {
				anyQueueAtLimit = true
				// fmt.Printf("⚠️ Queue %d has hit the limit: %d tasks\n", i, updateTaskQueues[i].Size())
				break
			}
		}

		// Try to read from blockInfoCh (non-blocking) only if no queue is at limit
		if !blockInfoChClosed && !anyQueueAtLimit {
			select {
			case blk, ok := <-blockInfoCh:
				if !ok {
					blockInfoChClosed = true
				} else {
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
		allQueuesEmpty := true
		anyWorkDone := false

		for i := range cfg.Workers.CommitWorkerCount {
			// Drain as many items as possible from queue[i] to channel[i]
			for !updateTaskQueues[i].IsEmpty() {
				allQueuesEmpty = false

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

		// Exit condition: all queues empty and input closed
		if allQueuesEmpty && blockInfoChClosed {
			break
		}

		// Prevent busy-wait: sleep briefly if no work was done this iteration
		if !anyWorkDone && allQueuesEmpty {
			time.Sleep(time.Microsecond * 100)
		}
	}

	fmt.Println("📤 Closing worker channels, letting in-flight updates complete...")
	for i := range cfg.Workers.CommitWorkerCount {
		close(updateTaskChs[i])
	}
	fmt.Println("📤 All worker channels closed")
}
