package main

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/segmenttree"
	"github.com/nepal80m/samurai/internal/utils"
)

func generateProofs(addr common.Address, queryStartBlock uint64, queryEndBlock uint64, dbs []segmenttree.DB, precomputedData *config.PrecomputedData, config *config.Config) {

	shardIdx := utils.AddressToShardIndex(addr, config.Database.Shards)
	db := dbs[shardIdx]
	// DB_DIR := "samurai.db"

	// Opening the database
	// db, err := segmenttree.NewPebbleDB(DB_DIR, &pebble.Options{})
	// if err != nil {
	// 	panic(err)
	// }
	// defer db.Close()
	fmt.Println("Generating range proofs for account", addr.Hex())
	// convert query range in block numbers terms to version number terms
	fmt.Println("Query start block", queryStartBlock, "Query end block", queryEndBlock)
	startingVersion, endingVersion := proof.BlockRangeToVersionRange(addr, queryStartBlock, queryEndBlock, config, db)
	fmt.Println("Starting version", startingVersion, "Ending version", endingVersion)

	start := time.Now()
	rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startingVersion, endingVersion, precomputedData, config.Blocks.StartingBlockNumber, db)
	fmt.Println("Time taken to generate range proofs", time.Since(start))

	// rangeProofs, balances := proof.GetRangeProofs(addr, int(startingVersion), int(endingVersion), precomputedData.V, precomputedData.Weights, precomputedData.SRS, config.StartingBlockNumber)
	// _ = rangeProofs
	// _ = balances
	start = time.Now()
	// proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
	// proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, precomputedData)
	fmt.Println("Time taken to verify range proofs", time.Since(start))

	// dump proof and balances to file
	// proof.DumpNewProofsAndBalances(rangeProofs, balanceInfos)

}
