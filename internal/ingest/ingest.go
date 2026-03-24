package ingest

import (
	"fmt"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/utils"
)

type UpdateTask struct {
	BlockNumber uint64
	Account     common.Address
	Balance     *big.Int
}

type mptBlockInfo struct {
	blockNumber    uint64
	updateCount    uint64 // selected updates (after filtering)
	rawUpdateCount uint64 // raw updates (before filtering)
	queuedAtNs     int64  // UnixNano timestamp when queued; 0 when not benchmarking
}

type mptUpdateCommitmentInfo struct {
	blockNumber uint64
	address     common.Address
	commitment  common.Hash
}

// startCommitWorkers spawns one goroutine per shard that pops UpdateTasks,
// computes the Samurai commitment, and optionally forwards the result to commitCh.
// If commitCh is nil, commitment results are discarded (samurai-only mode).
func startCommitWorkers(cfg Config, queues []*utils.BoundedQueue[UpdateTask], commitCh chan<- mptUpdateCommitmentInfo, wg *sync.WaitGroup) {
	// Capture update-level metrics collector (may be nil when not benchmarking).
	updateMetrics := cfg.Bench != nil && cfg.Bench.UpdateMetrics != nil

	for i := 0; i < cfg.Workers.CommitWorkerCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				task, ok := queues[i].Pop()
				if !ok {
					return
				}

				var computeStart time.Time
				if updateMetrics {
					computeStart = time.Now()
				}

				commitmentHash, err := storage.CreateOrUpdateAccountInfo(
					task.Account,
					task.Balance,
					task.BlockNumber,
					cfg.Caches[i],
				)
				if err != nil {
					panic(err)
				}

				if updateMetrics {
					cfg.Bench.UpdateMetrics.Record(time.Since(computeStart).Nanoseconds())
				}

				if commitCh != nil {
					commitCh <- mptUpdateCommitmentInfo{
						blockNumber: task.BlockNumber,
						address:     task.Account,
						commitment:  commitmentHash,
					}
				}
			}
		}()
	}
}

// runMPTWorker sequentially processes one block at a time: collects all
// commitment updates for the block, applies them to the StateTrie, and
// commits + flushes the result to disk (archive mode).
func runMPTWorker(cfg Config, blockInfoCh <-chan mptBlockInfo, commitCh <-chan mptUpdateCommitmentInfo) {
	currentRoot, err := meta.GetRoot(cfg.MPTStore.DiskDB, cfg.Blocks.Start-1)
	if err != nil {
		panic(err)
	}

	bench := cfg.Bench // may be nil
	buffered := make(map[uint64][]mptUpdateCommitmentInfo)

	const flushInterval = 1024
	const logInterval = 1000
	batch := cfg.MPTStore.DiskDB.NewBatch()
	blocksProcessed := uint64(0)
	var lastBlockNumber uint64
	logStart := time.Now()

	for blockInfo := range blockInfoCh {
		pending := blockInfo.updateCount

		startAtNs := time.Now().UnixNano()

		// --- Phase: OpenStateTrie ---
		tr, err := cfg.MPTStore.OpenState(currentRoot)
		if err != nil {
			panic(err)
		}

		// --- Phase: Apply buffered + drain commitCh ---
		if buf, ok := buffered[blockInfo.blockNumber]; ok {
			for _, u := range buf {
				st.SetAccountCommitmentAsBalance(tr, u.address, u.commitment)
				pending--
			}
			delete(buffered, blockInfo.blockNumber)
		}

		// Drain commitCh until we have all pending updates for this block.
		// Track time spent blocked waiting for commitments.
		var waitCommitmentsNs int64
		for pending > 0 {
			tWait := time.Now()
			u := <-commitCh
			waitCommitmentsNs += time.Since(tWait).Nanoseconds()

			if u.blockNumber != blockInfo.blockNumber {
				buffered[u.blockNumber] = append(buffered[u.blockNumber], u)
			} else {
				st.SetAccountCommitmentAsBalance(tr, u.address, u.commitment)
				pending--
			}
		}

		// --- Phase: CommitStateTrie ---
		newRoot, err := cfg.MPTStore.CommitState(tr, currentRoot, blockInfo.blockNumber)
		if err != nil {
			panic(fmt.Sprintf("commit state for block %d: %v", blockInfo.blockNumber, err))
		}
		meta.PutRootBatch(batch, blockInfo.blockNumber, newRoot)

		// --- Phase: FlushTrieDB ---
		if err := cfg.MPTStore.FlushTrieDB(newRoot); err != nil {
			panic(err)
		}

		completedAtNs := time.Now().UnixNano()

		// --- record per-block metrics (bench mode) ---
		if bench != nil {
			_ = bench.CSV.WriteRow(
				strconv.FormatUint(blockInfo.blockNumber, 10),
				strconv.FormatUint(blockInfo.rawUpdateCount, 10),
				strconv.FormatUint(blockInfo.updateCount, 10),
				strconv.FormatInt(blockInfo.queuedAtNs, 10),
				strconv.FormatInt(startAtNs, 10),
				strconv.FormatInt(completedAtNs, 10),
				strconv.FormatInt(waitCommitmentsNs, 10),
			)
		}

		currentRoot = newRoot
		lastBlockNumber = blockInfo.blockNumber
		blocksProcessed++

		if blocksProcessed%flushInterval == 0 {
			if err := batch.Write(); err != nil {
				panic(fmt.Sprintf("batch write at block %d: %v", blockInfo.blockNumber, err))
			}
			batch.Reset()
		}

		// --- periodic log ---
		if blocksProcessed%logInterval == 0 {
			fmt.Printf("[MPT] blk=%d elapsed=%s buffered=%d\n",
				blockInfo.blockNumber,
				time.Since(logStart).Truncate(time.Millisecond),
				len(buffered),
			)
			logStart = time.Now()
		}
	}

	// Write remaining roots and record the last processed block.
	if blocksProcessed > 0 {
		meta.PutLastBatch(batch, lastBlockNumber)
	}
	if err := batch.Write(); err != nil {
		panic(fmt.Sprintf("final batch write: %v", err))
	}
}

// runMetricsCollector is a lightweight replacement for runMPTWorker used in
// samurai-only benchmark mode. It drains blockInfoCh and commitCh, counts
// per-block commitment completions, and writes CSV timing rows — but performs
// no trie operations.
func runMetricsCollector(cfg Config, blockInfoCh <-chan mptBlockInfo, commitCh <-chan mptUpdateCommitmentInfo) {
	bench := cfg.Bench                  // guaranteed non-nil in bench mode
	buffered := make(map[uint64]uint64) // blockNumber -> count of pre-arrived commitments

	const logInterval = 1000
	blocksProcessed := uint64(0)
	logStart := time.Now()

	for blockInfo := range blockInfoCh {
		pending := blockInfo.updateCount

		startAtNs := time.Now().UnixNano()

		// Apply pre-buffered commitment counts.
		if n, ok := buffered[blockInfo.blockNumber]; ok {
			pending -= n
			delete(buffered, blockInfo.blockNumber)
		}

		// Drain commitCh until all commitments for this block arrive.
		var waitCommitmentsNs int64
		for pending > 0 {
			tWait := time.Now()
			u := <-commitCh
			waitCommitmentsNs += time.Since(tWait).Nanoseconds()

			if u.blockNumber != blockInfo.blockNumber {
				buffered[u.blockNumber]++
			} else {
				pending--
			}
		}

		completedAtNs := time.Now().UnixNano()

		_ = bench.CSV.WriteRow(
			strconv.FormatUint(blockInfo.blockNumber, 10),
			strconv.FormatUint(blockInfo.rawUpdateCount, 10),
			strconv.FormatUint(blockInfo.updateCount, 10),
			strconv.FormatInt(blockInfo.queuedAtNs, 10),
			strconv.FormatInt(startAtNs, 10),
			strconv.FormatInt(completedAtNs, 10),
			strconv.FormatInt(waitCommitmentsNs, 10),
		)

		blocksProcessed++
		if blocksProcessed%logInterval == 0 {
			fmt.Printf("[MetricsCollector] blk=%d elapsed=%s buffered=%d\n",
				blockInfo.blockNumber,
				time.Since(logStart).Truncate(time.Millisecond),
				len(buffered),
			)
			logStart = time.Now()
		}
	}
}

// produceBlocks reads blocks from the dataset and distributes entries
// to shard queues (for commitment workers). If blockInfoCh is non-nil,
// block metadata is also sent to the MPT worker.
//
// When cfg.Bench is set the producer additionally:
//   - checks the benchmark deadline at each block boundary
//   - filters entries through the HotAccountFilter (if configured)
//   - stamps EnqueuedAt on each UpdateTask
//   - sends enriched mptBlockInfo with raw/discarded counts and emittedAt
//   - increments global atomic counters on the MetricsCollector
func produceBlocks(cfg Config, blockInfoCh chan<- mptBlockInfo, queues []*utils.BoundedQueue[UpdateTask]) error {
	r := dataset.NewDatasetReader(cfg.Blocks.DataDir, dataset.SEGMENT_SIZE)
	defer r.Close()

	bench := cfg.Bench // may be nil

	return r.IterateRange(
		uint32(cfg.Blocks.Start),
		uint32(cfg.Blocks.End),
		func(n uint32, entries []dataset.Entry) error {
			// --- benchmark deadline check (before starting this block) ---
			if bench != nil && !bench.Deadline.IsZero() && time.Now().After(bench.Deadline) {
				return errBenchDeadline
			}

			if n%10000 == 0 {
				fmt.Printf("Commit Phase: progressing, currently at block %d\n", n)
			}

			// --- filter entries when benchmarking with a hot-account set ---
			rawCount := uint64(len(entries))
			selected := entries
			if bench != nil && bench.Filter != nil {
				filtered := make([]dataset.Entry, 0, len(entries))
				for i := range entries {
					addr := common.BytesToAddress(entries[i].Address[:])
					if bench.Filter.Contains(addr) {
						filtered = append(filtered, entries[i])
					}
				}
				selected = filtered
			}
			selectedCount := uint64(len(selected))

			// --- emit block metadata ---
			if blockInfoCh != nil {
				info := mptBlockInfo{
					blockNumber:    uint64(n),
					updateCount:    selectedCount,
					rawUpdateCount: rawCount,
				}
				if bench != nil {
					info.queuedAtNs = time.Now().UnixNano()
				}
				blockInfoCh <- info
			}

			// --- distribute selected entries to shard queues ---
			for _, entry := range selected {
				idx := utils.AddressToShardIndex(entry.Address, cfg.Workers.CommitWorkerCount)
				task := UpdateTask{
					BlockNumber: uint64(n),
					Account:     common.BytesToAddress(entry.Address[:]),
					Balance:     new(big.Int).SetBytes(entry.Balance),
				}
				if err := queues[idx].Push(task); err != nil {
					return fmt.Errorf("push to shard queue %d: %w", idx, err)
				}
			}
			return nil
		},
	)
}

// Run orchestrates the samurai+MPT ingestion pipeline.
func Run(cfg Config) error {
	totalStart := time.Now()

	queues := make([]*utils.BoundedQueue[UpdateTask], cfg.Workers.CommitWorkerCount)
	for i := 0; i < cfg.Workers.CommitWorkerCount; i++ {
		queues[i] = utils.NewBoundedQueue[UpdateTask](1024, cfg.Workers.CommitWorkerQueueSize)
	}

	blockInfoCh := make(chan mptBlockInfo, 10)
	commitCh := make(chan mptUpdateCommitmentInfo, 10000)

	var commitWG sync.WaitGroup
	var mptWG sync.WaitGroup

	startCommitWorkers(cfg, queues, commitCh, &commitWG)

	mptWG.Add(1)
	go func() {
		defer mptWG.Done()
		runMPTWorker(cfg, blockInfoCh, commitCh)
	}()

	err := produceBlocks(cfg, blockInfoCh, queues)

	for _, q := range queues {
		q.Close()
	}
	close(blockInfoCh)

	commitWG.Wait()
	close(commitCh)

	mptWG.Wait()

	if err != nil {
		return fmt.Errorf("iterate dataset range: %w", err)
	}

	fmt.Println(
		"Time taken to process",
		cfg.Blocks.End-cfg.Blocks.Start+1,
		"blocks",
		time.Since(totalStart),
		time.Now(),
	)

	return nil
}

// RunSamuraiOnly runs the samurai commitment pipeline without the MPT worker.
// At the end it flushes all caches to ensure every account is persisted to DB.
func RunSamuraiOnly(cfg Config) error {
	totalStart := time.Now()

	queues := make([]*utils.BoundedQueue[UpdateTask], cfg.Workers.CommitWorkerCount)
	for i := 0; i < cfg.Workers.CommitWorkerCount; i++ {
		queues[i] = utils.NewBoundedQueue[UpdateTask](1024, cfg.Workers.CommitWorkerQueueSize)
	}

	var commitWG sync.WaitGroup
	startCommitWorkers(cfg, queues, nil, &commitWG)

	err := produceBlocks(cfg, nil, queues)

	for _, q := range queues {
		q.Close()
	}
	commitWG.Wait()

	// Flush all caches to DB so Phase 2 sees the final state.
	for _, c := range cfg.Caches {
		c.Close()
	}

	if err != nil {
		return fmt.Errorf("iterate dataset range: %w", err)
	}

	fmt.Printf("Phase 1 (Samurai) complete: processed %d blocks in %s\n",
		cfg.Blocks.End-cfg.Blocks.Start+1, time.Since(totalStart))

	return nil
}

// RunDeferred runs the two-phase deferred pipeline:
// Phase 1 processes blocks with samurai only (no MPT bottleneck),
// Phase 2 builds the MPT from the final DB state in a single pass.
func RunDeferred(cfg Config) error {
	if err := RunSamuraiOnly(cfg); err != nil {
		return fmt.Errorf("phase 1 (samurai): %w", err)
	}
	if err := BuildMPT(cfg); err != nil {
		return fmt.Errorf("phase 2 (build MPT): %w", err)
	}
	return nil
}
