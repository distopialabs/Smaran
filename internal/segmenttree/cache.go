package segmenttree

import (
	"fmt"
	"sync"
	"time"

	ristretto "github.com/dgraph-io/ristretto/v2"
	otter "github.com/maypok86/otter/v2"
	"github.com/maypok86/otter/v2/stats"

	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/nepal80m/samurai/internal/config"
)

type Cache struct {
	C  *ristretto.Cache[[]byte, *AccountInfo]
	db DB

	precomputedData *config.PrecomputedData
}
type OtterCache struct {
	c  *otter.Cache[common.Address, *AccountInfo]
	db DB

	precomputedData *config.PrecomputedData
}
type LRUCache struct {
	c  *lru.Cache[common.Address, *AccountInfo]
	db DB

	precomputedData *config.PrecomputedData
}

func NewOtterCache(db DB, precomputedData *config.PrecomputedData) (*OtterCache, error) {
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

func NewLRUCache(db DB, precomputedData *config.PrecomputedData) (*LRUCache, error) {
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

func NewCache(db DB, precomputedData *config.PrecomputedData) (*Cache, error) {
	rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *AccountInfo]{
		NumCounters: 2_097_152, // TODO: recommended is 10x maxCost (2^18)
		MaxCost:     32_768,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &Cache{
		C:               rc,
		db:              db,
		precomputedData: precomputedData,
	}, nil
}

type SeenAccountInfo struct {
	Count            int
	DBFetchCount     int
	TotalDBFetchTime time.Duration
}

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address) *AccountInfo, mutate func(*AccountInfo), accountsSeen *sync.Map) (*AccountInfo, error) {

	var ai *AccountInfo
	start := time.Now()
	seenInfo, seen := accountsSeen.Load(k)

	if seen {
		seenAccountInfo := seenInfo.(SeenAccountInfo)
		fmt.Printf("Account %s seen %d times before\n", k.Hex(), seenAccountInfo.Count)
		if v, ok := c.C.Get(k[:]); ok {
			// fmt.Println("Cache hit")
			ai = v
			accountsSeen.Store(k, SeenAccountInfo{
				Count:            seenAccountInfo.Count + 1,
				DBFetchCount:     seenAccountInfo.DBFetchCount,
				TotalDBFetchTime: seenAccountInfo.TotalDBFetchTime,
			})
		} else {

			fmt.Println("Cache miss for account", k.Hex(), " seen", seenAccountInfo.Count, "times before, fetching from db")
			fetchStart := time.Now()
			ai = loadFn(k)
			fetchTime := time.Since(fetchStart)
			accountsSeen.Store(k, SeenAccountInfo{
				Count:            seenAccountInfo.Count + 1,
				DBFetchCount:     seenAccountInfo.DBFetchCount + 1,
				TotalDBFetchTime: seenAccountInfo.TotalDBFetchTime + fetchTime,
			})
			if ai == nil {
				panic("Seen account not found in db, initializing")
				// ai = initFn(k)
			}
		}
	} else {
		ai = initFn(k)
		accountsSeen.Store(k, SeenAccountInfo{
			Count:            1,
			DBFetchCount:     0,
			TotalDBFetchTime: 0,
		})
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
	fmt.Println(k.Hex(), "get/init time:", time.Since(start))
	start = time.Now()
	mutate(ai)
	fmt.Println(k.Hex(), "mutate time:", time.Since(start))
	// quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	// start = time.Now()
	admitted := c.C.Set(k[:], ai, 1)
	if !admitted {
		fmt.Println(k.Hex(), "❌Cache set rejected")
	}
	// fmt.Println(k.Hex(), "cache set time:", time.Since(start))
	// start := time.Now()
	start = time.Now()
	ai.Save(c.db)
	fmt.Println(k.Hex(), "save time:", time.Since(start))
	// close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *Cache) Close() {
	c.C.Close()
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
