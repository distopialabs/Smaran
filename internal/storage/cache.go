// Package storage provides caching and file I/O for Samurai.
package storage

import (
	"log"

	ristretto "github.com/dgraph-io/ristretto/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
)

// Cache wraps a Ristretto cache with database persistence.
type Cache struct {
	C               *ristretto.Cache[[]byte, *tree.AccountInfo]
	DB              *db.SamuraiDB
	PrecomputedData *config.PrecomputedData
}

// NewCache creates a new Cache with the given configuration.
func NewCache(sdb *db.SamuraiDB, cfg *config.Cache, precomputedData *config.PrecomputedData) (*Cache, error) {
	cache := &Cache{
		DB:              sdb,
		PrecomputedData: precomputedData,
	}

	rc, err := ristretto.NewCache(&ristretto.Config[[]byte, *tree.AccountInfo]{
		NumCounters: int64(cfg.NumCounters),
		MaxCost:     int64(cfg.MaxCost),
		BufferItems: 64,
		Metrics:     cfg.EnableMetrics,
		OnExit: func(val *tree.AccountInfo) {
			val.Save(sdb)
		},
	})
	if err != nil {
		return nil, err
	}
	cache.C = rc
	return cache, nil
}

// Metrics returns the underlying Ristretto cache metrics.
func (c *Cache) Metrics() *ristretto.Metrics {
	return c.C.Metrics
}

// Update retrieves or creates an AccountInfo, applies mutations, and stores in cache.
func (c *Cache) Update(k common.Address, initFn func(common.Address) *tree.AccountInfo, loadFn func(common.Address, *db.SamuraiDB) *tree.AccountInfo, mutate func(*tree.AccountInfo, *db.SamuraiDB)) (*tree.AccountInfo, error) {
	var ai *tree.AccountInfo
	found := false

	if v, ok := c.C.Get(k[:]); ok {
		ai = v
		found = true
	} else {
		ai = loadFn(k, c.DB)
		if ai == nil {
			ai = initFn(k)
		}
	}

	mutate(ai, c.DB)
	if !found {
		admitted := c.C.Set(k[:], ai, 525312)
		if !admitted {
			log.Fatal("❌Cache set rejected")
		}
		c.C.Wait() // Ensure the item is actually stored before returning
	}

	return ai, nil
}

// Close closes the cache, flushing all items to the database first.
func (c *Cache) Close() {
	// Clear() triggers OnExit for all stored items, persisting them to DB.
	// This is necessary because Close() alone doesn't call OnExit for remaining items.
	c.C.Clear()
	c.C.Close()
}
