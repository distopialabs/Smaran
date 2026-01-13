package segmenttree

import (
	"fmt"
	"sync"
	"time"

	ristretto "github.com/dgraph-io/ristretto/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
)

// const DB_SHARDS = 32

type Cache struct {
	C  *ristretto.Cache[[]byte, *AccountInfo]
	db *SamuraiDB

	precomputedData *config.PrecomputedData
}

func NewCache(db *SamuraiDB, cfg *config.Cache, precomputedData *config.PrecomputedData) (*Cache, error) {
	rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *AccountInfo]{
		NumCounters: int64(cfg.NumCounters), // TODO: recommended is 10x maxCost (2^18)
		MaxCost:     int64(cfg.MaxCost),     //2048,      //16_384,    //8192 = 4GB,      //131_072,    //32_768
		BufferItems: 64,
		Metrics:     cfg.EnableMetrics, // Enable ristretto metrics collection for benchmarking
		// Cost: func(value *AccountInfo) int64 {
		// 	return 513 * 1024 // 513kb
		// },
		OnExit: func(val *AccountInfo) {
			val.Save(db)
		},
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

// Metrics returns the underlying Ristretto cache metrics (nil if metrics not enabled)
func (c *Cache) Metrics() *ristretto.Metrics {
	return c.C.Metrics
}

type SeenAccountInfo struct {
	Count            int
	DBFetchCount     int
	TotalDBFetchTime time.Duration
}

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address, *SamuraiDB) *AccountInfo, mutate func(*AccountInfo, *SamuraiDB), accountsSeen *sync.Map) (*AccountInfo, error) {

	var ai *AccountInfo
	// start := time.Now()
	seenInfo, seen := accountsSeen.Load(k)

	// dbIndex := xxhash.Sum64(k[:]) % DB_SHARDS
	db := c.db

	if seen {
		seenAccountInfo := seenInfo.(SeenAccountInfo)
		// fmt.Printf("Account %s seen %d times before\n", k.Hex(), seenAccountInfo.Count)
		if v, ok := c.C.Get(k[:]); ok {
			// fmt.Println("Cache hit")
			ai = v
			accountsSeen.Store(k, SeenAccountInfo{
				Count:            seenAccountInfo.Count + 1,
				DBFetchCount:     seenAccountInfo.DBFetchCount,
				TotalDBFetchTime: seenAccountInfo.TotalDBFetchTime,
			})
		} else {

			// fmt.Println("Cache miss for account", k.Hex(), " seen", seenAccountInfo.Count, "times before, fetching from db")
			fetchStart := time.Now()
			ai = loadFn(k, db)
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
	// fmt.Println(k.Hex(), "get/init time:", time.Since(start))
	// start = time.Now()
	mutate(ai, db)
	// fmt.Println(k.Hex(), "mutate time:", time.Since(start))
	// quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	// start = time.Now()
	admitted := c.C.Set(k[:], ai, 525312) //525312
	if !admitted {
		fmt.Println(k.Hex(), "❌Cache set rejected")
	}
	c.C.Wait()
	// fmt.Println(k.Hex(), "cache set time:", time.Since(start))
	// start := time.Now()
	// start = time.Now()
	// ai.Save(db)
	// fmt.Println(k.Hex(), "save time:", time.Since(start))
	// close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *Cache) Close() {
	c.C.Close()
}
