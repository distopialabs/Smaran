package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verklekzg/ingest"
	"github.com/urfave/cli/v2"
)

func ingestCmd() *cli.Command {
	return &cli.Command{
		Name:  "ingest",
		Usage: "Ingest block data into a Verkle-KZG trie",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "/data/local/tmp/verklekzg", Usage: "Path to persistent DB directory"},
			&cli.StringFlag{Name: "blocks-dir", Value: "data/blocks", Usage: "Path to dataset block segments"},
			&cli.Uint64Flag{Name: "n", Value: 1000, Usage: "Number of blocks to ingest"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.IntFlag{Name: "flush-every", Value: 1000, Usage: "Reload tree from DB every N blocks"},
			&cli.BoolFlag{Name: "fresh", Value: false, Usage: "Delete existing DB and start from scratch"},
			&cli.StringFlag{Name: "params-dir", Value: "data/params/verklekzg", Usage: "Directory for precomputed SRS/barycentric files"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			if dbDir == "" {
				return fmt.Errorf("--db-dir is required")
			}

			if c.Bool("fresh") {
				if _, err := os.Stat(dbDir); err == nil {
					log.Printf("--fresh: removing existing database at %s", dbDir)
					if err := os.RemoveAll(dbDir); err != nil {
						return fmt.Errorf("--fresh: failed to remove %s: %w", dbDir, err)
					}
				}
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
				ParamsDir:  c.String("params-dir"),
				Start:      startBlock,
				End:        endBlock,
				FlushEvery: c.Int("flush-every"),
			})
		},
	}
}
