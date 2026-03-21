package main

import (
	"fmt"

	"github.com/nepal80m/samurai/internal/verkle/server"
	"github.com/nepal80m/samurai/internal/verkle/store"
	"github.com/spf13/cobra"
)

func serveCmd() *cobra.Command {
	var (
		dbDir     string
		dbBackend string
		port      int
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the gRPC Verkle proof server",
		Long:  `Starts a gRPC server that serves Verkle proof queries for ingested blocks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			kv, err := store.OpenKVStore(dbBackend, dbDir)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer kv.Close()

			ns := store.NewNodeStore(kv)
			proofServer := server.NewProofServer(ns)
			addr := fmt.Sprintf(":%d", port)
			return server.ListenAndServe(addr, proofServer)
		},
	}

	cmd.Flags().StringVar(&dbDir, "db-dir", "", "Path to state database directory (required)")
	cmd.Flags().StringVar(&dbBackend, "db-backend", "pebble", "DB backend: pebble or leveldb")
	cmd.Flags().IntVar(&port, "port", 50051, "gRPC server port")
	cmd.MarkFlagRequired("db-dir")

	return cmd
}
