package main

import (
	"encoding/csv"
	"flag"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("count_updates")

func main() {
	var (
		numBlocks       = flag.Int("n", 10000, "Number of blocks to process")
		startBlock      = flag.Int("start", 18908895, "Starting block number")
		toolsOutputPath = flag.String("o", "account_stats.csv", "Output file path for account statistics")
		datasetDir      = flag.String("dataset", "./data/blocks", "Path to blocks dataset directory")
	)
	flag.Parse()

	log.Infof("Starting account update analysis...")
	log.Infof("Dataset: %s", *datasetDir)
	log.Infof("Start Block: %d", *startBlock)
	log.Infof("Blocks: %d", *numBlocks)
	log.Infof("Output: %s", *toolsOutputPath)

	ds := dataset.NewDatasetReader(*datasetDir, dataset.SEGMENT_SIZE)
	defer ds.Close()

	updateCounts := make(map[common.Address]uint64)
	totalUpdates := uint64(0)

	startTime := time.Now()

	// Helper to track updates
	processBlock := func(n uint32, entries []dataset.Entry) error {
		for _, entry := range entries {
			var addr common.Address
			copy(addr[:], entry.Address[:])
			updateCounts[addr]++
			totalUpdates++
		}
		if n%1000 == 0 {
			log.Infof("Processed block %d... (Total updates: %d, Unique accounts: %d)", n, totalUpdates, len(updateCounts))
		}
		return nil
	}

	// Iterate
	err := ds.IterateRange(uint32(*startBlock), uint32(*startBlock+*numBlocks-1), processBlock)
	if err != nil {
		log.Fatalf("Error iterating dataset: %v", err)
	}

	log.Infof("Analysis complete in %v", time.Since(startTime))
	log.Infof("Total unique accounts: %d", len(updateCounts))

	// Sort by count descending
	type accountStat struct {
		Address common.Address
		Count   uint64
	}
	stats := make([]accountStat, 0, len(updateCounts))
	for addr, count := range updateCounts {
		stats = append(stats, accountStat{Address: addr, Count: count})
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	// Write to CSV
	f, err := os.Create(*toolsOutputPath)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header
	if err := w.Write([]string{"Address", "UpdateCount"}); err != nil {
		log.Fatalf("Failed to write header: %v", err)
	}

	for _, stat := range stats {
		if err := w.Write([]string{stat.Address.Hex(), strconv.FormatUint(stat.Count, 10)}); err != nil {
			log.Fatalf("Failed to write record: %v", err)
		}
	}

	log.Infof("Results written to %s", *toolsOutputPath)
}
