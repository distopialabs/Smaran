package main

import (
	"fmt"
	"os"

	"github.com/nepal80m/samurai/internal/verkle/ingest"
	"github.com/spf13/cobra"
)

const defaultStartBlock = 18908895

func ingestCmd() *cobra.Command {
	var (
		blocksDir  string
		dbDir      string
		start      uint64
		end        uint64
		dbBackend  string
		startSet   bool
		flushEvery int
	)

	cmd := &cobra.Command{
		Use:   "ingest",
		Short: "Ingest block data into a Verkle tree",
		Long:  `Reads block data from dataset segments and builds a persisted Verkle tree with EIP-6800 basic-data encoding.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			startSet = cmd.Flags().Changed("start")
			if !startSet {
				start = defaultStartBlock
			}
			if end == 0 {
				end = start // at least process the start block
			}

			if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
				return fmt.Errorf("blocks directory does not exist: %s", blocksDir)
			}

			return ingest.Run(ingest.Config{
				BlocksDir:  blocksDir,
				DBDir:      dbDir,
				DBBackend:  dbBackend,
				Start:      start,
				End:        end,
				StartSet:   startSet,
				FlushEvery: flushEvery,
			})
		},
	}

	cmd.Flags().StringVar(&blocksDir, "blocks-dir", "data/blocks", "Path to dataset block segments")
	cmd.Flags().StringVar(&dbDir, "db-dir", "", "Path to persistent DB directory (required)")
	cmd.Flags().Uint64Var(&start, "start", defaultStartBlock, "Start block number")
	cmd.Flags().Uint64Var(&end, "end", 0, "End block number (inclusive)")
	cmd.Flags().StringVar(&dbBackend, "db-backend", "pebble", "DB backend: pebble or leveldb")
	cmd.Flags().IntVar(&flushEvery, "flush-every", 1000, "Reload tree from DB every N blocks for memory management")
	cmd.MarkFlagRequired("db-dir")

	return cmd
}
