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
	"github.com/nepal80m/samurai/internal/ledger"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

// DBBackend specifies which database backend to use
type DBBackend string

const (
	PebbleBackend DBBackend = "pebble"
	SqliteBackend DBBackend = "sqlite"
)

type blockInfo struct {
	Number           uint64
	ModifiedAccounts []common.Address
	Balances         []*big.Int
}
type updateTask struct {
	BlockNumber uint64
	Account     common.Address
	Balance     *big.Int
}

func generateCommitmentsV2(config *config.Config, precomputedData *config.PrecomputedData) {

	// Choose database backend: PebbleBackend or SqliteBackend
	// Change this to switch between backends
	backend := PebbleBackend
	// backend := SqliteBackend

	var DB_DIR string
	var db segmenttree.DB
	var err error

	switch backend {
	case PebbleBackend:
		DB_DIR = "samurai-with-cache-pebble.db"
		fmt.Println("Using Pebble database backend")
		fmt.Println("Removing database directory", DB_DIR)
		err = os.RemoveAll(DB_DIR)
		if err != nil {
			panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
		} else {
			fmt.Println("Database directory", DB_DIR, "removed")
		}

		// Opening the Pebble database
		// TODO: tune the options
		pebbleDB, err := segmenttree.NewPebbleDB(DB_DIR, &pebble.Options{
			MemTableSize: 2_147_483_648,
			DisableWAL:   true,
			// Cache:        pebble.NewCache(2_147_483_648),
		})
		if err != nil {
			panic(err)
		}
		db = pebbleDB

	case SqliteBackend:
		DB_DIR = "samurai-with-cache-sqlite.db"
		fmt.Println("Using SQLite database backend")
		fmt.Println("Removing database file", DB_DIR)
		err = segmenttree.RemoveSqliteDB(DB_DIR)
		if err != nil {
			panic(fmt.Errorf("failed to remove database file %s: %w", DB_DIR, err))
		} else {
			fmt.Println("Database file", DB_DIR, "removed")
		}

		// Opening the SQLite database
		sqliteDB, err := segmenttree.NewSqliteDB(DB_DIR)
		if err != nil {
			panic(err)
		}
		db = sqliteDB

	default:
		panic(fmt.Errorf("unknown database backend: %s", backend))
	}

	cache, err := segmenttree.NewCache(db, precomputedData)
	if err != nil {
		panic(err)
	}
	// otterCache, err := segmenttree.NewOtterCache(db, precomputedData)
	// if err != nil {
	// 	panic(err)
	// }
	// lruCache, err := segmenttree.NewLRUCache(db, precomputedData)
	// if err != nil {
	// 	panic(err)
	// }

	// log the cache stats
	go func() {
		for {
			time.Sleep(1 * time.Second)
			fmt.Println("Cache cost added:", cache.C.Metrics.CostAdded())
			fmt.Println("Cache cost evicted:", cache.C.Metrics.CostEvicted())
			fmt.Println("Cache cost present:", cache.C.Metrics.CostAdded()-cache.C.Metrics.CostEvicted())
			fmt.Println("Cache metrics:", cache.C.Metrics.String())

		}
	}()

	workers := runtime.NumCPU()
	fmt.Println("Workers:", workers)
	blockInfoCh := make(chan blockInfo, 1024)
	orderedBlockInfoCh := make(chan blockInfo, 1024)
	// updateTaskCh := make(chan updateTask, 1<<10)
	fetchWorkerCount := 32           //workers * 2 = 32
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
	for range fetchWorkerCount {
		wg1.Add(1)
		go func() {
			defer wg1.Done()
			for {
				bn := atomic.AddUint64(&nextBlockToFetch, 1) - 1
				if bn > config.EndingBlockNumber {
					break
				}
				// start := time.Now()
				// fetch all the modified accounts in this block
				modifiedAccounts, err := ledger.GetModifiedAccountsByNumber(bn, config.Client)
				if err != nil {
					panic(fmt.Errorf("failed to get modified accounts by number %d: %w", bn, err))
				}
				// fetch balances for all the modified accounts
				// if len(modifiedAccounts) == 0 {
				// 	continue
				// }
				// ? do not just skip if there are no modified accounts, because orderWorker is waiting for the next block info to be sent to the channel. instead, send an empty block info with empty modified accounts and balances.
				balances, err := ledger.BatchMultiUserBalance(modifiedAccounts, bn, config)
				if err != nil {
					panic(fmt.Errorf("failed to get balances for block %d: %w", bn, err))
				}
				fmt.Println("Block", bn, "fetched and sent to the channel")

				// TODO: remove this override
				// balances := []*big.Int{new(big.Int).SetUint64(1000000000000000000 + uint64(bn))}
				// modifiedAccounts := []common.Address{common.HexToAddress("0x0000000000000000000000000000000000000027")}
				// send the block info to the channel
				// fmt.Println("Block", bn, "fetched and sent to the channel")
				// fmt.Println("Waiting for blockInfoCh to be ready", time.Now(), len(blockInfoCh), "items in the channel of size", cap(blockInfoCh))
				// start := time.Now()
				blockInfoCh <- blockInfo{
					Number:           bn,
					ModifiedAccounts: modifiedAccounts,
					Balances:         balances,
				}
				// fmt.Println("Block", bn, "sent to the channel", time.Since(start))
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

			for i, addr := range blk.ModifiedAccounts {
				h := xxhash.Sum64(addr[:])
				chIdx := int(h % uint64(updateWorkerCount))
				fmt.Println("Sending update task for account", addr.Hex(), "to worker", chIdx)
				updateTaskChs[chIdx] <- updateTask{
					BlockNumber: blk.Number,
					Account:     addr,
					Balance:     blk.Balances[i],
				}
			}
			fmt.Println("Block", blk.Number, "with", len(blk.ModifiedAccounts), "accounts sent to updateTaskCh")
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
				// segmenttree.NewCreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, cache)
				// segmenttree.NewCreateOrUpdateAccountInfoOtter(task.Account, task.Balance, task.BlockNumber, otterCache)
				// fmt.Println("Block", task.BlockNumber, "account", task.Account.Hex(), "time:", time.Since(start))
			}
		}()
	}
	wg.Wait()

	// print the accounts seen
	accountsSeen.Range(func(key, value interface{}) bool {

		seenAccountInfo := value.(segmenttree.SeenAccountInfo)
		fmt.Println("Account", key.(common.Address).Hex(), "seen", seenAccountInfo.Count, "times, fetched from db", seenAccountInfo.DBFetchCount, "times, total time", seenAccountInfo.TotalDBFetchTime)
		return true
	})
	// Ensure cache is fully flushed and closed before DB shutdown
	fmt.Println("Cache hit ratio:", cache.C.Metrics.Ratio())
	fmt.Println("Cache miss ratio:", 1-cache.C.Metrics.Ratio())
	fmt.Println("Cache hit count:", cache.C.Metrics.Hits())
	fmt.Println("Cache miss count:", cache.C.Metrics.Misses())
	fmt.Println("Cache eviction count:", cache.C.Metrics.KeysEvicted())
	fmt.Println("Cache cost added:", cache.C.Metrics.CostAdded())
	fmt.Println("Cache cost evicted:", cache.C.Metrics.CostEvicted())
	fmt.Println("Cache sets dropped:", cache.C.Metrics.SetsDropped())
	fmt.Println("Cache sets rejected:", cache.C.Metrics.SetsRejected())
	fmt.Println("Cache gets dropped:", cache.C.Metrics.GetsDropped())
	fmt.Println("Cache gets kept:", cache.C.Metrics.GetsKept())
	fmt.Println("Cache life expectancy:", cache.C.Metrics.LifeExpectancySeconds())
	fmt.Println("Cache keys added:", cache.C.Metrics.KeysAdded())
	fmt.Println("Cache keys updated:", cache.C.Metrics.KeysUpdated())
	fmt.Println("Cache keys evicted:", cache.C.Metrics.KeysEvicted())
	fmt.Println("Cache keys present:", cache.C.Metrics.KeysAdded()-cache.C.Metrics.KeysEvicted())
	fmt.Println("Cache metrics:", cache.C.Metrics.String())

	cache.Close()
	db.Close()

	fmt.Println("Time taken to process all blocks", time.Since(total_start), time.Now())

}

func logChannelSize(blockInfoCh chan blockInfo, orderedBlockInfoCh chan blockInfo, updateTaskChs []chan updateTask) {
	// keep logging the size of the channel every 5 seconds until the channel is closed
	for {
		time.Sleep(1 * time.Second)
		// remaining := cap(blockInfoCh) - len(blockInfoCh)
		// if remaining > 5 {
		// 	fmt.Printf("BlockInfoCh: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
		// }
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
			remaining := cap(updateTaskCh) - len(updateTaskCh)
			if remaining > 5 {
				fmt.Printf("UpdateTaskCh %d: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			}
			if remaining > 0 && remaining < 5 {
				fmt.Printf("⚠️ UpdateTaskCh %d is almost full: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			}
			if remaining <= 0 {
				fmt.Printf("🚨 UpdateTaskCh %d is full: %d/%d\n", i, len(updateTaskCh), cap(updateTaskCh))
			}
		}

	}
}
