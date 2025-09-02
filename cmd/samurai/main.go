package main

import (
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/nepal80m/samurai/internal/crypto/kzg"

	"github.com/nepal80m/samurai/internal/math/polynomial"
	"github.com/nepal80m/samurai/internal/proof"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/ledger"
	"github.com/nepal80m/samurai/internal/math/segmenttree"
)

func main() {
	// usage: go run main.go -numBlocks 100 -numTrackedAccounts 100 -concurrency 10

	mode := flag.String("mode", "commit", "Mode to run: commit, proof, verify")
	concurrency := flag.Int("c", 1, "Concurrency level")
	profile := flag.Bool("p", false, "Profile the program")

	// flags to generate commitments
	numBlocks := flag.Int("numBlocks", 1000, "Number of blocks to process")
	numTrackedAccounts := flag.Int("a", 1, "Number of tracked accounts")

	// flags to generate proofs & verify proofs
	queryStartBlock := flag.Int("queryStartBlock", 20, "Start block for query")
	queryEndBlock := flag.Int("queryEndBlock", 200019, "End block for query")
	queryAccount := flag.String("queryAccount", "0x0000000000000000000000000000000000000027", "Account to query")

	flag.Parse()

	if *profile {
		f, err := os.Create("cpu.prof")
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

	config := config.Config{
		// GethIPC:             "/mydata/erigon/mainnet/geth.ipc",
		Client:              client,
		StartingBlockNumber: 18908895,                      // first block of 2024
		EndingBlockNumber:   18908895 + uint64(*numBlocks), // last block of 2024
		// EndingBlockNumber:   18908895 + 2050, // last block of 2024
		// EndingBlockNumber: 21525890, // last block of 2024
	}

	switch *mode {
	case "commit":
		start := time.Now()
		fmt.Println("Setting up tracked accounts...")
		config.SetTrackedAccounts(*numTrackedAccounts)
		fmt.Printf("Time taken to set %d tracked accounts: %v\n", len(config.TrackedAccounts), time.Since(start))
		generateCommitments(*concurrency, &config, V, weights, weightCommits, srs)
	case "proof":
		generateProofs(common.HexToAddress(*queryAccount), *queryStartBlock, *queryEndBlock, V, weights, srs, &config)
	case "verify":
		verifyProofs(*queryStartBlock, *queryEndBlock, V, weights, srs)
	}
}

func generateCommitments(concurrency int, config *config.Config, V polynomial.Polynomial, weights []fr.Element, weightCommits []gnark_kzg.Digest, srs *kzg.MultiSRS) {

	os.RemoveAll(segmenttree.StoragePath)

	accountTrees := make(map[common.Address]*segmenttree.LayeredSegmentTree, len(config.TrackedAccounts))
	for _, addr := range config.TrackedAccounts {
		accountTrees[addr] = segmenttree.NewLayeredSegmentTree(addr, V, weights, weightCommits, srs)
	}

	total_start := time.Now()
	for bn := config.StartingBlockNumber; bn <= config.EndingBlockNumber; bn += 1 {
		fmt.Println("\nProcessing block", bn)

		inner_total_start := time.Now()
		start := time.Now()
		balances, err := ledger.BatchMultiUserBalance(config.TrackedAccounts, bn, config)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Time taken to get balances for block %d: %v\n", bn, time.Since(start))

		start = time.Now()
		rel_bn := bn - config.StartingBlockNumber
		var wg sync.WaitGroup
		sem := make(chan struct{}, concurrency)

		for i, addr := range config.TrackedAccounts {
			wg.Add(1)
			sem <- struct{}{}

			go func(i int, addr common.Address) {
				defer wg.Done()
				defer func() { <-sem }()

				balance := balances[i]
				if balance.Cmp(big.NewInt(0)) == 0 {
					fmt.Println("Balance is 0 for account", addr.Hex())
				}
				accountTrees[addr].Update(int(rel_bn), balance)
			}(i, addr)
		}

		wg.Wait()
		fmt.Printf("Time taken to update account trees for block %d: %v\n", bn, time.Since(start))

		fmt.Printf("Time taken to process block %d: %v\n", bn, time.Since(inner_total_start))
		// every 100 blocks, print the time elapsed
		if bn&127 == 0 {
			fmt.Printf("Time elapsed: %v\n", time.Since(total_start))
		}
	}
	fmt.Printf("\nTime taken to process all blocks: %v\n", time.Since(total_start))
	start := time.Now()
	for _, addr := range config.TrackedAccounts {
		accountTrees[addr].FlushIfRemaining(int(config.EndingBlockNumber - config.StartingBlockNumber))
	}
	fmt.Printf("Time taken to flush account trees: %v\n", time.Since(start))

	// queryStartBlock := 20
	// queryEndBlock := 200000

	// for _, addr := range config.TrackedAccounts {
	// 	start := time.Now()
	// 	fmt.Println("Generating range proofs for account", addr.Hex())
	// 	rangeProofs, balances := proof.GetRangeProofs(addr, queryStartBlock, queryEndBlock, V, weights, srs, config.StartingBlockNumber)
	// 	_ = rangeProofs
	// 	_ = balances
	// 	fmt.Println("Time taken to generate range proofs", time.Since(start))
	// 	start = time.Now()
	// 	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
	// 	fmt.Println("Time taken to verify range proofs", time.Since(start))
	// 	break
	// }

	// main2()
}

func generateProofs(addr common.Address, queryStartBlock int, queryEndBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS, config *config.Config) {
	// 0x0000000000000000000000000000000000000027
	start := time.Now()
	fmt.Println("Generating range proofs for account", addr.Hex())
	rangeProofs, balances := proof.GetRangeProofs(addr, queryStartBlock, queryEndBlock, V, weights, srs, config.StartingBlockNumber)
	_ = rangeProofs
	_ = balances
	fmt.Println("Time taken to generate range proofs", time.Since(start))
	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
	fmt.Println("Time taken to verify range proofs", time.Since(start))

	// dump proof and balances to file
	proof.DumpProofsAndBalances(rangeProofs, balances)

}

func verifyProofs(queryStartBlock int, queryEndBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) {
	numBlocks := queryEndBlock - queryStartBlock + 1
	start := time.Now()
	proofs, balances, err := proof.ReadProofsAndBalances(numBlocks)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Time taken to read proofs and balances: %v\n", time.Since(start))
	// TODO: FIX REBUILD PARTIAL TREE. issue is in commitment json marsharling and unmarshaling
	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, proofs, balances, V, weights, srs)
	fmt.Println("Time taken to verify range proofs", time.Since(start))

	// start = time.Now()
	// proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
	// fmt.Println("Time taken to verify range proofs", time.Since(start))

}
