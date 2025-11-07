package segmenttree

import (
	"fmt"

	"github.com/cockroachdb/pebble"
	ristretto "github.com/dgraph-io/ristretto/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
)

type Cache struct {
	rc *ristretto.Cache[[]byte, *AccountInfo]
	db *pebble.DB

	precomputedData *config.PrecomputedData
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
		rc:              rc,
		db:              db,
		precomputedData: precomputedData,
	}, nil
}

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address) *AccountInfo, mutate func(*AccountInfo)) (*AccountInfo, error) {

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
	// fmt.Println(k.Hex(), "get/init time:", time.Since(start))
	// start = time.Now()
	mutate(ai)
	// fmt.Println(k.Hex(), "mutate time:", time.Since(start))
	// quitLog = logBlockedTime("CacheSet", 100*time.Millisecond)
	// start = time.Now()
	admitted := c.rc.Set(k[:], ai, 1)
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
	c.rc.Close()
}
