package main

import (
	"fmt"
	"math/big"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/segmenttree"
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
}

func generateCommitmentsSimplified(config *config.Config, precomputedData *config.PrecomputedData) {

	var DB_DIR string
	// var db segmenttree.DB
	var dbs [segmenttree.DB_SHARDS]segmenttree.DB
	var err error

	for i := range segmenttree.DB_SHARDS {
		DB_DIR = fmt.Sprintf("storage/samurai-shard-%d.db", i)
		fmt.Println("Using Pebble database backend")
		fmt.Println("Removing database directory", DB_DIR)
		err = os.RemoveAll(DB_DIR)
		if err != nil {
			panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
		} else {
			fmt.Println("Database directory", DB_DIR, "removed")
		}
		dbs[i], err = segmenttree.NewPebbleDB(DB_DIR, &pebble.Options{
			MemTableSize: 2_147_483_648,
			DisableWAL:   true,
			Cache:        pebble.NewCache(2_147_483_648),
		})
		if err != nil {
			panic(fmt.Errorf("failed to create Pebble database %s: %w", DB_DIR, err))
		}
	}

	cache, err := segmenttree.NewCache(dbs, precomputedData)
	if err != nil {
		panic(err)
	}

	workers := runtime.NumCPU()
	fmt.Println("Workers:", workers)
	blockInfoCh := make(chan blockInfo, 1024)
	// updateTaskCh := make(chan updateTask, 1<<10)
	updateWorkerCount := workers * 4 //workers * 4 = 64

	// create separate updateTaskCh for each worker
	updateTaskChs := make([]chan updateTask, updateWorkerCount)
	for i := range updateWorkerCount {
		updateTaskChs[i] = make(chan updateTask, 10_000)
	}

	total_start := time.Now()

	go logChannelSize(blockInfoCh, updateTaskChs)

	var wg1 sync.WaitGroup
	wg1.Add(1)
	go func() {
		defer wg1.Done()
		r := dataset.NewDatasetReader(dataset.DATASET_DIR, dataset.SEGMENT_SIZE)
		defer r.Close()
		for bn := config.StartingBlockNumber; bn <= config.EndingBlockNumber; bn++ {
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
		fmt.Println("Time taken to fetch all blocks", time.Since(total_start))
	}()

	// feed update tasks

	go func() {
		for blk := range blockInfoCh {
			// fmt.Println("Block", blk.Number, "with", len(blk.ModifiedAccounts), "accounts ready to be sent to updateTaskCh, waiting for channel to be ready", time.Since(total_start))

			for _, entry := range blk.Entries {
				h := xxhash.Sum64(entry.Address[:])
				chIdx := int(h % uint64(updateWorkerCount))
				updateTaskChs[chIdx] <- updateTask{
					BlockNumber: blk.Number,
					Account:     common.BytesToAddress(entry.Address[:]),
					Balance:     new(big.Int).SetBytes(entry.Balance),
				}
			}
			fmt.Println("Block", blk.Number, "with", len(blk.Entries), "accounts sent to updateTaskCh")
		}
		for i := range updateWorkerCount {
			close(updateTaskChs[i])
		}
		fmt.Println("Time taken to feed all update tasks", time.Since(total_start))
	}()

	wg := sync.WaitGroup{}
	// create syncmap to track the account seen
	var accountsSeen sync.Map
	for i := range updateWorkerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range updateTaskChs[i] {
				// old_value, seen := accountsSeen.Load(task.Account)
				// if !seen {
				// 	accountsSeen.Store(task.Account, 1)
				// } else {
				// 	accountsSeen.Store(task.Account, old_value.(int)+1)
				// }
				// var seenCount int
				// if old_value == nil {
				// 	seenCount = 0
				// } else {
				// 	seenCount = old_value.(int)
				// }
				// _, seen := accountsSeen.LoadOrStore(task.Account, struct{}{})
				// start := time.Now()
				segmenttree.CreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, cache, &accountsSeen)
				// fmt.Println("Block", task.BlockNumber, "account", task.Account.Hex(), "time:", time.Since(start))
			}
		}()
	}
	wg.Wait()

	// print the accounts seen
	// accountsSeen.Range(func(key, value interface{}) bool {

	// 	seenAccountInfo := value.(segmenttree.SeenAccountInfo)
	// 	fmt.Println("Account", key.(common.Address).Hex(), "seen", seenAccountInfo.Count, "times, fetched from db", seenAccountInfo.DBFetchCount, "times, total time", seenAccountInfo.TotalDBFetchTime)
	// 	return true
	// })

	cache.Close()
	for i := range segmenttree.DB_SHARDS {
		dbs[i].Close()
		fmt.Println("Database", i, "closed")
	}

	fmt.Println("Time taken to process all blocks", time.Since(total_start), time.Now())

}

func generateCommitmentsV2(config *config.Config, precomputedData *config.PrecomputedData) {

	var DB_DIR string
	// var db segmenttree.DB
	var dbs [segmenttree.DB_SHARDS]segmenttree.DB
	var err error

	for i := range segmenttree.DB_SHARDS {
		DB_DIR = fmt.Sprintf("storage/samurai-shard-%d.db", i)
		fmt.Println("Using Pebble database backend")
		fmt.Println("Removing database directory", DB_DIR)
		err = os.RemoveAll(DB_DIR)
		if err != nil {
			panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
		} else {
			fmt.Println("Database directory", DB_DIR, "removed")
		}
		dbs[i], err = segmenttree.NewPebbleDB(DB_DIR, &pebble.Options{
			MemTableSize: 2_147_483_648,
			DisableWAL:   true,
			// Cache:        pebble.NewCache(2_147_483_648),
		})
		if err != nil {
			panic(fmt.Errorf("failed to create Pebble database %s: %w", DB_DIR, err))
		}
	}

	cache, err := segmenttree.NewCache(dbs, precomputedData)
	if err != nil {
		panic(err)
	}

	workers := runtime.NumCPU()
	fmt.Println("Workers:", workers)
	blockInfoCh := make(chan blockInfo, 1024)
	orderedBlockInfoCh := make(chan blockInfo, 1024)
	// updateTaskCh := make(chan updateTask, 1<<10)
	fetchWorkerCount := 10           //workers * 2 = 32
	updateWorkerCount := workers * 4 //workers * 4 = 64

	// create separate updateTaskCh for each worker
	updateTaskChs := make([]chan updateTask, updateWorkerCount)
	for i := range updateWorkerCount {
		updateTaskChs[i] = make(chan updateTask, 1024)
	}

	total_start := time.Now()

	// go logChannelSize(blockInfoCh, orderedBlockInfoCh, updateTaskChs)

	var wg1 sync.WaitGroup
	nextBlockToFetch := config.StartingBlockNumber
	r := dataset.NewDatasetReader(dataset.DATASET_DIR, dataset.SEGMENT_SIZE)
	defer r.Close()
	for range fetchWorkerCount {
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			for {
				bn := atomic.AddUint64(&nextBlockToFetch, 1) - 1
				if bn > config.EndingBlockNumber {
					break
				}

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
	}
	go func() {
		wg1.Wait()
		close(blockInfoCh)
		fmt.Println("Time taken to fetch all blocks", time.Since(total_start))
	}()

	// Reorder the blockCh by the block number
	// TODO: implement fixed sized array with rotating pointer instead of map.
	go func() {
		nextBlockToProcess := config.StartingBlockNumber
		pendingBlocks := make(map[uint64]blockInfo)

		for blkInfo := range blockInfoCh {
			if blkInfo.Number == nextBlockToProcess {
				orderedBlockInfoCh <- blkInfo
				nextBlockToProcess++
				for {
					if blk, ok := pendingBlocks[nextBlockToProcess]; ok {
						fmt.Println("Block", nextBlockToProcess, "ordered and sent to the channel")
						orderedBlockInfoCh <- blk
						delete(pendingBlocks, nextBlockToProcess)
						nextBlockToProcess++
					} else {
						break
					}
				}
			} else {
				pendingBlocks[blkInfo.Number] = blkInfo
			}
			if len(pendingBlocks) > 1000 {
				fmt.Println("🚨💾💣 Pending blocks:", len(pendingBlocks))
				panic(fmt.Sprintf("Pending blocks exceeded safe limit: %d", len(pendingBlocks)))
			} else if len(pendingBlocks) > 50 {
				fmt.Println("⚠️💾💣 Pending blocks:", len(pendingBlocks))
				// }
			} else {
				// fmt.Println("Pending blocks:", len(pendingBlocks))
			}
		}
		close(orderedBlockInfoCh)
		fmt.Println("Time taken to order all blocks", time.Since(total_start))
	}()

	// feed update tasks

	go func() {
		for blk := range orderedBlockInfoCh {
			// fmt.Println("Block", blk.Number, "with", len(blk.ModifiedAccounts), "accounts ready to be sent to updateTaskCh, waiting for channel to be ready", time.Since(total_start))

			for _, entry := range blk.Entries {
				h := xxhash.Sum64(entry.Address[:])
				chIdx := int(h % uint64(updateWorkerCount))
				fmt.Println("Sending update task for account", common.BytesToAddress(entry.Address[:]).Hex(), "to worker", chIdx)
				updateTaskChs[chIdx] <- updateTask{
					BlockNumber: blk.Number,
					Account:     common.BytesToAddress(entry.Address[:]),
					Balance:     new(big.Int).SetBytes(entry.Balance),
				}
			}
			fmt.Println("Block", blk.Number, "with", len(blk.Entries), "accounts sent to updateTaskCh")
		}
		for i := range updateWorkerCount {
			close(updateTaskChs[i])
		}
		fmt.Println("Time taken to feed all update tasks", time.Since(total_start))
	}()

	wg := sync.WaitGroup{}
	// create syncmap to track the account seen
	var accountsSeen sync.Map
	for i := range updateWorkerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range updateTaskChs[i] {
				// old_value, seen := accountsSeen.Load(task.Account)
				// if !seen {
				// 	accountsSeen.Store(task.Account, 1)
				// } else {
				// 	accountsSeen.Store(task.Account, old_value.(int)+1)
				// }
				// var seenCount int
				// if old_value == nil {
				// 	seenCount = 0
				// } else {
				// 	seenCount = old_value.(int)
				// }
				// _, seen := accountsSeen.LoadOrStore(task.Account, struct{}{})
				// start := time.Now()
				segmenttree.CreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, cache, &accountsSeen)
				// fmt.Println("Block", task.BlockNumber, "account", task.Account.Hex(), "time:", time.Since(start))
			}
		}()
	}
	wg.Wait()

	// print the accounts seen
	// accountsSeen.Range(func(key, value interface{}) bool {

	// 	seenAccountInfo := value.(segmenttree.SeenAccountInfo)
	// 	fmt.Println("Account", key.(common.Address).Hex(), "seen", seenAccountInfo.Count, "times, fetched from db", seenAccountInfo.DBFetchCount, "times, total time", seenAccountInfo.TotalDBFetchTime)
	// 	return true
	// })

	cache.Close()
	for i := range segmenttree.DB_SHARDS {
		dbs[i].Close()
		fmt.Println("Database", i, "closed")
	}

	fmt.Println("Time taken to process all blocks", time.Since(total_start), time.Now())

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
