package main

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/segmenttree"
	"github.com/nepal80m/samurai/internal/utils"
)

// DBBackend specifies which database backend to use
type DBBackend string

const (
	PebbleBackend DBBackend = "pebble"
)

type blockInfo struct {
	Number  uint64
	Entries []dataset.Entry
}
type updateTask struct {
	BlockNumber uint64
	Account     common.Address
	Balance     *big.Int
	EnqueuedAt  int64 // UnixNano timestamp when added to channel

}

func generateCommitmentsSimplified(config *config.Config, caches []*segmenttree.Cache) {

	// var DB_DIR string
	// // var db segmenttree.DB
	// var dbs = make([]segmenttree.DB, config.Database.Shards)
	// var err error

	// for i := range config.Database.Shards {
	// 	DB_DIR = fmt.Sprintf(STORAGE_PATH+"/samurai-shard-%d.db", i)
	// 	fmt.Println("Removing database directory", DB_DIR)
	// 	err = os.RemoveAll(DB_DIR)
	// 	if err != nil {
	// 		panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
	// 	} else {
	// 		fmt.Println("Database directory", DB_DIR, "removed")
	// 	}
	// 	dbs[i], err = segmenttree.NewPebbleDB(DB_DIR, &pebble.Options{
	// 		MemTableSize: 1_073_741_824, // 1_073_741_824, 2_147_483_648
	// 		DisableWAL:   true,
	// 		Cache:        pebble.NewCache(2_147_483_648),
	// 	})
	// 	if err != nil {
	// 		panic(fmt.Errorf("failed to create Pebble database %s: %w", DB_DIR, err))
	// 	}
	// }

	blockInfoCh := make(chan blockInfo, 1024)
	updateTaskChs := make([]chan updateTask, config.Workers.CommitWorkerCount)
	for i := range config.Workers.CommitWorkerCount {
		updateTaskChs[i] = make(chan updateTask, config.Workers.CommitWorkerChannelSize)
	}

	total_start := time.Now()

	// go logChannelSize(blockInfoCh, updateTaskChs)

	spawnBlockFetcher(config.Blocks.StartingBlockNumber, config.Blocks.EndingBlockNumber, blockInfoCh)

	// feed update tasks
	go func() {
		updateTaskQueues := make([]utils.Queue[updateTask], config.Workers.CommitWorkerCount)
		for i := range config.Workers.CommitWorkerCount {
			updateTaskQueues[i] = utils.NewQueue[updateTask]()
		}

		// loop until the blockInfoCh is closed and all the updateTaskQueues are empty
		// first check if the blockInfoCh is closed
		// if not listen for blockInfoCh and enqueue the update tasks to the updateTaskQueues
		blockInfoChClosed := false
		allUpdateTaskQueuesEmpty := false
		for !(blockInfoChClosed && allUpdateTaskQueuesEmpty) {

			// check if all updateTaskCh are full
			allUpdateTaskChsFull := true
			for i := range config.Workers.CommitWorkerCount {
				if len(updateTaskChs[i]) < config.Workers.CommitWorkerChannelSize {
					allUpdateTaskChsFull = false
					break
				}
			}
			if allUpdateTaskChsFull {
				sleepTime := time.Millisecond * 500
				fmt.Println("All updateTaskChs are full, skipping the fetch of new blocks and sleeping for", sleepTime)
				// TODO: decide whether to sleep for a longer time or not
				time.Sleep(sleepTime)
				continue
			}

			// Check if any queue has hit the memory limit
			anyQueueAtLimit := false
			// maxQueueSize := 0
			for i := range config.Workers.CommitWorkerCount {
				queueSize := updateTaskQueues[i].Size()
				// if queueSize > maxQueueSize {
				// 	maxQueueSize = queueSize
				// }

				if queueSize >= config.Workers.CommitWorkerQueueSize {
					anyQueueAtLimit = true
					fmt.Printf("⚠️ Queue %d has hit the limit: %d tasks\n", i, queueSize)
					break
				}
			}
			// fmt.Printf("Max queue size utilized: %d\n", maxQueueSize)
			anyWorkDone := false
			// Try to read from blockInfoCh and add entries to the updateTaskQueues only if no queue is at limit (prevent memory bloat)
			if !blockInfoChClosed && !anyQueueAtLimit {
				select {
				case blk, ok := <-blockInfoCh:
					if ok {
						anyWorkDone = true
						for _, entry := range blk.Entries {
							chIdx := utils.AddressToShardIndex(entry.Address, config.Workers.CommitWorkerCount)
							updateTaskQueues[chIdx].Enqueue(updateTask{
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

			for i := range config.Workers.CommitWorkerCount {
				// Drain as many items as possible from queue[i] to channel[i]
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
						// Successfully sent, continue draining this queue
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

		fmt.Println("All update tasks fed to the updateTaskChs")
		for i := range config.Workers.CommitWorkerCount {
			close(updateTaskChs[i])
		}
		fmt.Println("All updateTaskChs closed")
	}()

	wg := sync.WaitGroup{}
	// create syncmap to track the account seen
	var accountsSeen sync.Map
	for i := range config.Workers.CommitWorkerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range updateTaskChs[i] {

				segmenttree.CreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, caches[i], &accountsSeen)
			}
		}()
	}
	wg.Wait()

	fmt.Println("Time taken to process", config.Blocks.EndingBlockNumber-config.Blocks.StartingBlockNumber+1, "blocks", time.Since(total_start), time.Now())

	fmt.Println("Total time taken to process all blocks", time.Since(total_start), time.Now())

}

func logChannelSize(blockInfoCh chan blockInfo, updateTaskChs []chan updateTask) {
	// keep logging the size of the channel every 5 seconds until the channel is closed
	for {
		time.Sleep(1 * time.Second)
		remaining := cap(blockInfoCh) - len(blockInfoCh)
		// if remaining > 5 {
		// 	fmt.Printf("BlockInfoCh: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		// }
		if remaining > 0 && remaining < 5 {
			fmt.Printf("⚠️ BlockInfoCh is almost full: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		}
		if remaining <= 0 {
			fmt.Printf("🚨 BlockInfoCh is full: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		}

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
			remaining := cap(updateTaskCh) - len(updateTaskCh)
			// if remaining > 5 {
			// 	fmt.Printf("UpdateTaskCh %d: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			// }
			if remaining > 0 && remaining < 5 {
				fmt.Printf("⚠️ UpdateTaskCh %d is almost full: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			}
			if remaining <= 0 {
				fmt.Printf("🚨 UpdateTaskCh %d is full: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			}
		}

	}
}

// spawn a goroutine to fetch blocks from the dataset and send them to the blockInfoCh
func spawnBlockFetcher(startingBlockNumber uint64, endingBlockNumber uint64, blockInfoCh chan blockInfo) {
	var wg1 sync.WaitGroup
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		r := dataset.NewDatasetReader(dataset.DATASET_DIR, dataset.SEGMENT_SIZE)
		defer r.Close()
		for bn := startingBlockNumber; bn <= endingBlockNumber; bn++ {
			entries, err := r.GetBlock(uint32(bn))
			if err != nil {
				panic(fmt.Errorf("failed to get block %d from dataset: %w", bn, err))
			}

			blockInfoCh <- blockInfo{
				Number:  bn,
				Entries: entries,
			}
			fmt.Println("Block", bn, "sent to the channel")
		}
	}()
	go func() {
		wg1.Wait()
		close(blockInfoCh)
		fmt.Println("All blocks fetched and sent to the blockInfoCh, closing the channel")
	}()
}
