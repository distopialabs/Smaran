package main

import (
	"fmt"
	"log"
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
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/bench-verkle", Usage: "Path to persistent DB directory"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to dataset block segments"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.IntFlag{Name: "flush-every", Value: 1000, Usage: "Reload tree every N blocks for memory management"},
			&cli.DurationFlag{Name: "duration", Value: 5 * time.Minute, Usage: "Benchmark duration (e.g. 5m, 10m, 1h)"},
			&cli.IntFlag{Name: "k-users", Value: 0, Usage: "Top-K hot accounts to include (0 = all, no filtering)"},
			&cli.StringFlag{Name: "accounts-list", Value: "account_stats_all.csv", Usage: "CSV with hot accounts sorted by update count descending"},
			&cli.BoolFlag{Name: "fresh", Value: true, Usage: "Delete existing DB and start from scratch"},
			&cli.StringFlag{Name: "output-dir", Value: benchutil.DefaultOutputDir, Usage: "Root directory for benchmark output"},
		},
		Action: func(c *cli.Context) error {
			if c.Bool("fresh") {
				if _, err := os.Stat(c.String("db-dir")); err == nil {
					log.Printf("--fresh: removing existing database at %s", c.String("db-dir"))
					if err := os.RemoveAll(c.String("db-dir")); err != nil {
						return fmt.Errorf("--fresh: failed to remove %s: %w", c.String("db-dir"), err)
					}
				}
			}

			blocksDir := c.String("blocks-dir")
			if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
				return fmt.Errorf("blocks directory does not exist: %s", blocksDir)
			}

			kUsers := c.Int("k-users")
			outputDir := c.String("output-dir")
			csvPath, err := benchutil.IngestionOutputPath(outputDir, "verkle", kUsers)
			if err != nil {
				return err
			}
			updateMetricsPath, err := benchutil.UpdateMetricsOutputPath(outputDir, "verkle", kUsers)
			if err != nil {
				return err
			}

			startBlock := dataset.FIRST_BLOCK
			endBlock := dataset.LAST_BLOCK

			return ingest.RunBench(ingest.BenchConfig{
				BlocksDir:         blocksDir,
				DBDir:             c.String("db-dir"),
				DBBackend:         c.String("db-backend"),
				Start:             startBlock,
				End:               endBlock,
				FlushEvery:        c.Int("flush-every"),
				Duration:          c.Duration("duration"),
				KUsers:            kUsers,
				AccountsList:      c.String("accounts-list"),
				OutCSV:            csvPath,
				UpdateMetricsPath: updateMetricsPath,
			})
		},
	}
}
