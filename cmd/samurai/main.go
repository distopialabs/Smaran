package main

import (
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/nepal80m/samurai/internal/crypto/kzg"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

func main() {
	// usage: go run main.go -numBlocks 100 -numTrackedAccounts 100 -concurrency 10

	mode := flag.String("mode", "commit", "Mode to run: commit, proof, verify")
	concurrency := flag.Int("c", 1, "Concurrency level")
	_ = concurrency
	profile := flag.Bool("p", true, "Profile the program")

	// flags to generate commitments
	numBlocks := flag.Int("numBlocks", 900, "Number of blocks to process")
	// numTrackedAccounts := flag.Int("a", 1, "Number of tracked accounts")

	// flags to generate proofs & verify proofs
	queryStartBlock := flag.Int("queryStartBlock", 20, "Start block for query")
	queryEndBlock := flag.Int("queryEndBlock", 880, "End block for query")
	queryAccount := flag.String("queryAccount", "0x0000000000000000000000000000000000000027", "Account to query")

	flag.Parse()
	if *profile {
		f, err := os.Create("profiles/cpu.prof")
		if err != nil {
			panic(err)
		}

		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	client, err := rpc.Dial("/mydata/erigon/mainnet/erigon.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to Erigon IPC: %v", err)
	}
	defer client.Close()

	srs, err := kzg.SetupSRS(segmenttree.SegmentTreeSize)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}
	// V, weights := polynomial.LoadBarycentricData(segmenttree.SegmentTreeSize)
	V, weights, weightCommits := kzg.LoadBarycentricData(segmenttree.SegmentTreeSize, srs)
	precomputedData := &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}
	config := config.Config{
		// GethIPC:             "/mydata/erigon/mainnet/geth.ipc",
		Client: client,
		// StartingBlockNumber: 20908895,                      // first block of 2024
		// EndingBlockNumber:   20908895 + uint64(*numBlocks), // last block of 2024
		StartingBlockNumber: 18908895,                        // first block of 2024
		EndingBlockNumber:   18908895 + uint64(*numBlocks-1), // last block of 2024
		// EndingBlockNumber:   18908895 + 2050, // last block of 2024
		// EndingBlockNumber: 21525890, // last block of 2024
	}

	switch *mode {
	case "commit":
		// start := time.Now()
		// fmt.Println("Setting up tracked accounts...")
		// config.SetTrackedAccounts(*numTrackedAccounts)
		// fmt.Printf("Time taken to set %d tracked accounts: %v\n", len(config.TrackedAccounts), time.Since(start))
		// generateCommitments(*concurrency, &config, precomputedData)
		generateCommitmentsV2(&config, precomputedData)
	case "proof":
		generateProofs(common.HexToAddress(*queryAccount), uint64(*queryStartBlock)+config.StartingBlockNumber, uint64(*queryEndBlock)+config.StartingBlockNumber, precomputedData, &config)
	case "verify":
		verifyProofs(*queryStartBlock, *queryEndBlock, V, weights, srs)
	}
}

type blockModifiedAccountsBalances struct {
	Number           uint64
	ModifiedAccounts []common.Address
	Balances         []*big.Int
}

func generateCommitmentsV2(config *config.Config, precomputedData *config.PrecomputedData) {

	DB_DIR := "samurai.db"
	fmt.Println("Removing database directory", DB_DIR)
	err := os.RemoveAll(DB_DIR)
	if err != nil {
		panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
	} else {
		fmt.Println("Database directory", DB_DIR, "removed")
	}

	// Opening the database
	db, err := pebble.Open(DB_DIR, &pebble.Options{})
	if err != nil {
		panic(err)
	}
	defer db.Close()

	workers := runtime.NumCPU()
	blockCh := make(chan blockModifiedAccountsBalances, workers*2)

	// Feed blocks
	go func() {
		for bn := config.StartingBlockNumber; bn <= config.EndingBlockNumber; bn += 1 {
			// start := time.Now()
			// // fetch all the modified accounts in this block
			// modifiedAccounts, err := ledger.GetModifiedAccountsByNumber(bn, config.Client)
			// if err != nil {
			// 	panic(fmt.Errorf("failed to get modified accounts by number %d: %w", bn, err))
			// }
			// fmt.Println("Block", bn, "has", len(modifiedAccounts), "modified accounts", time.Since(start))
			// start = time.Now()
			// // fetch balances for all the modified accounts
			// balances, err := ledger.BatchMultiUserBalance(modifiedAccounts, bn, config)
			// if err != nil {
			// 	panic(fmt.Errorf("failed to get balances for block %d: %w", bn, err))
			// }
			// fmt.Println("Block", bn, "fetched balances for", len(modifiedAccounts), "accounts", time.Since(start))

			// TODO: remove this override
			balances := []*big.Int{new(big.Int).SetUint64(1000000000000000000 + uint64(bn))}
			modifiedAccounts := []common.Address{common.HexToAddress("0x0000000000000000000000000000000000000027")}
			// send the block info to the channel
			blockCh <- blockModifiedAccountsBalances{
				Number:           bn,
				ModifiedAccounts: modifiedAccounts,
				Balances:         balances,
			}
		}
		close(blockCh)
	}()

	total_start := time.Now()
	for blk := range blockCh {
		start := time.Now()
		fmt.Println("Block", blk.Number, "Processing for", len(blk.ModifiedAccounts), "accounts")
		// todo: use multiple workers to update the segment trees for different accounts
		for i, addr := range blk.ModifiedAccounts {
			segmenttree.CreateOrUpdateAccountInfo(addr, blk.Balances[i], blk.Number, db, precomputedData)

		}
		fmt.Println("Block", blk.Number, "Processed for", len(blk.ModifiedAccounts), "accounts", time.Since(start))
	}

	fmt.Println("Time taken to process all blocks", time.Since(total_start))

}
