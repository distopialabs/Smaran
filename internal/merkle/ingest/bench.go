package ingest

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"

	"golang.design/x/chann"

	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/dataset"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"github.com/nepal80m/samurai/internal/utils"
)

// BenchConfig holds configuration for the ingestion benchmark.
type BenchConfig struct {
	BlocksDir         string
	Store             *st.MPTStateStore
	Start             uint64
	End               uint64
	Duration          time.Duration
	KUsers            int
	NumShards         int
	AccountsList      string
	OutCSV            string
	UpdateMetricsPath string
}

type updateTask struct {
	blockNumber uint64
	address     common.Address
	balance     *big.Int
}

type blockInfo struct {
	blockNumber    uint64
	updateCount    uint64
	rawUpdateCount uint64
	queuedAtNs     int64
}

type updateForward struct {
	blockNumber uint64
	address     common.Address
	balance     *big.Int
}

var errDurationExceeded = errors.New("bench: duration exceeded")

// produceBlocks reads blocks from the dataset, applies the hot-account filter
// and deadline check, sends block metadata on blockInfoCh, and distributes
// entries across shard queues.
func produceBlocks(
	blocksDir string, start, end uint64,
	filter *benchutil.HotAccountFilter, deadline time.Time,
	blockInfoCh chan<- blockInfo,
	queues []*chann.Chann[updateTask],
	numShards int,
) error {
	r := dataset.NewDatasetReader(blocksDir, dataset.SEGMENT_SIZE)
	defer r.Close()

	return r.IterateRange(
		uint32(start),
		uint32(end),
		func(n uint32, entries []dataset.Entry) error {
			if time.Now().After(deadline) {
				return errDurationExceeded
			}

			rawCount := uint64(len(entries))
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

			for _, entry := range selected {
				addr := common.BytesToAddress(entry.Address[:])
				idx := utils.AddressToShardIndex(addr, numShards)
				queues[idx].In() <- updateTask{
					blockNumber: uint64(n),
					address:     addr,
					balance:     new(big.Int).SetBytes(entry.Balance),
				}
			}

			blockInfoCh <- blockInfo{
				blockNumber:    uint64(n),
				updateCount:    selectedCount,
				rawUpdateCount: rawCount,
				queuedAtNs:     time.Now().UnixNano(),
			}

			return nil
		},
	)
}

// startShardWorkers spawns one goroutine per shard queue. Each worker pops
// updateTasks and forwards them to updateCh without any computation.
func startShardWorkers(
	queues []*chann.Chann[updateTask],
	updateCh chan<- updateForward,
	wg *sync.WaitGroup,
) {
	for i := range queues {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range queues[i].Out() {
				updateCh <- updateForward{
					blockNumber: task.blockNumber,
					address:     task.address,
					balance:     task.balance,
				}
			}
		}()
	}
}

// --- helpers for runMPTWorker ---

type defaultDict struct {
	internal map[uint64]uint64
}

func (d *defaultDict) Get(key uint64) uint64 {
	if val, ok := d.internal[key]; ok {
		return val
	}
	return math.MaxUint64
}

func (d *defaultDict) Set(key uint64, value uint64) {
	d.internal[key] = value
}

func (d *defaultDict) Delete(key uint64) {
	delete(d.internal, key)
}

const maxHangLen = 1

func mayHang(blockInfoCh *<-chan blockInfo, hangChan *<-chan blockInfo, currLen int, maxLen int) *<-chan blockInfo {
	if currLen > maxLen {
		return hangChan
	}
	return blockInfoCh
}

// runMPTWorker uses a select loop over blockInfoCh and updateCh to buffer
// forwarded updates, then applies them to the trie once all updates for a
// block have arrived.
func runMPTWorker(
	store *st.MPTStateStore, startBlock uint64, initialRoot common.Hash,
	blockInfoCh <-chan blockInfo, updateCh <-chan updateForward,
	csvWriter *benchutil.BenchCSVWriter,
	updateMetrics *benchutil.UpdateMetricsCollector,
) {
	currentRoot := initialRoot

	var hangChan chan blockInfo
	_hangChan := (<-chan blockInfo)(hangChan)

	var blockInfoChClosed bool

	buffered := make(map[uint64][]updateForward)
	bufferedFirstTime := make(map[uint64]time.Time)
	blockInfoBuffered := make(map[uint64]blockInfo)
	pendingCount := defaultDict{internal: make(map[uint64]uint64)}

	const flushInterval = 1024
	const logInterval = 1000
	batch := store.DiskDB.NewBatch()
	blocksProcessed := uint64(0)
	var lastBlockNumber uint64
	logStart := time.Now()

	currentBlock := startBlock

outerLoop:
	for {
		var activeBlockInfoCh *<-chan blockInfo
		if blockInfoChClosed {
			activeBlockInfoCh = &_hangChan
		} else {
			activeBlockInfoCh = mayHang(&blockInfoCh, &_hangChan, len(blockInfoBuffered), maxHangLen)
		}

		select {
		case bi, ok := <-*activeBlockInfoCh:
			if !ok {
				blockInfoChClosed = true
				continue
			}

			pending := bi.updateCount
			pendingCount.Set(bi.blockNumber, pending)
			blockInfoBuffered[bi.blockNumber] = bi

			if _, ok := buffered[bi.blockNumber]; !ok {
				buffered[bi.blockNumber] = []updateForward{}
			}

		case fwd, ok := <-updateCh:
			if !ok {
				break outerLoop
			}
			if buf, ok := buffered[fwd.blockNumber]; ok {
				buffered[fwd.blockNumber] = append(buf, fwd)
			} else {
				buffered[fwd.blockNumber] = []updateForward{fwd}
			}

			if _, ok := bufferedFirstTime[fwd.blockNumber]; !ok {
				bufferedFirstTime[fwd.blockNumber] = time.Now()
			}
		}

		for {
			created := maybeCreateNewBlock(
				currentBlock, &pendingCount, &buffered, &blockInfoBuffered,
				&currentRoot, store, csvWriter, updateMetrics,
				&blocksProcessed, &lastBlockNumber,
				flushInterval, logInterval, &logStart, &batch, &bufferedFirstTime,
			)
			if !created {
				break
			}
			currentBlock++
		}
	}

	if blocksProcessed > 0 {
		meta.PutLastBatch(batch, lastBlockNumber)
		if err := batch.Write(); err != nil {
			panic(fmt.Sprintf("final batch write: %v", err))
		}
		if err := store.FlushTrieDB(currentRoot); err != nil {
			panic(fmt.Sprintf("final triedb flush: %v", err))
		}
	}
}

func maybeCreateNewBlock(
	currentBlock uint64, pendingCount *defaultDict,
	buffered *map[uint64][]updateForward, blockInfoBuffered *map[uint64]blockInfo,
	currentRoot *common.Hash, store *st.MPTStateStore,
	csvWriter *benchutil.BenchCSVWriter,
	updateMetrics *benchutil.UpdateMetricsCollector,
	blocksProcessed *uint64, lastBlockNumber *uint64,
	flushInterval uint64, logInterval uint64, logStart *time.Time,
	batch *ethdb.Batch,
	bufferedFirstTime *map[uint64]time.Time,
) bool {
	if _, ok := (*buffered)[currentBlock]; !ok {
		return false
	}

	buffer := (*buffered)[currentBlock]
	pending := pendingCount.Get(currentBlock)
	bi := (*blockInfoBuffered)[currentBlock]

	if uint64(len(buffer)) < pending {
		return false
	}

	stateDB, err := store.OpenState(*currentRoot)
	if err != nil {
		panic(fmt.Sprintf("block %d: open state: %v", currentBlock, err))
	}

	defer pendingCount.Delete(currentBlock)
	defer delete(*buffered, currentBlock)
	defer delete(*blockInfoBuffered, currentBlock)
	if _, ok := (*bufferedFirstTime)[currentBlock]; ok {
		// if time.Since(ft) > 10*time.Millisecond {
		// 	fmt.Println("Time taken to buffer first time:", time.Since(ft))
		// }
		defer delete(*bufferedFirstTime, currentBlock)
	}

	startAtNs := time.Now().UnixNano()

	for _, u := range buffer {
		st.SetAccountBalance(stateDB, u.address, u.balance)
	}

	newRoot, err := store.CommitState(stateDB, *currentRoot, currentBlock)
	if err != nil {
		panic(fmt.Sprintf("block %d: commit: %v", currentBlock, err))
	}
	meta.PutRootBatch(*batch, currentBlock, newRoot)

	if err := store.FlushTrieDB(newRoot); err != nil {
		panic(fmt.Sprintf("block %d: triedb flush: %v", currentBlock, err))
	}

	completedAtNs := time.Now().UnixNano()

	if updateMetrics != nil && pending > 0 {
		updateMetrics.RecordN(pending, completedAtNs-startAtNs)
	}

	_ = csvWriter.WriteRow(
		strconv.FormatUint(currentBlock, 10),
		strconv.FormatUint(bi.rawUpdateCount, 10),
		strconv.FormatUint(pending, 10),
		strconv.FormatInt(bi.queuedAtNs, 10),
		strconv.FormatInt(startAtNs, 10),
		strconv.FormatInt(completedAtNs, 10),
	)

	*currentRoot = newRoot
	*lastBlockNumber = bi.blockNumber
	(*blocksProcessed)++

	if (*blocksProcessed)%flushInterval == 0 {
		if err := (*batch).Write(); err != nil {
			panic(fmt.Sprintf("batch write at block %d: %v", bi.blockNumber, err))
		}
		(*batch).Reset()
	}

	if (*blocksProcessed)%logInterval == 0 {
		fmt.Printf("[MPT] blk=%d elapsed=%s buffered=%d elapsed2=%dms\n",
			bi.blockNumber,
			time.Since(*logStart).Truncate(time.Millisecond),
			len(*buffered),
			(completedAtNs-startAtNs)/1e6,
		)
		*logStart = time.Now()
	}

	return true
}

// BenchRun performs block ingestion for a fixed duration using a sharded
// producer/worker/MPT pipeline and writes per-block timing data to a CSV.
func BenchRun(cfg BenchConfig) error {
	// --- load hot-account filter ---
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

	// --- create CSV writer ---
	csvWriter, err := benchutil.NewBenchCSVWriter(cfg.OutCSV, benchutil.IngestionCSVHeader)
	if err != nil {
		return err
	}
	defer csvWriter.Close()

	// --- setup update-level metrics collector ---
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

	// --- determine start block and current root ---
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

	end := cfg.End
	deadline := time.Now().Add(cfg.Duration)

	// --- create pipeline plumbing ---
	numShards := cfg.NumShards
	// if numShards < 1 {
	// 	numShards = 1
	// }

	queues := make([]*chann.Chann[updateTask], numShards)
	for i := range queues {
		queues[i] = chann.New[updateTask]()
	}

	blockInfoCh := make(chan blockInfo, 1)
	updateCh := make(chan updateForward, 10000)

	var shardWG sync.WaitGroup
	var mptWG sync.WaitGroup

	startShardWorkers(queues, updateCh, &shardWG)

	mptWG.Add(1)
	go func() {
		defer mptWG.Done()
		runMPTWorker(cfg.Store, start, currentRoot, blockInfoCh, updateCh, csvWriter, updateMetrics)
	}()

	benchStart := time.Now()
	log.Printf("[bench] starting at block %d, duration %s, shards %d (output: %s)",
		start, cfg.Duration, numShards, cfg.OutCSV)

	// --- run producer (blocking) ---
	prodErr := produceBlocks(cfg.BlocksDir, start, end, filter, deadline, blockInfoCh, queues, numShards)

	// --- shutdown pipeline ---
	for _, q := range queues {
		q.Close()
	}
	close(blockInfoCh)

	shardWG.Wait()
	close(updateCh)

	mptWG.Wait()

	// --- interpret result ---
	if prodErr != nil && !errors.Is(prodErr, errDurationExceeded) {
		return prodErr
	}

	elapsed := time.Since(benchStart)
	log.Printf("[bench] complete: %s wall-clock, output=%s",
		elapsed.Round(time.Millisecond), cfg.OutCSV)

	return nil
}
