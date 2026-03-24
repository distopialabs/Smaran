package ingest

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/utils"
)

// BenchConfig holds user-facing benchmark parameters that are translated into
// a BenchContext at runtime.
type BenchConfig struct {
	Duration          time.Duration
	KUsers            int
	AccountsList      string
	UpdateMetricsPath string
}

// errBenchDeadline is a sentinel returned by the producer when the benchmark
// duration has been reached. It is not a real error -- BenchRun handles it as
// a clean stop.
var errBenchDeadline = errors.New("benchmark deadline reached")

// BenchRun orchestrates a benchmarked samurai+MPT ingestion run. It sets up
// the hot-account filter, CSV recorder, and deadline, then runs the same
// pipeline as Run with instrumentation enabled.
func BenchRun(cfg Config, benchCfg BenchConfig, csvPath string) error {
	// --- load hot-account filter ---
	var filter *benchutil.HotAccountFilter
	if benchCfg.KUsers > 0 {
		log.Printf("[bench] loading top %d hot accounts from %s", benchCfg.KUsers, benchCfg.AccountsList)
		var err error
		filter, err = benchutil.LoadHotAccountFilter(benchCfg.AccountsList, benchCfg.KUsers)
		if err != nil {
			return fmt.Errorf("load hot account filter: %w", err)
		}
		log.Printf("[bench] loaded %d hot accounts", filter.Size())
	}

	// --- create CSV writer ---
	header := append(benchutil.IngestionCSVHeader, "wait_commitments_ns")
	csvWriter, err := benchutil.NewBenchCSVWriter(csvPath, header)
	if err != nil {
		return fmt.Errorf("create bench CSV writer: %w", err)
	}
	defer csvWriter.Close()

	// --- configure deadline and block range ---
	cfg.Blocks.End = dataset.LAST_BLOCK
	deadline := time.Now().Add(benchCfg.Duration)

	// --- setup update-level metrics collector ---
	var updateMetrics *benchutil.UpdateMetricsCollector
	if benchCfg.UpdateMetricsPath != "" {
		var umErr error
		updateMetrics, umErr = benchutil.NewUpdateMetricsCollector(benchCfg.UpdateMetricsPath, time.Second)
		if umErr != nil {
			return fmt.Errorf("create update metrics collector: %w", umErr)
		}
		go updateMetrics.Run()
		defer updateMetrics.Stop()
	}

	// --- wire BenchContext into Config ---
	cfg.Bench = &BenchContext{
		Filter:        filter,
		CSV:           csvWriter,
		Deadline:      deadline,
		UpdateMetrics: updateMetrics,
	}

	// --- create pipeline plumbing (same as Run) ---
	queues := make([]*utils.BoundedQueue[UpdateTask], cfg.Workers.CommitWorkerCount)
	for i := 0; i < cfg.Workers.CommitWorkerCount; i++ {
		queues[i] = utils.NewBoundedQueue[UpdateTask](1024, cfg.Workers.CommitWorkerQueueSize)
	}

	blockInfoCh := make(chan mptBlockInfo, 1)
	commitCh := make(chan mptUpdateCommitmentInfo, 10000)

	var commitWG sync.WaitGroup
	var mptWG sync.WaitGroup

	startCommitWorkers(cfg, queues, commitCh, &commitWG)

	mptWG.Add(1)
	go func() {
		defer mptWG.Done()
		runMPTWorker(cfg, blockInfoCh, commitCh)
	}()

	benchStart := time.Now()
	log.Printf("[bench] starting benchmark: duration=%s startBlock=%d kUsers=%d output=%s",
		benchCfg.Duration, cfg.Blocks.Start, benchCfg.KUsers, csvPath)

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

	// --- interpret producer result ---
	if prodErr != nil && !errors.Is(prodErr, errBenchDeadline) {
		return fmt.Errorf("producer error: %w", prodErr)
	}

	if prodErr == nil && time.Now().Before(deadline) {
		return fmt.Errorf("ran out of blocks (reached block %d) before benchmark duration elapsed", dataset.LAST_BLOCK)
	}

	wallSec := time.Since(benchStart).Seconds()
	log.Printf("[bench] complete: %.1fs wall-clock, output=%s", wallSec, csvPath)

	return nil
}

// BenchRunSamuraiOnly orchestrates a benchmarked samurai-only ingestion run.
// It skips MPT entirely and uses a lightweight metrics collector to record
// per-block KZG completion times in the same CSV format as BenchRun.
func BenchRunSamuraiOnly(cfg Config, benchCfg BenchConfig, csvPath string) error {
	// --- load hot-account filter ---
	var filter *benchutil.HotAccountFilter
	if benchCfg.KUsers > 0 {
		log.Printf("[bench] loading top %d hot accounts from %s", benchCfg.KUsers, benchCfg.AccountsList)
		var err error
		filter, err = benchutil.LoadHotAccountFilter(benchCfg.AccountsList, benchCfg.KUsers)
		if err != nil {
			return fmt.Errorf("load hot account filter: %w", err)
		}
		log.Printf("[bench] loaded %d hot accounts", filter.Size())
	}

	// --- create CSV writer (same columns as BenchRun) ---
	header := append(benchutil.IngestionCSVHeader, "wait_commitments_ns")
	csvWriter, err := benchutil.NewBenchCSVWriter(csvPath, header)
	if err != nil {
		return fmt.Errorf("create bench CSV writer: %w", err)
	}
	defer csvWriter.Close()

	// --- configure deadline and block range ---
	cfg.Blocks.End = dataset.LAST_BLOCK
	deadline := time.Now().Add(benchCfg.Duration)

	// --- setup update-level metrics collector ---
	var updateMetrics *benchutil.UpdateMetricsCollector
	if benchCfg.UpdateMetricsPath != "" {
		var umErr error
		updateMetrics, umErr = benchutil.NewUpdateMetricsCollector(benchCfg.UpdateMetricsPath, time.Second)
		if umErr != nil {
			return fmt.Errorf("create update metrics collector: %w", umErr)
		}
		go updateMetrics.Run()
		defer updateMetrics.Stop()
	}

	// --- wire BenchContext into Config ---
	cfg.Bench = &BenchContext{
		Filter:        filter,
		CSV:           csvWriter,
		Deadline:      deadline,
		UpdateMetrics: updateMetrics,
	}

	// --- create pipeline plumbing ---
	queues := make([]*utils.BoundedQueue[UpdateTask], cfg.Workers.CommitWorkerCount)
	for i := 0; i < cfg.Workers.CommitWorkerCount; i++ {
		queues[i] = utils.NewBoundedQueue[UpdateTask](1024, cfg.Workers.CommitWorkerQueueSize)
	}

	blockInfoCh := make(chan mptBlockInfo, 10)
	commitCh := make(chan mptUpdateCommitmentInfo, 10000)

	var commitWG sync.WaitGroup
	var collectorWG sync.WaitGroup

	startCommitWorkers(cfg, queues, commitCh, &commitWG)

	collectorWG.Add(1)
	go func() {
		defer collectorWG.Done()
		runMetricsCollector(cfg, blockInfoCh, commitCh)
	}()

	benchStart := time.Now()
	log.Printf("[bench] starting samurai-only benchmark: duration=%s startBlock=%d kUsers=%d output=%s",
		benchCfg.Duration, cfg.Blocks.Start, benchCfg.KUsers, csvPath)

	// --- run producer ---
	prodErr := produceBlocks(cfg, blockInfoCh, queues)

	// --- shutdown pipeline ---
	for _, q := range queues {
		q.Close()
	}
	close(blockInfoCh)

	commitWG.Wait()
	close(commitCh)

	collectorWG.Wait()

	// --- interpret producer result ---
	if prodErr != nil && !errors.Is(prodErr, errBenchDeadline) {
		return fmt.Errorf("producer error: %w", prodErr)
	}

	if prodErr == nil && time.Now().Before(deadline) {
		return fmt.Errorf("ran out of blocks (reached block %d) before benchmark duration elapsed", dataset.LAST_BLOCK)
	}

	wallSec := time.Since(benchStart).Seconds()
	log.Printf("[bench] samurai-only complete: %.1fs wall-clock, output=%s", wallSec, csvPath)

	return nil
}
