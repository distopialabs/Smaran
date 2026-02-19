package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"
)

func main() {
	var datadir string
	var accountHex string
	var numShards int

	flag.StringVar(&datadir, "datadir", "/data/local/samurai/db/", "Data directory containing the databases")
	flag.StringVar(&accountHex, "account", "", "Account address (hex) to debug")
	flag.IntVar(&numShards, "shards", 32, "Number of shards")
	flag.Parse()

	if accountHex == "" {
		fmt.Println("Usage: go run . -account 0x... [-datadir /path/to/data] [-shards 32]")
		os.Exit(1)
	}

	account := common.HexToAddress(accountHex)
	shardIdx := utils.AddressToShardIndex(account, numShards)

	fmt.Printf("Debugging account: %s\n", account.Hex())
	fmt.Printf("Shard index: %d\n\n", shardIdx)

	// Open shard databases (read-only)
	stateDBPath := fmt.Sprintf("%ssamurai-shard-%d-state.db", datadir, shardIdx)
	historyDBPath := fmt.Sprintf("%ssamurai-shard-%d-history.db", datadir, shardIdx)

	stateDB, err := db.NewPebbleDB(stateDBPath, &pebble.Options{ReadOnly: true})
	if err != nil {
		log.Fatalf("Failed to open stateDB at %s: %v", stateDBPath, err)
	}
	defer stateDB.Close()

	historyDB, err := db.NewPebbleDB(historyDBPath, &pebble.Options{ReadOnly: true})
	if err != nil {
		log.Fatalf("Failed to open historyDB at %s: %v", historyDBPath, err)
	}
	defer historyDB.Close()

	// 1. Get Current Balance Info
	fmt.Println("=== Current Version (CurrentBalance) ===")
	cbInfo, err := tree.GetCurrentBalanceInfo(account, stateDB)
	if err != nil {
		fmt.Printf("Error: Account not found in stateDB: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Version:    %d\n", cbInfo.Version)
	fmt.Printf("Balance:    %s\n", cbInfo.Balance.String())
	fmt.Printf("StartBlock: %d\n", cbInfo.StartBlock)

	// 2. Get First Historical Balance (version 0)
	fmt.Println("\n=== First Historical Balance (version 0) ===")
	firstHb := tree.GetHistoricalBalance(account, 0, historyDB)
	fmt.Printf("Version:    %d\n", firstHb.Version)
	fmt.Printf("Balance:    %s\n", firstHb.Balance.String())
	fmt.Printf("StartBlock: %d\n", firstHb.StartBlock)
	fmt.Printf("EndBlock:   %d\n", firstHb.EndBlock)

	// 3. Check if query range [18908915, 19108915] overlaps
	queryStart := uint64(18908915)
	queryEnd := uint64(19108915)

	fmt.Println("\n=== Query Range Check ===")
	fmt.Printf("Query range: [%d, %d]\n", queryStart, queryEnd)
	fmt.Printf("First recorded block: %d\n", firstHb.StartBlock)
	fmt.Printf("Current version starts at block: %d\n", cbInfo.StartBlock)

	if queryEnd < firstHb.StartBlock {
		fmt.Printf("\n❌ Query ENDS BEFORE first recorded block!\n")
		fmt.Printf("   Query ends at:     %d\n", queryEnd)
		fmt.Printf("   First block is:    %d\n", firstHb.StartBlock)
		fmt.Printf("   Gap: %d blocks\n", firstHb.StartBlock-queryEnd)
	} else if queryStart > cbInfo.StartBlock {
		fmt.Printf("\n❌ Query STARTS AFTER current version!\n")
	} else {
		fmt.Printf("\n✅ Query range overlaps with recorded history\n")
	}
}
