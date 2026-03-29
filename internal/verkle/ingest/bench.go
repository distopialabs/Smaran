package ingest

import (
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"time"

	verkle "github.com/ethereum/go-verkle"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verkle/store"
)

var errTimeLimitReached = errors.New("time limit reached")

// BenchConfig holds configuration for the ingestion benchmark.
type BenchConfig struct {
	BlocksDir         string
	DBDir             string
	DBBackend         string
	Start             uint64
	End               uint64
	FlushEvery        int
	Duration          time.Duration
	KUsers            int
	AccountsList      string
	OutCSV            string
	UpdateMetricsPath string
}

// RunBench runs block ingestion for a fixed duration, logging per-block
// timestamps to a CSV file. After the deadline, the current in-flight block
// completes before stopping.
//
// CSV columns: block_num,num_raw_updates,num_selected_updates,queued_at_ns,start_at_ns,completed_at_ns
func RunBench(cfg BenchConfig) error {
	logger := log.New(os.Stderr, "[bench-ingest] ", log.LstdFlags)

	if cfg.FlushEvery <= 0 {
		cfg.FlushEvery = 1000
	}

	// Load hot-account filter if configured.
	var filter *benchutil.HotAccountFilter
	if cfg.KUsers > 0 {
		logger.Printf("loading top %d hot accounts from %s", cfg.KUsers, cfg.AccountsList)
		var err error
		filter, err = benchutil.LoadHotAccountFilter(cfg.AccountsList, cfg.KUsers)
		if err != nil {
			return fmt.Errorf("load hot account filter: %w", err)
		}
		logger.Printf("loaded %d hot accounts", filter.Size())
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

	csvWriter, err := benchutil.NewBenchCSVWriter(cfg.OutCSV, benchutil.IngestionCSVHeader)
	if err != nil {
		return err
	}
	defer csvWriter.Close()

	// Setup update-level metrics collector.
	var updateMetrics *benchutil.UpdateMetricsCollector
	if cfg.UpdateMetricsPath != "" {
		var umErr error
		updateMetrics, umErr = benchutil.NewUpdateMetricsCollector(cfg.UpdateMetricsPath, time.Second)
		if umErr != nil {
			return fmt.Errorf("create update metrics collector: %w", umErr)
		}
		go updateMetrics.Run()
		defer updateMetrics.Stop()
	}

	deadline := time.Now().Add(cfg.Duration)
	totalBlocks := 0
	blocksSinceFlush := 0
	benchStart := time.Now()

	logger.Printf("starting benchmark: duration=%s, start_block=%d, end_block=%d, flush_every=%d, output=%s",
		cfg.Duration, start, cfg.End, cfg.FlushEvery, cfg.OutCSV)

	err = dr.IterateRange(uint32(start), uint32(cfg.End), func(blockNum uint32, entries []dataset.Entry) error {
		rawCount := uint64(len(entries))

		// Filter entries if hot-account filter is configured.
		selected := entries
		if filter != nil {
			filtered := make([]dataset.Entry, 0, len(entries))
			for i := range entries {
				if filter.ContainsBytes(entries[i].Address) {
					filtered = append(filtered, entries[i])
				}
			}
			selected = filtered
		}
		selectedCount := uint64(len(selected))

		startAtNs := time.Now().UnixNano()

		dirtyStems := make(map[string]struct{})

		for _, entry := range selected {
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

		// fmt.Printf("root commitment: %x\n", root.Commitment())

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
		}

		completedAtNs := time.Now().UnixNano()

		if updateMetrics != nil && selectedCount > 0 {
			updateMetrics.RecordN(selectedCount, completedAtNs-startAtNs)
		}

		// queued_at_ns = start_at_ns for sequential pipeline
		_ = csvWriter.WriteRow(
			strconv.FormatUint(uint64(blockNum), 10),
			strconv.FormatUint(rawCount, 10),
			strconv.FormatUint(selectedCount, 10),
			strconv.FormatInt(startAtNs, 10),
			strconv.FormatInt(startAtNs, 10),
			strconv.FormatInt(completedAtNs, 10),
		)

		if totalBlocks%500 == 0 {
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
	logger.Printf("done: %d blocks in %s (%.1f blk/s) → %s",
		totalBlocks, elapsed.Round(time.Second),
		float64(totalBlocks)/elapsed.Seconds(),
		cfg.OutCSV)

	return nil
}
