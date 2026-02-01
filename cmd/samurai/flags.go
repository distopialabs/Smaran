package main

import "flag"

// Flags holds all command-line flags for the samurai application.
type Flags struct {
	// Mode selection
	Mode string

	// Block processing
	NumBlocks int

	// Profiling
	Profile     bool
	ProfilePath string

	// Resume
	Resume bool

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

	// Mode selection
	flag.StringVar(&f.Mode, "mode", "commit", "Mode to run: commit, proof, verify")
	flag.IntVar(&f.NumBlocks, "n", 10000, "Number of blocks to process")

	// Profiling flags
	flag.BoolVar(&f.Profile, "p", true, "Profile the program")
	flag.StringVar(&f.ProfilePath, "profilePath", "/data/local/samurai/test/profiles", "Path to write profile files")

	// Benchmark flags
	flag.BoolVar(&f.Bench, "bench", false, "Enable benchmark mode for commit generation")
	flag.IntVar(&f.BenchDuration, "benchDuration", 300, "Benchmark duration in seconds (default: 5 minutes)")
	flag.StringVar(&f.BenchOutputDir, "benchOutputDir", "/data/local/samurai/test/benchmark", "Directory to write benchmark CSV files")
	flag.BoolVar(&f.BenchDBMetrics, "benchDBMetrics", false, "Collect Pebble DB metrics (compaction, L0 files, etc.)")
	flag.BoolVar(&f.BenchPipeline, "benchPipeline", false, "Collect pipeline sizes (queue and channel sizes per shard)")
	flag.BoolVar(&f.BenchCacheMetrics, "benchCacheMetrics", true, "Collect Ristretto cache metrics (hits, misses, size)")

	// Query flags for proof/verify modes
	flag.IntVar(&f.QueryStartBlock, "queryStartBlock", int(StartBlock)+20, "Start block for query")
	flag.IntVar(&f.QueryEndBlock, "queryEndBlock", int(StartBlock)+20-1+100, "End block for query")
	flag.StringVar(&f.QueryAccount, "queryAccount", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account to query")

	// Server flags
	flag.IntVar(&f.ServerPort, "port", 50051, "gRPC server port")

	// Resume flag
	flag.BoolVar(&f.Resume, "resume", false, "Resume from existing database")

	flag.Parse()
	return f
}
