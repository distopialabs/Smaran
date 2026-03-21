package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nepal80m/samurai/internal/verkle/ingest"
	"github.com/spf13/cobra"
)

func benchIngestCmd() *cobra.Command {
	var (
		blocksDir  string
		dbDir      string
		dbBackend  string
		start      uint64
		end        uint64
		startSet   bool
		flushEvery int
		duration   time.Duration
		outputDir  string
	)

	cmd := &cobra.Command{
		Use:   "bench-ingest",
		Short: "Benchmark block ingestion latency and throughput",
		Long: `Runs block ingestion for a fixed duration, logging per-block
timestamps (block_number, entries, dirty_nodes, start_ns, end_ns, flush) to a
CSV file. After the time limit, the current in-flight block completes before
stopping. Use the companion notebook to visualize windowed latency/throughput.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startSet = cmd.Flags().Changed("start")
			if !startSet {
				start = defaultStartBlock
			}
			if end == 0 {
				end = start + 10_000_000
			}

			if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
				return fmt.Errorf("blocks directory does not exist: %s", blocksDir)
			}

			if err := os.MkdirAll(outputDir, 0o755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}
			outputFile := filepath.Join(outputDir,
				fmt.Sprintf("ingest_bench_%s.csv", time.Now().Format("20060102_150405")))

			return ingest.RunBench(ingest.BenchConfig{
				BlocksDir:  blocksDir,
				DBDir:      dbDir,
				DBBackend:  dbBackend,
				Start:      start,
				End:        end,
				StartSet:   startSet,
				FlushEvery: flushEvery,
				Duration:   duration,
				OutputFile: outputFile,
			})
		},
	}

	cmd.Flags().StringVar(&blocksDir, "blocks-dir", "data/blocks", "Path to dataset block segments")
	cmd.Flags().StringVar(&dbDir, "db-dir", "", "Path to persistent DB directory (required)")
	cmd.Flags().Uint64Var(&start, "start", defaultStartBlock, "Start block number")
	cmd.Flags().Uint64Var(&end, "end", 0, "End block number (0 = start + 10M)")
	cmd.Flags().StringVar(&dbBackend, "db-backend", "pebble", "DB backend: pebble or leveldb")
	cmd.Flags().IntVar(&flushEvery, "flush-every", 1000, "Reload tree every N blocks for memory management")
	cmd.Flags().DurationVar(&duration, "duration", 5*time.Minute, "Benchmark duration (e.g. 5m, 10m, 1h)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "benchmark_output", "Directory for output CSV files")
	cmd.MarkFlagRequired("db-dir")

	return cmd
}
