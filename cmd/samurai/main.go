package main

import (
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof"
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
	numBlocks := flag.Int("n", 10000, "Number of blocks to process")
	// numTrackedAccounts := flag.Int("a", 1, "Number of tracked accounts")

	// benchmark flags
	bench := flag.Bool("bench", false, "Enable benchmark mode for commit generation")
	benchDuration := flag.Int("benchDuration", 300, "Benchmark duration in seconds (default: 5 minutes)")
	benchOutputDir := flag.String("benchOutputDir", "/data/local/samurai/test/benchmark", "Directory to write benchmark CSV files")
	benchDBMetrics := flag.Bool("benchDBMetrics", false, "Collect Pebble DB metrics (compaction, L0 files, etc.)")
	benchPipeline := flag.Bool("benchPipeline", false, "Collect pipeline sizes (queue and channel sizes per shard)")
	benchCacheMetrics := flag.Bool("benchCacheMetrics", false, "Collect Ristretto cache metrics (hits, misses, size)")

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
	// Determine block range based on mode
	startBlock := uint64(18908895) // first block of 2024
	var endBlock uint64
	if *bench {
		// Benchmark mode: use unbounded range (will stop on timer or when dataset exhausted)
		endBlock = 21_700_000 // Last block in dataset, but fetcher will stop gracefully if exhausted
	} else {
		// Production mode: use -n flag
		endBlock = startBlock + uint64(*numBlocks-1)
	}

	cfg := config.Config{
		Blocks: config.Blocks{
			StartingBlockNumber: startBlock,
			EndingBlockNumber:   endBlock,
		},

		Workers: config.Workers{
			CommitWorkerCount:       32,
			CommitWorkerQueueSize:   1_000_000,
			CommitWorkerChannelSize: 1_000,
		},
		Database: config.Database{
			Shards:       32,
			MemTableSize: 1_073_741_824, // 536_870_912 = 500MB ,1_073_741_824, 2_147_483_648 = 2GB
			DisableWAL:   true,
			CacheSize:    8_000_000, // TODO: make 1gb 8_000_000
			StoragePath:  "/data/local/samurai/test/storage",
		},
		Cache: config.Cache{
			NumCounters:   2_097_152,  // ?: recommended is 10x maxCost (2^18)
			MaxCost:       75_000_000, //536_870_912, 1_073_741_824, 2_147_483_648
			EnableMetrics: *benchCacheMetrics,
		},
		Queue: config.Queue{
			BlockInfoChannelSize:  1024,
			UpdateTaskChannelSize: 5_000,
		},
		Benchmark: config.Benchmark{
			Enabled:              *bench,
			DurationSecs:         *benchDuration,
			OutputDir:            *benchOutputDir,
			CollectDBMetrics:     *benchDBMetrics,
			CollectPipelineSizes: *benchPipeline,
			CollectCacheMetrics:  *benchCacheMetrics,
		},
	}

	// Setting up the databases and caches
	dbs := make([]segmenttree.DB, cfg.Database.Shards)
	pebbleDbs := make([]*segmenttree.PebbleDB, cfg.Database.Shards)
	caches := make([]*segmenttree.Cache, cfg.Database.Shards)

	for i := range cfg.Database.Shards {
		DB_DIR := fmt.Sprintf(cfg.Database.StoragePath+"/samurai-shard-%d.db", i)

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
			MemTableSize: cfg.Database.MemTableSize,
			DisableWAL:   cfg.Database.DisableWAL,
			Cache:        pebble.NewCache(int64(cfg.Database.CacheSize)), //TODO: increase to 8 gb
		})
		if err != nil {
			panic(fmt.Errorf("failed to create Pebble database %s: %w", DB_DIR, err))
		}
		cache, err := segmenttree.NewCache(db, &cfg.Cache, precomputedData)
		if err != nil {
			panic(err)
		}
		dbs[i] = db
		pebbleDbs[i] = db
		caches[i] = cache
	}

	switch *mode {
	case "commit":
		if cfg.Benchmark.Enabled {
			generateCommitmentsBenchmark(&cfg, caches, pebbleDbs)
		} else {
			generateCommitmentsSimplified(&cfg, caches)
		}
	case "proof":
		generateProofs(common.HexToAddress(*queryAccount), uint64(*queryStartBlock)+cfg.Blocks.StartingBlockNumber, uint64(*queryEndBlock)+cfg.Blocks.StartingBlockNumber, dbs, precomputedData, &cfg)
	case "verify":
		verifyProofs(*queryStartBlock, *queryEndBlock, V, weights, srs)
	}

	for i := range cfg.Database.Shards {
		caches[i].Close()
		fmt.Println("Cache", i, "closed")
	}
	for i := range cfg.Database.Shards {
		dbs[i].Close()
		fmt.Println("Database", i, "closed")
	}
}
