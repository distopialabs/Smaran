package main

import (
	"fmt"

	"github.com/nepal80m/samurai/internal/verklekzg/server"
	"github.com/nepal80m/samurai/internal/verklekzg/store"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
	"github.com/urfave/cli/v2"
)

func serveCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the gRPC Verkle-KZG proof server",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "", Usage: "Path to state database directory (required)"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.StringFlag{Name: "host", Value: "0.0.0.0", Usage: "gRPC server host"},
			&cli.IntFlag{Name: "port", Value: 50052, Usage: "gRPC server port"},
			&cli.StringFlag{Name: "params-dir", Value: "data/params/verklekzg", Usage: "Directory for precomputed SRS/barycentric files"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			host := c.String("host")
			port := c.Int("port")
			addr := fmt.Sprintf("%s:%d", host, port)
			if dbDir == "" {
				return fmt.Errorf("--db-dir is required")
			}

			treeCfg, err := tree.NewTreeConfig(c.String("params-dir"))
			if err != nil {
				return fmt.Errorf("init tree config: %w", err)
			}

			kv, err := store.OpenKVStore(c.String("db-backend"), dbDir)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer kv.Close()

			ns := store.NewNodeStore(kv)
			proofServer := server.NewProofServer(ns, treeCfg)
			return server.ListenAndServe(addr, proofServer)
		},
	}
}
