package config

import (
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
)

// Config holds all configuration for the Samurai application.
type Config struct {
	Resume          bool
	BlocksDataDir   string
	CryptoParamsDir string
	Blocks          Blocks
	Workers   Workers
	Database  Database
	Cache     Cache
	Queue     Queue
	Benchmark Benchmark
}

// Benchmark holds configuration for benchmark mode.
type Benchmark struct {
	Enabled              bool
	DurationSecs         int    // How long to run the benchmark (seconds)
	OutputDir            string // Directory to write benchmark CSV files
	CollectDBMetrics     bool   // Collect Pebble DB metrics (compaction, L0 files, etc.)
	CollectPipelineSizes bool   // Collect queue and channel sizes per shard
	CollectCacheMetrics  bool   // Collect Ristretto cache metrics (hits, misses, size)
}

// Blocks specifies the block range to process.
type Blocks struct {
	StartingBlockNumber uint64
	EndingBlockNumber   uint64
}

// Workers configures the worker pool for commit generation.
type Workers struct {
	CommitWorkerCount       int
	CommitWorkerQueueSize   int
	CommitWorkerChannelSize int
}

// Database configures the Pebble database settings.
type Database struct {
	Shards       int
	MemTableSize uint64
	DisableWAL   bool
	CacheSize    uint64
	StoragePath  string
}

// Cache configures the Ristretto in-memory cache.
type Cache struct {
	NumCounters   uint64
	MaxCost       uint64
	EnableMetrics bool // Enable Ristretto metrics collection (has some overhead)
}

// Queue configures channel buffer sizes for the processing pipeline.
type Queue struct {
	BlockInfoChannelSize  int
	UpdateTaskChannelSize int
}

// PrecomputedData holds precomputed cryptographic data for polynomial commitments.
type PrecomputedData struct {
	V             polynomial.Polynomial
	Weights       []fr.Element
	WeightCommits []bls.G1Affine
	SRS           *kzg.MultiSRS
}
