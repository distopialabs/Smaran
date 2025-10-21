package segmenttree

import (
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
)

type CacheEntry struct {
	key common.Address
	val *AccountInfo

	dirty bool
	size  uint64

	prev, next *CacheEntry // doubly linked list
}

type Cache struct {
	mu sync.RWMutex

	entries    map[common.Address]*CacheEntry
	head, tail *CacheEntry

	// bytesUsed             uint64
	// maxBytes                 uint64
	entriesCount    uint64
	maxEntriesCount uint64 // max number of entries allowed in cache (dirty or clean)

	// dirtyBytes            uint64
	// maxDirtyBytes            uint64
	dirtyEntriesCount    uint64
	maxDirtyEntriesCount uint64 // max number of dirty entries allowed

	updatesSinceLastFlush    uint64
	maxUpdatesSinceLastFlush uint64 // max number of updates allowed since last flush

	flushInterval time.Duration
	// flushTimer    *time.Timer

	db *pebble.DB

	stopCh chan struct{}
}

// TODO: check if this is correct
func NewCache(db *pebble.DB, maxEntriesCount, maxDirtyEntriesCount, maxUpdatesSinceLastFlush uint64, flushInterval time.Duration) *Cache {
	cache := &Cache{
		entries:                  make(map[common.Address]*CacheEntry, 1<<16),
		maxEntriesCount:          maxEntriesCount,
		maxDirtyEntriesCount:     maxDirtyEntriesCount,
		maxUpdatesSinceLastFlush: maxUpdatesSinceLastFlush,
		flushInterval:            flushInterval,
		db:                       db,
		stopCh:                   make(chan struct{}),
	}
	go cache.flusher()
	return cache
}

func (c *Cache) Close() {
	close(c.stopCh)
}

// checks if the account info is in the cache, and returns it if it is. otherwise, loads it from the db and adds it to the cache.
func (c *Cache) Get(id common.Address) (*AccountInfo, error) {
	c.mu.RLock()
	if entry, ok := c.entries[id]; ok {
		c.moveToHead(entry)
		// TODO: check if i am making a copy correctly. or do i need a deep copy at all?
		cb := entry.val.CurrentBalanceInfo
		currentBatchTree := entry.val.CurrentBatchTree
		// var currentBatchTree BatchTree
		// for i := range MaxLayer {
		// 	currentBatchTree[i] = make([]common.Hash, SegmentTreeSize)
		// 	copy(currentBatchTree[i], entry.val.CurrentBatchTree[i])
		// }
		currentBatchTreeCommitments := entry.val.CurrentBatchTreeCommitments
		// var currentBatchTreeCommitments BatchCommitments
		// for i := range MaxLayer {
		// 	currentBatchTreeCommitments[i] = entry.val.CurrentBatchTreeCommitments[i]
		// }

		accountInfo := &AccountInfo{
			Account:                     id,
			CurrentBalanceInfo:          cb,
			CurrentBatchTree:            currentBatchTree,
			CurrentBatchTreeCommitments: currentBatchTreeCommitments,
		}

		c.mu.RUnlock()
		return accountInfo, nil

	}

	c.mu.RUnlock()
	// cache miss: load from db
	cbInfo, err := GetCurrentBalanceInfo(id, c.db)
	if err != nil {
		// either key not found or error
		return nil, err
	}
	batchTree := GetCurrentBatchTree(id, c.db)
	batchCommitments := GetBatchCommitments(id, cbInfo.Version, c.db)

	c.mu.Lock()
	e := c.entries[id]
	if e != nil {
		panic("entry already exists; looks like a concurrent write to the same account")
	}
	accountInfo := &AccountInfo{
		Account:                     id,
		CurrentBalanceInfo:          cbInfo,
		CurrentBatchTree:            *batchTree,
		CurrentBatchTreeCommitments: *batchCommitments,
	}
	e = &CacheEntry{
		key: id,
		val: accountInfo,
	}
	c.entries[id] = e
	c.attach(e)
	c.mu.Unlock()
	return accountInfo, nil
}

// TODO: check if this is correct
func (c *Cache) Update(id common.Address, mutate func(*CurrentBalance) *CurrentBalance) error {
	c.mu.Lock()
	e := c.entries[id]
	if e == nil {
		c.mu.Unlock()
		ce, err := c.Get(id)
		if err != nil {
			return err
		}
		c.mu.Lock()
		e = c.entries[id]
		if e == nil {
			e = &CacheEntry{
				key:                         id,
				CurrentBalance:              ce.CurrentBalance,
				CurrentBatchTree:            ce.CurrentBatchTree,
				CurrentBatchTreeCommitments: ce.CurrentBatchTreeCommitments,
			}
			c.entries[id] = e
			c.attach(e)
			c.bytesUsed += uint64(e.size)
		}
	}
	newCurrentBalance := mutate(e.CurrentBalance)
	c.dirtyBytes += uint64(e.size) - uint64(ce.size)
	if !e.dirty {
		e.dirty = true
		c.dirtyBytes += uint64(e.size)
	} else {
		c.dirtyBytes += uint64(e.size) - uint64(ce.size)
	}

	e.CurrentBalance = newCurrentBalance
	c.updatesSinceLastFlush++
	c.moveToHead(e)

	c.evictIfNeeded()

	doFlush := c.updatesSinceLastFlush >= c.maxUpdatesSinceLastFlush || c.dirtyBytes >= c.maxDirtyBytes
	c.mu.Unlock()
	if doFlush {
		c.flushSome(false)
	}

	return nil
}

func (c *Cache) flusher() {
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

// TODO: check if this is correct
func (c *Cache) flushSome(full bool) {
	c.mu.Lock()
	list := make([]*CacheEntry, 0, 8192)
	for e := c.head; e != nil; e = e.next {
		if e.dirty {
			list = append(list, e)
		}
		if !full && len(list) >= 8192 {
			break
		}
	}
	c.mu.Unlock()

	if len(list) == 0 {
		return
	}
	b := c.db.NewBatch()
	for _, e := range list {
		// TODO:store account info in db
	}
	if err := b.Commit(pebble.Sync); err != nil {
		return
	}

	c.mu.Lock()
	for _, e := range list {
		if e.dirty {
			e.dirty = false
			c.dirtyBytes -= uint64(e.size)
		}
	}
	c.updatesSinceLastFlush = 0
	c.mu.Unlock()
}

// TODO: check if this is correct
func (c *Cache) evictIfNeeded() {
	for c.bytesUsed > c.maxBytes {
		e := c.tail
		if e == nil {
			return
		}
		if e.dirty {
			// TODO: flush dirty entries
		}
		c.detach(e)
		delete(c.entries, e.key)
		c.bytesUsed -= uint64(e.size)
	}
}

func (c *Cache) attach(e *CacheEntry) {
	if c.tail == nil {
		c.head = e
		c.tail = e
	} else {
		c.tail.next = e
		e.prev = c.tail
		c.tail = e
	}
	// TODO: check if i want this here or in the caller
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
	if c.head == e {
		return
	}
	if c.tail == e {
		c.tail = e.prev
	}
	e.prev.next = e.next
	e.next.prev = e.prev
	e.prev = nil
	e.next = c.head
	c.head.prev = e
	c.head = e
}
