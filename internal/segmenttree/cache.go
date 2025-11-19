package segmenttree

import (
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	ristretto "github.com/dgraph-io/ristretto/v2"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
)

const DB_SHARDS = 4

type Cache struct {
	C   *ristretto.Cache[[]byte, *AccountInfo]
	dbs [DB_SHARDS]DB

	precomputedData *config.PrecomputedData
}

func NewCache(dbs [DB_SHARDS]DB, precomputedData *config.PrecomputedData) (*Cache, error) {
	rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *AccountInfo]{
		NumCounters: 2_097_152, // TODO: recommended is 10x maxCost (2^18)
		MaxCost:     131072,    //32_768
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &Cache{
		C:               rc,
		dbs:             dbs,
		precomputedData: precomputedData,
	}, nil
}

type SeenAccountInfo struct {
	Count            int
	DBFetchCount     int
	TotalDBFetchTime time.Duration
}

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address, DB) *AccountInfo, mutate func(*AccountInfo, DB), accountsSeen *sync.Map) (*AccountInfo, error) {

	var ai *AccountInfo
	// start := time.Now()
	seenInfo, seen := accountsSeen.Load(k)

	dbIndex := xxhash.Sum64(k[:]) % DB_SHARDS
	db := c.dbs[dbIndex]

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
	admitted := c.C.Set(k[:], ai, 1)
	if !admitted {
		fmt.Println(k.Hex(), "❌Cache set rejected")
	}
	// fmt.Println(k.Hex(), "cache set time:", time.Since(start))
	// start := time.Now()
	// start = time.Now()
	ai.Save(db)
	// fmt.Println(k.Hex(), "save time:", time.Since(start))
	// close(quitLog)
	// c.rc.Wait() // TODO: do i need to wait here?
	return ai, nil
}

func (c *Cache) Close() {
	c.C.Close()
}
