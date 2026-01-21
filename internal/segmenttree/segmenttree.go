package segmenttree

import (
	"fmt"
	"math/big"
	"unsafe"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
)

const L1BatchSize = 2048

// const L1BatchSize = 1024
// const L1BatchSize = 512

// const L1BatchSize = 8

const L2BatchSize = 1365

// const L2BatchSize = 682
// const L2BatchSize = 341

// const L2BatchSize = 5

const MaxLayer = 4
const ChunkSize = 64

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
	DirtyChunks              [MaxLayer]map[int]bool

	// TODO: do i need to store this here? can i just store it in cache struct?
	PrecomputedData *config.PrecomputedData

	// cache metadata
	// Dirty bool
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
		DirtyChunks:              InitDirtyChunks(),
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

func CreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, cache *Cache) common.Hash {

	initFn := func(account common.Address) *AccountInfo {
		accountInfo := NewAccountInfo(account, cache.precomputedData)
		return accountInfo
	}
	// return account info from db, if not found return nil
	loadFn := func(account common.Address, db *SamuraiDB) *AccountInfo {
		// start := time.Now()
		cbInfo, err := GetCurrentBalanceInfo(account, db.StateDB)
		if err != nil {
			if err != ErrNotFound {
				panic(err)
			}
			// key not found in db
			return nil
		} else {
			// key found in db
			// innerStart := time.Now()
			// TODO: tried using iterator here, but it was slower. need to investigate again.
			// batchTree, batchCommitments := GetCurrentLXBatchTreeAndCommitments(account, cbInfo.Version, cache.db)
			batchTree := GetCurrentLXBatchTree(account, db.TreeDB)
			batchCommitments := GetLXBatchCommitments(account, cbInfo.Version, db.StateDB)
			// fmt.Println("Time taken to get batch tree and commitments from db", time.Since(innerStart))
			accountInfo := &AccountInfo{
				Account:                  account,
				CurrentBalanceInfo:       cbInfo,
				CurrentLXBatchTree:       batchTree,
				CurrentLXBatchCommitment: batchCommitments,
				PrecomputedData:          cache.precomputedData,
				DirtyChunks:              InitDirtyChunks(),
			}

			// fmt.Println("Time taken to load account info from db", time.Since(start))
			return accountInfo

		}
	}
	mutate := func(accountInfo *AccountInfo, db *SamuraiDB) {
		// start := time.Now()
		accountInfo.Update(blockNumber, balance, db)
		// fmt.Println("Time taken to mutate account info", time.Since(start))
	}

	// quitLog := logBlockedTime("Update", 100*time.Millisecond)
	// start := time.Now()
	accountInfo, err := cache.Update(account, initFn, loadFn, mutate)
	if err != nil {
		panic(err)
	}
	// fmt.Println(account.Hex(), "update time:", time.Since(start))
	// close(quitLog)
	commitmentHash := accountInfo.CalculateFinalCommitment()
	return commitmentHash
}

func InitDirtyChunks() [MaxLayer]map[int]bool {
	var dirtyChunks [MaxLayer]map[int]bool
	for i := 0; i < MaxLayer; i++ {
		dirtyChunks[i] = make(map[int]bool)
	}
	return dirtyChunks
}
