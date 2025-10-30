package segmenttree

import (
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/maypok86/otter/v2"
	"github.com/nepal80m/samurai/internal/config"
)

// OtterCache is an alternative in-memory cache backed by the Otter library.
// It preserves the high-level semantics of the existing Cache:
// - keep AccountInfo in memory for fast, repeated updates
// - track dirty entries and flush them to Pebble in the background
// - support periodic/bulk flush triggers
//
// NOTE: This implementation intentionally does not force eviction-time flushes
// via a callback. Configure capacity high enough to avoid evictions during a run,
// and rely on periodic/bulk flushes. If eviction-time flushing is required, wire
// up Otter's deletion callback to call flushOne for the evicted key.
type OtterCache struct {
	db              *pebble.DB
	precomputedData *config.PrecomputedData

	cache *otter.Cache[common.Address, *AccountInfo]

	// background flushing
	flushInterval            time.Duration
	stopCh                   chan struct{}
	wg                       sync.WaitGroup
	maxDirtyEntriesCount     uint64
	maxUpdatesSinceLastFlush uint64

	// dirty tracking and flush coordination
	dirtyMu               sync.Mutex
	dirty                 map[common.Address]uint64 // address -> last version queued
	updatesSinceLastFlush uint64
}

// accountInfoApproxWeightBytes returns an approximate size for an AccountInfo entry.
// This intentionally overestimates to keep overall memory under the configured budget.
func accountInfoApproxWeightBytes() uint32 {
	// LXBatchTree: MaxLayer * SegmentTreeSize * 32 bytes
	// Add ~10% headroom for other fields/overhead
	const perEntry = MaxLayer * SegmentTreeSize * 32
	const headroom = perEntry / 10
	return uint32(perEntry + headroom)
}

func NewOtterCache(db *pebble.DB, capacity, maxDirtyEntriesCount, maxUpdatesSinceLastFlush uint64, flushInterval time.Duration, precomputedData *config.PrecomputedData) *OtterCache {
	// Build an Otter cache with the requested capacity. No TTL; evictions happen by size.
	oc := &OtterCache{
		db:                       db,
		precomputedData:          precomputedData,
		flushInterval:            flushInterval,
		stopCh:                   make(chan struct{}),
		maxDirtyEntriesCount:     maxDirtyEntriesCount,
		maxUpdatesSinceLastFlush: maxUpdatesSinceLastFlush,
		dirty:                    make(map[common.Address]uint64, 1024),
	}
	// Use weight-based capacity: approximate each AccountInfo size with headroom.
	// capacity here is intended as approximate number of entries.
	maxWeight := uint64(capacity) * uint64(accountInfoApproxWeightBytes())
	oc.cache = otter.Must(&otter.Options[common.Address, *AccountInfo]{
		MaximumWeight: maxWeight,
		Weigher:       func(_ common.Address, _ *AccountInfo) uint32 { return accountInfoApproxWeightBytes() },
		OnDeletion: func(e otter.DeletionEvent[common.Address, *AccountInfo]) {
			if !e.Cause.IsEviction() {
				return
			}
			// Best-effort flush on eviction if marked dirty. Runs on Otter's executor (goroutine),
			// so it won't block the eviction path and avoids memory buildup during evictions.
			oc.dirtyMu.Lock()
			_, isDirty := oc.dirty[e.Key]
			oc.dirtyMu.Unlock()
			if isDirty && e.Value != nil {
				StoreAccountInfo(e.Value, oc.db)
				oc.dirtyMu.Lock()
				delete(oc.dirty, e.Key)
				oc.dirtyMu.Unlock()
			}
		},
	})
	oc.wg.Add(1)
	go oc.flusher()
	return oc
}

// NewOtterCacheWithMemoryBudget constructs an OtterCache bounded by a memory budget in bytes.
// This avoids ambiguity around interpreting capacity as entry count. The budget is approximate
// and conservative due to the overestimating weigher.
func NewOtterCacheWithMemoryBudget(db *pebble.DB, budgetBytes uint64, maxDirtyEntriesCount, maxUpdatesSinceLastFlush uint64, flushInterval time.Duration, precomputedData *config.PrecomputedData) *OtterCache {
	oc := &OtterCache{
		db:                       db,
		precomputedData:          precomputedData,
		flushInterval:            flushInterval,
		stopCh:                   make(chan struct{}),
		maxDirtyEntriesCount:     maxDirtyEntriesCount,
		maxUpdatesSinceLastFlush: maxUpdatesSinceLastFlush,
		dirty:                    make(map[common.Address]uint64, 1024),
	}
	oc.cache = otter.Must(&otter.Options[common.Address, *AccountInfo]{
		MaximumWeight: budgetBytes,
		Weigher:       func(_ common.Address, _ *AccountInfo) uint32 { return accountInfoApproxWeightBytes() },
		OnDeletion: func(e otter.DeletionEvent[common.Address, *AccountInfo]) {
			if !e.Cause.IsEviction() {
				return
			}
			oc.dirtyMu.Lock()
			_, isDirty := oc.dirty[e.Key]
			oc.dirtyMu.Unlock()
			if isDirty && e.Value != nil {
				StoreAccountInfo(e.Value, oc.db)
				oc.dirtyMu.Lock()
				delete(oc.dirty, e.Key)
				oc.dirtyMu.Unlock()
			}
		},
	})
	oc.wg.Add(1)
	go oc.flusher()
	return oc
}

func (c *OtterCache) Close() {
	close(c.stopCh)
	c.wg.Wait()
}

// Get returns a deep copy of AccountInfo if present; otherwise loads from DB,
// stores in cache, and returns the loaded copy.
// Returns pebble.ErrNotFound if the account is not found in DB.
func (c *OtterCache) Get(id common.Address) (*AccountInfo, error) {
	if v, ok := c.cache.GetIfPresent(id); ok {
		return v.DeepCopy(), nil
	}
	// cache miss: load from db via Loader
	v, err := c.cache.Get(context.Background(), id, otter.LoaderFunc[common.Address, *AccountInfo](func(ctx context.Context, key common.Address) (*AccountInfo, error) {
		cbInfo, err := GetCurrentBalanceInfo(key, c.db)
		if err != nil {
			return nil, err
		}
		batchTree := GetCurrentLXBatchTree(key, c.db)
		batchCommitments := GetLXBatchCommitments(key, cbInfo.Version, c.db)
		return &AccountInfo{
			Account:                  key,
			CurrentBalanceInfo:       cbInfo,
			CurrentLXBatchTree:       batchTree,
			CurrentLXBatchCommitment: batchCommitments,
			PrecomputedData:          c.precomputedData,
		}, nil
	}))
	if err != nil {
		return nil, err
	}
	return v.DeepCopy(), nil
}

// Set marks the entry dirty and stores it in the cache. It may trigger a background flush.
func (c *OtterCache) Set(id common.Address, accountInfo *AccountInfo) {
	c.cache.Set(id, accountInfo)

	c.dirtyMu.Lock()
	c.dirty[id] = accountInfo.CurrentBalanceInfo.Version
	c.updatesSinceLastFlush++
	shouldFlush := c.updatesSinceLastFlush >= c.maxUpdatesSinceLastFlush || uint64(len(c.dirty)) >= c.maxDirtyEntriesCount
	c.dirtyMu.Unlock()

	if shouldFlush {
		c.flushSome(false)
	}
}

func (c *OtterCache) flusher() {
	t := time.NewTicker(c.flushInterval)
	defer t.Stop()
	defer c.wg.Done()
	for {
		select {
		case <-t.C:
			c.flushSome(false)
		case <-c.stopCh:
			c.flushSome(true)
			return
		}
	}
}

func (c *OtterCache) flushSome(full bool) {
	// snapshot keys to flush
	c.dirtyMu.Lock()
	if len(c.dirty) == 0 {
		c.dirtyMu.Unlock()
		return
	}
	// choose up to half of dirty entries unless full flush requested
	toFlush := len(c.dirty)
	if !full {
		if toFlush > 1 {
			toFlush = toFlush / 2
		}
	}
	type item struct {
		id      common.Address
		version uint64
	}
	batch := make([]item, 0, toFlush)
	for id, ver := range c.dirty {
		batch = append(batch, item{id: id, version: ver})
		if len(batch) >= toFlush && !full {
			break
		}
	}
	c.dirtyMu.Unlock()

	if len(batch) == 0 {
		return
	}

	b := c.db.NewBatch()
	defer b.Close()

	// collect values and write
	for _, it := range batch {
		if v, ok := c.cache.GetIfPresent(it.id); ok {
			BatchStoreAccountInfo(v, b)
		}
	}
	if err := b.Commit(pebble.Sync); err != nil {
		panic(err)
	}

	// finalize: clear dirty for versions that match what we wrote
	c.dirtyMu.Lock()
	for _, it := range batch {
		if v, ok := c.cache.GetIfPresent(it.id); ok {
			if v.CurrentBalanceInfo.Version == it.version {
				delete(c.dirty, it.id)
			}
		} else {
			// value no longer present; consider the entry flushed for this version
			delete(c.dirty, it.id)
		}
	}
	c.updatesSinceLastFlush = 0
	c.dirtyMu.Unlock()
}

// NewCreateOrUpdateAccountInfoOtter mirrors NewCreateOrUpdateAccountInfo semantics using OtterCache.
// It creates or updates the AccountInfo for the given account, applies the update in-memory,
// and returns the final commitment hash. Writes of the current state to DB are deferred to the
// background flusher; only historical balance is written synchronously inside Update.
func NewCreateOrUpdateAccountInfoOtter(account common.Address, balance *big.Int, blockNumber uint64, cache *OtterCache) common.Hash {
	var commitmentHash common.Hash
	var updated *AccountInfo

	_, _ = cache.cache.Compute(account, func(oldValue *AccountInfo, found bool) (*AccountInfo, otter.ComputeOp) {
		if found {
			updated = oldValue
			updated.Update(blockNumber, balance, cache.db)
			commitmentHash = updated.CalculateFinalCommitment()
			return updated, otter.WriteOp
		}
		// Not found in cache: try DB
		cbInfo, err := GetCurrentBalanceInfo(account, cache.db)
		if err != nil {
			if err != pebble.ErrNotFound {
				panic(err)
			}
			// Not in DB: create fresh AccountInfo
			updated = NewAccountInfo(account, balance, blockNumber, cache.precomputedData)
			commitmentHash = updated.CalculateFinalCommitment()
			return updated, otter.WriteOp
		}
		// Found in DB: hydrate and then update
		batchTree := GetCurrentLXBatchTree(account, cache.db)
		batchCommitments := GetLXBatchCommitments(account, cbInfo.Version, cache.db)
		updated = &AccountInfo{
			Account:                  account,
			CurrentBalanceInfo:       cbInfo,
			CurrentLXBatchTree:       batchTree,
			CurrentLXBatchCommitment: batchCommitments,
			PrecomputedData:          cache.precomputedData,
		}
		updated.Update(blockNumber, balance, cache.db)
		commitmentHash = updated.CalculateFinalCommitment()
		return updated, otter.WriteOp
	})

	// Mark dirty and maybe flush
	if updated != nil {
		cache.dirtyMu.Lock()
		cache.dirty[account] = updated.CurrentBalanceInfo.Version
		cache.updatesSinceLastFlush++
		shouldFlush := cache.updatesSinceLastFlush >= cache.maxUpdatesSinceLastFlush || uint64(len(cache.dirty)) >= cache.maxDirtyEntriesCount
		cache.dirtyMu.Unlock()
		if shouldFlush {
			cache.flushSome(false)
		}
	}

	return commitmentHash
}
