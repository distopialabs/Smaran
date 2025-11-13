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

	DB_DIR := "samurai-with-cache.db"
	fmt.Println("Removing database directory", DB_DIR)
	err := os.RemoveAll(DB_DIR)
	if err != nil {
		panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
	} else {
		fmt.Println("Database directory", DB_DIR, "removed")
	}

	// Opening the database
	// TODO: tune the options
	db, err := pebble.Open(DB_DIR, &pebble.Options{
		// MemTableSize: 1 << 31,
		MemTableSize: 2_147_483_648,
		DisableWAL:   true,
		// Cache:        pebble.NewCache(2_147_483_648),
	})
	if err != nil {
		panic(err)
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
				balances, err := ledger.BatchMultiUserBalance(modifiedAccounts, bn, config.Client)
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
				_, seen := accountsSeen.LoadOrStore(task.Account, struct{}{})
				// start := time.Now()
				segmenttree.CreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, cache, seen)
				// fmt.Println("Block", task.BlockNumber, "account", task.Account.Hex(), "time:", time.Since(start))
				// segmenttree.NewCreateOrUpdateAccountInfo(task.Account, task.Balance, task.BlockNumber, cache)
				// segmenttree.NewCreateOrUpdateAccountInfoOtter(task.Account, task.Balance, task.BlockNumber, otterCache)
				// fmt.Println("Block", task.BlockNumber, "account", task.Account.Hex(), "time:", time.Since(start))
			}
		}()
	}
	wg.Wait()

	// Ensure cache is fully flushed and closed before DB shutdown
	cache.Close()
	db.Close()

	fmt.Println("Time taken to process all blocks", time.Since(total_start), time.Now())

}

func logChannelSize(blockInfoCh chan blockInfo, orderedBlockInfoCh chan blockInfo, updateTaskChs []chan updateTask) {
	// keep logging the size of the channel every 5 seconds until the channel is closed
	for {
		time.Sleep(1 * time.Second)
		remaining := cap(blockInfoCh) - len(blockInfoCh)
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
			remaining = cap(updateTaskCh) - len(updateTaskCh)
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
