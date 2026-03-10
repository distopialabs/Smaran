package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/logging"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"
)

var log = logging.GetLogger("debug_version")

func main() {
	var datadir string
	var accountHex string
	var numShards int

	flag.StringVar(&datadir, "datadir", "/data/local/samurai/db/", "Data directory containing the databases")
	flag.StringVar(&accountHex, "account", "", "Account address (hex) to debug")
	flag.IntVar(&numShards, "shards", 32, "Number of shards")
	flag.Parse()

	if accountHex == "" {
		log.Infof("Usage: go run . -account 0x... [-datadir /path/to/data] [-shards 32]")
		os.Exit(1)
	}

	account := common.HexToAddress(accountHex)
	shardIdx := utils.AddressToShardIndex(account, numShards)

	log.Infof("Debugging account: %s", account.Hex())
	log.Infof("Shard index: %d", shardIdx)

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
	log.Infof("=== Current Version (CurrentBalance) ===")
	cbInfo, err := tree.GetCurrentBalanceInfo(account, stateDB)
	if err != nil {
		log.Errorf("Account not found in stateDB: %v", err)
		os.Exit(1)
	}
	log.Infof("Version:    %d", cbInfo.Version)
	log.Infof("Balance:    %s", cbInfo.Balance.String())
	log.Infof("StartBlock: %d", cbInfo.StartBlock)

	// 2. Get First Historical Balance (version 0)
	log.Infof("=== First Historical Balance (version 0) ===")
	firstHb := tree.GetHistoricalBalance(account, 0, historyDB)
	log.Infof("Version:    %d", firstHb.Version)
	log.Infof("Balance:    %s", firstHb.Balance.String())
	log.Infof("StartBlock: %d", firstHb.StartBlock)
	log.Infof("EndBlock:   %d", firstHb.EndBlock)

	// 3. Check if query range [18908915, 19108915] overlaps
	queryStart := uint64(18908915)
	queryEnd := uint64(19108915)

	log.Infof("=== Query Range Check ===")
	log.Infof("Query range: [%d, %d]", queryStart, queryEnd)
	log.Infof("First recorded block: %d", firstHb.StartBlock)
	log.Infof("Current version starts at block: %d", cbInfo.StartBlock)

	if queryEnd < firstHb.StartBlock {
		log.Warningf("Query ENDS BEFORE first recorded block!")
		log.Warningf("   Query ends at:     %d", queryEnd)
		log.Warningf("   First block is:    %d", firstHb.StartBlock)
		log.Warningf("   Gap: %d blocks", firstHb.StartBlock-queryEnd)
	} else if queryStart > cbInfo.StartBlock {
		log.Warningf("Query STARTS AFTER current version!")
	} else {
		log.Infof("Query range overlaps with recorded history")
	}
}
