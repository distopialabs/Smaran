package segmenttree

import (
	"log"
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

func (c *Cache) Update(k common.Address, initFn func(common.Address) *AccountInfo, loadFn func(common.Address, *SamuraiDB) *AccountInfo, mutate func(*AccountInfo, *SamuraiDB)) (*AccountInfo, error) {

	var ai *AccountInfo

	db := c.db

	if v, ok := c.C.Get(k[:]); ok {
		// cache hit
		ai = v
	} else {
		// cache miss
		// load from db
		ai = loadFn(k, db)

		if ai == nil {
			// not found in db, initialize
			ai = initFn(k)
		}
	}

	mutate(ai, db)
	admitted := c.C.Set(k[:], ai, 525312) //525312
	if !admitted {
		log.Fatal("❌Cache set rejected")
	}
	// c.C.Wait()

	return ai, nil
}

func (c *Cache) Close() {
	c.C.Close()
}
