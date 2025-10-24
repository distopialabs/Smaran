package segmenttree

import (
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
)

type CacheEntry struct {
	mu sync.RWMutex

	key common.Address
	val *AccountInfo // TODO: do i need a pointer here? or can i just use the value directly?

	dirty bool

	prev, next *CacheEntry // doubly linked list
}

type Cache struct {
	mu sync.RWMutex

	entries    map[common.Address]*CacheEntry
	head, tail *CacheEntry

	entriesCount    uint64
	maxEntriesCount uint64 // max number of entries allowed in cache (dirty or clean)

	dirtyEntriesCount    uint64
	maxDirtyEntriesCount uint64 // max number of dirty entries allowed

	updatesSinceLastFlush    uint64
	maxUpdatesSinceLastFlush uint64 // max number of updates allowed since last flush

	flushInterval time.Duration

	db              *pebble.DB
	precomputedData *config.PrecomputedData

	stopCh chan struct{}

	wg sync.WaitGroup
}

func NewCache(db *pebble.DB, maxEntriesCount, maxDirtyEntriesCount, maxUpdatesSinceLastFlush uint64, flushInterval time.Duration, precomputedData *config.PrecomputedData) *Cache {
	cache := &Cache{
		// TODO: tune this
		entries:                  make(map[common.Address]*CacheEntry, maxEntriesCount+100),
		maxEntriesCount:          maxEntriesCount,
		maxDirtyEntriesCount:     maxDirtyEntriesCount,
		maxUpdatesSinceLastFlush: maxUpdatesSinceLastFlush,
		flushInterval:            flushInterval,
		db:                       db,
		precomputedData:          precomputedData,
		stopCh:                   make(chan struct{}),
	}
	cache.wg.Add(1)
	go cache.flusher()
	return cache
}

func (c *Cache) Close() {
	fmt.Println("Closing cache", time.Now())
	close(c.stopCh)
	c.wg.Wait()
	fmt.Println("Cache closed", time.Now())
}

// checks if the account info is in the cache, and returns it if it is. otherwise, loads it from the db and adds it to the cache.
// returns pebble.ErrNotFound if the account info is not found in the db. panics if there is an error other than pebble.ErrNotFound.
func (c *Cache) Get(id common.Address) (*AccountInfo, error) {
	c.mu.Lock()
	if entry, ok := c.entries[id]; ok {
		entry.mu.RLock()
		c.moveToHead(entry)
		accountInfoCopy := entry.val.DeepCopy() // am i making a copy correctly here?
		entry.mu.RUnlock()
		c.mu.Unlock()
		return accountInfoCopy, nil
	}
	c.mu.Unlock()
	// cache miss: load from db
	cbInfo, err := GetCurrentBalanceInfo(id, c.db)
	if err != nil {
		// key not found in db
		return nil, err
	}
	batchTree := GetCurrentLXBatchTree(id, c.db)
	batchCommitments := GetLXBatchCommitments(id, cbInfo.Version, c.db)

	c.mu.Lock()
	e := c.entries[id]
	accountInfo := &AccountInfo{
		Account:                  id,
		CurrentBalanceInfo:       cbInfo,
		CurrentLXBatchTree:       batchTree,
		CurrentLXBatchCommitment: batchCommitments,
		PrecomputedData:          c.precomputedData,
	}

	if e == nil {
		// still a miss
		e = &CacheEntry{
			mu:  sync.RWMutex{},
			key: id,
			val: accountInfo,
		}
		c.entries[id] = e
		c.attach(e)
		c.evictIfNeeded()
	} else {
		// TODO: entry already exists; looks like a concurrent update to the same account
		// panic("concurrent update on same account detected; this should never happen")
		e.mu.RLock()
		accountInfo = e.val.DeepCopy()
		e.mu.RUnlock()
		c.moveToHead(e)
	}
	c.mu.Unlock()

	return accountInfo, nil

}

func (c *Cache) Set(id common.Address, accountInfo *AccountInfo) {
	c.mu.Lock()
	e := c.entries[id]

	if e == nil {
		// new cache entry (dirty)
		ne := &CacheEntry{
			mu:    sync.RWMutex{},
			key:   id,
			val:   accountInfo,
			dirty: true,
		}
		c.entries[id] = ne
		c.attach(ne)
		c.dirtyEntriesCount++
		c.evictIfNeeded()
	} else {
		// update existing cache entry
		e.mu.Lock()
		if e.val.CurrentBalanceInfo.Version != accountInfo.CurrentBalanceInfo.Version-1 {
			// account info was already updated by another concurrent update before this one got completed.
			panic(fmt.Errorf("account info version mismatch: expected %d, got %d", accountInfo.CurrentBalanceInfo.Version-1, e.val.CurrentBalanceInfo.Version))
		}
		e.val = accountInfo
		if !e.dirty {
			e.dirty = true
			c.dirtyEntriesCount++
		}
		e.mu.Unlock()
		c.moveToHead(e)
	}
	c.updatesSinceLastFlush++
	shouldFlush := c.updatesSinceLastFlush >= c.maxUpdatesSinceLastFlush || c.dirtyEntriesCount >= c.maxDirtyEntriesCount

	c.mu.Unlock()

	if shouldFlush {
		c.flushSome(false)
	}
}

func (c *Cache) flusher() {
	defer c.wg.Done()
	t := time.NewTicker(c.flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			c.flushSome(false)
		case <-c.stopCh:
			c.flushSome(true) // flush all remaining entries
			return
		}
	}
}

func (c *Cache) flushOne(e *CacheEntry) error {
	e.mu.Lock()
	StoreAccountInfo(e.val, c.db)
	if e.dirty {
		e.dirty = false
		c.dirtyEntriesCount--
	}
	e.mu.Unlock()
	return nil
}

func (c *Cache) flushSome(full bool) {
	c.mu.Lock()
	// TODO: tune this
	var toFlush int
	if c.dirtyEntriesCount < 2 {
		toFlush = int(c.dirtyEntriesCount)
	} else {
		toFlush = int(c.dirtyEntriesCount / 2)
	}

	if toFlush == 0 {
		c.mu.Unlock()
		return
	}

	type flushEntry struct {
		e        *CacheEntry
		snapshot *AccountInfo
		version  uint64
	}
	flushEntries := make([]flushEntry, 0, toFlush)

	for e := c.tail; e != nil; e = e.prev {
		if e.dirty {
			e.mu.Lock()
			// TODO: no need to keep a snapshot here. we can just use the value directly. we have a lock on the entry here.
			flushEntries = append(flushEntries, flushEntry{
				e:        e,
				snapshot: e.val, // TODO: do i need a deep copy here?
				version:  e.val.CurrentBalanceInfo.Version,
			})
		}
		if !full && len(flushEntries) >= toFlush {
			break
		}
	}
	c.mu.Unlock()

	if len(flushEntries) == 0 {
		return
	}
	b := c.db.NewBatch()
	defer b.Close()
	for _, fe := range flushEntries {
		BatchStoreAccountInfo(fe.snapshot, b)
	}
	if err := b.Commit(pebble.Sync); err != nil {
		panic(fmt.Errorf("failed to commit batch: %w", err))
	}

	c.mu.Lock()
	for _, fe := range flushEntries {
		if fe.e.dirty && fe.e.val.CurrentBalanceInfo.Version == fe.version {
			fe.e.dirty = false
			fe.e.mu.Unlock()
			c.dirtyEntriesCount--
		} else {
			fe.e.mu.Unlock()
			panic("I had a lock, how is this possible?")
		}
	}
	c.updatesSinceLastFlush = 0
	c.mu.Unlock()
}

func (c *Cache) evictIfNeeded() {
	for c.entriesCount > c.maxEntriesCount {
		// c.detachAndDelete(c.tail)
		e := c.tail
		if e == nil {
			break
		}
		if e.dirty {
			c.flushOne(e)
		}
		c.detach(e)
		delete(c.entries, e.key)
	}
}

// attach to head of the LRU list
func (c *Cache) attach(e *CacheEntry) {
	if c.head == nil {
		c.head = e
		c.tail = e
	} else {
		e.next = c.head
		e.prev = nil
		c.head.prev = e
		c.head = e
	}
	c.entriesCount++

}

func (c *Cache) detach(e *CacheEntry) {

	if c.head == e {
		if c.head == c.tail {
			c.head = nil
			c.tail = nil
		} else {
			c.head = e.next
			c.head.prev = nil
		}
	} else if c.tail == e {
		c.tail = e.prev
		c.tail.next = nil
	} else {
		e.prev.next = e.next
		e.next.prev = e.prev
	}
	// TODO: check if i want this here or in the caller
	c.entriesCount--

}
func (c *Cache) moveToHead(e *CacheEntry) {

	if e == nil {
		panic("entry is nil")
	}
	if c.head == e {
		return
	}
	// unlink e
	if e.prev != nil {
		e.prev.next = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		// e was tail
		c.tail = e.prev
	}

	// insert e at the head
	e.prev = nil
	e.next = c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e

	// if the list empty after unlinking, e is also the tail
	if c.tail == nil {
		c.tail = e
	}
}

// // TODO: check if this is correct
// func (c *Cache) Update(id common.Address, mutate func(*CurrentBalance) *CurrentBalance) error {
// 	c.mu.Lock()
// 	e := c.entries[id]
// 	if e == nil {
// 		c.mu.Unlock()
// 		ce, err := c.Get(id)
// 		if err != nil {
// 			return err
// 		}
// 		c.mu.Lock()
// 		e = c.entries[id]
// 		if e == nil {
// 			e = &CacheEntry{
// 				key:                         id,
// 				CurrentBalance:              ce.CurrentBalance,
// 				CurrentBatchTree:            ce.CurrentBatchTree,
// 				CurrentBatchTreeCommitments: ce.CurrentBatchTreeCommitments,
// 			}
// 			c.entries[id] = e
// 			c.attach(e)
// 			c.bytesUsed += uint64(e.size)
// 		}
// 	}
// 	newCurrentBalance := mutate(e.CurrentBalance)
// 	c.dirtyBytes += uint64(e.size) - uint64(ce.size)
// 	if !e.dirty {
// 		e.dirty = true
// 		c.dirtyBytes += uint64(e.size)
// 	} else {
// 		c.dirtyBytes += uint64(e.size) - uint64(ce.size)
// 	}

// 	e.CurrentBalance = newCurrentBalance
// 	c.updatesSinceLastFlush++
// 	c.moveToHead(e)

// 	c.evictIfNeeded()

// 	doFlush := c.updatesSinceLastFlush >= c.maxUpdatesSinceLastFlush || c.dirtyBytes >= c.maxDirtyBytes
// 	c.mu.Unlock()
// 	if doFlush {
// 		c.flushSome(false)
// 	}

// 	return nil
// }
