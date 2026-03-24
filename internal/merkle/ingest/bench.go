package ingest

import (
	"errors"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// BenchConfig holds configuration for the ingestion benchmark.
type BenchConfig struct {
	BlocksDir         string
	Store             *st.MPTStateStore
	Start             uint64
	Duration          time.Duration
	KUsers            int
	AccountsList      string
	OutCSV            string
	UpdateMetricsPath string
}

var errDurationExceeded = errors.New("bench: duration exceeded")

// BenchRun performs block ingestion for a fixed duration and writes per-block
// timing data to a CSV.
// CSV columns: block_num,num_raw_updates,num_selected_updates,queued_at_ns,start_at_ns,completed_at_ns
func BenchRun(cfg BenchConfig) error {
	reader := dataset.NewDatasetReader(cfg.BlocksDir, dataset.SEGMENT_SIZE)
	defer reader.Close()

	// Load hot-account filter if configured.
	var filter *benchutil.HotAccountFilter
	if cfg.KUsers > 0 {
		log.Printf("[bench] loading top %d hot accounts from %s", cfg.KUsers, cfg.AccountsList)
		var err error
		filter, err = benchutil.LoadHotAccountFilter(cfg.AccountsList, cfg.KUsers)
		if err != nil {
			return fmt.Errorf("load hot account filter: %w", err)
		}
		log.Printf("[bench] loaded %d hot accounts", filter.Size())
	}

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

	start := cfg.Start
	if meta.HasLast(cfg.Store.DiskDB) {
		last, err := meta.GetLast(cfg.Store.DiskDB)
		if err != nil {
			return fmt.Errorf("read meta:last: %w", err)
		}
		resume := last + 1
		if resume > start {
			start = resume
			log.Printf("Resuming from block %d (meta:last was %d)", start, last)
		}
	} else {
		if err := meta.PutStart(cfg.Store.DiskDB, cfg.Start); err != nil {
			return fmt.Errorf("write meta:start: %w", err)
		}
	}

	var currentRoot common.Hash
	if start > cfg.Start {
		root, err := meta.GetRoot(cfg.Store.DiskDB, start-1)
		if err != nil {
			return fmt.Errorf("load root for block %d: %w", start-1, err)
		}
		currentRoot = root
	}
	if currentRoot == (common.Hash{}) {
		currentRoot = st.EmptyRootHash
	}

	end := uint64(^uint32(0)) // iterate as far as data allows; duration is the real limit

	const flushInterval = 1024

	log.Printf("Bench-ingest: starting at block %d, duration %s (output: %s)", start, cfg.Duration, cfg.OutCSV)

	batch := cfg.Store.DiskDB.NewBatch()
	blocksProcessed := uint64(0)
	benchStart := time.Now()
	deadline := benchStart.Add(cfg.Duration)

	err = reader.IterateRange(uint32(start), uint32(end), func(blockNum uint32, entries []dataset.Entry) error {
		if time.Now().After(deadline) {
			return errDurationExceeded
		}

		blk := uint64(blockNum)
		rawCount := uint64(len(entries))

		// Filter entries if hot-account filter is configured.
		selected := entries
		if filter != nil {
			filtered := make([]dataset.Entry, 0, len(entries))
			for i := range entries {
				addr := common.BytesToAddress(entries[i].Address[:])
				if filter.Contains(addr) {
					filtered = append(filtered, entries[i])
				}
			}
			selected = filtered
		}
		selectedCount := uint64(len(selected))

		startAtNs := time.Now().UnixNano()

		stateDB, err := cfg.Store.OpenState(currentRoot)
		if err != nil {
			return fmt.Errorf("block %d: open state: %w", blk, err)
		}

		for _, e := range selected {
			addr := common.BytesToAddress(e.Address[:])
			bal := new(big.Int).SetBytes(e.Balance)
			st.SetAccountBalance(stateDB, addr, bal)
		}

		newRoot, err := cfg.Store.CommitState(stateDB, currentRoot, blk)
		if err != nil {
			return fmt.Errorf("block %d: commit: %w", blk, err)
		}

		meta.PutRootBatch(batch, blk, newRoot)
		meta.PutLastBatch(batch, blk)

		if err := cfg.Store.FlushTrieDB(newRoot); err != nil {
			return fmt.Errorf("block %d: triedb flush: %w", blk, err)
		}

		if blocksProcessed%flushInterval == 0 && blocksProcessed > 0 {
			if err := batch.Write(); err != nil {
				return fmt.Errorf("block %d: batch write: %w", blk, err)
			}
			batch.Reset()
		}

		completedAtNs := time.Now().UnixNano()

		if updateMetrics != nil && selectedCount > 0 {
			updateMetrics.RecordN(selectedCount, completedAtNs-startAtNs)
		}

		// queued_at_ns = start_at_ns for sequential pipeline
		_ = csvWriter.WriteRow(
			strconv.FormatUint(blk, 10),
			strconv.FormatUint(rawCount, 10),
			strconv.FormatUint(selectedCount, 10),
			strconv.FormatInt(startAtNs, 10),
			strconv.FormatInt(startAtNs, 10),
			strconv.FormatInt(completedAtNs, 10),
		)

		currentRoot = newRoot
		blocksProcessed++

		if blocksProcessed%5000 == 0 {
			elapsed := time.Since(benchStart)
			remaining := cfg.Duration - elapsed
			log.Printf("  block %d  (%d blocks, %.1f blk/s, %s remaining)",
				blk, blocksProcessed, float64(blocksProcessed)/elapsed.Seconds(),
				remaining.Round(time.Second))
		}
		return nil
	})

	if err != nil && !errors.Is(err, errDurationExceeded) {
		return err
	}

	if blocksProcessed > 0 {
		if err := batch.Write(); err != nil {
			return fmt.Errorf("final batch write: %w", err)
		}
		if err := cfg.Store.FlushTrieDB(currentRoot); err != nil {
			return fmt.Errorf("final triedb flush: %w", err)
		}
	}

	elapsed := time.Since(benchStart)
	log.Printf("Bench-ingest complete: %d blocks in %s (%.1f blk/s), CSV: %s",
		blocksProcessed, elapsed.Round(time.Millisecond),
		float64(blocksProcessed)/elapsed.Seconds(), cfg.OutCSV)

	return nil
}
