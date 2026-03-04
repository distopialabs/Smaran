// Package commands contains the subcommand implementations for the samurai CLI.
package commands

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/utils"
)

// BlockInfo holds block data for processing.
type BlockInfo struct {
	Number  uint64
	Entries []dataset.Entry
}

// UpdateTask represents a single account update to process.
type UpdateTask struct {
	BlockNumber uint64
	Account     common.Address
	Balance     *big.Int
	EnqueuedAt  int64 // UnixNano timestamp when added to channel
}

// RunCommit executes the commit generation mode.
func RunCommit(cfg *config.Config, caches []*storage.Cache) {
	blockInfoCh := make(chan BlockInfo, 1024)
	updateTaskChs := make([]chan UpdateTask, cfg.Workers.CommitWorkerCount)
	for i := range cfg.Workers.CommitWorkerCount {
		updateTaskChs[i] = make(chan UpdateTask, cfg.Workers.CommitWorkerChannelSize)
	}

	totalStart := time.Now()

	// Resume logic
	if !cfg.Clean {
		meta, err := storage.LoadMetadata(cfg.Database.StoragePath)
		if err != nil {
			fmt.Println("⚠️ Failed to load metadata:", err)
		} else if meta.LastProcessedBlock > 0 {
			if meta.LastProcessedBlock > cfg.Blocks.StartingBlockNumber {
				fmt.Printf("Resume: Fast-forwarding start block from %d to %d\n", cfg.Blocks.StartingBlockNumber, meta.LastProcessedBlock+1)
				// Maintain the batch size
				batchSize := cfg.Blocks.EndingBlockNumber - cfg.Blocks.StartingBlockNumber
				cfg.Blocks.StartingBlockNumber = meta.LastProcessedBlock + 1
				cfg.Blocks.EndingBlockNumber = cfg.Blocks.StartingBlockNumber + batchSize
			}
		} else {
			fmt.Println("Resume: No previous progress found in metadata")
		}
	}

	SpawnBlockFetcher(cfg.Blocks.StartingBlockNumber, cfg.Blocks.EndingBlockNumber, blockInfoCh, cfg.BlocksDataDir)

	// Feed update tasks
	go func() {
		updateTaskQueues := make([]utils.Queue[UpdateTask], cfg.Workers.CommitWorkerCount)
		for i := range cfg.Workers.CommitWorkerCount {
			updateTaskQueues[i] = utils.NewQueue[UpdateTask]()
		}

		blockInfoChClosed := false
		allUpdateTaskQueuesEmpty := false
		for !(blockInfoChClosed && allUpdateTaskQueuesEmpty) {
			// Check if any queue has hit the memory limit
			anyQueueAtLimit := false
			for i := range cfg.Workers.CommitWorkerCount {
				queueSize := updateTaskQueues[i].Size()
				if queueSize >= cfg.Workers.CommitWorkerQueueSize {
					anyQueueAtLimit = true
					// fmt.Printf("⚠️ Queue %d has hit the limit: %d tasks\n", i, queueSize)
					break
				}
			}

			anyWorkDone := false
			// Try to read from blockInfoCh and add entries to the updateTaskQueues
			if !blockInfoChClosed && !anyQueueAtLimit {
				select {
				case blk, ok := <-blockInfoCh:
					if ok {
						anyWorkDone = true
						if blk.Number%10000 == 0 {
							fmt.Printf("Commit Phase: progressing, currently at block %d\n", blk.Number)
						}
						for _, entry := range blk.Entries {
							chIdx := utils.AddressToShardIndex(entry.Address, cfg.Workers.CommitWorkerCount)
							updateTaskQueues[chIdx].Enqueue(UpdateTask{
								BlockNumber: blk.Number,
								Account:     common.BytesToAddress(entry.Address[:]),
								Balance:     new(big.Int).SetBytes(entry.Balance),
							})
						}
					} else {
						blockInfoChClosed = true
					}
				default:
				}
			}

			// Drain updateTaskQueues to updateTaskChs
			allUpdateTaskQueuesEmpty = true
			for i := range cfg.Workers.CommitWorkerCount {
				for !updateTaskQueues[i].IsEmpty() {
					allUpdateTaskQueuesEmpty = false

					task, err := updateTaskQueues[i].Peek()
					if err != nil {
						panic(fmt.Errorf("failed to peek update task: %w", err))
					}

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

			// Prevent busy-wait
			if !anyWorkDone {
				time.Sleep(time.Microsecond * 100)
			}
		}

		fmt.Println("All update tasks fed to the updateTaskChs")
		for i := range cfg.Workers.CommitWorkerCount {
			close(updateTaskChs[i])
		}
		fmt.Println("All updateTaskChs closed")
	}()

	// Process update tasks
	wg := sync.WaitGroup{}
	for i := range cfg.Workers.CommitWorkerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range updateTaskChs[i] {
				storage.CreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, caches[i])
			}
		}()
	}
	wg.Wait()

	fmt.Println("Time taken to process", cfg.Blocks.EndingBlockNumber-cfg.Blocks.StartingBlockNumber+1, "blocks", time.Since(totalStart), time.Now())
	fmt.Println("Total time taken to process all blocks", time.Since(totalStart), time.Now())

	// Save progress
	if err := storage.SaveMetadata(cfg.Database.StoragePath, storage.Metadata{
		LastProcessedBlock: cfg.Blocks.EndingBlockNumber,
	}); err != nil {
		fmt.Printf("⚠️ Failed to save metadata: %v\n", err)
	} else {
		fmt.Printf("Saved progress: LastProcessedBlock = %d\n", cfg.Blocks.EndingBlockNumber)
	}
}

// SpawnBlockFetcher spawns a goroutine to fetch blocks from the dataset.
func SpawnBlockFetcher(startingBlockNumber uint64, endingBlockNumber uint64, blockInfoCh chan BlockInfo, dataDir string) {
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		r := dataset.NewDatasetReader(dataDir, dataset.SEGMENT_SIZE)
		defer r.Close()
		for bn := startingBlockNumber; bn <= endingBlockNumber; bn++ {
			entries, err := r.GetBlock(uint32(bn))
			if err != nil {
				panic(fmt.Errorf("failed to get block %d from dataset: %w", bn, err))
			}

			blockInfoCh <- BlockInfo{
				Number:  bn,
				Entries: entries,
			}
		}
	}()
	go func() {
		wg1.Wait()
		close(blockInfoCh)
		fmt.Println("All blocks fetched and sent to the blockInfoCh, closing the channel")
	}()
}
