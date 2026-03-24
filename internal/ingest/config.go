package ingest

import (
	"time"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/storage"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// type Config struct {
// 	BlocksDir string
// 	Store     *st.MPTStateStore
// 	Start     uint64
// 	End       uint64 // 0 means "until no more data"
// }

// Config holds all configuration for the Samurai application.
type Config struct {
	Shards int
	Blocks BlocksConfig

	Workers WorkersConfig
	// Database  DatabaseConfig
	// Cache     CacheConfig
	Queue QueueConfig

	Caches        []*storage.Cache
	SamuraiStores []*db.SamuraiStore
	MPTStore      *st.MPTStateStore

	// Bench is non-nil only during benchmark runs. Pipeline functions check
	// this for nil before recording any metrics, so the normal path pays only
	// a pointer comparison.
	Bench *BenchContext
}

// BenchContext carries all benchmark-specific runtime state. Stored as a
// pointer in Config so that a nil check gates all instrumentation.
type BenchContext struct {
	Filter        *benchutil.HotAccountFilter
	CSV           *benchutil.BenchCSVWriter
	Deadline      time.Time
	UpdateMetrics *benchutil.UpdateMetricsCollector
}

// BlocksConfig specifies the block range to process.
type BlocksConfig struct {
	DataDir string
	Start   uint64
	End     uint64
}

// WorkersConfig configures the worker pool for commit generation.
type WorkersConfig struct {
	CommitWorkerCount       int
	CommitWorkerQueueSize   int
	CommitWorkerChannelSize int
}

// DatabaseConfig configures the Pebble database settings.
// type DBConfig struct {
// 	MemTableSize uint64
// 	CacheSize    uint64
// }

// type StoreConfig struct {
// 	Shards    int
// 	Default   DBConfig
// 	StateDB   DBConfig
// 	TreeDB    DBConfig
// 	HistoryDB DBConfig
// 	Dir       string
// }

// CacheConfig configures the LRU in-memory cache.
type CacheConfig struct {
	Size          int
	EnableMetrics bool
}

// QueueConfig configures channel buffer sizes for the processing pipeline.
type QueueConfig struct {
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
