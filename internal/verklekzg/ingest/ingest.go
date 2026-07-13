// Package ingest processes block data from the dataset reader and builds
// the Verkle-KZG trie with EIP-6800 basic-data encoding.
//
// Each block follows: Insert entries -> Commit() -> serialize dirty paths -> persist.
// Periodically the tree is fully serialized and reloaded to cap memory.
package ingest

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verklekzg/store"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
)

// Config holds the configuration for the ingest process.
type Config struct {
	BlocksDir  string
	DBDir      string
	DBBackend  string
	ParamsDir  string // directory for precomputed SRS / barycentric files
	Start      uint64
	End        uint64
	FlushEvery int
}

// Run performs the ingestion of blocks into the Verkle-KZG trie.
func Run(cfg Config) error {
	logger := log.New(os.Stderr, "[verklekzg-ingest] ", log.LstdFlags)

	if cfg.FlushEvery <= 0 {
		cfg.FlushEvery = 1000
	}

	treeCfg, err := tree.NewTreeConfig(cfg.ParamsDir)
	if err != nil {
		return fmt.Errorf("init tree config: %w", err)
	}

	kv, err := store.OpenKVStore(cfg.DBBackend, cfg.DBDir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer kv.Close()

	ns := store.NewNodeStore(kv)
	resolver := ns.NodeResolverFn()

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

	if _, ok := ns.GetStartBlock(); !ok {
		if err := ns.SetStartBlock(cfg.Start); err != nil {
			return fmt.Errorf("set start block: %w", err)
		}
	}

	root := loadOrCreateRoot(ns, start, logger)

	dr := dataset.NewDatasetReader(cfg.BlocksDir, dataset.SEGMENT_SIZE)
	defer dr.Close()

	totalBlocks := 0
	totalEntries := 0
	totalDirtyNodes := 0
	startTime := time.Now()
	lastLog := time.Now()
	blocksSinceFlush := 0

	err = dr.IterateRange(uint32(start), uint32(cfg.End), func(blockNum uint32, entries []dataset.Entry) error {
		dirtyStems := make(map[string]struct{})

		for _, entry := range entries {
			bal := new(big.Int).SetBytes(entry.Balance)
			treeKey := keys.GetTreeKeyForBasicData(entry.Address)
			keySlice := treeKey[:]

			dirtyStems[string(keySlice[:31])] = struct{}{}

			if bal.Sign() == 0 {
				if _, err := root.Delete(keySlice, resolver); err != nil {
					// Ignore "key not found" on delete.
				}
			} else {
				if bal.BitLen() > 128 {
					return fmt.Errorf("block %d: balance overflow for %x: %d bits",
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

		root.Commit(treeCfg)

		dirtyNodes, rootCommitment, err := serializeDirtyPaths(root, dirtyStems)
		if err != nil {
			return fmt.Errorf("block %d: serialize dirty paths: %w", blockNum, err)
		}
		totalDirtyNodes += len(dirtyNodes)

		if err := ns.SaveBlockState(dirtyNodes, uint64(blockNum), rootCommitment); err != nil {
			return fmt.Errorf("block %d: save block state: %w", blockNum, err)
		}

		totalBlocks++
		blocksSinceFlush++

		if blocksSinceFlush >= cfg.FlushEvery {
			allNodes, err := root.BatchSerialize()
			if err != nil {
				return fmt.Errorf("block %d: BatchSerialize: %w", blockNum, err)
			}
			if err := ns.SaveNodes(allNodes, uint64(blockNum)); err != nil {
				return fmt.Errorf("block %d: save all nodes: %w", blockNum, err)
			}
			if err := kv.Sync(); err != nil {
				return fmt.Errorf("sync: %w", err)
			}

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
			logger.Printf("block %d | %d blocks (%.1f blk/s) | %d entries | %d dirty nodes | elapsed %s",
				blockNum, totalBlocks, bps, totalEntries, totalDirtyNodes, elapsed.Round(time.Second))
			lastLog = time.Now()
		}
		return nil
	})

	if err != nil {
		return err
	}

	if err := kv.Sync(); err != nil {
		return fmt.Errorf("final sync: %w", err)
	}

	elapsed := time.Since(startTime)
	logger.Printf("done: %d blocks, %d entries, %d dirty nodes in %s",
		totalBlocks, totalEntries, totalDirtyNodes, elapsed.Round(time.Second))
	return nil
}

func loadOrCreateRoot(ns *store.NodeStore, start uint64, logger *log.Logger) *tree.InternalNode {
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
	logger.Printf("creating new empty Verkle-KZG trie")
	return tree.New()
}
