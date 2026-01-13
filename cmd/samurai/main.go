package main

import (
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"runtime"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/crypto/kzg"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

// const STORAGE_PATH = "/data/local/samurai/test/storage"
// const PROFILE_PATH = "/data/local/samurai/test/profiles"

func main() {
	mode := flag.String("mode", "commit", "Mode to run: commit, proof, verify")
	numBlocks := flag.Int("n", 10000, "Number of blocks to process")

	// profiling flags
	profile := flag.Bool("p", true, "Profile the program")
	profilePath := flag.String("profilePath", "/data/local/samurai/test/profiles", "Path to write profile files")
	// benchmark flags
	bench := flag.Bool("bench", false, "Enable benchmark mode for commit generation")
	benchDuration := flag.Int("benchDuration", 300, "Benchmark duration in seconds (default: 5 minutes)")
	benchOutputDir := flag.String("benchOutputDir", "/data/local/samurai/test/benchmark", "Directory to write benchmark CSV files")
	benchDBMetrics := flag.Bool("benchDBMetrics", false, "Collect Pebble DB metrics (compaction, L0 files, etc.)")
	benchPipeline := flag.Bool("benchPipeline", false, "Collect pipeline sizes (queue and channel sizes per shard)")
	benchCacheMetrics := flag.Bool("benchCacheMetrics", true, "Collect Ristretto cache metrics (hits, misses, size)")

	// flags to generate proofs & verify proofs
	// TODO: accept these as parameters on rpc calls
	queryStartBlock := flag.Int("queryStartBlock", 20, "Start block for query")
	queryEndBlock := flag.Int("queryEndBlock", 20-1+100, "End block for query")
	queryAccount := flag.String("queryAccount", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account to query")

	flag.Parse()

	fmt.Println("Starting Samurai", time.Now())
	fmt.Println("NumCPU:", runtime.NumCPU())
	fmt.Println("Mode:", *mode)

	if *profile {
		defer ProfileCPU(*profilePath)()
	}

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

	startBlock := uint64(18908895) // first block of 2024 18908895
	var endBlock uint64            // last block of 2024, 21525890
	if *bench {
		// benchmark mode: use unbounded range (will stop on timer or when dataset exhausted)
		endBlock = 21_700_000 // last block in dataset, but fetcher will stop gracefully if exhausted
	} else {
		// production mode
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
			CommitWorkerChannelSize: 5_000,
		},
		Database: config.Database{
			Shards:       32,
			MemTableSize: 1_073_741_824, // 536_870_912 = 500MB ,1_073_741_824, 2_147_483_648 = 2GB, default is 4mb
			DisableWAL:   true,
			CacheSize:    8_000_000, // default is 8MB
			StoragePath:  "/data/local/samurai/test/storage",
		},
		Cache: config.Cache{
			NumCounters:   2_097_152,   // ?: recommended is 10x maxCost (2^18)
			MaxCost:       536_870_912, // 75_000_000, //536_870_912, 1_073_741_824, 2_147_483_648
			EnableMetrics: *benchCacheMetrics,
		},
		Queue: config.Queue{
			BlockInfoChannelSize:  1024,
			UpdateTaskChannelSize: 5_000, // todo: not being used, using CommitWorkerChannelSize instead
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
	dbs := make([]*segmenttree.SamuraiDB, cfg.Database.Shards)
	// TODO: remove this from main
	pebbleDbs := make([]*segmenttree.PebbleDB, cfg.Database.Shards)
	caches := make([]*segmenttree.Cache, cfg.Database.Shards)

	for i := range cfg.Database.Shards {
		// DB_DIR := fmt.Sprintf(cfg.Database.StoragePath+"/samurai-shard-%d.db", i)
		stateDBPath := fmt.Sprintf(cfg.Database.StoragePath+"/samurai-shard-%d-state.db", i)
		treeDBPath := fmt.Sprintf(cfg.Database.StoragePath+"/samurai-shard-%d-tree.db", i)
		historyDBPath := fmt.Sprintf(cfg.Database.StoragePath+"/samurai-shard-%d-history.db", i)

		if *mode == "commit" && true {
			// fmt.Println("Removing database directory", DB_DIR)
			// err = os.RemoveAll(DB_DIR)
			// if err != nil {
			// 	panic(fmt.Errorf("failed to remove database directory %s: %w", DB_DIR, err))
			// } else {
			// 	fmt.Println("Database directory", DB_DIR, "removed")
			// }
			dirsToRemove := []string{stateDBPath, treeDBPath, historyDBPath}
			for _, dir := range dirsToRemove {
				fmt.Println("Removing database directory", dir)
				err = os.RemoveAll(dir)
				if err != nil {
					panic(fmt.Errorf("failed to remove database directory %s: %w", dir, err))
				}
			}
		}

		// Create StateDB
		stateDB, err := segmenttree.NewPebbleDB(stateDBPath, &pebble.Options{
			MemTableSize: cfg.Database.MemTableSize, // Default memtable size
			DisableWAL:   cfg.Database.DisableWAL,
			Cache:        pebble.NewCache(int64(cfg.Database.CacheSize)),
		})
		if err != nil {
			panic(fmt.Errorf("failed to create StateDB %s: %w", stateDBPath, err))
		}

		// Create TreeDB (optimized for large values if needed, for now same options)
		treeDB, err := segmenttree.NewPebbleDB(treeDBPath, &pebble.Options{
			MemTableSize: cfg.Database.MemTableSize,
			DisableWAL:   cfg.Database.DisableWAL,
			Cache:        pebble.NewCache(int64(cfg.Database.CacheSize)),
		})
		if err != nil {
			panic(fmt.Errorf("failed to create TreeDB %s: %w", treeDBPath, err))
		}

		// Create HistoryDB (append-only)
		historyDB, err := segmenttree.NewPebbleDB(historyDBPath, &pebble.Options{
			MemTableSize: cfg.Database.MemTableSize,
			DisableWAL:   cfg.Database.DisableWAL,
			Cache:        pebble.NewCache(int64(cfg.Database.CacheSize)),
		})
		if err != nil {
			panic(fmt.Errorf("failed to create HistoryDB %s: %w", historyDBPath, err))
		}

		samuraiDB := &segmenttree.SamuraiDB{
			StateDB:   stateDB,
			TreeDB:    treeDB,
			HistoryDB: historyDB,
		}

		cache, err := segmenttree.NewCache(samuraiDB, &cfg.Cache, precomputedData)
		if err != nil {
			panic(err)
		}
		dbs[i] = samuraiDB
		pebbleDbs[i] = treeDB // Just for benchmarking hooks if needed, or update to nil
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
