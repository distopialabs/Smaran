package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
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
	// f, err := os.Create("cpu.prof")
	// if err != nil {
	// 	panic(err)
	// }

	// defer f.Close()
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	// main1()
	client, err := rpc.Dial("/mydata/erigon/mainnet/geth.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to Erigon IPC: %v", err)
	}
	defer client.Close()

	config := Config{
		// GethIPC:             "/mydata/erigon/mainnet/geth.ipc",
		client:              client,
		StartingBlockNumber: 18908895, // first block of 2024
		EndingBlockNumber:   21525890, // last block of 2024
	}
	start := time.Now()
	fmt.Println("Setting up tracked accounts...")
	setTrackedAccounts(50, &config)
	fmt.Println("Time taken to set tracked accounts:", time.Since(start))
	time.Sleep(2 * time.Second)

	for bn := config.StartingBlockNumber; bn <= config.EndingBlockNumber; bn++ {
		fmt.Println("Processing block", bn)
		start := time.Now()

		balances, err := batchBalances(config.TrackedAccounts, bn, &config)
		if err != nil {
			log.Printf("failed to get balances at block %d: %v", bn, err)
			continue
		}
		_ = balances
		fmt.Printf("Time taken to get balances for block %d: %v\n", bn, time.Since(start))

		// for i, addr := range config.TrackedAccounts {
		// 	balance := balances[i]
		// 	log.Printf("balance for account %s at block %d: %v", addr.Hex(), bn, balance)
		// }
	}
}

func batchBalances(addrs []common.Address, blockNum uint64, config *Config) ([]*big.Int, error) {
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
	segmentTree := generateSegmentTreeAndCommitments(10000, V, weights, weightCommits, srs)
	fmt.Println("Time taken to generate segment tree and commitments", time.Since(start))
	_ = segmentTree
	start = time.Now()
	segmentTree.DumpStorage()
	fmt.Println("Time taken to dump storage", time.Since(start))

	// // storage := segmentTree.Storage

	// start = time.Now()
	// storage := segmenttree.LoadStorage()
	// fmt.Println("Time taken to load storage", time.Since(start))

	// testPolynomials(storage)

	// queryStartBlock := 20
	// queryEndBlock := 8049

	// start = time.Now()
	// rangeProofs := proof.GetRangeProofs(queryStartBlock, queryEndBlock, storage, V, weights, srs)
	// fmt.Println("Time taken to generate range proofs", time.Since(start))

	// start = time.Now()
	// proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, V, weights, srs, storage)
	// fmt.Println("Time taken to verify range proofs", time.Since(start))

	// _ = rangeProofs

}
func generateSegmentTreeAndCommitments(maxBlockNumber int, V polynomial.Polynomial, weights []fr.Element, weightCommits []gnark_kzg.Digest, srs *kzg.MultiSRS) *segmenttree.LayeredSegmentTree {

	segmentTree := segmenttree.NewLayeredSegmentTree(V, weights, weightCommits, srs)

	for blockNumber := range maxBlockNumber {
		// fmt.Println("Processing block", blockNumber, "...")
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
