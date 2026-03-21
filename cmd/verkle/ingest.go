package main

import (
	"fmt"
	"os"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/ingest"
	"github.com/urfave/cli/v2"
)

const defaultStartBlock = 18908895

func ingestCmd() *cli.Command {
	return &cli.Command{
		Name:  "ingest",
		Usage: "Ingest block data into a Verkle tree",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "", Usage: "Path to persistent DB directory (required)"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to dataset block segments"},
			&cli.Uint64Flag{Name: "n", Value: 1000, Usage: "Number of blocks to ingest"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.IntFlag{Name: "flush-every", Value: 1000, Usage: "Reload tree from DB every N blocks for memory management"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			if dbDir == "" {
				return fmt.Errorf("--db-dir is required")
			}

			blocksDir := c.String("blocks-dir")
			startBlock := dataset.FIRST_BLOCK
			endBlock := startBlock + c.Uint64("n") - 1

			if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
				return fmt.Errorf("blocks directory does not exist: %s", blocksDir)
			}

			return ingest.Run(ingest.Config{
				BlocksDir:  blocksDir,
				DBDir:      dbDir,
				DBBackend:  c.String("db-backend"),
				Start:      startBlock,
				End:        endBlock,
				FlushEvery: c.Int("flush-every"),
			})
		},
	}
}
