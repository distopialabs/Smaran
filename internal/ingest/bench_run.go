package ingest

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/utils"
)

// BenchConfig holds user-facing benchmark parameters that are translated into
// a BenchContext at runtime.
type BenchConfig struct {
	Duration        time.Duration
	NumUsers        int
	HotAccountsFile string
	OutputDir       string
}

// errBenchDeadline is a sentinel returned by the producer when the benchmark
// duration has been reached. It is not a real error -- BenchRun handles it as
// a clean stop.
var errBenchDeadline = errors.New("benchmark deadline reached")

// BenchRun orchestrates a benchmarked samurai+MPT ingestion run. It sets up
// the hot-account filter, metrics collector, and deadline, then runs the same
// pipeline as Run with instrumentation enabled.
func BenchRun(cfg Config, benchCfg BenchConfig) error {
	// --- load hot-account filter ---
	var filter *HotAccountFilter
	if benchCfg.NumUsers > 0 {
		log.Printf("[bench] loading top %d hot accounts from %s", benchCfg.NumUsers, benchCfg.HotAccountsFile)
		var err error
		filter, err = LoadHotAccountFilter(benchCfg.HotAccountsFile, benchCfg.NumUsers)
		if err != nil {
			return fmt.Errorf("load hot account filter: %w", err)
		}
		log.Printf("[bench] loaded %d hot accounts", filter.Size())
	}

	// --- create metrics collector ---
	mc, err := NewMetricsCollector(benchCfg.OutputDir, cfg.Blocks.Start, benchCfg.Duration, benchCfg.NumUsers)
	if err != nil {
		return fmt.Errorf("create metrics collector: %w", err)
	}
	defer mc.Close()

	// --- configure deadline and block range ---
	// For duration-based runs we process as many blocks as possible up to
	// LAST_BLOCK. The deadline check inside the producer stops iteration at
	// a block boundary.
	cfg.Blocks.End = dataset.LAST_BLOCK
	deadline := time.Now().Add(benchCfg.Duration)

	// --- wire BenchContext into Config ---
	cfg.Bench = &BenchContext{
		Filter:   filter,
		Metrics:  mc,
		Deadline: deadline,
	}

	// --- create pipeline plumbing (same as Run) ---
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

	// --- start telemetry ---
	mc.StartTelemetry(5*time.Second, queues, commitCh)
	mc.BenchStart = time.Now()

	log.Printf("[bench] starting benchmark: duration=%s startBlock=%d numUsers=%d",
		benchCfg.Duration, cfg.Blocks.Start, benchCfg.NumUsers)

	// --- run producer ---
	prodErr := produceBlocks(cfg, blockInfoCh, queues)

	// --- shutdown pipeline ---
	for _, q := range queues {
		q.Close()
	}
	close(blockInfoCh)

	commitWG.Wait()
	close(commitCh)

	mptWG.Wait()

	mc.BenchEnd = time.Now()
	mc.StopTelemetry()

	// --- interpret producer result ---
	if prodErr != nil && !errors.Is(prodErr, errBenchDeadline) {
		return fmt.Errorf("producer error: %w", prodErr)
	}

	if prodErr == nil && time.Now().Before(deadline) {
		return fmt.Errorf("ran out of blocks (reached block %d) before benchmark duration elapsed; increase block range or reduce duration", dataset.LAST_BLOCK)
	}

	// --- write summary ---
	if err := mc.WriteSummary(); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	wallSec := mc.BenchEnd.Sub(mc.BenchStart).Seconds()
	log.Printf("[bench] complete: %.1fs wall-clock, %d blocks, %d selected updates, %d discarded",
		wallSec,
		mc.BlocksCommitted.Load(),
		mc.SelectedUpdates.Load(),
		mc.DiscardedUpdates.Load(),
	)

	return nil
}
