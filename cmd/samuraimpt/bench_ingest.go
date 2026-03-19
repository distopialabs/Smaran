package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/urfave/cli/v2"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/ingest"
	"github.com/nepal80m/samurai/mpt/meta"
	st "github.com/nepal80m/samurai/mpt/state"
)

func benchIngestCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench-ingest",
		Usage: "Run a duration-based ingestion benchmark with optional hot-account filtering",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "tmp-bench-db-dir", Usage: "Root directory for all databases"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to block dataset directory"},
			&cli.DurationFlag{Name: "duration", Value: 5 * time.Minute, Usage: "How long to run the benchmark"},
			&cli.IntFlag{Name: "num-users", Value: 0, Usage: "Top-K hot accounts to include (0 = all, no filtering)"},
			&cli.StringFlag{Name: "hot-accounts-file", Value: "account_stats_all.csv", Usage: "CSV with hot accounts sorted by update count descending"},
			&cli.StringFlag{Name: "output-dir", Value: "benchmark_output/ingest", Usage: "Directory for benchmark CSV output files"},
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
			mptDBDir := filepath.Join(dbDir, "mpt")

			// Always start fresh for benchmark runs.
			if _, err := os.Stat(dbDir); err == nil {
				log.Printf("removing existing database at %s", dbDir)
				if err := os.RemoveAll(dbDir); err != nil {
					return fmt.Errorf("failed to remove %s: %w", dbDir, err)
				}
			}

			startBlock := dataset.FIRST_BLOCK

			// Samurai setup
			cryptoParamsDir := filepath.Join(dbDir, "params")
			cryptoParams, err := ingest.SetupPrecomputedData(cryptoParamsDir)
			if err != nil {
				log.Fatalf("failed to setup crypto params: %v", err)
			}

			shardsNum := 32
			shardedSamuraiStores, err := ingest.SetupDatabases(shardsNum, filepath.Join(dbDir, "db"))
			if err != nil {
				log.Fatalf("failed to setup databases: %v", err)
			}

			cacheSize := 2048
			caches, err := ingest.SetupCaches(cacheSize, shardedSamuraiStores, cryptoParams)
			if err != nil {
				log.Fatalf("failed to setup caches: %v", err)
			}
			defer ingest.Cleanup(caches, shardedSamuraiStores)

			// MPT setup
			mptStore, err := st.OpenDB(mptDBDir)
			if err != nil {
				return err
			}
			meta.PutRoot(mptStore.DiskDB, startBlock-1, types.EmptyRootHash)
			defer mptStore.Close()

			// Build config -- End is set to LAST_BLOCK inside BenchRun.
			cfg := ingest.Config{
				Shards: shardsNum,
				Blocks: ingest.BlocksConfig{
					DataDir: c.String("blocks-dir"),
					Start:   startBlock,
				},
				Workers: ingest.WorkersConfig{
					CommitWorkerCount:       shardsNum,
					CommitWorkerQueueSize:   1_000_000,
					CommitWorkerChannelSize: 5_000,
				},
				Queue: ingest.QueueConfig{
					BlockInfoChannelSize:  1024,
					UpdateTaskChannelSize: 5_000,
				},
				Caches:        caches,
				SamuraiStores: shardedSamuraiStores,
				MPTStore:      mptStore,
			}

			benchCfg := ingest.BenchConfig{
				Duration:        c.Duration("duration"),
				NumUsers:        c.Int("num-users"),
				HotAccountsFile: c.String("hot-accounts-file"),
				OutputDir:       c.String("output-dir"),
			}

			return ingest.BenchRun(cfg, benchCfg)
		},
	}
}
