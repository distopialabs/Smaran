package segmenttree

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	ristretto "github.com/dgraph-io/ristretto/v2"
	otter "github.com/maypok86/otter/v2"
	"github.com/maypok86/otter/v2/stats"

	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/nepal80m/samurai/internal/config"
)

type Cache struct {
	c  *ristretto.Cache[[]byte, *AccountInfo]
	db *pebble.DB

	precomputedData *config.PrecomputedData
}
type OtterCache struct {
	c  *otter.Cache[common.Address, *AccountInfo]
	db *pebble.DB

	precomputedData *config.PrecomputedData
}
type LRUCache struct {
	c  *lru.Cache[common.Address, *AccountInfo]
	db *pebble.DB

	precomputedData *config.PrecomputedData
}

func NewOtterCache(db *pebble.DB, precomputedData *config.PrecomputedData) (*OtterCache, error) {
	counter := stats.NewCounter()
	cache, err := otter.New(&otter.Options[common.Address, *AccountInfo]{
		MaximumSize:   32_768,
		StatsRecorder: counter,
	})
	if err != nil {
		return nil, err
	}

	return &OtterCache{
		c:               cache,
		db:              db,
		precomputedData: precomputedData,
	}, nil
}

func NewLRUCache(db *pebble.DB, precomputedData *config.PrecomputedData) (*LRUCache, error) {
	lruCache, err := lru.New[common.Address, *AccountInfo](32_768)
	if err != nil {
		return nil, err
	}
	return &LRUCache{
		c:               lruCache,
		db:              db,
		precomputedData: precomputedData,
	}, nil
}

func NewCache(db *pebble.DB, precomputedData *config.PrecomputedData) (*Cache, error) {
	rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *AccountInfo]{
		NumCounters: 1 << 21, // TODO: recommended is 10x maxCost (2^18)
		MaxCost:     1 << 15,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &Cache{
		c:               rc,
		db:              db,
		precomputedData: precomputedData,
	}, nil
}

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address) *AccountInfo, mutate func(*AccountInfo), seenAccount bool) (*AccountInfo, error) {

	var ai *AccountInfo
	// start := time.Now()
	if seenAccount {
		if v, ok := c.c.Get(k[:]); ok {
			// fmt.Println("Account", k.Hex(), " seen ", seenAccount found in cache")
			ai = v
		} else {
			// fmt.Println("Cache miss")
			ai = loadFn(k)
			if ai == nil {
				fmt.Println("Account not found in db, initializing")
				ai = initFn(k)
			} else {
				fmt.Println("Account found in db, loading")
			}
		}
	} else {
		ai = initFn(k)
	}
	// if v, ok := c.rc.Get(k[:]); ok {
	// 	// fmt.Println("Cache hit")
	// 	ai = v
	// } else {
	// 	// fmt.Println("Cache miss")
	// 	ai = loadFn(k)
	// 	if ai == nil {
	// 		ai = initFn(k)
	// 	}
	// }
	// fmt.Println(k.Hex(), "get/init time:", time.Since(start))
	// start = time.Now()
	mutate(ai)
	// fmt.Println(k.Hex(), "mutate time:", time.Since(start))
	// quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	// start = time.Now()
	admitted := c.c.Set(k[:], ai, 1)
	if !admitted {
		fmt.Println(k.Hex(), "❌Cache set rejected")
	}
	// fmt.Println(k.Hex(), "cache set time:", time.Since(start))
	// start := time.Now()
	// start = time.Now()
	ai.Save(c.db)
	// fmt.Println(k.Hex(), "save time:", time.Since(start))
	// close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *Cache) Close() {
	c.c.Close()
}

func (c *OtterCache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address) *AccountInfo, mutate func(*AccountInfo), seenAccount bool) (*AccountInfo, error) {

	var ai *AccountInfo
	// start := time.Now()
	if seenAccount {
		if v, ok := c.c.GetIfPresent(k); ok {
			// fmt.Println("Cache hit")
			ai = v
		} else {
			// fmt.Println("Cache miss")
			ai = loadFn(k)
			if ai == nil {
				fmt.Println("Account not found in db, initializing")
				ai = initFn(k)
			} else {
				fmt.Println("Account found in db, loading")
			}
		}
	} else {
		ai = initFn(k)
	}
	// if v, ok := c.rc.Get(k[:]); ok {
	// 	// fmt.Println("Cache hit")
	// 	ai = v
	// } else {
	// 	// fmt.Println("Cache miss")
	// 	ai = loadFn(k)
	// 	if ai == nil {
	// 		ai = initFn(k)
	// 	}
	// }
	// fmt.Println(k.Hex(), "get/init time:", time.Since(start))
	// start = time.Now()
	mutate(ai)
	// fmt.Println(k.Hex(), "mutate time:", time.Since(start))
	// quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	// start = time.Now()
	c.c.Set(k, ai)

	// fmt.Println(k.Hex(), "cache set time:", time.Since(start))
	// start := time.Now()
	// start = time.Now()
	ai.Save(c.db)
	// fmt.Println(k.Hex(), "save time:", time.Since(start))
	// close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *OtterCache) Close() {
	c.c.InvalidateAll()
}

func (c *LRUCache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address) *AccountInfo, mutate func(*AccountInfo), seenAccount bool) (*AccountInfo, error) {

	var ai *AccountInfo
	// start := time.Now()
	if seenAccount {
		if v, ok := c.c.Get(k); ok {
			// fmt.Println("Cache hit")
			ai = v
		} else {
			// fmt.Println("Cache miss")
			ai = loadFn(k)
			if ai == nil {
				fmt.Println("Account not found in db, initializing")
				ai = initFn(k)
			} else {
				fmt.Println("Account found in db, loading")
			}
		}
	} else {
		ai = initFn(k)
	}
	// if v, ok := c.rc.Get(k[:]); ok {
	// 	// fmt.Println("Cache hit")
	// 	ai = v
	// } else {
	// 	// fmt.Println("Cache miss")
	// 	ai = loadFn(k)
	// 	if ai == nil {
	// 		ai = initFn(k)
	// 	}
	// }
	// fmt.Println(k.Hex(), "get/init time:", time.Since(start))
	// start = time.Now()
	mutate(ai)
	// fmt.Println(k.Hex(), "mutate time:", time.Since(start))
	// quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	// start = time.Now()
	c.c.Add(k, ai)

	// fmt.Println(k.Hex(), "cache set time:", time.Since(start))
	// start := time.Now()
	// start = time.Now()
	ai.Save(c.db)
	// fmt.Println(k.Hex(), "save time:", time.Since(start))
	// close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *LRUCache) Close() {
	c.c.Purge()
}
