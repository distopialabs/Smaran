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

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/ingest"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

func benchIngestCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench-ingest",
		Usage: "Run a duration-based ingestion benchmark with optional hot-account filtering",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/bench-samurai", Usage: "Root directory for all databases"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to block dataset directory"},
			&cli.Uint64Flag{Name: "n", Value: 50000, Usage: "Number of blocks to ingest"},
			&cli.DurationFlag{Name: "duration", Value: 15 * time.Minute, Usage: "Deadline/timeout for the benchmark"},
			&cli.IntFlag{Name: "k-users", Value: 0, Usage: "Top-K hot accounts to include (0 = all, no filtering)"},
			&cli.StringFlag{Name: "accounts-list", Value: "account_stats_all.csv", Usage: "CSV with hot accounts sorted by update count descending"},
			&cli.StringFlag{Name: "cpuprofile", Value: "", Usage: "Write CPU profile to file"},
			&cli.BoolFlag{Name: "skip-mpt", Value: false, Usage: "Skip MPT and run samurai-only (KZG) benchmark"},
			&cli.StringFlag{Name: "output-dir", Value: benchutil.DefaultOutputDir, Usage: "Root directory for benchmark output"},
			&cli.IntFlag{Name: "shards", Value: 1000, Usage: "Number of shards to use"},
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
			kUsers := c.Int("k-users")
			outputDir := c.String("output-dir")

			// Always start fresh for benchmark runs.
			if _, err := os.Stat(dbDir); err == nil {
				log.Printf("removing existing database at %s", dbDir)
				if err := os.RemoveAll(dbDir); err != nil {
					return fmt.Errorf("failed to remove %s: %w", dbDir, err)
				}
			}

			skipMPT := c.Bool("skip-mpt")

			// Determine output CSV path -- use distinct protocol name.
			protocol := "samuraimpt"
			if skipMPT {
				protocol = "samurai"
			}
			csvPath, err := benchutil.IngestionOutputPath(outputDir, protocol, kUsers)
			if err != nil {
				return err
			}
			updateMetricsPath, err := benchutil.UpdateMetricsOutputPath(outputDir, protocol, kUsers)
			if err != nil {
				return err
			}

			startBlock := dataset.FIRST_BLOCK
			endBlock := startBlock + c.Uint64("n") - 1

			// Samurai setup (always needed).
			cryptoParamsDir := filepath.Join(dbDir, "params")
			cryptoParams, err := ingest.SetupPrecomputedData(cryptoParamsDir)
			if err != nil {
				log.Fatalf("failed to setup crypto params: %v", err)
			}

			shardsNum := c.Int("shards")
			shardedSamuraiStores, err := ingest.SetupDatabases(shardsNum, filepath.Join(dbDir, "db"))
			if err != nil {
				log.Fatalf("failed to setup databases: %v", err)
			}

			cacheSize := 64
			caches, err := ingest.SetupCaches(cacheSize, shardedSamuraiStores, cryptoParams)
			if err != nil {
				log.Fatalf("failed to setup caches: %v", err)
			}
			defer ingest.Cleanup(caches, shardedSamuraiStores)

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
				Queue: ingest.QueueConfig{
					BlockInfoChannelSize:  1024,
					UpdateTaskChannelSize: 5_000,
				},
				Caches:        caches,
				SamuraiStores: shardedSamuraiStores,
			}

			benchCfg := ingest.BenchConfig{
				Duration:          c.Duration("duration"),
				KUsers:            kUsers,
				AccountsList:      c.String("accounts-list"),
				UpdateMetricsPath: updateMetricsPath,
			}

			if skipMPT {
				return ingest.BenchRunSamuraiOnly(cfg, benchCfg, csvPath)
			}

			// Full samurai+MPT benchmark: set up MPT store.
			mptDBDir := filepath.Join(dbDir, "mpt")
			mptStore, err := st.OpenDB(mptDBDir)
			if err != nil {
				return err
			}
			meta.PutRoot(mptStore.DiskDB, startBlock-1, types.EmptyRootHash)
			defer mptStore.Close()

			cfg.MPTStore = mptStore
			return ingest.BenchRun(cfg, benchCfg, csvPath)
		},
	}
}
