package segmenttree

import (
	"fmt"
	"strconv"

	"github.com/cockroachdb/pebble"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/nepal80m/samurai/internal/math"
)

func GetCurrentBalanceInfoKey(account common.Address) string {
	// key := "current_balance_info:" + account.Hex()
	return "user:" + account.Hex() + ":current_balance_info"
}

func StoreCurrentBalanceInfo(account common.Address, currentBalance *CurrentBalance, db *pebble.DB) {
	key := GetCurrentBalanceInfoKey(account)
	val, err := rlp.EncodeToBytes(currentBalance)
	if err != nil {
		panic(fmt.Errorf("failed to encode current balance info: %w", err))
	}
	err = db.Set([]byte(key), val, pebble.Sync)
	if err != nil {
		panic(fmt.Errorf("failed to store current balance info: %w", err))
	}
}

func GetCurrentBalanceInfo(account common.Address, db *pebble.DB) (*CurrentBalance, error) {
	key := GetCurrentBalanceInfoKey(account)
	val, closer, err := db.Get([]byte(key))
	if err != nil {
		// if err == pebble.ErrNotFound {
		// 	return nil, fmt.Errorf("key %s not found", key)
		// } else {
		// 	return nil, err
		// }
		return nil, err
	}
	var cbInfo CurrentBalance
	rlp.DecodeBytes(val, &cbInfo)
	closer.Close()
	return &cbInfo, nil
}

func GetHistoricalBalanceByHashKey(hbHash common.Hash) string {
	return "historical_balance_info:" + hbHash.Hex()
}
func GetHistoricalBalanceKey(account common.Address, version uint64) string {
	return "user:" + account.Hex() + ":historical_balance_info:" + strconv.Itoa(int(version))
}

func StoreHistoricalBalanceByHash(historicalBalance *HistoricalBalance, db *pebble.DB) {
	hbBytes, err := rlp.EncodeToBytes(historicalBalance)
	if err != nil {
		panic(fmt.Errorf("failed to encode historical balance: %w", err))
	}
	hbHash := BytesToPoseidonHash(hbBytes)
	key := GetHistoricalBalanceByHashKey(hbHash)
	err = db.Set([]byte(key), hbBytes, pebble.Sync)
	if err != nil {
		panic(fmt.Errorf("failed to store historical balance: %w", err))
	}
}

func StoreHistoricalBalance(account common.Address, historicalBalance *HistoricalBalance, db *pebble.DB) {
	key := GetHistoricalBalanceKey(account, historicalBalance.Version)
	hbBytes, err := rlp.EncodeToBytes(historicalBalance)
	if err != nil {
		panic(fmt.Errorf("failed to encode historical balance: %w", err))
	}
	err = db.Set([]byte(key), hbBytes, pebble.Sync)
	if err != nil {
		panic(fmt.Errorf("failed to store historical balance: %w", err))
	}
}

func GetHistoricalBalanceByHash(hbHash common.Hash, db *pebble.DB) *HistoricalBalance {
	key := GetHistoricalBalanceByHashKey(hbHash)
	val, closer, err := db.Get([]byte(key))
	if err != nil {
		if err == pebble.ErrNotFound {
			panic(fmt.Errorf("key %s not found", key))
		} else {
			panic(fmt.Errorf("failed to get historical balance: %w", err))
		}
	}
	var historicalBalance HistoricalBalance
	rlp.DecodeBytes(val, &historicalBalance)
	closer.Close()
	return &historicalBalance
}

func GetHistoricalBalance(account common.Address, version uint64, db *pebble.DB) *HistoricalBalance {
	key := GetHistoricalBalanceKey(account, version)
	val, closer, err := db.Get([]byte(key))
	if err != nil {
		panic(fmt.Errorf("failed to get historical balance: %w", err))
	}
	var historicalBalance HistoricalBalance
	rlp.DecodeBytes(val, &historicalBalance)
	closer.Close()
	return &historicalBalance
}

func GetCurrentBatchTreeKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_tree:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

func StoreCurrentBatchTree(account common.Address, version uint64, batchTree *BatchTree, db *pebble.DB) {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(uint64(layer))
		key := GetCurrentBatchTreeKey(account, layer, batchIdx)
		val, err := rlp.EncodeToBytes(batchTree[layer-1])
		if err != nil {
			panic(fmt.Errorf("failed to encode batch tree: %w", err))
		}
		err = db.Set([]byte(key), val, pebble.Sync)
		if err != nil {
			panic(fmt.Errorf("failed to store batch tree: %w", err))
		}
	}
}

func GetCurrentBatchTree(account common.Address, version uint64, db *pebble.DB) *BatchTree {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	var batchTree BatchTree
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(uint64(layer))
		key := GetCurrentBatchTreeKey(account, layer, batchIdx)
		val, closer, err := db.Get([]byte(key))
		if err != nil {
			if err == pebble.ErrNotFound {
				panic(fmt.Errorf("batch tree not found for layer %d and batch index %d", layer, batchIdx))
			} else {
				panic(fmt.Errorf("failed to get batch tree: %w", err))
			}
		}
		var tree []common.Hash
		rlp.DecodeBytes(val, &tree)
		batchTree[layer-1] = tree
		closer.Close()
	}
	return &batchTree
}

func GetCurrentBatchCommitmentsKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_commitments:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

func StoreCurrentBatchCommitments(account common.Address, version uint64, batchCommitments *BatchCommitments, db *pebble.DB) {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(uint64(layer))
		key := GetCurrentBatchCommitmentsKey(account, layer, batchIdx)
		val, err := rlp.EncodeToBytes(batchCommitments[layer-1])
		if err != nil {
			panic(fmt.Errorf("failed to encode batch commitments: %w", err))
		}
		err = db.Set([]byte(key), val, pebble.Sync)
		if err != nil {
			panic(fmt.Errorf("failed to store batch commitments: %w", err))
		}
	}
}

func GetCurrentBatchCommitments(account common.Address, version uint64, db *pebble.DB) *BatchCommitments {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	var batchCommitments BatchCommitments
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(uint64(layer))
		key := GetCurrentBatchCommitmentsKey(account, layer, batchIdx)
		val, closer, err := db.Get([]byte(key))
		if err != nil {
			if err == pebble.ErrNotFound {
				panic(fmt.Errorf("batch commitments not found for layer %d and batch index %d", layer, batchIdx))
			} else {
				panic(fmt.Errorf("failed to get batch commitments: %w", err))
			}
		}
		var commitments gnark_kzg.Digest
		rlp.DecodeBytes(val, &commitments)
		batchCommitments[layer-1] = commitments
		closer.Close()
	}
	return &batchCommitments
}

// func GetBatchTree(account common.Address, batchIdx uint64, db *pebble.DB) (*BatchTree, error) {
// 	key := "batch_tree:" + account.Hex() + ":" + strconv.Itoa(int(batchIdx))
// 	val, closer, err := db.Get([]byte(key))
// 	if err != nil {
// 		if err == pebble.ErrNotFound {
// 			return nil, fmt.Errorf("key %s not found", key)
// 		} else {
// 			return nil, err
// 		}
// 	}
// 	var batchTree BatchTree
// 	rlp.DecodeBytes(val, &batchTree)
// 	closer.Close()
// 	return &batchTree, nil

// }

// func GetAccountInfo(account common.Address, db *pebble.DB) (*segmenttree.AccountInfo, error) {
// 	cbInfo, err := GetCurrentBalanceInfo(account, db)
// 	if err != nil {
// 		return nil, err
// 	}

// 	key := "account_balance_info:" + account.Hex()
// 	val, closer, err := db.Get([]byte(key))
// 	if err != nil {
// 		if err == pebble.ErrNotFound {
// 			return nil, fmt.Errorf("key %s not found", key)
// 		} else {
// 			return nil, err
// 		}
// 	}
// 	var accountBalanceInfo segmenttree.AccountBalanceInfo
// 	rlp.DecodeBytes(val, &accountBalanceInfo)
// 	closer.Close()
// 	return &accountBalanceInfo, nil
// }

// func StoreSegmentTree(account common.Address, segmentTree *segmenttree.SegmentTree, db *pebble.DB, maxLayer int) error {
// 	for layer := 1; layer <= maxLayer; layer++ {
// 		tree_key := "user:" + account.Hex() + ":tree:" + strconv.Itoa(layer)
// 		val, err := rlp.EncodeToBytes(segmentTree.LXTreeV3[layer])
// 		if err != nil {
// 			return fmt.Errorf("failed to encode segment tree: %w", err)
// 		}
// 		err = db.Set([]byte(tree_key), val, pebble.Sync)
// 		if err != nil {
// 			return fmt.Errorf("failed to store segment tree: %w", err)
// 		}
// 	}
// 	return nil
// }

// func GetSegmentTree(account common.Address, db *pebble.DB, maxLayer int) (map[int][]common.Hash, error) {
// 	segmentTree := make(map[int][]common.Hash)
// 	for layer := 1; layer <= maxLayer; layer++ {
// 		tree_key := "user:" + account.Hex() + ":tree:" + strconv.Itoa(layer)
// 		val, closer, err := db.Get([]byte(tree_key))
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to get segment tree: %w", err)
// 		}
// 		var tree []common.Hash
// 		rlp.DecodeBytes(val, &tree)
// 		segmentTree[layer] = tree
// 		closer.Close()
// 	}
// 	return segmentTree, nil
// }

// func StorePolynomial(account common.Address, segmentTree *segmenttree.SegmentTree, db *pebble.DB, maxLayer int) error {
// 	for layer := 1; layer <= maxLayer; layer++ {
// 		polynomial_key := "user:" + account.Hex() + ":polynomial:" + strconv.Itoa(layer)
// 		val, err := rlp.EncodeToBytes(segmentTree.LXPolynomialV3[layer])
// 		if err != nil {
// 			return fmt.Errorf("failed to encode polynomial: %w", err)
// 		}
// 		err = db.Set([]byte(polynomial_key), val, pebble.Sync)
// 		if err != nil {
// 			return fmt.Errorf("failed to store polynomial: %w", err)
// 		}
// 	}
// 	return nil
// }

// func GetPolynomial(account common.Address, db *pebble.DB, maxLayer int) (map[int]polynomial.Polynomial, error) {
// 	polyMap := make(map[int]polynomial.Polynomial)
// 	for layer := 1; layer <= maxLayer; layer++ {
// 		polynomial_key := "user:" + account.Hex() + ":polynomial:" + strconv.Itoa(layer)
// 		val, closer, err := db.Get([]byte(polynomial_key))
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to get polynomial: %w", err)
// 		}
// 		var poly polynomial.Polynomial
// 		rlp.DecodeBytes(val, &poly)
// 		polyMap[layer] = poly
// 		closer.Close()
// 	}
// 	return polyMap, nil
// }
