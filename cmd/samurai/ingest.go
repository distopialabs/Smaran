package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/urfave/cli/v2"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/ingest"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"github.com/nepal80m/samurai/internal/storage"
)

const NUM_SHARDS = 32

func IngestCmd() *cli.Command {
	return &cli.Command{
		Name:  "ingest",
		Usage: "Ingesting blocks",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/samurai", Usage: "Path to blocks data directory"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to blocks data directory"},
			&cli.Uint64Flag{Name: "n", Value: 1000, Usage: "Number of blocks to ingest"},
			&cli.BoolFlag{Name: "fresh", Value: false, Usage: "Delete existing DB and start from scratch"},
			&cli.BoolFlag{Name: "defer-mpt", Value: true, Usage: "Defer MPT building to after samurai ingestion (faster ingestion)"},
			&cli.StringFlag{Name: "cpuprofile", Value: "", Usage: "Write CPU profile to file"},
		},
		Action: func(c *cli.Context) error {
			if cpuprof := c.String("cpuprofile"); cpuprof != "" {
				f, err := os.Create(cpuprof)
				if err != nil {
					return fmt.Errorf("create cpu profile: %w", err)
				}
				pprof.StartCPUProfile(f)
				defer pprof.StopCPUProfile()
			}

			dbDir := c.String("db-dir")

			// Setup DB
			startBlock := dataset.FIRST_BLOCK
			endBlock := dataset.FIRST_BLOCK + c.Uint64("n") - 1
			if endBlock > dataset.LAST_BLOCK {
				endBlock = dataset.LAST_BLOCK
			}

			if c.Bool("fresh") {
				if _, err := os.Stat(dbDir); err == nil {
					log.Printf("removing existing database at %s", dbDir)
					if err := os.RemoveAll(dbDir); err != nil {
						return fmt.Errorf("failed to remove %s: %w", dbDir, err)
					}
				}
			} else {
				lastProcessedBlock, err := storage.GetLastProcessedBlockNumber(dbDir)
				if err != nil {
					return fmt.Errorf("failed to get last processed block number: %w", err)
				}
				if lastProcessedBlock >= startBlock {
					startBlock = lastProcessedBlock + 1
				}
			}

			// ## Samurai setup
			// Setup Crypto Params
			cryptoParams, err := SetupCryptoParams(dbDir)
			if err != nil {
				return err
			}
			// Setup Database

			shardsNum := NUM_SHARDS
			shardedSamuraiStores, err := SetupSamuraiStores(dbDir)
			if err != nil {
				return err
			}

			// Setup caches
			cacheSize := 2048 // max number of entries per cache
			caches, err := ingest.SetupCaches(cacheSize, shardedSamuraiStores, cryptoParams)
			if err != nil {
				log.Fatalf("failed to setup caches: %v", err)
			}

			// Cleanup on exit
			defer ingest.Cleanup(caches, shardedSamuraiStores)

			// MPT setup

			mptStore, err := OpenMPTStore(dbDir)
			if err != nil {
				return err
			}
			defer mptStore.Close()

			if c.Bool("fresh") {
				meta.PutRoot(mptStore.DiskDB, startBlock-1, types.EmptyRootHash)
			}

			// build config
			cfg := ingest.Config{
				Shards: shardsNum,
				Blocks: ingest.BlocksConfig{
					DataDir: c.String("blocks-dir"),
					Start:   startBlock,
					End:     endBlock,
				},
				Workers: ingest.WorkersConfig{
					CommitWorkerCount:       shardsNum,
					CommitWorkerQueueSize:   1_000_000,
					CommitWorkerChannelSize: 5_000,
				},
				// Database: ingest.DatabaseConfig{
				// 	Shards:       32,
				// 	MemTableSize: 64 << 20, // 64MB
				// 	DisableWAL:   true,
				// 	CacheSize:    80_000_000,
				// 	StoragePath:  samuraiDBDir,
				// },
				// Cache: ingest.CacheConfig{
				// 	Size: 2048,
				// },
				Queue: ingest.QueueConfig{
					BlockInfoChannelSize:  1024,
					UpdateTaskChannelSize: 5_000,
				},
				Caches:        caches,
				SamuraiStores: shardedSamuraiStores,
				MPTStore:      mptStore,
			}

			if c.Bool("defer-mpt") {
				err = ingest.RunDeferred(cfg)
			} else {
				err = ingest.Run(cfg)
			}
			if err != nil {
				return err
			}
			if err := storage.SetLastProcessedBlockNumber(dbDir, cfg.Blocks.End); err != nil {
				return fmt.Errorf("save metadata: %w", err)
			}

			fmt.Printf("Saved progress: LastProcessedBlock = %d\n", cfg.Blocks.End)
			return nil
		},
	}
}

func SetupCryptoParams(dbDir string) (*config.PrecomputedData, error) {
	cryptoParamsDir := filepath.Join(dbDir, "params")
	cryptoParams, err := ingest.SetupPrecomputedData(cryptoParamsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to setup crypto params: %v", err)
	}
	return cryptoParams, nil
}

func SetupSamuraiStores(dbDir string) ([]*db.SamuraiStore, error) {
	shardedSamuraiStores, err := ingest.SetupDatabases(NUM_SHARDS, filepath.Join(dbDir, "db"))
	if err != nil {
		return nil, fmt.Errorf("failed to setup databases: %v", err)
	}
	return shardedSamuraiStores, nil

}

func OpenMPTStore(dbDir string) (*st.MPTStateStore, error) {
	mptDBDir := filepath.Join(dbDir, "mpt")
	mptStore, err := st.OpenDB(mptDBDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open MPT database: %v", err)
	}
	return mptStore, nil

}
