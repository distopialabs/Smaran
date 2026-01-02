package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/crypto/kzg"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

// const STORAGE_PATH = "/data/local/samurai/test/storage"
const PROFILE_PATH = "/data/local/samurai/test/profiles"

func main() {
	// usage: go run main.go -numBlocks 100 -numTrackedAccounts 100 -concurrency 10

	mode := flag.String("mode", "commit", "Mode to run: commit, proof, verify")
	dbEncoding := flag.String("dbEncoding", "rlp", "DB encoding: proto or rlp")
	concurrency := flag.Int("c", 1, "Concurrency level")
	_ = concurrency
	profile := flag.Bool("p", true, "Profile the program")

	// flags to generate commitments
	numBlocks := flag.Int("n", 1000, "Number of blocks to process")
	// numTrackedAccounts := flag.Int("a", 1, "Number of tracked accounts")

	// flags to generate proofs & verify proofs
	queryStartBlock := flag.Int("queryStartBlock", 20, "Start block for query")
	queryEndBlock := flag.Int("queryEndBlock", 20-1+100, "End block for query")
	queryAccount := flag.String("queryAccount", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account to query")

	flag.Parse()

	fmt.Println("Starting Samurai", time.Now())
	fmt.Println("NumCPU:", runtime.NumCPU())
	fmt.Println("Mode:", *mode)

	switch *dbEncoding {
	case "rlp":
		segmenttree.SetDBEncoding(segmenttree.EncodingRLP)
	default:
		segmenttree.SetDBEncoding(segmenttree.EncodingProto)
	}
	if *profile {
		// create PROFILE_PATH if it doesn't exist
		if _, err := os.Stat(PROFILE_PATH); os.IsNotExist(err) {
			err = os.MkdirAll(PROFILE_PATH, 0755)
			if err != nil {
				panic(err)
			}
		}
		f, err := os.Create(PROFILE_PATH + "/cpu.prof")
		if err != nil {
			panic(err)
		}

		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	// client, err := rpc.Dial("/mydata/erigon/mainnet/erigon.ipc")
	// if err != nil {
	// 	log.Fatalf("Failed to connect to Erigon IPC: %v", err)
	// }
	// defer client.Close()

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
		Blocks: config.Blocks{
			// StartingBlockNumber: 18910944,                        // first block of 2024
			// EndingBlockNumber:   18910944 + uint64(*numBlocks-1), // last block of 2024
			StartingBlockNumber: 18908895,                        // first block of 2024
			EndingBlockNumber:   18908895 + uint64(*numBlocks-1), // last block of 2024
			// EndingBlockNumber: 21525890, // last block of 2024
		},

		Workers: config.Workers{
			CommitWorkerCount:       32,
			CommitWorkerQueueSize:   1_000_000,
			CommitWorkerChannelSize: 5_000,
		},
		Database: config.Database{
			Shards:       32,
			MemTableSize: 1_073_741_824, // 1_073_741_824, 2_147_483_648
			DisableWAL:   true,
			CacheSize:    2_147_483_648,
			StoragePath:  "/data/local/samurai/test/storage",
		},
		Queue: config.Queue{
			BlockInfoChannelSize:  1024,
			UpdateTaskChannelSize: 5_000,
		},
	}

	// Setting up the databases and caches
	dbs := make([]segmenttree.DB, config.Database.Shards)
	caches := make([]*segmenttree.Cache, config.Database.Shards)

	for i := range config.Database.Shards {
		DB_DIR := fmt.Sprintf(config.Database.StoragePath+"/samurai-shard-%d.db", i)

		if *mode == "commit" && true {
			fmt.Println("Removing database directory", DB_DIR)
			err = os.RemoveAll(DB_DIR)
			if err != nil {
				panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
			} else {
				fmt.Println("Database directory", DB_DIR, "removed")
			}
		}
		db, err := segmenttree.NewPebbleDB(DB_DIR, &pebble.Options{
			MemTableSize: config.Database.MemTableSize,
			DisableWAL:   config.Database.DisableWAL,
			Cache:        pebble.NewCache(int64(config.Database.CacheSize)),
		})
		if err != nil {
			panic(fmt.Errorf("failed to create Pebble database %s: %w", DB_DIR, err))
		}
		cache, err := segmenttree.NewCache(db, precomputedData)
		if err != nil {
			panic(err)
		}
		dbs[i] = db
		caches[i] = cache
	}

	switch *mode {
	case "commit":
		// generateCommitmentsV2(&config, precomputedData)
		generateCommitmentsSimplified(&config, caches)
	case "proof":
		generateProofs(common.HexToAddress(*queryAccount), uint64(*queryStartBlock)+config.Blocks.StartingBlockNumber, uint64(*queryEndBlock)+config.Blocks.StartingBlockNumber, dbs, precomputedData, &config)
	case "verify":
		verifyProofs(*queryStartBlock, *queryEndBlock, V, weights, srs)
	}

	for i := range config.Database.Shards {
		caches[i].Close()
		fmt.Println("Cache", i, "closed")
	}
	for i := range config.Database.Shards {
		dbs[i].Close()
		fmt.Println("Database", i, "closed")
	}
}
