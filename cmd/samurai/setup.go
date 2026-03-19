package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/pebble"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/tree"
)

// StartBlock is the first block of 2024 (Ethereum mainnet).
const StartBlock = uint64(18908895)

// BuildConfig creates a Config from parsed flags.
func BuildConfig(flags *Flags) *config.Config {
	startBlock := StartBlock
	var endBlock uint64
	if flags.Bench {
		// benchmark mode: use unbounded range (will stop on timer or when dataset exhausted)
		endBlock = 21_700_000
	} else {
		// production mode
		endBlock = startBlock + uint64(flags.NumBlocks-1)
	}

	return &config.Config{
		Clean:           flags.Clean,
		BlocksDataDir:   "./data/blocks",
		CryptoParamsDir: filepath.Join(flags.DataDir, "params"),
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
			MemTableSize: 64 << 20, // 64MB
			DisableWAL:   true,
			CacheSize:    80_000_000,
			StoragePath:  filepath.Join(flags.DataDir, "db"),
		},
		Cache: config.Cache{
			Size:          2048,
			EnableMetrics: flags.BenchCacheMetrics,
		},
		Queue: config.Queue{
			BlockInfoChannelSize:  1024,
			UpdateTaskChannelSize: 5_000,
		},
		Benchmark: config.Benchmark{
			Enabled:              flags.Bench,
			DurationSecs:         flags.BenchDuration,
			OutputDir:            flags.BenchOutputDir,
			CollectDBMetrics:     flags.BenchDBMetrics,
			CollectPipelineSizes: flags.BenchPipeline,
			CollectCacheMetrics:  flags.BenchCacheMetrics,
		},
	}
}

// SetupPrecomputedData initializes SRS and precomputed polynomial data.
func SetupPrecomputedData(cfg *config.Config) (*config.PrecomputedData, error) {
	srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
	if err != nil {
		return nil, fmt.Errorf("failed to setup SRS: %w", err)
	}

	V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, cfg.CryptoParamsDir)
	return &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}, nil
}

// SetupDatabases creates sharded Pebble databases for state, tree, and history.
func SetupDatabases(cfg *config.Config, cleanOnCommit bool) ([]*db.SamuraiStore, []*db.PebbleDB, error) {
	dbs := make([]*db.SamuraiStore, cfg.Database.Shards)
	pebbleDbs := make([]*db.PebbleDB, cfg.Database.Shards)

	for i := range cfg.Database.Shards {
		stateDBPath := fmt.Sprintf(cfg.Database.StoragePath+"/shard-%d-state", i)
		treeDBPath := fmt.Sprintf(cfg.Database.StoragePath+"/shard-%d-tree", i)
		historyDBPath := fmt.Sprintf(cfg.Database.StoragePath+"/shard-%d-history", i)

		if cleanOnCommit {
			dirsToRemove := []string{stateDBPath, treeDBPath, historyDBPath}
			for _, dir := range dirsToRemove {
				fmt.Println("Removing database directory", dir)
				if err := os.RemoveAll(dir); err != nil {
					return nil, nil, fmt.Errorf("failed to remove database directory %s: %w", dir, err)
				}
			}
		}

		sharedCache := pebble.NewCache(int64(cfg.Database.CacheSize))

		// StateDB receives tiny struct updates, limit MemTable to 16MB
		stateDBOpts := &pebble.Options{
			MemTableSize:              16 << 20,
			L0CompactionThreshold:     2,
			L0CompactionFileThreshold: 2000,
			LBaseMaxBytes:             2147483648,              // 2GB
			MaxConcurrentCompactions:  func() int { return 4 }, // 4 threads per DB
			DisableWAL:                cfg.Database.DisableWAL,
			Cache:                     sharedCache,
		}

		// TreeDB receives massive 4KB Merkle updates, allocate the configured 64MB MemTable
		treeDBOpts := &pebble.Options{
			MemTableSize:              cfg.Database.MemTableSize,
			L0CompactionThreshold:     2,
			L0CompactionFileThreshold: 2000,
			LBaseMaxBytes:             2147483648,              // 2GB
			MaxConcurrentCompactions:  func() int { return 4 }, // 4 threads per DB
			DisableWAL:                cfg.Database.DisableWAL,
			Cache:                     sharedCache,
		}

		// HistoryDB receives steady append-only updates, limit MemTable to 32MB
		historyDBOpts := &pebble.Options{
			MemTableSize:              32 << 20,
			L0CompactionThreshold:     2,
			L0CompactionFileThreshold: 2000,
			LBaseMaxBytes:             2147483648,              // 2GB
			MaxConcurrentCompactions:  func() int { return 4 }, // 4 threads per DB
			DisableWAL:                cfg.Database.DisableWAL,
			Cache:                     sharedCache,
		}

		// Create StateDB
		stateDB, err := db.NewPebbleDB(stateDBPath, stateDBOpts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create StateDB %s: %w", stateDBPath, err)
		}

		// Create TreeDB
		treeDB, err := db.NewPebbleDB(treeDBPath, treeDBOpts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create TreeDB %s: %w", treeDBPath, err)
		}

		// Create HistoryDB
		historyDB, err := db.NewPebbleDB(historyDBPath, historyDBOpts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create HistoryDB %s: %w", historyDBPath, err)
		}

		dbs[i] = &db.SamuraiStore{
			StateDB:   stateDB,
			TreeDB:    treeDB,
			HistoryDB: historyDB,
		}
		pebbleDbs[i] = treeDB
	}

	return dbs, pebbleDbs, nil
}

// SetupCaches creates Ristretto caches for each shard.
func SetupCaches(dbs []*db.SamuraiStore, cfg *config.Config, precomputed *config.PrecomputedData) ([]*storage.Cache, error) {
	caches := make([]*storage.Cache, cfg.Database.Shards)

	for i := range cfg.Database.Shards {
		cache, err := storage.NewCache(cfg.Cache.Size, dbs[i], precomputed)
		if err != nil {
			return nil, fmt.Errorf("failed to create cache for shard %d: %w", i, err)
		}
		caches[i] = cache
	}

	return caches, nil
}

// Cleanup closes all caches and databases.
func Cleanup(caches []*storage.Cache, dbs []*db.SamuraiStore) {
	for i := range caches {
		if caches[i] != nil {
			caches[i].Close()
			fmt.Println("Cache", i, "closed")
		}
	}
	for i := range dbs {
		if dbs[i] != nil {
			dbs[i].Close()
			fmt.Println("Database", i, "closed")
		}
	}
}
