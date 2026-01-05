package config

import (
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
)

type Config struct {
	Blocks    Blocks
	Workers   Workers
	Database  Database
	Queue     Queue
	Benchmark Benchmark
	// PrecomputedData PrecomputedData
}

type Benchmark struct {
	Enabled      bool
	DurationSecs int    // How long to run the benchmark (seconds)
	OutputDir    string // Directory to write benchmark CSV files
}

type Blocks struct {
	StartingBlockNumber uint64
	EndingBlockNumber   uint64
}

type Workers struct {
	CommitWorkerCount       int
	CommitWorkerQueueSize   int
	CommitWorkerChannelSize int
}

type Database struct {
	Shards       int
	MemTableSize uint64
	DisableWAL   bool
	CacheSize    uint64
	StoragePath  string
}

type Queue struct {
	BlockInfoChannelSize  int
	UpdateTaskChannelSize int
}

type PrecomputedData struct {
	V             polynomial.Polynomial
	Weights       []fr.Element
	WeightCommits []bls.G1Affine
	SRS           *kzg.MultiSRS
}

// func (config *Config) SetTrackedAccounts(count int) []common.Address {
// 	client := config.Client

// 	accountAddrs := make([]common.Address, 0, count)
// 	startKey := []byte{}
// 	for {
// 		var iteratorDump struct {
// 			Root     string                 `json:"root"`
// 			Accounts map[common.Address]any `json:"accounts"`
// 			Next     []byte                 `json:"next"`
// 		}
// 		blockNumber := config.StartingBlockNumber
// 		const batchSize = 256
// 		if err := client.Call(
// 			&iteratorDump,
// 			"debug_accountRange",
// 			blockNumber, // numeric block tag
// 			startKey,    // starting key for pagination
// 			batchSize,   // how many accounts to fetch per page
// 			true,        // exclude code info in account?
// 			true,        // exclude storage info in account?
// 		); err != nil {
// 			log.Fatalf("RPC error calling debug_accountRange: %v", err)
// 		}

// 		for addr := range iteratorDump.Accounts {
// 			if len(accountAddrs) >= count {
// 				break
// 			}
// 			accountAddrs = append(accountAddrs, addr)
// 		}
// 		if len(accountAddrs) >= count || len(iteratorDump.Next) == 0 {
// 			break
// 		}
// 		startKey = iteratorDump.Next
// 	}
// 	config.TrackedAccounts = accountAddrs
// 	return accountAddrs

// }
