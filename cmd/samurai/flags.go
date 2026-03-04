package main

import (
	"flag"
	"path/filepath"
)

// Flags holds all command-line flags for the samurai application.
type Flags struct {
	// Mode selection
	Mode string

	// Block processing
	NumBlocks int

	// Profiling
	Profile     bool
	ProfilePath string

	// Data Directory
	DataDir string

	// Clean start (wipe existing DB and start fresh)
	Clean bool

	// Benchmark mode
	Bench             bool
	BenchDuration     int
	BenchOutputDir    string
	BenchDBMetrics    bool
	BenchPipeline     bool
	BenchCacheMetrics bool

	// Query parameters for proof/verify modes
	QueryStartBlock int
	QueryEndBlock   int
	QueryAccount    string

	// Server parameters
	ServerPort int
}

// ParseFlags parses command-line flags and returns a Flags struct.
func ParseFlags() *Flags {
	f := &Flags{}

	// Data Directory
	flag.StringVar(&f.DataDir, "datadir", "samurai-data", "Directory to store data (db, profiles, benchmarks)")

	// Mode selection
	flag.StringVar(&f.Mode, "mode", "commit", "Mode to run: commit, proof, verify, serve")
	flag.IntVar(&f.NumBlocks, "n", 10000, "Number of blocks to process")

	// Profiling flags
	flag.BoolVar(&f.Profile, "p", false, "Profile the program")
	flag.StringVar(&f.ProfilePath, "profilePath", "", "Path to write profile files (default: <datadir>/profiles)")

	// Benchmark flags
	flag.BoolVar(&f.Bench, "bench", false, "Enable benchmark mode for commit generation")
	flag.IntVar(&f.BenchDuration, "benchDuration", 300, "Benchmark duration in seconds (default: 5 minutes)")
	flag.StringVar(&f.BenchOutputDir, "benchOutputDir", "", "Directory to write benchmark CSV files (default: <datadir>/benchmarks)")
	flag.BoolVar(&f.BenchDBMetrics, "benchDBMetrics", false, "Collect Pebble DB metrics (compaction, L0 files, etc.)")
	flag.BoolVar(&f.BenchPipeline, "benchPipeline", false, "Collect pipeline sizes (queue and channel sizes per shard)")
	flag.BoolVar(&f.BenchCacheMetrics, "benchCacheMetrics", true, "Collect Ristretto cache metrics (hits, misses, size)")

	// Query flags for proof/verify modes
	flag.IntVar(&f.QueryStartBlock, "queryStartBlock", int(StartBlock)+20, "Start block for query")
	flag.IntVar(&f.QueryEndBlock, "queryEndBlock", int(StartBlock)+20-1+1000, "End block for query")
	flag.StringVar(&f.QueryAccount, "queryAccount", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account to query")

	// Server flags
	flag.IntVar(&f.ServerPort, "port", 50051, "gRPC server port")

	// Clean flag (default: false, so resume is the default behavior)
	flag.BoolVar(&f.Clean, "clean", false, "Wipe existing database and start fresh")

	flag.Parse()

	// Set defaults that depend on DataDir
	if f.ProfilePath == "" {
		f.ProfilePath = filepath.Join(f.DataDir, "profiles")
	}
	if f.BenchOutputDir == "" {
		f.BenchOutputDir = filepath.Join(f.DataDir, "benchmarks")
	}

	return f
}
