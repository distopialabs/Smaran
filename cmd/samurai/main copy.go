package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

type batchResult struct {
	counts map[common.Address]uint32
	blocks int
}

func BlanceAccountChanges() {
	// Flags for scalability and configuration
	ipcPath := flag.String("ipc", "/mydata/erigon/mainnet/erigon.ipc", "Erigon IPC path")
	startBlock := flag.Uint64("start", 18908895, "Start block (inclusive)")
	numBlocks := flag.Uint64("numBlocks", 1000, "End block (inclusive)")
	endBlock := flag.Uint64("end", 18908895+1000-1, "End block (inclusive)")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of worker goroutines")
	batchSize := flag.Int("batch", 2, "Blocks per JSON-RPC batch (64-256 recommended)")
	timeout := flag.Duration("timeout", 60*time.Second, "Per-batch RPC timeout")
	maxRetries := flag.Int("retries", 3, "Max retries per batch on error")
	logEvery := flag.Int("log-every", 1000, "Log progress every N blocks processed")
	output := flag.String("out", "changeCount.json", "Output histogram JSON file")
	failOnError := flag.Bool("fail-on-error", true, "Exit on first RPC error instead of skipping after retries")

	flag.Parse()

	client, err := rpc.Dial(*ipcPath)
	if err != nil {
		log.Fatalf("Failed to connect to RPC: %v", err)
	}
	defer client.Close()
	*endBlock = *startBlock + *numBlocks - 1

	if *endBlock < *startBlock {
		log.Fatalf("invalid block range: end (%d) < start (%d)", *endBlock, *startBlock)
	}
	if *batchSize <= 0 {
		log.Fatalf("invalid batch size: %d", *batchSize)
	}
	if *workers <= 0 {
		log.Fatalf("invalid workers: %d", *workers)
	}

	// Channels
	blockCh := make(chan uint64, *workers*(*batchSize)*2)
	resultsCh := make(chan batchResult, *workers*2)
	doneCh := make(chan map[common.Address]uint32, 1)

	// Feed blocks
	go func() {
		for bn := *startBlock; bn <= *endBlock; bn++ {
			blockCh <- bn
		}
		close(blockCh)
	}()

	// Aggregator goroutine to avoid lock contention
	go func() {
		startTime := time.Now()

		globalCounts := make(map[common.Address]uint32)
		processedBlocks := 0
		for res := range resultsCh {
			for addr, cnt := range res.counts {
				globalCounts[addr] += cnt
			}
			processedBlocks += res.blocks
			if *logEvery > 0 && processedBlocks%*logEvery == 0 {
				log.Printf("processed %d blocks | unique addrs: %d | elapsed: %v", processedBlocks, len(globalCounts), time.Since(startTime))
			}
		}
		doneCh <- globalCounts
		close(doneCh)
	}()

	// Workers
	var wg sync.WaitGroup
	wg.Add(*workers)
	for w := 0; w < *workers; w++ {
		go func(workerId int) {
			defer wg.Done()
			for {
				// Collect a batch of block numbers
				bns := make([]uint64, 0, *batchSize)
				for len(bns) < *batchSize {
					bn, ok := <-blockCh
					if !ok {
						break
					}
					bns = append(bns, bn)
				}
				if len(bns) == 0 {
					return
				}

				// Prepare batch elements and retry on failure
				resPtrs := make([]*[]common.Address, len(bns))
				attempt := 0
				for {
					attempt++
					// Create fresh elems and results per attempt
					for i := range bns {
						var res []common.Address
						resPtrs[i] = &res
					}
					elems := make([]rpc.BatchElem, len(bns))
					for i, bn := range bns {
						elems[i] = rpc.BatchElem{
							Method: "debug_getModifiedAccountsByNumber",
							Args:   []any{hexutil.Uint64(bn)},
							Result: resPtrs[i],
						}
					}

					ctx, cancel := context.WithTimeout(context.Background(), *timeout)
					err := client.BatchCallContext(ctx, elems)
					cancel()

					// Determine if we need to retry
					needsRetry := err != nil
					if !needsRetry {
						for i := range elems {
							if elems[i].Error != nil {
								needsRetry = true
								break
							}
						}
					}

					if needsRetry {
						if attempt <= *maxRetries {
							backoff := time.Duration(attempt) * 500 * time.Millisecond
							log.Printf("worker %d: batch retry %d/%d (err=%v)", workerId, attempt, *maxRetries, err)
							time.Sleep(backoff)
							continue
						}
						msg := fmt.Sprintf("worker %d: dropping batch after %d retries (last err=%v)", workerId, *maxRetries, err)
						if *failOnError {
							log.Fatal(msg)
						} else {
							log.Println(msg)
							// Skip this batch
							break
						}
					}

					// Successful response; aggregate locally
					local := make(map[common.Address]uint32, 1<<10)
					for i := range bns {
						if resPtrs[i] == nil {
							continue
						}
						for _, addr := range *resPtrs[i] {
							local[addr]++
						}
					}
					// Send to aggregator and move to next batch
					resultsCh <- batchResult{counts: local, blocks: len(bns)}
					break
				}
			}
		}(w)
	}

	// Wait for workers and close results
	wg.Wait()
	close(resultsCh)

	// Wait for aggregation to finish and get the global counts
	changeCount := <-doneCh

	// Build histogram: count -> number of addresses with that count
	result := make(map[uint32]uint64)
	for _, count := range changeCount {
		result[count]++
	}

	outputBase := strings.TrimSuffix(*output, ".json")
	changeCountJsonFile, err := os.Create(outputBase + "_change_count.json")
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer changeCountJsonFile.Close()
	if err := json.NewEncoder(changeCountJsonFile).Encode(changeCount); err != nil {
		log.Fatalf("Failed to write JSON: %v", err)
	}

	jsonFile, err := os.Create(*output)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}
	defer jsonFile.Close()
	if err := json.NewEncoder(jsonFile).Encode(result); err != nil {
		log.Fatalf("Failed to write JSON: %v", err)
	}

	fmt.Println("done. wrote histogram to", *output)
}
