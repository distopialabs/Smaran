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

	"github.com/ethereum/go-ethereum/common"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// BenchConfig holds configuration for the ingestion benchmark.
type BenchConfig struct {
	BlocksDir string
	Store     *st.MPTStateStore
	Start     uint64
	Duration  time.Duration // run for this long, then stop after current block completes
	OutCSV    string        // path to write per-block CSV results
}

var errDurationExceeded = errors.New("bench: duration exceeded")

// BenchRun performs block ingestion for a fixed duration and writes per-block
// timing data to a CSV.
// Each row: block_id, num_entries, start_ns, end_ns
// where start_ns/end_ns are Unix nanosecond timestamps.
// After the duration elapses, the current in-progress block is allowed to
// complete and gets logged before the benchmark stops.
func BenchRun(cfg BenchConfig) error {
	reader := dataset.NewDatasetReader(cfg.BlocksDir, dataset.SEGMENT_SIZE)
	defer reader.Close()

	csvFile, err := os.Create(cfg.OutCSV)
	if err != nil {
		return fmt.Errorf("create CSV %s: %w", cfg.OutCSV, err)
	}
	defer csvFile.Close()

	w := csv.NewWriter(csvFile)
	defer w.Flush()

	if err := w.Write([]string{"block_id", "num_entries", "start_ns", "end_ns"}); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
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
		// Check deadline *before* starting the next block.
		// The previous block already completed and was logged.
		if time.Now().After(deadline) {
			return errDurationExceeded
		}

		blk := uint64(blockNum)

		t0 := time.Now().UnixNano()

		stateDB, err := cfg.Store.OpenState(currentRoot)
		if err != nil {
			return fmt.Errorf("block %d: open state: %w", blk, err)
		}

		for _, e := range entries {
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

		t1 := time.Now().UnixNano()

		if err := w.Write([]string{
			strconv.FormatUint(blk, 10),
			strconv.Itoa(len(entries)),
			strconv.FormatInt(t0, 10),
			strconv.FormatInt(t1, 10),
		}); err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}

		currentRoot = newRoot
		blocksProcessed++

		if blocksProcessed%5000 == 0 {
			w.Flush()
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

	w.Flush()

	elapsed := time.Since(benchStart)
	log.Printf("Bench-ingest complete: %d blocks in %s (%.1f blk/s), CSV: %s",
		blocksProcessed, elapsed.Round(time.Millisecond),
		float64(blocksProcessed)/elapsed.Seconds(), cfg.OutCSV)

	return nil
}
