package segmenttree

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/semaphore"
)

type Key [20]byte

type KV struct {
	Key      common.Address
	Version  uint64
	Snapshot *AccountInfo
	// Blob    []byte // immutable snapshot
}

type DB interface {
	// BatchUpsert(ctx context.Context, batch []KV) error
}

// Batcher: single flusher goroutine + byte-bounded queue (via weighted semaphore)
type Batcher struct {
	db  *pebble.DB
	in  chan KV
	sem *semaphore.Weighted // capacity = max in-flight bytes

	maxBatchItems int
	maxBatchWait  time.Duration

	stop chan struct{}

	runnerWg sync.WaitGroup
	closed   atomic.Bool
}

func NewBatcher(db *pebble.DB) *Batcher {
	b := &Batcher{
		db:            db,
		in:            make(chan KV, 2048),
		sem:           semaphore.NewWeighted(2048 + 512), // 1024 + 512 = 1536
		maxBatchItems: 512,
		maxBatchWait:  50 * time.Second,
		stop:          make(chan struct{}),
	}
	b.runnerWg.Add(1)
	go func() {
		defer b.runnerWg.Done()
		b.run()
	}()
	return b
}

func (b *Batcher) Enqueue(kv KV) {
	if err := b.sem.Acquire(context.Background(), 1); err != nil {
		panic(err)
	}
	b.in <- kv
}

func (b *Batcher) run() {
	t := time.NewTicker(b.maxBatchWait)
	defer t.Stop()

	go logChannelSize(b.in)

	for {
		// Gather one item (or exit)
		var first KV
		var ok bool
		select {
		case <-b.stop:
			b.drainAndFlush()
			return
		case first, ok = <-b.in:
			if !ok {
				b.drainAndFlush()
				return
			}
		}

		// Build batch with simple coalescing (keep newest per key)
		batch := make([]KV, 0, b.maxBatchItems)
		// latest := map[Key]int{} // delete these few lines if you don't want coalescing

		// add := func(kv KV) {
		// 	if idx, exists := latest[kv.Key]; exists {
		// 		batch[idx] = kv
		// 	} else {
		// 		latest[kv.Key] = len(batch)
		// 		batch = append(batch, kv)
		// 	}
		// }
		batch = append(batch, first)

	GATHER:
		for len(batch) < b.maxBatchItems {
			select {
			case kv := <-b.in:
				batch = append(batch, kv)
			case <-t.C:
				break GATHER
			case <-b.stop:
				break GATHER
			}
		}

		// Flush (retry until success; fixed backoff keeps it simple)
		for {
			// ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			// err := b.db.BatchUpsert(ctx, batch)
			start := time.Now()
			fmt.Println("Flushing batch", len(batch), start)
			// bq := b.db.NewBatch()
			for _, kv := range batch {
				// BatchStoreAccountInfo(kv.Snapshot, bq)
				StoreAccountInfo(kv.Snapshot, b.db)
			}
			// err := bq.Commit(pebble.NoSync)
			// bq.Close()
			// if err == nil {
			// 	fmt.Println("Flushed batch", len(batch), time.Since(start))
			// 	break
			// }

			// panic(fmt.Errorf("failed to commit batch: %w", err))
			// time.Sleep(200 * time.Millisecond)
		}
		// Release all bytes at once
		b.sem.Release(int64(len(batch)))
	}
}

func logChannelSize(ch chan KV) {
	// print the in channel size every 5 seconds
	var start time.Time
	var wasBlocking = false
	for {
		time.Sleep(1 * time.Second)
		if cap(ch)-len(ch) > 5 {
			fmt.Printf("🍀 In channel size: %d/%d\n", len(ch), cap(ch))
			wasBlocking = false
		}
		if cap(ch)-len(ch) > 0 && cap(ch)-len(ch) < 5 {
			fmt.Printf("⚠️ In channel size: %d/%d\n", len(ch), cap(ch))
			wasBlocking = false
		}
		if cap(ch)-len(ch) <= 0 {
			if !wasBlocking {
				wasBlocking = true
				start = time.Now()
				fmt.Printf("🚨 In channel size: %d/%d\n", len(ch), cap(ch))
			} else {
				fmt.Printf("🚨 In channel size: %d/%d, blocking time: %s\n", len(ch), cap(ch), time.Since(start))
			}
		}
	}

}

func (b *Batcher) drainAndFlush() {
	// Drain everything quickly into one or more batches
	buf := make([]KV, 0, 4096)
	for {
		select {
		case kv := <-b.in:
			buf = append(buf, kv)
		default:
			goto FLUSH
		}
	}
FLUSH:
	for len(buf) > 0 {
		// cut to size limits
		end := len(buf)
		if end > b.maxBatchItems {
			end = b.maxBatchItems
		}
		chunk := buf[:end]
		for {
			// err := b.db.BatchUpsert(ctx, chunk)
			// bq := b.db.NewBatch()
			// TODO: encode them parallelly
			for _, kv := range chunk {
				StoreAccountInfo(kv.Snapshot, b.db)
				// BatchStoreAccountInfo(kv.Snapshot, bq)
			}
			// err := bq.Commit(pebble.Sync)
			// bq.Close()
			// if err == nil {
			// 	break
			// }

			// panic(fmt.Errorf("failed to commit batch: %w", err))

			// time.Sleep(200 * time.Millisecond)
		}
		b.sem.Release(int64(len(chunk)))
		buf = buf[end:]
	}
}

func (b *Batcher) Close() {
	close(b.stop)
	b.runnerWg.Wait()
	// let run() call drainAndFlush and exit
}
