package segmenttree

import (
	"fmt"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
	ristretto "github.com/dgraph-io/ristretto/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
)

// Per-key lock using sync.Map for lock storage
type keyLocks struct {
	locks sync.Map // map[string]*sync.Mutex
}

func (kl *keyLocks) lock(k common.Address) {
	key := string(k[:]) // use string as map key
	mu, _ := kl.locks.LoadOrStore(key, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
}

func (kl *keyLocks) unlock(k common.Address) {
	key := string(k[:])
	if mu, ok := kl.locks.Load(key); ok {
		mu.(*sync.Mutex).Unlock()
	}
}

type Cache struct {
	rc *ristretto.Cache[[]byte, *AccountInfo]
	wb *Batcher
	db *pebble.DB

	klocks keyLocks

	precomputedData *config.PrecomputedData
}

func NewCache(db *pebble.DB, precomputedData *config.PrecomputedData) (*Cache, error) {
	wb := NewBatcher(db)
	rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *AccountInfo]{
		NumCounters: 1 << 21, // TODO: recommended is 10x maxCost (2^18)
		MaxCost:     1 << 15,
		BufferItems: 64,
		// Cost:        func(v *AccountInfo) int64 { return 1 },
		// OnEvict: func(val *ristretto.Item[*AccountInfo]) {
		// 	fmt.Println("OnEvict", val.Value.Account, val.Value.CurrentBalanceInfo.Version)
		// },
		// OnReject: func(val *ristretto.Item[*AccountInfo]) {
		// 	fmt.Println("OnReject", val.Value.Account, val.Value.CurrentBalanceInfo.Version)
		// },
		OnExit: func(val *AccountInfo) {
			if !val.Dirty {
				return
			}
			// snap := val.DeepCopy()
			// TODO: do i need a deep copy here?
			snap := val
			quitLog := logBlockedTime("Enqueue", 100*time.Millisecond)
			wb.Enqueue(KV{Key: val.Account, Version: val.CurrentBalanceInfo.Version, Snapshot: snap})
			close(quitLog)
		},
	})
	if err != nil {
		return nil, err
	}
	return &Cache{
		rc:              rc,
		wb:              wb,
		db:              db,
		precomputedData: precomputedData,
	}, nil
}

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address) *AccountInfo, mutate func(*AccountInfo)) (*AccountInfo, error) {
	// TODO: do i need to acquire a lock here?
	quitLog := logBlockedTime("AcquireLock", 100*time.Millisecond)
	c.klocks.lock(k)
	defer c.klocks.unlock(k)
	close(quitLog)

	var ai *AccountInfo
	// start := time.Now()
	if v, ok := c.rc.Get(k[:]); ok {
		// fmt.Println("Cache hit")
		ai = v
	} else {
		// fmt.Println("Cache miss")
		ai = loadFn(k)
		if ai == nil {
			ai = initFn(k)
		}
	}
	// fmt.Println("Time taken to get/init account info from cache/db", time.Since(start))
	mutate(ai)
	ai.Dirty = true

	quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	admitted := c.rc.Set(k[:], ai, 1)
	if !admitted {
		fmt.Println("❌Cache set rejected")
	}
	close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *Cache) Close() {
	c.rc.Close()
	c.wb.Close()
}
