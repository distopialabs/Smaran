package main

import (
	"fmt"

	"github.com/nepal80m/samurai/internal/verkle/server"
	"github.com/nepal80m/samurai/internal/verkle/store"
	"github.com/urfave/cli/v2"
)

func serveCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the gRPC Verkle proof server",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Value: "", Usage: "Path to state database directory (required)"},
			&cli.StringFlag{Name: "db-backend", Value: "pebble", Usage: "DB backend: pebble or leveldb"},
			&cli.StringFlag{Name: "host", Value: "0.0.0.0", Usage: "gRPC server host"},
			&cli.IntFlag{Name: "port", Value: 50051, Usage: "gRPC server port"},
		},
		Action: func(c *cli.Context) error {
			dbDir := c.String("db-dir")
			host := c.String("host")
			port := c.Int("port")
			addr := fmt.Sprintf("%s:%d", host, port)
			if dbDir == "" {
				return fmt.Errorf("--db-dir is required")
			}

			kv, err := store.OpenKVStore(c.String("db-backend"), dbDir)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer kv.Close()

			ns := store.NewNodeStore(kv)
			proofServer := server.NewProofServer(ns)
			return server.ListenAndServe(addr, proofServer)
		},
	}
}
