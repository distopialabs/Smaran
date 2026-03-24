// Package ingest processes block data from the dataset reader
// and builds the Verkle tree with EIP-6800 basic-data encoding.
//
// Each block follows the go-verkle pattern:
//
//	Insert entries → Commit() → serialize dirty paths → persist
//
// Per-block serialization uses O(dirty) dirty-path tracking.
// Periodically, the tree is fully serialized via go-verkle's BatchSerialize()
// and reloaded from DB to cap memory usage.
package ingest

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verkle/store"
	verkle "github.com/ethereum/go-verkle"
)

// Config holds the configuration for the ingest process.
type Config struct {
	BlocksDir  string
	DBDir      string
	DBBackend  string
	Start      uint64
	End        uint64
	FlushEvery int // reload tree every N blocks for memory management (default 1000)
}

// Run performs the ingestion of blocks into the Verkle tree.
func Run(cfg Config) error {
	logger := log.New(os.Stderr, "[ingest] ", log.LstdFlags)

	if cfg.FlushEvery <= 0 {
		cfg.FlushEvery = 1000
	}

	// Open KV store
	kv, err := store.OpenKVStore(cfg.DBBackend, cfg.DBDir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer kv.Close()

	ns := store.NewNodeStore(kv)
	resolver := ns.NodeResolverFn()

	// Determine start block (resume if DB has progress)
	start := cfg.Start
	if last, ok := ns.GetLastProcessed(); ok {
		resume := last + 1
		if resume > start {
			start = resume
			logger.Printf("resuming from block %d (last processed: %d)", start, last)
		}
	}

	if start > cfg.End {
		logger.Printf("nothing to do: start %d > end %d", start, cfg.End)
		return nil
	}

	// Set start block metadata if not already set
	if _, ok := ns.GetStartBlock(); !ok {
		if err := ns.SetStartBlock(cfg.Start); err != nil {
			return fmt.Errorf("set start block: %w", err)
		}
	}

	// Load or create the Verkle tree root
	root := loadOrCreateRoot(ns, start, logger)

	// Open dataset reader
	dr := dataset.NewDatasetReader(cfg.BlocksDir, dataset.SEGMENT_SIZE)
	defer dr.Close()

	totalBlocks := 0
	totalEntries := 0
	totalDirtyNodes := 0
	startTime := time.Now()
	lastLog := time.Now()
	blocksSinceFlush := 0

	err = dr.IterateRange(uint32(start), uint32(cfg.End), func(blockNum uint32, entries []dataset.Entry) error {
		// Track modified stems for dirty-path serialization
		dirtyStems := make(map[string]struct{})

		// --- Insert all entries for this block ---
		for _, entry := range entries {
			bal := new(big.Int).SetBytes(entry.Balance)
			treeKey := keys.GetTreeKeyForBasicData(entry.Address)
			keySlice := treeKey[:]

			// Track this key's stem as dirty
			dirtyStems[string(keySlice[:31])] = struct{}{}

			if bal.Sign() == 0 {
				if _, err := root.Delete(keySlice, resolver); err != nil {
					if err.Error() != "key not found" {
						logger.Printf("block %d: delete warning for %x: %v", blockNum, entry.Address, err)
					}
				}
			} else {
				if bal.BitLen() > 128 {
					return fmt.Errorf("block %d: balance overflow for address %x: %d bits",
						blockNum, entry.Address, bal.BitLen())
				}
				val, err := keys.PackBasicData(bal)
				if err != nil {
					return fmt.Errorf("block %d: pack basic data: %w", blockNum, err)
				}
				if err := root.Insert(keySlice, val[:], resolver); err != nil {
					return fmt.Errorf("block %d: insert key %x: %w", blockNum, keySlice, err)
				}
			}
			totalEntries++
		}

		// --- Commit: recompute Pedersen commitments (COW + parallel) ---
		root.Commit()

		// --- Serialize only dirty paths: O(k * depth) ---
		iroot, ok := root.(*verkle.InternalNode)
		if !ok {
			return fmt.Errorf("block %d: root is not InternalNode", blockNum)
		}

		dirtyNodes, rootCommitment, err := serializeDirtyPaths(iroot, dirtyStems)
		if err != nil {
			return fmt.Errorf("block %d: serialize dirty paths: %w", blockNum, err)
		}
		totalDirtyNodes += len(dirtyNodes)

		// --- Persist dirty nodes + metadata atomically ---
		if err := ns.SaveBlockState(dirtyNodes, uint64(blockNum), rootCommitment); err != nil {
			return fmt.Errorf("block %d: save block state: %w", blockNum, err)
		}

		totalBlocks++
		blocksSinceFlush++

		// --- Periodic: full serialize + sync + reload for memory management ---
		if blocksSinceFlush >= cfg.FlushEvery {
			// Full serialize via go-verkle's BatchSerialize to ensure
			// all in-memory nodes are persisted before reload
			allNodes, err := iroot.BatchSerialize()
			if err != nil {
				return fmt.Errorf("block %d: BatchSerialize: %w", blockNum, err)
			}
			if err := ns.SaveNodes(allNodes, uint64(blockNum)); err != nil {
				return fmt.Errorf("block %d: save all nodes: %w", blockNum, err)
			}
			if err := kv.Sync(); err != nil {
				return fmt.Errorf("sync: %w", err)
			}

			// Reload: all children become HashedNode stubs, capping memory
			newRoot, err := ns.LoadTree(uint64(blockNum))
			if err != nil {
				return fmt.Errorf("block %d: reload tree: %w", blockNum, err)
			}
			root = newRoot
			blocksSinceFlush = 0
		}

		if time.Since(lastLog) > 5*time.Second {
			elapsed := time.Since(startTime)
			bps := float64(totalBlocks) / elapsed.Seconds()
			logger.Printf("block %d | %d blocks (%.1f blk/s) | %d entries | %d dirty nodes written | elapsed %s",
				blockNum, totalBlocks, bps, totalEntries, totalDirtyNodes, elapsed.Round(time.Second))
			lastLog = time.Now()
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Final sync
	if err := kv.Sync(); err != nil {
		return fmt.Errorf("final sync: %w", err)
	}

	elapsed := time.Since(startTime)
	logger.Printf("done: %d blocks, %d entries, %d dirty nodes in %s",
		totalBlocks, totalEntries, totalDirtyNodes, elapsed.Round(time.Second))
	return nil
}

// loadOrCreateRoot loads the persisted Verkle tree for the block just before
// `start`, or creates a new empty tree if no prior state exists.
func loadOrCreateRoot(ns *store.NodeStore, start uint64, logger *log.Logger) verkle.VerkleNode {
	if start > 0 {
		prevBlock := start - 1
		if _, err := ns.GetRootCommitment(prevBlock); err == nil {
			root, err := ns.LoadTree(prevBlock)
			if err == nil {
				logger.Printf("loaded tree from block %d", prevBlock)
				return root
			}
			logger.Printf("failed to load tree for block %d: %v", prevBlock, err)
		}
	}
	logger.Printf("creating new empty Verkle tree")
	return verkle.New()
}
