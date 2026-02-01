package commands

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
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/utils"
)

// RunCommitBenchmark runs the commit generation in benchmark mode.
// It runs for a fixed duration, collecting per-worker latency and throughput.
func RunCommitBenchmark(cfg *config.Config, caches []*storage.Cache, dbs []*db.PebbleDB) {
	fmt.Printf("\n🚀 Starting Benchmark Mode\n")
	fmt.Printf("   Duration: %d seconds\n", cfg.Benchmark.DurationSecs)
	fmt.Printf("   Workers: %d\n", cfg.Workers.CommitWorkerCount)

	// Initialize metrics collector (per-worker latency and throughput)
	metrics, err := benchmark.NewMetricsCollector(cfg.Benchmark.OutputDir)
	if err != nil {
		panic(fmt.Errorf("failed to create metrics collector: %w", err))
	}
	defer metrics.Close()

	// Channels
	blockInfoCh := make(chan BlockInfo, cfg.Queue.BlockInfoChannelSize)
	updateTaskChs := make([]chan UpdateTask, cfg.Workers.CommitWorkerCount)
	for i := range cfg.Workers.CommitWorkerCount {
		updateTaskChs[i] = make(chan UpdateTask, cfg.Workers.CommitWorkerChannelSize)
	}

	// Control flag for stopping block fetcher
	var stopFetching atomic.Bool

	// Start benchmark timer
	benchStart := time.Now()
	benchDuration := time.Duration(cfg.Benchmark.DurationSecs) * time.Second

	// Start block fetcher
	go spawnBlockFetcherBenchmark(
		cfg.Blocks.StartingBlockNumber,
		cfg.Blocks.EndingBlockNumber,
		blockInfoCh,
		&stopFetching,
		metrics,
		cfg.BlocksDataDir,
	)

	// Stop fetcher after benchmark duration
	go func() {
		time.Sleep(benchDuration)
		fmt.Printf("\n⏱️  Benchmark duration reached (%v), stopping block fetcher...\n", benchDuration)
		stopFetching.Store(true)
	}()

	// Feed update tasks
	go feedUpdateTasksBenchmark(cfg, blockInfoCh, updateTaskChs, &stopFetching)

	// Start workers
	wg := sync.WaitGroup{}
	for i := range cfg.Workers.CommitWorkerCount {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range updateTaskChs[workerID] {
				storage.CreateOrUpdateAccountInfo(
					task.Account,
					task.Balance,
					task.BlockNumber,
					caches[workerID],
				)
				metrics.RecordUpdateCompleted(workerID, task.BlockNumber, task.EnqueuedAt)
			}
		}(i)
	}

	wg.Wait()

	totalDuration := time.Since(benchStart)
	fmt.Printf("\n✅ Benchmark completed in %v\n", totalDuration.Round(time.Millisecond))
}

func spawnBlockFetcherBenchmark(
	startingBlockNumber uint64,
	endingBlockNumber uint64,
	blockInfoCh chan BlockInfo,
	stopFetching *atomic.Bool,
	metrics *benchmark.MetricsCollector,
	dataDir string,
) {
	defer close(blockInfoCh)

	r := dataset.NewDatasetReader(dataDir, dataset.SEGMENT_SIZE)
	defer r.Close()

	blocksSubmitted := 0
	for bn := startingBlockNumber; bn <= endingBlockNumber; bn++ {
		if stopFetching.Load() {
			fmt.Printf("⏱️ Block fetcher stopped by timer after %d blocks\n", blocksSubmitted)
			return
		}

		entries, err := r.GetBlock(uint32(bn))
		if err != nil {
			fmt.Printf("⚠️Block fetcher stopped: dataset exhausted at block %d (%d blocks submitted): %v\n", bn, blocksSubmitted, err)
			return
		}

		blockInfoCh <- BlockInfo{
			Number:  bn,
			Entries: entries,
		}
		metrics.RecordBlockSubmitted(bn, len(entries))
		blocksSubmitted++

		if blocksSubmitted%10000 == 0 {
			fmt.Printf("📦 Submitted %d blocks to pipeline\n", blocksSubmitted)
		}
	}

	fmt.Printf("📦 Block fetcher completed full range (%d blocks)\n", blocksSubmitted)
}

func feedUpdateTasksBenchmark(
	cfg *config.Config,
	blockInfoCh chan BlockInfo,
	updateTaskChs []chan UpdateTask,
	stopFeeding *atomic.Bool,
) {
	updateTaskQueues := make([]utils.Queue[UpdateTask], cfg.Workers.CommitWorkerCount)
	for i := range cfg.Workers.CommitWorkerCount {
		updateTaskQueues[i] = utils.NewQueue[UpdateTask]()
	}

	blockInfoChClosed := false
	allUpdateTaskQueuesEmpty := false
	for !(blockInfoChClosed && allUpdateTaskQueuesEmpty) {
		if stopFeeding.Load() {
			var totalDropped int
			for i := range cfg.Workers.CommitWorkerCount {
				totalDropped += updateTaskQueues[i].Size()
			}
			fmt.Printf("⏱️ Update feeder stopped by timer. Dropped %d queued updates.\n", totalDropped)
			break
		}

		// Check if all channels are full
		allUpdateTaskChsFull := true
		for i := range cfg.Workers.CommitWorkerCount {
			if len(updateTaskChs[i]) < cfg.Workers.CommitWorkerChannelSize {
				allUpdateTaskChsFull = false
				break
			}
		}
		if allUpdateTaskChsFull {
			time.Sleep(time.Millisecond * 500)
			continue
		}

		// Check if any queue has hit the limit
		anyQueueAtLimit := false
		for i := range cfg.Workers.CommitWorkerCount {
			if updateTaskQueues[i].Size() >= cfg.Workers.CommitWorkerQueueSize {
				anyQueueAtLimit = true
				break
			}
		}

		anyWorkDone := false
		if !blockInfoChClosed && !anyQueueAtLimit {
			select {
			case blk, ok := <-blockInfoCh:
				if !ok {
					blockInfoChClosed = true
				} else {
					anyWorkDone = true
					for _, entry := range blk.Entries {
						chIdx := utils.AddressToShardIndex(entry.Address, cfg.Workers.CommitWorkerCount)
						updateTaskQueues[chIdx].Enqueue(UpdateTask{
							BlockNumber: blk.Number,
							Account:     common.BytesToAddress(entry.Address[:]),
							Balance:     new(big.Int).SetBytes(entry.Balance),
						})
					}
				}
			default:
			}
		}

		// Drain queues to channels
		allUpdateTaskQueuesEmpty = true
		for i := range cfg.Workers.CommitWorkerCount {
			for !updateTaskQueues[i].IsEmpty() {
				allUpdateTaskQueuesEmpty = false

				task, err := updateTaskQueues[i].Peek()
				if err != nil {
					panic(fmt.Errorf("failed to peek update task: %w", err))
				}

				task.EnqueuedAt = time.Now().UnixNano()

				select {
				case updateTaskChs[i] <- task:
					_, err := updateTaskQueues[i].Dequeue()
					if err != nil {
						panic(fmt.Errorf("failed to dequeue update task: %w", err))
					}
					anyWorkDone = true
				default:
					goto nextQueue
				}
			}
		nextQueue:
		}

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
