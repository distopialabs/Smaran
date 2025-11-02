package segmenttree

import (
	"fmt"
	"time"

	"github.com/cockroachdb/pebble"
	ristretto "github.com/dgraph-io/ristretto/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/nepal80m/samurai/internal/config"
)

type RistrettoAccountInfo struct {
	Account                  common.Address
	CurrentBalanceInfo       *CurrentBalance
	CurrentLXBatchTree       *LXBatchTree
	CurrentLXBatchCommitment *LXBatchCommitment

	// TODO: do i need to store this here? can i just store it in cache struct?
	PrecomputedData *config.PrecomputedData

	// cache metadata
	Version uint64
	Dirty   bool
}

type flushItem struct {
	Key     common.Address
	Version uint64
	Snap    []byte
}

var flushQ = make(chan flushItem, 1<<15)

func newCache(maxBytes int64) (*ristretto.Cache[[]byte, RistrettoAccountInfo], error) {
	config := &ristretto.Config[[]byte, RistrettoAccountInfo]{
		NumCounters: 1e7,      // number of keys to track frequency of (10M).
		MaxCost:     maxBytes, // maximum cost of cache (1GB).
		BufferItems: 64,       // number of keys per Get buffer.

		OnEvict: func(item *ristretto.Item[RistrettoAccountInfo]) {
			if !item.Value.Dirty {
				return
			}
			snap, err := rlp.EncodeToBytes(item.Value)
			if err != nil {
				panic(err)
			}
			select {
			case flushQ <- flushItem{
				Key:     item.Value.Account,
				Version: item.Value.Version,
				Snap:    snap,
			}:
			default:
				panic("flushQ is full")
			}
		},
	}
	return ristretto.NewCache(config)
}

func startFlusher(n int, db *pebble.DB) {
	for range n {
		go func() {
			batch := make([]flushItem, 0, 1<<7)
			timer := time.NewTimer(50 * time.Second)
			defer timer.Stop()
			for {
				batch = batch[:0] // clear batch
			collect:
				for len(batch) < 1<<7 {
					select {
					case item := <-flushQ:
						batch = append(batch, item)
					case <-timer.C:
						break collect
					}
				}
				if len(batch) == 0 {
					timer.Reset(50 * time.Second)
					continue
				}
				// db.Set(batch)
				// latestOnlyBatch := coalesceBatch(batch)
				// TODO: rethink if there will be any issue if there are multiple versions of the same account in the batch
				// TODO: write batch to db
				//
				timer.Reset(50 * time.Second)
			}
		}()
	}
}

func main() {
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		panic(err)
	}
	defer cache.Close()

	// set a value with a cost of 1
	cache.Set("key", "value", 1)

	// wait for value to pass through buffers
	cache.Wait()

	// get value from cache
	value, found := cache.Get("key")
	if !found {
		panic("missing value")
	}
	fmt.Println(value)

	// del value from cache
	cache.Del("key")
}
