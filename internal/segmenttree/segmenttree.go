package segmenttree

import (
	"fmt"
	"math/big"

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

type BatchTree [MaxLayer][SegmentTreeSize]common.Hash
type BatchCommitments [MaxLayer]gnark_kzg.Digest

type AccountInfo struct {
	Account            common.Address
	CurrentBalanceInfo *CurrentBalance
	CurrentBatchTree   BatchTree
	// LXPolynomial   [MaxLayer]polynomial.Polynomial
	CurrentBatchTreeCommitments BatchCommitments

	// SegmentTree       *SegmentTree
	// CurrentCommitment common.Hash
	// HistoricalBalancesKV map[common.Hash][]byte
	PrecomputedData *config.PrecomputedData
}

func NewAccountInfo(account common.Address, precomputedData *config.PrecomputedData) *AccountInfo {
	// var tree BatchTree
	// for i := range MaxLayer {
	// 	tree[i] = make([]common.Hash, SegmentTreeSize)
	// }

	// var commitments BatchCommitments

	// cbInfo := &CurrentBalance{
	// 	Version:    0,
	// 	Balance:    balance,
	// 	StartBlock: blockNumber,
	// }

	accountInfo := &AccountInfo{
		Account: account,

		// CurrentBalanceInfo: cbInfo,

		// CurrentBatchTree:            tree,
		// CurrentBatchTreeCommitments: commitments,

		PrecomputedData: precomputedData,
	}
	return accountInfo
}

func (accountInfo *AccountInfo) DeepCopy() *AccountInfo {
	var currentBatchTree BatchTree
	for i := range MaxLayer {
		currentBatchTree[i] = make([]common.Hash, SegmentTreeSize)
		copy(currentBatchTree[i], accountInfo.CurrentBatchTree[i])
	}
	var currentBatchTreeCommitments BatchCommitments
	for i := range MaxLayer {
		currentBatchTreeCommitments[i] = accountInfo.CurrentBatchTreeCommitments[i]
	}
	accountInfoCopy := &AccountInfo{
		Account:                     accountInfo.Account,
		CurrentBalanceInfo:          accountInfo.CurrentBalanceInfo.DeepCopy(),
		CurrentBatchTree:            currentBatchTree,
		CurrentBatchTreeCommitments: currentBatchTreeCommitments,
		PrecomputedData:             accountInfo.PrecomputedData,
	}
	return accountInfoCopy
}

func CreateOrUpdateAccountInfo(account common.Address, balance *big.Int, blockNumber uint64, precomputedData *config.PrecomputedData, cache *Cache) *AccountInfo {

	accountInfo, err := cache.Get(account)
	if err != nil {
		if err != pebble.ErrNotFound {
			panic(err)
		}
		// first encounter; create a new account info
		accountInfo := NewAccountInfo(account, precomputedData)
		commitmentHash := accountInfo.FirstUpdate(blockNumber, balance, db)

		_ = commitmentHash
		return accountInfo
	}

	// cbInfo, err := GetCurrentBalanceInfo(account, db)
	if err != nil {
		if err == pebble.ErrNotFound {
			// first encounter; create a new account info
			accountInfo := NewAccountInfo(account, precomputedData)
			commitmentHash := accountInfo.FirstUpdate(blockNumber, balance, db)
			// StoreCurrentBalanceInfo(account, accountInfo.CurrentBalanceInfo, db)
			// StoreCurrentBatchTree(account, &accountInfo.CurrentBatchTree, db)
			// StoreBatchCommitments(account, accountInfo.CurrentBalanceInfo.Version, &accountInfo.CurrentBatchTreeCommitments, db)
			// final commitment
			// treeCommitHash := CommitmentToHash(accountInfo.CurrentBatchTreeCommitments[3])
			// cbHash := accountInfo.CurrentBalanceInfo.Hash()
			// commitmentHash := BytesToPoseidonHash(cbHash.Bytes(), treeCommitHash.Bytes())
			_ = commitmentHash
			// fmt.Println("block number", blockNumber, "account", account.Hex(), "commitmentHash", commitmentHash.Hex())

			return accountInfo
		} else {
			fmt.Printf("%T\n", err)
			panic(err)
		}
	}
	batchTree := GetCurrentBatchTree(account, db)
	batchCommitments := GetBatchCommitments(account, cbInfo.Version, db)
	accountInfo := &AccountInfo{
		Account:                     account,
		CurrentBalanceInfo:          cbInfo,
		CurrentBatchTree:            *batchTree,
		CurrentBatchTreeCommitments: *batchCommitments,
		PrecomputedData:             precomputedData,
	}
	commitmentHash := accountInfo.Update(blockNumber, balance, db)
	// fmt.Println("block number", blockNumber, "account", account.Hex(), "commitmentHash", commitmentHash.Hex())
	_ = commitmentHash

	return accountInfo
}

// func GetOrCreateAccountInfo(account common.Address, db *pebble.DB, precomputedData *config.PrecomputedData) *AccountInfo {

// 	cbInfo, err := GetCurrentBalanceInfo(account, db)
// 	if err != nil {
// 		if err == pebble.ErrNotFound {
// 			// create a new account info
// 			return nil, err
// 		}

// 		accountInfo, err := GetAccountInfo(account, db)
// 		if err != nil {
// 			accountInfo = NewAccountInfo(account, precomputedData)
// 			StoreAccountInfo(accountInfo, db)
// 		}
// 	}
// }

// type LXTree [MaxLayer][]common.Hash
// type LXPolynomial [MaxLayer]polynomial.Polynomial
// type LXCommitment [MaxLayer]gnark_kzg.Digest

// type SegmentTree struct {
// 	Account      common.Address
// 	CurrentIndex uint64

// 	LXTreeV3       map[int][]common.Hash
// 	LXPolynomialV3 map[int]polynomial.Polynomial
// 	LXCommitmentV3 map[int]gnark_kzg.Digest

// 	// LXPrevCIncCommitmentV3 map[int]gnark_kzg.Digest
// 	PrecomputedData *config.PrecomputedData
// 	// CachedData      *CachedData
// 	Storage *Storage
// }

// func New(account common.Address, precomputedData *config.PrecomputedData) *SegmentTree {
// 	return &SegmentTree{
// 		Account: account,

// 		// LXTreeV3: make(map[int][]common.Hash),
// 		LXTreeV3: map[int][]common.Hash{
// 			1: make([]common.Hash, SegmentTreeSize),
// 			2: make([]common.Hash, SegmentTreeSize),
// 			3: make([]common.Hash, SegmentTreeSize),
// 			4: make([]common.Hash, SegmentTreeSize),
// 		},
// 		LXPolynomialV3: map[int]polynomial.Polynomial{
// 			1: make(polynomial.Polynomial, SegmentTreeSize),
// 			2: make(polynomial.Polynomial, SegmentTreeSize),
// 			3: make(polynomial.Polynomial, SegmentTreeSize),
// 			4: make(polynomial.Polynomial, SegmentTreeSize),
// 		},
// 		LXCommitmentV3: make(map[int]gnark_kzg.Digest),
// 		// LXPrevCIncCommitmentV3: make(map[int]gnark_kzg.Digest),

// 		PrecomputedData: precomputedData,
// 		Storage: &Storage{
// 			L1Commitments: make(map[int]bls.G1Affine),
// 			L2Commitments: make(map[int]bls.G1Affine),
// 			L3Commitments: make(map[int]bls.G1Affine),
// 			L4Commitments: make(map[int]bls.G1Affine),
// 			L1Tree:        make(map[int][]common.Hash),
// 			L2Tree:        make(map[int][]common.Hash),
// 			L3Tree:        make(map[int][]common.Hash),
// 			L4Tree:        make(map[int][]common.Hash),
// 			L1Polynomial:  make(map[int]polynomial.Polynomial),
// 			L2Polynomial:  make(map[int]polynomial.Polynomial),
// 			L3Polynomial:  make(map[int]polynomial.Polynomial),
// 			L4Polynomial:  make(map[int]polynomial.Polynomial),
// 		},
// 	}
// }
