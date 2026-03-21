package ingest

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verkle/store"
	verkle "github.com/ethereum/go-verkle"
)

var errTimeLimitReached = errors.New("time limit reached")

// BenchConfig holds configuration for the ingestion benchmark.
type BenchConfig struct {
	BlocksDir  string
	DBDir      string
	DBBackend  string
	Start      uint64
	End        uint64
	StartSet   bool
	FlushEvery int
	Duration   time.Duration
	OutputFile string
}

// RunBench runs block ingestion for a fixed duration, logging per-block
// timestamps to a CSV file. After the deadline, the current in-flight block
// completes before stopping.
//
// CSV columns: block_number, entries, dirty_nodes, start_ns, end_ns, flush
//   - start_ns/end_ns: wall-clock nanosecond timestamps (time.Now().UnixNano())
//   - flush: 1 if a periodic full-tree serialize+reload occurred on this block
func RunBench(cfg BenchConfig) error {
	logger := log.New(os.Stderr, "[bench-ingest] ", log.LstdFlags)

	if cfg.FlushEvery <= 0 {
		cfg.FlushEvery = 1000
	}

	kv, err := store.OpenKVStore(cfg.DBBackend, cfg.DBDir)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer kv.Close()

	ns := store.NewNodeStore(kv)
	resolver := ns.NodeResolverFn()

	start := cfg.Start
	if !cfg.StartSet {
		if last, ok := ns.GetLastProcessed(); ok {
			start = last + 1
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

	outFile, err := os.Create(cfg.OutputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer outFile.Close()

	csvW := csv.NewWriter(outFile)
	defer csvW.Flush()

	if err := csvW.Write([]string{"block_number", "entries", "dirty_nodes", "start_ns", "end_ns", "flush"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	deadline := time.Now().Add(cfg.Duration)
	totalBlocks := 0
	blocksSinceFlush := 0
	benchStart := time.Now()

	logger.Printf("starting benchmark: duration=%s, start_block=%d, end_block=%d, flush_every=%d",
		cfg.Duration, start, cfg.End, cfg.FlushEvery)

	err = dr.IterateRange(uint32(start), uint32(cfg.End), func(blockNum uint32, entries []dataset.Entry) error {
		blockStart := time.Now().UnixNano()

		dirtyStems := make(map[string]struct{})

		for _, entry := range entries {
			bal := new(big.Int).SetBytes(entry.Balance)
			treeKey := keys.GetTreeKeyForBasicData(entry.Address)
			keySlice := treeKey[:]

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
		}

		root.Commit()

		iroot, ok := root.(*verkle.InternalNode)
		if !ok {
			return fmt.Errorf("block %d: root is not InternalNode", blockNum)
		}

		dirtyNodes, rootCommitment, err := serializeDirtyPaths(iroot, dirtyStems)
		if err != nil {
			return fmt.Errorf("block %d: serialize dirty paths: %w", blockNum, err)
		}

		if err := ns.SaveBlockState(dirtyNodes, uint64(blockNum), rootCommitment); err != nil {
			return fmt.Errorf("block %d: save block state: %w", blockNum, err)
		}

		totalBlocks++
		blocksSinceFlush++

		flushed := 0
		if blocksSinceFlush >= cfg.FlushEvery {
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

			newRoot, err := ns.LoadTree(uint64(blockNum))
			if err != nil {
				return fmt.Errorf("block %d: reload tree: %w", blockNum, err)
			}
			root = newRoot
			blocksSinceFlush = 0
			flushed = 1
		}

		blockEnd := time.Now().UnixNano()

		if err := csvW.Write([]string{
			strconv.FormatUint(uint64(blockNum), 10),
			strconv.Itoa(len(entries)),
			strconv.Itoa(len(dirtyNodes)),
			strconv.FormatInt(blockStart, 10),
			strconv.FormatInt(blockEnd, 10),
			strconv.Itoa(flushed),
		}); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}

		if totalBlocks%500 == 0 {
			csvW.Flush()
			elapsed := time.Since(benchStart)
			logger.Printf("block %d | %d blocks (%.1f blk/s) | elapsed %s | remaining %s",
				blockNum, totalBlocks,
				float64(totalBlocks)/elapsed.Seconds(),
				elapsed.Round(time.Second),
				time.Until(deadline).Round(time.Second))
		}

		if time.Now().After(deadline) {
			return errTimeLimitReached
		}

		return nil
	})

	if err != nil && !errors.Is(err, errTimeLimitReached) {
		return err
	}

	if err := kv.Sync(); err != nil {
		return fmt.Errorf("final sync: %w", err)
	}

	elapsed := time.Since(benchStart)
	if errors.Is(err, errTimeLimitReached) {
		logger.Printf("time limit reached after %s", elapsed.Round(time.Second))
	}
	logger.Printf("done: %d blocks in %s (%.1f blk/s) → %s",
		totalBlocks, elapsed.Round(time.Second),
		float64(totalBlocks)/elapsed.Seconds(),
		cfg.OutputFile)

	return nil
}
