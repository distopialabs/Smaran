package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nepal80m/samurai/internal/verkle/ingest"
	"github.com/urfave/cli/v2"
)

func benchIngestCmd() *cli.Command {
	return &cli.Command{
		Name:  "bench-ingest",
		Usage: "Benchmark block ingestion latency and throughput",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to dataset block segments"},
			&cli.StringFlag{Name: "db-dir", Value: "", Usage: "Path to persistent DB directory (required)"},
			&cli.Uint64Flag{Name: "start", Value: 0, Usage: "Start block number (default: 18908895)"},
			&cli.Uint64Flag{Name: "end", Value: 0, Usage: "End block number (0 = start + 10M)"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.IntFlag{Name: "flush-every", Value: 1000, Usage: "Reload tree every N blocks for memory management"},
			&cli.DurationFlag{Name: "duration", Value: 5 * time.Minute, Usage: "Benchmark duration (e.g. 5m, 10m, 1h)"},
			&cli.StringFlag{Name: "output-dir", Value: "benchmark_output", Usage: "Directory for output CSV files"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			if dbDir == "" {
				return fmt.Errorf("--db-dir is required")
			}

			blocksDir := c.String("blocks-dir")
			startSet := c.IsSet("start")
			start := c.Uint64("start")
			if !startSet {
				start = defaultStartBlock
			}
			end := c.Uint64("end")
			if end == 0 {
				end = start + 10_000_000
			}

			if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
				return fmt.Errorf("blocks directory does not exist: %s", blocksDir)
			}

			outputDir := c.String("output-dir")
			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}
			outputFile := filepath.Join(outputDir,
				fmt.Sprintf("ingest_bench_%s.csv", time.Now().Format("20060102_150405")))

			return ingest.RunBench(ingest.BenchConfig{
				BlocksDir:  blocksDir,
				DBDir:      dbDir,
				DBBackend:  c.String("db-backend"),
				Start:      start,
				End:        end,
				StartSet:   startSet,
				FlushEvery: c.Int("flush-every"),
				Duration:   c.Duration("duration"),
				OutputFile: outputFile,
			})
		},
	}
}
