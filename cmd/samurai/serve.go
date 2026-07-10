package main

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	"github.com/nepal80m/samurai/internal/server"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
)

func ServeCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the gRPC proof server",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Required: true, Usage: "Database directory"},
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
			cryptoParams, err := SetupCryptoParams(dbDir)
			if err != nil {
				return err
			}
			numShards := NUM_SHARDS
			samuraiStores, err := SetupSamuraiStores(dbDir, numShards)
			if err != nil {
				return err
			}
			for _, db := range samuraiStores {
				defer db.Close()
			}
			mptStore, err := OpenMPTStore(dbDir)
			if err != nil {
				return err
			}
			defer mptStore.Close()

			// Log the latest MPT state root so operators can provide it to verifiers.
			if lastBlock, err := meta.GetLast(mptStore.DiskDB); err == nil {
				if root, err := meta.GetRoot(mptStore.DiskDB, lastBlock); err == nil {
					log.Printf("MPT latest block: %d, state root: %s", lastBlock, root.Hex())
				}
			}

			// Setup bench logging if requested.
			var benchLog *benchutil.BenchLogger
			var grpcOpts []grpc.ServerOption
			if c.Bool("bench") {
				benchPath := c.String("bench-output")
				if benchPath == "" {
					ts := time.Now().Format("20060102_150405")
					benchPath = filepath.Join(".", fmt.Sprintf("bench_server_%s.csv", ts))
				}
				var err error
				benchLog, err = benchutil.NewBenchLogger(benchPath)
				if err != nil {
					return fmt.Errorf("create bench logger: %w", err)
				}
				go benchLog.Run()
				defer benchLog.Stop()
				grpcOpts = append(grpcOpts, grpc.MaxConcurrentStreams(100))
				log.Printf("Bench logging enabled, writing to %s", benchPath)
			}

			proofServer := server.NewProofServer(samuraiStores, cryptoParams, mptStore, benchLog)
			log.Printf("Starting Samurai gRPC server on port %d", port)
			return server.ListenAndServe(addr, proofServer, grpcOpts...)
		},
	}
}
