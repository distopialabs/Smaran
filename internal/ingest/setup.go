package ingest

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/tree"
)

// SetupPrecomputedData initializes SRS and precomputed polynomial data.
func SetupPrecomputedData(dir string) (*config.PrecomputedData, error) {
	srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
	if err != nil {
		return nil, fmt.Errorf("failed to setup SRS: %w", err)
	}

	V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, dir)
	return &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}, nil
}

// SetupDatabases creates sharded Pebble databases for state, tree, and history.
func SetupDatabases(shards int, dir string) ([]*db.SamuraiStore, error) {
	shardedStores := make([]*db.SamuraiStore, shards)
	// pebbleDbs := make([]*db.PebbleDB, cfg.Database.Shards)

	for i := range shards {
		stateDBPath := fmt.Sprintf(dir+"/shard-%d-state", i)
		treeDBPath := fmt.Sprintf(dir+"/shard-%d-tree", i)
		historyDBPath := fmt.Sprintf(dir+"/shard-%d-history", i)

		sharedCache := pebble.NewCache(80_000_000)

		// StateDB receives tiny struct updates, limit MemTable to 16MB
		stateDBOpts := &pebble.Options{
			MemTableSize:              16 << 20,
			L0CompactionThreshold:     2,
			L0CompactionFileThreshold: 2000,
			LBaseMaxBytes:             2147483648,              // 2GB
			MaxConcurrentCompactions:  func() int { return 4 }, // 4 threads per DB
			DisableWAL:                true,
			Cache:                     sharedCache,
		}

		// TreeDB receives massive 4KB Merkle updates, allocate the configured 64MB MemTable
		treeDBOpts := &pebble.Options{
			MemTableSize:              64 << 20,
			L0CompactionThreshold:     2,
			L0CompactionFileThreshold: 2000,
			LBaseMaxBytes:             2147483648,              // 2GB
			MaxConcurrentCompactions:  func() int { return 4 }, // 4 threads per DB
			DisableWAL:                true,
			Cache:                     sharedCache,
		}

		// HistoryDB receives steady append-only updates, limit MemTable to 32MB
		historyDBOpts := &pebble.Options{
			MemTableSize:              32 << 20,
			L0CompactionThreshold:     2,
			L0CompactionFileThreshold: 2000,
			LBaseMaxBytes:             2147483648,              // 2GB
			MaxConcurrentCompactions:  func() int { return 4 }, // 4 threads per DB
			DisableWAL:                true,
			Cache:                     sharedCache,
		}

		// Create StateDB
		stateDB, err := db.NewPebbleDB(stateDBPath, stateDBOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create StateDB %s: %w", stateDBPath, err)
		}

		// Create TreeDB
		treeDB, err := db.NewPebbleDB(treeDBPath, treeDBOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create TreeDB %s: %w", treeDBPath, err)
		}

		// Create HistoryDB
		historyDB, err := db.NewPebbleDB(historyDBPath, historyDBOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to create HistoryDB %s: %w", historyDBPath, err)
		}

		shardedStores[i] = &db.SamuraiStore{
			StateDB:   stateDB,
			TreeDB:    treeDB,
			HistoryDB: historyDB,
		}
		// pebbleDbs[i] = treeDB
	}

	return shardedStores, nil
}

// SetupCaches creates Ristretto caches for each shard.
func SetupCaches(size int, dbStores []*db.SamuraiStore, precomputed *config.PrecomputedData) ([]*storage.Cache, error) {
	caches := make([]*storage.Cache, len(dbStores))

	for i, dbStore := range dbStores {
		cache, err := storage.NewCache(size, dbStore, precomputed)
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
