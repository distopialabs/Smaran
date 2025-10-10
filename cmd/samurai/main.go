package main

import (
	"flag"
	"log"
	"os"
	"runtime/pprof"

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
	numBlocks := flag.Int("numBlocks", 10, "Number of blocks to process")
	// numTrackedAccounts := flag.Int("a", 1, "Number of tracked accounts")

	// flags to generate proofs & verify proofs
	queryStartBlock := flag.Int("queryStartBlock", 20, "Start block for query")
	queryEndBlock := flag.Int("queryEndBlock", 20-1+100, "End block for query")
	queryAccount := flag.String("queryAccount", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account to query")

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
		Client: client,
		// StartingBlockNumber: 18910944,                        // first block of 2024
		// EndingBlockNumber:   18910944 + uint64(*numBlocks-1), // last block of 2024
		StartingBlockNumber: 18908895,                        // first block of 2024
		EndingBlockNumber:   18908895 + uint64(*numBlocks-1), // last block of 2024
		// EndingBlockNumber: 21525890, // last block of 2024
	}

	switch *mode {
	case "commit":
		generateCommitmentsV2(&config, precomputedData)
	case "proof":
		generateProofs(common.HexToAddress(*queryAccount), uint64(*queryStartBlock)+config.StartingBlockNumber, uint64(*queryEndBlock)+config.StartingBlockNumber, precomputedData, &config)
	case "verify":
		verifyProofs(*queryStartBlock, *queryEndBlock, V, weights, srs)
	}
}
