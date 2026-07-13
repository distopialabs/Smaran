package main

import (
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v2"

	"github.com/nepal80m/samurai/internal/ingest"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"github.com/nepal80m/samurai/internal/storage"
)

func BuildMPTCmd() *cli.Command {
	return &cli.Command{
		Name:  "build-mpt",
		Usage: "Build MPT tree from already-processed Samurai data (end block from metadata)",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Required: true, Usage: "Root directory containing db/ and metadata.json"},
			&cli.IntFlag{Name: "shards", Value: NUM_SHARDS, Usage: "Number of shards (must match the run that produced the data)"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			shardsNum := c.Int("shards")

			endBlock, err := storage.GetLastProcessedBlockNumber(dbDir)
			if err != nil {
				return fmt.Errorf("failed to get last processed block number: %w", err)
			}
			if endBlock == 0 {
				return fmt.Errorf("no metadata or last processed block is 0; run samurai ingest first")
			}

			samuraiDBDir := filepath.Join(dbDir, "db")
			shardedSamuraiStores, err := ingest.SetupDatabases(shardsNum, samuraiDBDir)
			if err != nil {
				return fmt.Errorf("failed to open samurai databases: %w", err)
			}
			defer ingest.Cleanup(nil, shardedSamuraiStores)

			mptDBDir := filepath.Join(dbDir, "mpt")
			mptStore, err := st.OpenDB(mptDBDir)
			if err != nil {
				return fmt.Errorf("failed to open MPT store: %w", err)
			}
			defer mptStore.Close()

			cfg := ingest.Config{
				Shards: shardsNum,
				Blocks: ingest.BlocksConfig{
					End: endBlock,
				},
				SamuraiStores: shardedSamuraiStores,
				MPTStore:      mptStore,
			}

			if err := ingest.BuildMPT(cfg); err != nil {
				return fmt.Errorf("build MPT: %w", err)
			}

			fmt.Printf("Build MPT complete: end block %d\n", endBlock)
			return nil
		},
	}
}
