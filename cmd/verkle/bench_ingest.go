package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/ingest"
	"github.com/urfave/cli/v2"
)

func benchIngestCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench-ingest",
		Usage: "Benchmark block ingestion latency and throughput",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to dataset block segments"},
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/bench-verkle", Usage: "Path to persistent DB directory"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.IntFlag{Name: "flush-every", Value: 1000, Usage: "Reload tree every N blocks for memory management"},
			&cli.DurationFlag{Name: "duration", Value: 5 * time.Minute, Usage: "Benchmark duration (e.g. 5m, 10m, 1h)"},
			&cli.IntFlag{Name: "k-users", Value: 0, Usage: "Top-K hot accounts to include (0 = all, no filtering)"},
			&cli.StringFlag{Name: "accounts-list", Value: "account_stats_all.csv", Usage: "CSV with hot accounts sorted by update count descending"},
		},
		Action: func(c *cli.Context) error {
			blocksDir := c.String("blocks-dir")
			if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
				return fmt.Errorf("blocks directory does not exist: %s", blocksDir)
			}

			kUsers := c.Int("k-users")
			csvPath, err := benchutil.IngestionOutputPath("verkle", kUsers)
			if err != nil {
				return err
			}

			startBlock := dataset.FIRST_BLOCK
			endBlock := startBlock + 10_000_000

			return ingest.RunBench(ingest.BenchConfig{
				BlocksDir:    blocksDir,
				DBDir:        c.String("db-dir"),
				DBBackend:    c.String("db-backend"),
				Start:        startBlock,
				End:          endBlock,
				FlushEvery:   c.Int("flush-every"),
				Duration:     c.Duration("duration"),
				KUsers:       kUsers,
				AccountsList: c.String("accounts-list"),
				OutCSV:       csvPath,
			})
		},
	}
}
