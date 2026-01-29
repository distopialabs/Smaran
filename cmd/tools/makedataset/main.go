package main

import (
	"flag"
	"fmt"
	"log"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/account"
)

type blockInfo struct {
	Number           uint64
	ModifiedAccounts []common.Address
	Balances         []*big.Int
}

func main() {
	// startBlock := flag.Int("startBlock", 18900000, "Start block") //18908895
	// endBlock := flag.Int("endBlock", 21525890, "End block")
	startBlock := flag.Int("startBlock", 20_600_000, "Start block")        //18908895
	endBlock := flag.Int("endBlock", 20_600_000+(100_000*10), "End block") //100k = 2hr
	testMode := flag.Bool("testMode", false, "Test mode")

	flag.Parse()

	client, err := rpc.Dial("/mydata/erigon/mainnet/erigon.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to Erigon IPC: %v", err)
	}
	defer client.Close()

	if *testMode {
		sanityCheck(uint64(18900001), client)
	} else {
		fetchAndWriteDataset(uint64(*startBlock), uint64(*endBlock), client)
	}

}

func fetchAndWriteDataset(startBlock uint64, endBlock uint64, client *rpc.Client) {
	workers := runtime.NumCPU()
	fmt.Println("Workers:", workers)
	blockInfoCh := make(chan blockInfo, 1024)
	orderedBlockInfoCh := make(chan blockInfo, 1024)
	fetchWorkerCount := workers

	// logChannelSize(blockInfoCh, orderedBlockInfoCh)

	spawnBlockFetcher(fetchWorkerCount, startBlock, endBlock, blockInfoCh, client)
	spawnBlockOrderer(blockInfoCh, orderedBlockInfoCh, startBlock)

	w := dataset.NewDatasetWriter(dataset.DATASET_DIR, dataset.SEGMENT_SIZE)
	defer w.Close()

	for blk := range orderedBlockInfoCh {
		entries := make([]dataset.Entry, 0, len(blk.ModifiedAccounts))
		for i, addr := range blk.ModifiedAccounts {
			entries = append(entries, dataset.Entry{
				Address: addr,
				Balance: blk.Balances[i].Bytes(),
			})
		}
		err := w.AppendBlock(uint32(blk.Number), entries)
		if err != nil {
			panic(fmt.Errorf("failed to append block %d: %w", blk.Number, err))
		}
		fmt.Println("Block", blk.Number, "with", len(blk.ModifiedAccounts), "accounts added to the dataset")
	}

}

func spawnBlockFetcher(fetchWorkerCount int, startBlock uint64, endBlock uint64, blockInfoCh chan blockInfo, client *rpc.Client) {

	var wg sync.WaitGroup
	nextBlockToFetch := startBlock
	for range fetchWorkerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				bn := atomic.AddUint64(&nextBlockToFetch, 1) - 1
				if bn > endBlock {
					break
				}
				// fetch all the modified accounts in this block
				modifiedAccounts, err := account.GetModifiedAccountsByNumber(bn, client)
				if err != nil {
					panic(fmt.Errorf("failed to get modified accounts by number %d: %w", bn, err))
				}
				// fetch balances for all the modified accounts
				// if len(modifiedAccounts) == 0 {
				// 	continue
				// }
				// ? do not just skip if there are no modified accounts, because orderWorker is waiting for the next block info to be sent to the channel. instead, send an empty block info with empty modified accounts and balances.
				balances, err := account.BatchMultiUserBalance(modifiedAccounts, bn, client)
				if err != nil {
					panic(fmt.Errorf("failed to get balances for block %d: %w", bn, err))
				}

				blockInfoCh <- blockInfo{
					Number:           bn,
					ModifiedAccounts: modifiedAccounts,
					Balances:         balances,
				}
				fmt.Println("Block", bn, "fetched and sent to the blockInfoCh")
			}
		}()
	}
	go func() {
		wg.Wait()
		fmt.Println("All blocks fetched and sent to the blockInfoCh, closing the channel")
		close(blockInfoCh)
	}()

}

func spawnBlockOrderer(blockInfoCh chan blockInfo, orderedBlockInfoCh chan blockInfo, startBlock uint64) {

	go func() {
		nextBlockToProcess := startBlock
		pendingBlocks := make(map[uint64]blockInfo)

		for blkInfo := range blockInfoCh {
			if blkInfo.Number == nextBlockToProcess {
				orderedBlockInfoCh <- blkInfo
				fmt.Println("Block", blkInfo.Number, "ordered and sent to the orderedBlockInfoCh")
				nextBlockToProcess++
				for {
					if blk, ok := pendingBlocks[nextBlockToProcess]; ok {
						orderedBlockInfoCh <- blk
						fmt.Println("Block", blk.Number, "ordered and sent to the orderedBlockInfoCh")
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
			}
		}
		close(orderedBlockInfoCh)
	}()
}

func sanityCheck(testBlock uint64, client *rpc.Client) {
	actualModifiedAccounts, err := account.GetModifiedAccountsByNumber(testBlock, client)
	if err != nil {
		panic(fmt.Errorf("failed to get modified accounts by number %d: %w", testBlock, err))
	}
	actualBalances, err := account.BatchMultiUserBalance(actualModifiedAccounts, testBlock, client)
	if err != nil {
		panic(fmt.Errorf("failed to get balances for block %d: %w", testBlock, err))
	}
	fmt.Println("Test block", testBlock, "with", len(actualModifiedAccounts), "accounts and", len(actualBalances), "balances")
	actualEntries := make([]dataset.Entry, 0, len(actualModifiedAccounts))
	for i, addr := range actualModifiedAccounts {
		actualEntries = append(actualEntries, dataset.Entry{
			Address: addr,
			Balance: actualBalances[i].Bytes(),
		})
	}

	r := dataset.NewDatasetReader(dataset.DATASET_DIR, dataset.SEGMENT_SIZE)
	defer r.Close()
	fetchedEntries, err := r.GetBlock(uint32(testBlock))
	if err != nil {
		panic(fmt.Errorf("failed to get block %d: %w", testBlock, err))
	}

	if len(actualEntries) != len(fetchedEntries) {
		panic(fmt.Errorf("actual entries and fetched entries have different lengths"))
	}
	for i := range actualEntries {
		fmt.Println("Account:", common.BytesToAddress(actualEntries[i].Address[:]), "Balance:", actualEntries[i].Balance)
		fmt.Println("Account:", common.BytesToAddress(fetchedEntries[i].Address[:]), "Balance:", fetchedEntries[i].Balance)
		fmt.Println("--------------------------------")
		if actualEntries[i].Address != fetchedEntries[i].Address {
			panic(fmt.Errorf("actual entries and fetched entries have different addresses"))
		}
		if new(big.Int).SetBytes(actualEntries[i].Balance).Cmp(new(big.Int).SetBytes(fetchedEntries[i].Balance)) != 0 {
			panic(fmt.Errorf("actual entries and fetched entries have different balances"))
		}
	}

	fmt.Println("Test block", testBlock, "with", len(actualModifiedAccounts), "accounts and", len(actualBalances), "balances passed")

}

func logChannelSize(blockInfoCh chan blockInfo, orderedBlockInfoCh chan blockInfo) {
	// keep logging the size of the channel every 5 seconds until the channel is closed
	go func() {
		for {
			time.Sleep(1 * time.Second)
			remaining := cap(blockInfoCh) - len(blockInfoCh)
			if remaining > 5 {
				fmt.Printf("BlockInfoCh: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
			}
			if remaining > 0 && remaining < 5 {
				fmt.Printf("⚠️ BlockInfoCh is almost full: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
			}
			if remaining <= 0 {
				fmt.Printf("🚨 BlockInfoCh is full: %d/%d\n", len(blockInfoCh), cap(blockInfoCh))
			}

			remaining = cap(orderedBlockInfoCh) - len(orderedBlockInfoCh)
			if remaining > 5 {
				fmt.Printf("OrderedBlockInfoCh: %d/%d\n", len(orderedBlockInfoCh), cap(orderedBlockInfoCh))
			}
			if remaining > 0 && remaining < 5 {
				fmt.Printf("⚠️ OrderedBlockInfoCh is almost full, %d/%d\n", len(orderedBlockInfoCh), cap(orderedBlockInfoCh))
			}
			if remaining <= 0 {
				fmt.Printf("🚨 OrderedBlockInfoCh is full: %d/%d\n", len(orderedBlockInfoCh), cap(orderedBlockInfoCh))
			}

		}
	}()
}
