package main

import (
	"flag"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/fetcher"
)

func fetchProofs() {
	alchemyURL := flag.String("alchemy", "https://eth-mainnet.g.alchemy.com/v2/3PBdxpi1SAyIg6TbJRf_glqpoWVkO3YT", "Alchemy HTTPS JSON-RPC URL")
	addrFlag := flag.String("addr", "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", "Account address to fetch proofs for")
	startBlock := flag.Uint64("start", 18908895, "Start block (inclusive)")
	endBlock := flag.Uint64("end", 19108895, "End block (inclusive)")
	outDir := flag.String("out", "/mydata/samurai/exp1/", "Directory to store proofs")
	rps := flag.Int("rps", 20, "Requests per second throttle for Alchemy (<=25)")

	flag.Parse()

	if err := fetcher.RunProofFetcher(*alchemyURL, common.HexToAddress(*addrFlag), *startBlock, *endBlock, *outDir, *rps); err != nil {
		log.Fatalf("proof fetcher failed: %v", err)
	}

}
