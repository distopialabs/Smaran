package main

import (
	"fmt"
	"log"

	"github.com/nepal80m/samurai/internal/merkle/meta"
	"github.com/nepal80m/samurai/internal/server"
	"github.com/urfave/cli/v2"
)

func ServeCmd() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the gRPC proof server",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db-dir", Required: true, Usage: "Database directory"},
			// host
			&cli.StringFlag{Name: "host", Value: "0.0.0.0", Usage: "gRPC server host"},
			&cli.IntFlag{Name: "port", Value: 50051, Usage: "gRPC server port"},
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
			samuraiStores, err := SetupSamuraiStores(dbDir)
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
			proofServer := server.NewProofServer(samuraiStores, cryptoParams, mptStore)
			log.Printf("Starting Samurai gRPC server on port %d", port)
			return server.ListenAndServe(addr, proofServer)
		},
	}
}
