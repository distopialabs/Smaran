package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime/pprof"
	"sync"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"

	"github.com/nepal80m/samurai/segmenttree"
)

func main() {
	main1()
	// tree, err := segmenttree.ReadTreeSegment("storage", common.HexToAddress("0x0000000000000000000000000000000000000006"), 3, 10)
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println(tree)
}
func main1() {
	f, err := os.Create("cpu.prof")
	if err != nil {
		panic(err)
	}

	defer f.Close()
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()

	os.RemoveAll("storage")

	// main1()
	client, err := rpc.Dial("/mydata/erigon/mainnet/geth.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to Erigon IPC: %v", err)
	}
	defer client.Close()

	config := Config{
		// GethIPC:             "/mydata/erigon/mainnet/geth.ipc",
		client:              client,
		StartingBlockNumber: 18908895,        // first block of 2024
		EndingBlockNumber:   18908895 + 2050, // last block of 2024
		// EndingBlockNumber: 21525890, // last block of 2024
	}
	start := time.Now()
	fmt.Println("Setting up tracked accounts...")
	setTrackedAccounts(50, &config)
	fmt.Printf("Time taken to set %d tracked accounts: %v\n", len(config.TrackedAccounts), time.Since(start))

	srs, err := kzg.SetupSRS(segmenttree.SegmentTreeSize)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}
	// // // V, weights := polynomial.LoadBarycentricData(segmenttree.SegmentTreeSize)
	V, weights, weightCommits := kzg.LoadBarycentricData(segmenttree.SegmentTreeSize, srs)
	_ = weightCommits

	accountTrees := make(map[common.Address]*segmenttree.LayeredSegmentTree, len(config.TrackedAccounts))
	for _, addr := range config.TrackedAccounts {
		accountTrees[addr] = segmenttree.NewLayeredSegmentTree(addr, V, weights, weightCommits, srs)
	}

	total_start := time.Now()
	for bn := config.StartingBlockNumber; bn <= config.EndingBlockNumber; bn += 1 {
		fmt.Println("\nProcessing block", bn)

		inner_total_start := time.Now()
		start := time.Now()
		balances, err := batchMultiUserBalance(config.TrackedAccounts, bn, &config)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Time taken to get balances for block %d: %v\n", bn, time.Since(start))

		start = time.Now()
		rel_bn := bn - config.StartingBlockNumber
		var wg sync.WaitGroup
		sem := make(chan struct{}, 16)

		for i, addr := range config.TrackedAccounts {
			wg.Add(1)
			sem <- struct{}{}

			go func(i int, addr common.Address) {
				defer wg.Done()
				defer func() { <-sem }()

				balance := balances[i]
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
	for _, addr := range config.TrackedAccounts {
		accountTrees[addr].FlushIfRemaining(int(config.EndingBlockNumber - config.StartingBlockNumber))
	}
	fmt.Printf("Time taken to process all blocks: %v\n", time.Since(total_start))

	// queryStartBlock := 20
	// queryEndBlock := 2000

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

func batchMultiUserBalance(addrs []common.Address, blockNum uint64, config *Config) ([]*big.Int, error) {
	client := config.client

	elems := make([]rpc.BatchElem, len(addrs))
	for i, addr := range addrs {
		var bal hexutil.Big
		elems[i] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args:   []any{addr, hexutil.Uint64(blockNum)},
			Result: &bal,
		}
	}

	if err := client.BatchCallContext(context.Background(), elems); err != nil {
		return nil, err
	}

	balances := make([]*big.Int, len(elems))
	for i, e := range elems {
		balances[i] = e.Result.(*hexutil.Big).ToInt()
	}
	return balances, nil
}

func batchSingleUserBalances(addr common.Address, startBlockNum, endBlockNum uint64, config *Config) ([]*big.Int, error) {
	client := config.client

	elems := make([]rpc.BatchElem, endBlockNum-startBlockNum+1)
	for i := range endBlockNum - startBlockNum + 1 {
		var bal hexutil.Big
		elems[i] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args:   []any{addr, hexutil.Uint64(startBlockNum + uint64(i))},
			Result: &bal,
		}
	}

	if err := client.BatchCallContext(context.Background(), elems); err != nil {
		return nil, err
	}

	balances := make([]*big.Int, len(elems))
	for i, e := range elems {
		balances[i] = e.Result.(*hexutil.Big).ToInt()
	}
	return balances, nil
}

func setTrackedAccounts(count int, config *Config) []common.Address {
	client := config.client

	accountAddrs := make([]common.Address, 0, count)
	startKey := []byte{}
	for {
		var iteratorDump struct {
			Root     string                 `json:"root"`
			Accounts map[common.Address]any `json:"accounts"`
			Next     []byte                 `json:"next"`
		}
		blockNumber := config.StartingBlockNumber
		const batchSize = 256
		if err := client.Call(
			&iteratorDump,
			"debug_accountRange",
			blockNumber, // numeric block tag
			startKey,    // starting key for pagination
			batchSize,   // how many accounts to fetch per page
			true,        // exclude code info in account?
			true,        // exclude storage info in account?
		); err != nil {
			log.Fatalf("RPC error calling debug_accountRange: %v", err)
		}

		for addr := range iteratorDump.Accounts {
			if len(accountAddrs) >= count {
				break
			}
			accountAddrs = append(accountAddrs, addr)
		}
		if len(accountAddrs) >= count || len(iteratorDump.Next) == 0 {
			break
		}
		startKey = iteratorDump.Next
	}
	config.TrackedAccounts = accountAddrs
	return accountAddrs

}

func main2() {
	// fmt.Println("Starting Samurai...\n")

	start := time.Now()

	srs, err := kzg.SetupSRS(segmenttree.SegmentTreeSize)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}
	// V, weights := polynomial.LoadBarycentricData(segmenttree.SegmentTreeSize)
	V, weights, weightCommits := kzg.LoadBarycentricData(segmenttree.SegmentTreeSize, srs)
	fmt.Println("Time taken to setup SRS", time.Since(start))
	_ = weightCommits

	start = time.Now()
	segmentTree := generateSegmentTreeAndCommitments(3000, V, weights, weightCommits, srs)
	fmt.Println("Time taken to generate segment tree and commitments", time.Since(start))
	_ = segmentTree

	segmentTree.FlushIfRemaining(3000)

	// start = time.Now()
	// segmentTree.DumpStorage()
	// fmt.Println("Time taken to dump storage", time.Since(start))

	// start = time.Now()
	storage := segmenttree.LoadStorage()
	fmt.Println("Time taken to load storage", time.Since(start))
	_ = storage
	// testPolynomials(storage)

	// queryStartBlock := 20
	// queryEndBlock := 3000

	// start = time.Now()
	// rangeProofs := proof.GetRangeProofs(queryStartBlock, queryEndBlock, storage, V, weights, srs)
	// fmt.Println("Time taken to generate range proofs", time.Since(start))

	// start = time.Now()
	// proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, V, weights, srs, storage)
	// fmt.Println("Time taken to verify range proofs", time.Since(start))

	// _ = rangeProofs

}
func generateSegmentTreeAndCommitments(maxBlockNumber int, V polynomial.Polynomial, weights []fr.Element, weightCommits []gnark_kzg.Digest, srs *kzg.MultiSRS) *segmenttree.LayeredSegmentTree {

	segmentTree := segmenttree.NewLayeredSegmentTree(common.HexToAddress("0x0000000000000000000000000000000000000001"), V, weights, weightCommits, srs)

	for blockNumber := range maxBlockNumber + 1 {
		fmt.Println("Processing block", blockNumber, "...")
		// random balance
		balance := big.NewInt(1000000000000000000)
		balance.Add(balance, big.NewInt(int64(blockNumber)))
		// balance := big.NewInt(rand.Int63n(1000000000000000000))

		segmentTree.Update(blockNumber, balance)
	}

	return segmentTree
}

func testPolynomials(storage *segmenttree.Storage) {
	// Test the segment tree with some sample queries

	nodeIdx := 0
	nodeIdxFr := fr.NewElement(uint64(nodeIdx))

	P1 := storage.L1Polynomial[0]
	P2 := storage.L2Polynomial[0]
	P3 := storage.L3Polynomial[0]
	P4 := storage.L4Polynomial[0]

	eval1Fr := P1.Eval(&nodeIdxFr)
	eval2Fr := P2.Eval(&nodeIdxFr)
	eval3Fr := P3.Eval(&nodeIdxFr)
	eval4Fr := P4.Eval(&nodeIdxFr)

	eval1Hash := polynomial.FieldElementToHash(eval1Fr)
	eval2Hash := polynomial.FieldElementToHash(eval2Fr)
	eval3Hash := polynomial.FieldElementToHash(eval3Fr)
	eval4Hash := polynomial.FieldElementToHash(eval4Fr)

	fmt.Println(eval1Hash)
	fmt.Println(eval2Hash)
	fmt.Println(eval3Hash)
	fmt.Println(eval4Hash)

}
