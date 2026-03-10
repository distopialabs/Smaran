package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/logging"
)

var log = logging.GetLogger("stress")

func main() {
	numBlocks := flag.Int("n", 100, "Number of blocks to fetch")
	dataDir := flag.String("dataDir", "/data/local/dataset/modified_accounts", "Path to dataset")
	flag.Parse()
	startBlock := 18_908_895
	endBlock := startBlock + *numBlocks - 1
	r := dataset.NewDatasetReader(*dataDir, dataset.SEGMENT_SIZE)
	start := time.Now()
	for n := startBlock; n <= endBlock; n++ {
		_, err := r.GetBlock(uint32(n))
		if err != nil {
			panic(fmt.Errorf("failed to get block %d from dataset: %w", n, err))
		}
		// fmt.Println("Block", n, "with", len(entries), "accounts")
	}
	log.Infof("Time taken to fetch %d blocks: %v", *numBlocks, time.Since(start))
	r.Close()
}
