package main

import (
	"fmt"
	"log"
	_ "net/http/pprof"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/cmd/samurai/commands"
)

func main() {
	flags := ParseFlags()

	fmt.Println("Starting Samurai", time.Now())
	fmt.Println("NumCPU:", runtime.NumCPU())
	fmt.Println("Mode:", flags.Mode)

	if flags.Profile {
		defer ProfileCPU(flags.ProfilePath)()
	}

	// Build configuration from flags
	cfg := BuildConfig(flags)

	// Setup precomputed cryptographic data
	precomputedData, err := SetupPrecomputedData(cfg)
	if err != nil {
		log.Fatalf("failed to setup precomputed data: %v", err)
	}

	// Setup databases (clean on commit mode if not resuming)
	cleanOnCommit := flags.Mode == "commit" && !flags.Resume
	dbs, pebbleDbs, err := SetupDatabases(cfg, cleanOnCommit)
	if err != nil {
		log.Fatalf("failed to setup databases: %v", err)
	}

	// Setup caches
	caches, err := SetupCaches(dbs, cfg, precomputedData)
	if err != nil {
		log.Fatalf("failed to setup caches: %v", err)
	}

	// Cleanup on exit
	defer Cleanup(caches, dbs)

	// Execute mode
	switch flags.Mode {
	case "commit":
		if cfg.Benchmark.Enabled {
			commands.RunCommitBenchmark(cfg, caches, pebbleDbs)
		} else {
			commands.RunCommit(cfg, caches)
		}
	case "proof":
		addr := common.HexToAddress(flags.QueryAccount)
		startBlock := uint64(flags.QueryStartBlock)
		endBlock := uint64(flags.QueryEndBlock)
		commands.RunProof(addr, startBlock, endBlock, dbs, precomputedData, cfg)
	case "serve":
		commands.RunServe(flags.ServerPort, dbs, precomputedData, cfg)
	case "verify":
		commands.RunVerify(flags.QueryStartBlock, flags.QueryEndBlock, precomputedData.V, precomputedData.Weights, precomputedData.SRS)
	default:
		log.Fatalf("unknown mode: %s", flags.Mode)
	}
}
