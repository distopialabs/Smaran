package ingest

import (
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// Config holds ingestion configuration.
type Config struct {
	BlocksDir string
	Store     *st.MPTStateStore
	Start     uint64
	End       uint64 // 0 means "until no more data"
}

// Run performs block ingestion.
func Run(cfg Config) error {
	reader := dataset.NewDatasetReader(cfg.BlocksDir, dataset.SEGMENT_SIZE)
	defer reader.Close()

	// Determine effective start block.
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
		// First run: store start block for sanity.
		if err := meta.PutStart(cfg.Store.DiskDB, cfg.Start); err != nil {
			return fmt.Errorf("write meta:start: %w", err)
		}
	}

	// Determine current root.
	var currentRoot common.Hash
	if start > cfg.Start {
		root, err := meta.GetRoot(cfg.Store.DiskDB, start-1)
		if err != nil {
			return fmt.Errorf("load root for block %d: %w", start-1, err)
		}
		currentRoot = root
	}
	// else: empty trie root (zero hash means empty for go-ethereum state).
	// Actually go-ethereum needs the types.EmptyRootHash for an empty state.
	if currentRoot == (common.Hash{}) {
		currentRoot = st.EmptyRootHash
	}

	end := cfg.End
	if end == 0 {
		end = uint64(^uint32(0)) // max uint32
	}
	if end < start {
		return fmt.Errorf("end block %d < start block %d", end, start)
	}

	startTime := time.Now()
	blocksProcessed := uint64(0)
	totalAccounts := uint64(0)

	const flushInterval = 1024

	log.Printf("Ingesting blocks %d .. %d into trie (root=%s)", start, end, currentRoot.Hex())

	batch := cfg.Store.DiskDB.NewBatch()

	err := reader.IterateRange(uint32(start), uint32(end), func(blockNum uint32, entries []dataset.Entry) error {
		blk := uint64(blockNum)

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

		// Buffer metadata writes in batch (no disk I/O).
		meta.PutRootBatch(batch, blk, newRoot)
		meta.PutLastBatch(batch, blk)

		currentRoot = newRoot
		blocksProcessed++
		totalAccounts += uint64(len(entries))

		// Flush trie DB on every block to ensure all historical states are available (archive mode)
		if err := cfg.Store.FlushTrieDB(newRoot); err != nil {
			return fmt.Errorf("block %d: triedb flush: %w", blk, err)
		}

		// Periodic flush: write batch to disk.
		if blocksProcessed%flushInterval == 0 {
			if err := batch.Write(); err != nil {
				return fmt.Errorf("block %d: batch write: %w", blk, err)
			}
			batch.Reset()
		}

		if blocksProcessed%1000 == 0 {
			elapsed := time.Since(startTime)
			bps := float64(blocksProcessed) / elapsed.Seconds()
			log.Printf("  block %d  root=%s  (%d blocks, %d accounts, %.1f blk/s)",
				blk, newRoot.Hex()[:18], blocksProcessed, totalAccounts, bps)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// Final flush: persist any remaining buffered data.
	if blocksProcessed > 0 {
		if err := batch.Write(); err != nil {
			return fmt.Errorf("final batch write: %w", err)
		}
		if err := cfg.Store.FlushTrieDB(currentRoot); err != nil {
			return fmt.Errorf("final triedb flush: %w", err)
		}
	}

	elapsed := time.Since(startTime)
	log.Printf("Ingestion complete: %d blocks, %d account updates in %s (%.1f blk/s)",
		blocksProcessed, totalAccounts, elapsed.Round(time.Millisecond), float64(blocksProcessed)/elapsed.Seconds())
	if blocksProcessed > 0 {
		log.Printf("Final root: %s", currentRoot.Hex())
	}

	return nil
}
