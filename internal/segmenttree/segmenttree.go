package segmenttree

import (
	"fmt"
	"math/big"
	"sync"
	"time"
	"unsafe"

	"github.com/cockroachdb/pebble"
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
)

const L1BatchSize = 2048

// const L1BatchSize = 8

const L2BatchSize = 1365

// const L2BatchSize = 5

const MaxLayer = 4

const SegmentTreeSize = L1BatchSize * 2 //2048 * 2 = 4096

type CachedData struct {
	V             polynomial.Polynomial
	Weights       []fr.Element
	WeightCommits []gnark_kzg.Digest
	SRS           *kzg.MultiSRS
}

type BatchTree [SegmentTreeSize]common.Hash

func (t *BatchTree) MarshalBinary() []byte {
	b := make([]byte, SegmentTreeSize*common.HashLength)
	for i := range SegmentTreeSize {
		copy(b[i*common.HashLength:(i+1)*common.HashLength], t[i][:])
	}
	return b
}

func (t *BatchTree) UnmarshalBinary(b []byte) error {
	if len(b) != SegmentTreeSize*common.HashLength {
		return fmt.Errorf("bad length: got %d, want %d", len(b), SegmentTreeSize*common.HashLength)
	}
	for i := range SegmentTreeSize {
		copy(t[i][:], b[i*common.HashLength:(i+1)*common.HashLength])
	}
	return nil
}

func (t *BatchTree) AsBytesUnsafe() []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(&t[0])), SegmentTreeSize*common.HashLength)
}

type LXBatchTree [MaxLayer]BatchTree
type LXBatchCommitment [MaxLayer]gnark_kzg.Digest

type AccountInfo struct {
	Account                  common.Address
	CurrentBalanceInfo       *CurrentBalance
	CurrentLXBatchTree       *LXBatchTree
	CurrentLXBatchCommitment *LXBatchCommitment

	// TODO: do i need to store this here? can i just store it in cache struct?
	PrecomputedData *config.PrecomputedData

	// cache metadata
	Dirty bool
}

func NewAccountInfo(account common.Address, precomputedData *config.PrecomputedData) *AccountInfo {

	accountInfo := &AccountInfo{
		Account: account,
		// CurrentBalanceInfo: &CurrentBalance{
		// 	Version:    0,
		// 	Balance:    balance,
		// 	StartBlock: blockNumber,
		// },
		CurrentLXBatchTree:       new(LXBatchTree),
		CurrentLXBatchCommitment: new(LXBatchCommitment),
		PrecomputedData:          precomputedData,
	}
	return accountInfo
}

func (t *LXBatchTree) DeepCopy() *LXBatchTree {
	c := *t
	return &c
}

func (t *LXBatchCommitment) DeepCopy() *LXBatchCommitment {
	c := *t
	return &c
}

func (a *AccountInfo) DeepCopy() *AccountInfo {
	c := &AccountInfo{
		Account:                  a.Account,
		CurrentBalanceInfo:       a.CurrentBalanceInfo.DeepCopy(),
		CurrentLXBatchTree:       a.CurrentLXBatchTree.DeepCopy(),
		CurrentLXBatchCommitment: a.CurrentLXBatchCommitment.DeepCopy(),
		PrecomputedData:          a.PrecomputedData,
	}
	return c
}

// check if the account info is in the cache or db. if it is, update it. if it isn't, create a new account info instance.
func CreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *OldCache) common.Hash {
	accountInfo, err := cache.Get(account)
	if err != nil {
		if err != pebble.ErrNotFound {
			panic(err)
		}
		// this account doesnt exist yet, not in cache, not in db.create a new account info instance

		accountInfo = NewAccountInfo(account, cache.precomputedData)

	} else {
		accountInfo.Update(blockNumber, balance, cache.db)
	}
	cache.Set(account, accountInfo)
	commitmentHash := accountInfo.CalculateFinalCommitment()

	return commitmentHash
}

func NewCreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *OldCache) common.Hash {
	shouldFlush := false
	var commitmentHash common.Hash
	cache.mu.Lock()
	if e, ok := cache.entries[account]; ok {
		// cache hit
		e.mu.Lock()
		cache.moveToHead(e)
		if !e.dirty {
			e.dirty = true
			cache.dirtyEntriesCount++
		}
		cache.updatesSinceLastFlush++

		shouldFlush = cache.updatesSinceLastFlush >= cache.maxUpdatesSinceLastFlush || cache.dirtyEntriesCount >= cache.maxDirtyEntriesCount
		cache.mu.Unlock()
		accountInfo := e.val
		accountInfo.Update(blockNumber, balance, cache.db)
		commitmentHash = accountInfo.CalculateFinalCommitment()

		e.mu.Unlock()

	} else {
		// cache miss: load from db
		// TODO: recheck if you want to keep a temp entry as placeholder in cache for this account
		e := &CacheEntry{
			mu:    sync.RWMutex{},
			key:   account,
			dirty: true,
		}
		e.mu.Lock()
		cache.entries[account] = e
		cache.attach(e)
		cache.dirtyEntriesCount++
		cache.evictIfNeeded()
		cache.updatesSinceLastFlush++

		shouldFlush = cache.updatesSinceLastFlush >= cache.maxUpdatesSinceLastFlush || cache.dirtyEntriesCount >= cache.maxDirtyEntriesCount
		cache.mu.Unlock()

		cbInfo, err := GetCurrentBalanceInfo(account, cache.db)
		if err != nil {
			if err != pebble.ErrNotFound {
				panic(err)
			}
			// key not found in db
			accountInfo := NewAccountInfo(account, cache.precomputedData)
			e.val = accountInfo
			commitmentHash = accountInfo.CalculateFinalCommitment()
		} else {
			// key found in db
			batchTree := GetCurrentLXBatchTree(account, cache.db)
			batchCommitments := GetLXBatchCommitments(account, cbInfo.Version, cache.db)
			accountInfo := &AccountInfo{
				Account:                  account,
				CurrentBalanceInfo:       cbInfo,
				CurrentLXBatchTree:       batchTree,
				CurrentLXBatchCommitment: batchCommitments,
				PrecomputedData:          cache.precomputedData,
			}
			accountInfo.Update(blockNumber, balance, cache.db)
			e.val = accountInfo
			commitmentHash = accountInfo.CalculateFinalCommitment()

		}
		e.mu.Unlock()
	}
	if shouldFlush {
		cache.flushSome(false)
	}

	return commitmentHash
}

func LibNewCreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *OldCache) common.Hash {
	shouldFlush := false
	var commitmentHash common.Hash
	cache.mu.Lock()
	if e, ok := cache.entries[account]; ok {
		// cache hit
		e.mu.Lock()
		cache.moveToHead(e)
		if !e.dirty {
			e.dirty = true
			cache.dirtyEntriesCount++
		}
		cache.updatesSinceLastFlush++

		shouldFlush = cache.updatesSinceLastFlush >= cache.maxUpdatesSinceLastFlush || cache.dirtyEntriesCount >= cache.maxDirtyEntriesCount
		cache.mu.Unlock()
		accountInfo := e.val
		accountInfo.Update(blockNumber, balance, cache.db)
		commitmentHash = accountInfo.CalculateFinalCommitment()

		e.mu.Unlock()

	} else {
		// cache miss: load from db
		// TODO: recheck if you want to keep a temp entry as placeholder in cache for this account
		e := &CacheEntry{
			mu:    sync.RWMutex{},
			key:   account,
			dirty: true,
		}
		e.mu.Lock()
		cache.entries[account] = e
		cache.attach(e)
		cache.dirtyEntriesCount++
		cache.evictIfNeeded()
		cache.updatesSinceLastFlush++

		shouldFlush = cache.updatesSinceLastFlush >= cache.maxUpdatesSinceLastFlush || cache.dirtyEntriesCount >= cache.maxDirtyEntriesCount
		cache.mu.Unlock()

		cbInfo, err := GetCurrentBalanceInfo(account, cache.db)
		if err != nil {
			if err != pebble.ErrNotFound {
				panic(err)
			}
			// key not found in db
			accountInfo := NewAccountInfo(account, cache.precomputedData)
			e.val = accountInfo
			commitmentHash = accountInfo.CalculateFinalCommitment()
		} else {
			// key found in db
			batchTree := GetCurrentLXBatchTree(account, cache.db)
			batchCommitments := GetLXBatchCommitments(account, cbInfo.Version, cache.db)
			accountInfo := &AccountInfo{
				Account:                  account,
				CurrentBalanceInfo:       cbInfo,
				CurrentLXBatchTree:       batchTree,
				CurrentLXBatchCommitment: batchCommitments,
				PrecomputedData:          cache.precomputedData,
			}
			accountInfo.Update(blockNumber, balance, cache.db)
			e.val = accountInfo
			commitmentHash = accountInfo.CalculateFinalCommitment()

		}
		e.mu.Unlock()
	}
	if shouldFlush {
		cache.flushSome(false)
	}

	return commitmentHash
}

func FinalCreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *Cache) common.Hash {
	initFn := func(account common.Address) *AccountInfo {
		accountInfo := NewAccountInfo(account, cache.precomputedData)
		return accountInfo
	}
	loadFn := func(account common.Address) *AccountInfo {
		// start := time.Now()
		cbInfo, err := GetCurrentBalanceInfo(account, cache.db)
		if err != nil {
			if err != pebble.ErrNotFound {
				panic(err)
			}
			// key not found in db
			return nil
		} else {
			// key found in db
			// innerStart := time.Now()
			// TODO: tried using iterator here, but it was slower. need to investigate again.
			// batchTree, batchCommitments := GetCurrentLXBatchTreeAndCommitments(account, cbInfo.Version, cache.db)
			batchTree := GetCurrentLXBatchTree(account, cache.db)
			batchCommitments := GetLXBatchCommitments(account, cbInfo.Version, cache.db)
			// fmt.Println("Time taken to get batch tree and commitments from db", time.Since(innerStart))
			accountInfo := &AccountInfo{
				Account:                  account,
				CurrentBalanceInfo:       cbInfo,
				CurrentLXBatchTree:       batchTree,
				CurrentLXBatchCommitment: batchCommitments,
				PrecomputedData:          cache.precomputedData,
			}
			// fmt.Println("Time taken to load account info from db", time.Since(start))
			return accountInfo

		}
	}
	mutate := func(accountInfo *AccountInfo) {
		start := time.Now()
		accountInfo.Update(blockNumber, balance, cache.db)
		fmt.Println("Time taken to mutate account info", time.Since(start))
	}

	// quitLog := logBlockedTime("Update", 100*time.Millisecond)
	start := time.Now()
	accountInfo, err := cache.Update(account, initFn, loadFn, mutate)
	if err != nil {
		panic(err)
	}
	fmt.Println("Time taken to update account info in cache", time.Since(start))
	// close(quitLog)
	commitmentHash := accountInfo.CalculateFinalCommitment()
	return commitmentHash
}
