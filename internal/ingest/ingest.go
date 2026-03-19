package ingest

import (
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/storage"
	"github.com/nepal80m/samurai/internal/utils"
	"github.com/nepal80m/samurai/mpt/meta"
	st "github.com/nepal80m/samurai/mpt/state"
)

type UpdateTask struct {
	BlockNumber uint64
	Account     common.Address
	Balance     *big.Int
	EnqueuedAt  int64 // UnixNano timestamp; 0 when not benchmarking
}

type mptBlockInfo struct {
	blockNumber    uint64
	updateCount    uint64 // selected updates (after filtering)
	rawUpdateCount uint64 // raw updates (before filtering)
	discardedCount uint64 // discarded by filter
	emittedAt      int64  // UnixNano timestamp; 0 when not benchmarking
}

type mptUpdateCommitmentInfo struct {
	blockNumber uint64
	address     common.Address
	commitment  common.Hash
}

// startCommitWorkers spawns one goroutine per shard that pops UpdateTasks,
// computes the Samurai commitment, and optionally forwards the result to commitCh.
// If commitCh is nil, commitment results are discarded (samurai-only mode).
//
// When cfg.Bench is set each worker additionally records:
//   - queue wait latency  (time from enqueue to dequeue)
//   - commitment latency  (time inside CreateOrUpdateAccountInfo)
func startCommitWorkers(cfg Config, queues []*utils.BoundedQueue[UpdateTask], commitCh chan<- mptUpdateCommitmentInfo, wg *sync.WaitGroup) {
	bench := cfg.Bench // may be nil
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

				if bench != nil && task.EnqueuedAt != 0 {
					queueWaitNs := time.Now().UnixNano() - task.EnqueuedAt
					bench.Metrics.RecordQueueWait(task.BlockNumber, queueWaitNs)
				}

				var commitStart time.Time
				if bench != nil {
					commitStart = time.Now()
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

				if bench != nil {
					bench.Metrics.RecordCommitLatency(task.BlockNumber, time.Since(commitStart).Nanoseconds())
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
//
// Always prints a per-phase timing summary every 1000 blocks so that
// bottlenecks are visible even in non-benchmark runs. When cfg.Bench is set,
// the worker additionally writes detailed per-block rows to blocks.csv.
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

	// Rolling accumulators for periodic log summary.
	var (
		logWaitNs   int64
		logOpenNs   int64
		logApplyNs  int64
		logCommitNs int64
		logFlushNs  int64
		logUpdates  uint64
		logBlocks   uint64
		logStart    = time.Now()
	)

	for blockInfo := range blockInfoCh {
		pending := blockInfo.updateCount

		var mptStartNs int64
		if bench != nil {
			mptStartNs = time.Now().UnixNano()
		}

		// --- Phase: OpenStateTrie ---
		t0 := time.Now()
		tr, err := cfg.MPTStore.OpenState(currentRoot)
		if err != nil {
			panic(err)
		}
		openNs := time.Since(t0).Nanoseconds()

		// --- Phase: Apply buffered + drain commitCh ---
		tApply := time.Now()
		// Apply any already-buffered commitments for this block.
		if buf, ok := buffered[blockInfo.blockNumber]; ok {
			for _, u := range buf {
				st.SetAccountCommitmentAsBalance(tr, u.address, u.commitment)
				pending--
			}
			delete(buffered, blockInfo.blockNumber)
		}
		applyBufferedNs := time.Since(tApply).Nanoseconds()

		// Drain commitCh until we have all pending updates for this block.
		var waitCommitmentsNs int64
		var applyCommitmentsNs int64
		for pending > 0 {
			tWait := time.Now()
			u := <-commitCh
			waitCommitmentsNs += time.Since(tWait).Nanoseconds()

			if u.blockNumber != blockInfo.blockNumber {
				buffered[u.blockNumber] = append(buffered[u.blockNumber], u)
			} else {
				tA := time.Now()
				st.SetAccountCommitmentAsBalance(tr, u.address, u.commitment)
				applyCommitmentsNs += time.Since(tA).Nanoseconds()
				pending--
			}
		}
		totalApplyNs := applyBufferedNs + applyCommitmentsNs

		// --- Phase: CommitStateTrie ---
		tCommit := time.Now()
		newRoot, err := cfg.MPTStore.CommitState(tr, currentRoot, blockInfo.blockNumber)
		if err != nil {
			panic(fmt.Sprintf("commit state for block %d: %v", blockInfo.blockNumber, err))
		}
		meta.PutRootBatch(batch, blockInfo.blockNumber, newRoot)
		commitNs := time.Since(tCommit).Nanoseconds()

		// --- Phase: FlushTrieDB ---
		tFlush := time.Now()
		if err := cfg.MPTStore.FlushTrieDB(newRoot); err != nil {
			panic(err)
		}
		flushNs := time.Since(tFlush).Nanoseconds()

		// --- record per-block metrics (bench mode) ---
		if bench != nil {
			completedAtNs := time.Now().UnixNano()
			mptPhaseNs := completedAtNs - mptStartNs
			bench.Metrics.WriteBlockRow(
				blockInfo.blockNumber,
				blockInfo.rawUpdateCount,
				blockInfo.updateCount,
				blockInfo.discardedCount,
				blockInfo.emittedAt,
				mptStartNs,
				completedAtNs,
				mptPhaseNs,
				waitCommitmentsNs,
				commitNs,
				flushNs,
			)
		}

		// --- accumulate for periodic log ---
		logWaitNs += waitCommitmentsNs
		logOpenNs += openNs
		logApplyNs += totalApplyNs
		logCommitNs += commitNs
		logFlushNs += flushNs
		logUpdates += blockInfo.updateCount
		logBlocks++

		currentRoot = newRoot
		lastBlockNumber = blockInfo.blockNumber
		blocksProcessed++

		if blocksProcessed%flushInterval == 0 {
			if err := batch.Write(); err != nil {
				panic(fmt.Sprintf("batch write at block %d: %v", blockInfo.blockNumber, err))
			}
			batch.Reset()
		}

		// --- periodic log summary ---
		if logBlocks > 0 && blocksProcessed%logInterval == 0 {
			n := float64(logBlocks)
			fmt.Printf("[MPT] blk=%d elapsed=%s upd/blk=%.0f wait=%.2fms open=%.2fms apply=%.2fms commit=%.2fms flush=%.2fms total=%.2fms buffered=%d\n",
				blockInfo.blockNumber,
				time.Since(logStart).Truncate(time.Millisecond),
				float64(logUpdates)/n,
				float64(logWaitNs)/n/1e6,
				float64(logOpenNs)/n/1e6,
				float64(logApplyNs)/n/1e6,
				float64(logCommitNs)/n/1e6,
				float64(logFlushNs)/n/1e6,
				float64(logWaitNs+logOpenNs+logApplyNs+logCommitNs+logFlushNs)/n/1e6,
				len(buffered),
			)
			logWaitNs, logOpenNs, logApplyNs, logCommitNs, logFlushNs = 0, 0, 0, 0, 0
			logUpdates, logBlocks = 0, 0
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
			discardedCount := rawCount - selectedCount

			// --- update global atomic counters ---
			if bench != nil {
				bench.Metrics.RawUpdates.Add(rawCount)
				bench.Metrics.SelectedUpdates.Add(selectedCount)
				bench.Metrics.DiscardedUpdates.Add(discardedCount)
			}

			// --- emit block metadata ---
			if blockInfoCh != nil {
				info := mptBlockInfo{
					blockNumber:    uint64(n),
					updateCount:    selectedCount,
					rawUpdateCount: rawCount,
					discardedCount: discardedCount,
				}
				if bench != nil {
					info.emittedAt = time.Now().UnixNano()
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
				if bench != nil {
					task.EnqueuedAt = time.Now().UnixNano()
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
