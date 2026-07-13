package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/verkle/server"
	"github.com/nepal80m/samurai/internal/verkle/store"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
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
			&cli.BoolFlag{Name: "bench", Value: false, Usage: "Enable per-request CSV logging for throughput benchmarking"},
			&cli.StringFlag{Name: "bench-output", Value: "", Usage: "Bench CSV output path (default: ./bench_server_<timestamp>.csv)"},
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

			var benchLog *benchutil.BenchLogger
			var grpcOpts []grpc.ServerOption
			if c.Bool("bench") {
				benchPath := c.String("bench-output")
				if benchPath == "" {
					ts := time.Now().Format("20060102_150405")
					benchPath = fmt.Sprintf("bench_server_%s.csv", ts)
				}
				benchLog, err = benchutil.NewBenchLogger(benchPath)
				if err != nil {
					return fmt.Errorf("create bench logger: %w", err)
				}
				go benchLog.Run()
				defer benchLog.Stop()
				grpcOpts = append(grpcOpts, grpc.MaxConcurrentStreams(100))
				log.Printf("Bench logging enabled, writing to %s", benchPath)
			}

			ns := store.NewNodeStore(kv)
			proofServer := server.NewProofServer(ns, benchLog)
			return server.ListenAndServe(addr, proofServer, grpcOpts...)
		},
	}
}
