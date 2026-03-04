// Package storage provides caching and file I/O for Samurai.
package storage

import (
	"github.com/ethereum/go-ethereum/common"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
)

// Cache wraps an LRU cache with database persistence.
type Cache struct {
	C               *lru.Cache[common.Address, *tree.AccountInfo]
	DB              *db.SamuraiDB
	PrecomputedData *config.PrecomputedData
}

// NewCache creates a new Cache with the given configuration.
func NewCache(sdb *db.SamuraiDB, cfg *config.Cache, precomputedData *config.PrecomputedData) (*Cache, error) {
	cache := &Cache{
		DB:              sdb,
		PrecomputedData: precomputedData,
	}

	onEvicted := func(k common.Address, v *tree.AccountInfo) {
		v.Save(sdb)
	}

	rc, err := lru.NewWithEvict(cfg.Size, onEvicted)
	if err != nil {
		return nil, err
	}
	cache.C = rc
	return cache, nil
}

// Update retrieves or creates an AccountInfo, applies mutations, and stores in cache.
func (c *Cache) Update(k common.Address, initFn func(common.Address) *tree.AccountInfo, loadFn func(common.Address, *db.SamuraiDB) *tree.AccountInfo, mutate func(*tree.AccountInfo, *db.SamuraiDB)) (*tree.AccountInfo, error) {
	var ai *tree.AccountInfo
	found := false

	if v, ok := c.C.Get(k); ok {
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
		c.C.Add(k, ai)
	}

	return ai, nil
}

// Close closes the cache, flushing all items to the database first.
func (c *Cache) Close() {
	// Purge invokes the registered onEvicted callback synchronously for all items
	c.C.Purge()
}
