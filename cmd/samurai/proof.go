package main

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/utils"
	"github.com/urfave/cli/v2"
)

func ProofCmd() *cli.Command {
	return &cli.Command{
		Name:  "proof",
		Usage: "Generate range proofs for an account",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{Name: "db-dir", Usage: "Database directory"},
			&cli.StringFlag{Name: "addr", Value: "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", Usage: "Account address"},
			&cli.Uint64Flag{Name: "start", Value: dataset.FIRST_BLOCK, Usage: "Start block"},
			&cli.Uint64Flag{Name: "n", Value: 1000, Usage: "Range size"},
		},
		Action: func(c *cli.Context) error {
			addr := common.HexToAddress(c.String("addr"))
			queryStartBlock := uint64(c.Uint64("start-block"))
			queryEndBlock := queryStartBlock + c.Uint64("n") - 1
			dbDir := c.String("db-dir")
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

			shardIdx := utils.AddressToShardIndex(addr, NUM_SHARDS)
			db := samuraiStores[shardIdx]

			fmt.Println("Generating range proofs for account", addr.Hex())
			fmt.Println("Query start block", queryStartBlock, "Query end block", queryEndBlock)

			startingVersion, endingVersion, err := proof.BlockRangeToVersionRange(addr, queryStartBlock, queryEndBlock, db)
			if err != nil {
				fmt.Printf("Error: Failed to convert block range [%d, %d] to version range for account %s\n", queryStartBlock, queryEndBlock, addr.Hex())
				fmt.Printf("Reason: %v\n", err)
				fmt.Println("Hint: Ensure the query block range overlaps with the account's recorded history.")
				return fmt.Errorf("failed to convert block range [%d, %d] to version range for account %s: %v", queryStartBlock, queryEndBlock, addr.Hex(), err)
			}
			fmt.Printf("Resolved version range: startVersion=%d, endVersion=%d\n", startingVersion, endingVersion)

			start := time.Now()
			rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startingVersion, endingVersion, cryptoParams, db)
			fmt.Println("Time taken to generate range proofs", time.Since(start))

			fmt.Println("Range proofs", rangeProofs)

			// Note: MPT proof verification is skipped in local proof mode (no state root available).
			// Pass nil/empty MPT arguments — MPT verification is gracefully skipped when no proof nodes are provided.
			if err := proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, cryptoParams, nil, common.Hash{}, nil); err != nil {
				return fmt.Errorf("verification failed: %v", err)
			}

			start = time.Now()
			fmt.Println("Time taken to verify range proofs", time.Since(start))

			return nil
		},
	}
}
