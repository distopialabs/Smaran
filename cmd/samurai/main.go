package main

import (
	_ "net/http/pprof"
	"os"
	"runtime"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/cmd/samurai/commands"
	"github.com/nepal80m/samurai/internal/logging"
)

func main() {
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		if err := logging.SetLevel(lvl); err != nil {
			log.Fatalf("Invalid LOG_LEVEL %q: %v", lvl, err)
		}
	}

	flags := ParseFlags()

	log.Infof("Starting Samurai %v", time.Now())
	log.Infof("NumCPU: %d", runtime.NumCPU())
	log.Infof("Mode: %s", flags.Mode)

	if flags.Profile {
		defer ProfileCPU(flags.ProfilePath)()
	}

	// Build configuration from flags
	cfg := BuildConfig(flags)

	// Setup precomputed cryptographic data
	precomputedData, err := SetupPrecomputedData(cfg)
	if err != nil {
		log.Fatalf("Failed to setup precomputed data: %v", err)
	}

	// Setup databases (clean only if explicitly requested with --clean)
	cleanOnCommit := flags.Mode == "commit" && cfg.Clean
	dbs, pebbleDbs, err := SetupDatabases(cfg, cleanOnCommit)
	if err != nil {
		log.Fatalf("Failed to setup databases: %v", err)
	}

	// Setup caches
	caches, err := SetupCaches(dbs, cfg, precomputedData)
	if err != nil {
		log.Fatalf("Failed to setup caches: %v", err)
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
		log.Fatalf("Unknown mode: %s", flags.Mode)
	}
}
