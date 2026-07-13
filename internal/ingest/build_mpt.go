package ingest

import (
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"github.com/nepal80m/samurai/internal/tree"
)

const (
	// mptBatchSize is the number of accounts to accumulate before committing
	// and flushing the trie to disk. 5M accounts ≈ 5–7 GB peak RAM, safe
	// for a 100 GB machine while minimising flush overhead (~13 flushes for
	// 64M accounts).
	mptBatchSize = 5_000_000

	// mptLogInterval is how often (in accounts) progress is printed within
	// a batch.
	mptLogInterval = 500_000
)

// BuildMPT scans every shard's StateDB for accounts, computes each account's
// final commitment hash, and inserts them into a StateTrie.
//
// To avoid going out-of-memory with tens of millions of accounts the trie is
// committed and flushed every mptBatchSize accounts; the resulting root is
// carried forward so the final trie is identical to a single-pass build.
func BuildMPT(cfg Config) error {
	totalStart := time.Now()

	currentRoot := types.EmptyRootHash

	tr, err := cfg.MPTStore.OpenState(currentRoot)
	if err != nil {
		return fmt.Errorf("open empty state trie: %w", err)
	}

	accountCount := 0
	batchCount := 0
	batchNum := 0

	for shardIdx, store := range cfg.SamuraiStores {
		pdb, ok := store.StateDB.(*db.PebbleDB)
		if !ok {
			return fmt.Errorf("shard %d: StateDB is not PebbleDB", shardIdx)
		}

		prefix := []byte("user:")
		iter, err := pdb.InnerDB().NewIter(&pebble.IterOptions{
			LowerBound: prefix,
			UpperBound: prefixUpperBound(prefix),
		})
		if err != nil {
			return fmt.Errorf("shard %d: create iterator: %w", shardIdx, err)
		}

		const suffix = ":current_balance_info"

		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			if !strings.HasSuffix(key, suffix) {
				continue
			}

			addrHex := key[len("user:") : len(key)-len(suffix)]
			addr := common.HexToAddress(addrHex)

			cbInfo, err := tree.GetCurrentBalanceInfo(addr, &store.StateDB)
			if err != nil {
				iter.Close()
				return fmt.Errorf("shard %d, account %s: load balance info: %w", shardIdx, addrHex, err)
			}

			var commitments *tree.LXBatchCommitment
			if cbInfo.Version == 0 {
				commitments = new(tree.LXBatchCommitment)
			} else {
				commitments = tree.GetLXBatchCommitments(addr, cbInfo.Version, &store.StateDB)
			}

			ai := &tree.AccountInfo{
				CurrentBalanceInfo:       cbInfo,
				CurrentLXBatchCommitment: commitments,
			}
			commitmentHash := ai.CalculateFinalCommitment()

			st.SetAccountCommitmentAsBalance(tr, addr, commitmentHash)
			accountCount++
			batchCount++

			// Progress log within the batch.
			if accountCount%mptLogInterval == 0 {
				fmt.Printf("  [MPT] progress: %d accounts processed (shard %d, elapsed %s)\n",
					accountCount, shardIdx, time.Since(totalStart))
			}

			// Flush the batch to cap memory usage.
			if batchCount >= mptBatchSize {
				batchStart := time.Now()
				newRoot, err := cfg.MPTStore.CommitState(tr, currentRoot, cfg.Blocks.End)
				if err != nil {
					iter.Close()
					return fmt.Errorf("batch %d commit: %w", batchNum, err)
				}
				if err := cfg.MPTStore.FlushTrieDB(newRoot); err != nil {
					iter.Close()
					return fmt.Errorf("batch %d flush: %w", batchNum, err)
				}

				fmt.Printf("  [MPT] batch %d committed: %d accounts total, root=%s, batch took %s, total %s\n",
					batchNum, accountCount, newRoot.Hex(), time.Since(batchStart), time.Since(totalStart))

				currentRoot = newRoot
				batchCount = 0
				batchNum++

				// Re-open the trie from the flushed root for the next batch.
				tr, err = cfg.MPTStore.OpenState(currentRoot)
				if err != nil {
					iter.Close()
					return fmt.Errorf("reopen state after batch %d: %w", batchNum, err)
				}
			}
		}

		if err := iter.Error(); err != nil {
			iter.Close()
			return fmt.Errorf("shard %d: iterator error: %w", shardIdx, err)
		}
		iter.Close()
	}

	// Final commit for the remaining accounts.
	root, err := cfg.MPTStore.CommitState(tr, currentRoot, cfg.Blocks.End)
	if err != nil {
		return fmt.Errorf("final commit: %w", err)
	}

	meta.PutRoot(cfg.MPTStore.DiskDB, cfg.Blocks.End, root)
	if err := meta.PutLast(cfg.MPTStore.DiskDB, cfg.Blocks.End); err != nil {
		return fmt.Errorf("store last block: %w", err)
	}

	if err := cfg.MPTStore.FlushTrieDB(root); err != nil {
		return fmt.Errorf("flush trie db: %w", err)
	}

	fmt.Printf("Phase 2 (MPT) complete: %d accounts, %d batches, root=%s, took %s\n",
		accountCount, batchNum+1, root.Hex(), time.Since(totalStart))

	return nil
}

// prefixUpperBound returns the immediate successor of prefix for use as
// an exclusive upper bound in Pebble iterators.
func prefixUpperBound(prefix []byte) []byte {
	ub := make([]byte, len(prefix))
	copy(ub, prefix)
	for i := len(ub) - 1; i >= 0; i-- {
		ub[i]++
		if ub[i] != 0 {
			break
		}
	}
	return ub
}
