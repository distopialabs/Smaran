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

func GenerateCurrentBalanceInfoKey(account common.Address) string {
	// key := "current_balance_info:" + account.Hex()
	return "user:" + account.Hex() + ":current_balance_info"
}

func StoreCurrentBalanceInfo(account common.Address, currentBalance *CurrentBalance, db *pebble.DB) {
	key := GenerateCurrentBalanceInfoKey(account)
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
	key := GenerateCurrentBalanceInfoKey(account)
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

func GenerateHistoricalBalanceByHashKey(hbHash common.Hash) string {
	return "historical_balance_info:" + hbHash.Hex()
}
func GenerateHistoricalBalanceKey(account common.Address, version uint64) string {
	return "user:" + account.Hex() + ":historical_balance_info:" + strconv.Itoa(int(version))
}

func StoreHistoricalBalanceByHash(historicalBalance *HistoricalBalance, db *pebble.DB) {
	hbBytes, err := rlp.EncodeToBytes(historicalBalance)
	if err != nil {
		panic(fmt.Errorf("failed to encode historical balance: %w", err))
	}
	hbHash := BytesToPoseidonHash(hbBytes)
	key := GenerateHistoricalBalanceByHashKey(hbHash)
	err = db.Set([]byte(key), hbBytes, pebble.Sync)
	if err != nil {
		panic(fmt.Errorf("failed to store historical balance: %w", err))
	}
}

func StoreHistoricalBalance(account common.Address, historicalBalance *HistoricalBalance, db *pebble.DB) {
	key := GenerateHistoricalBalanceKey(account, historicalBalance.Version)
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
	key := GenerateHistoricalBalanceByHashKey(hbHash)
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
	key := GenerateHistoricalBalanceKey(account, version)
	val, closer, err := db.Get([]byte(key))
	if err != nil {
		panic(fmt.Errorf("failed to get historical balance for version %d: %w", version, err))
	}
	var historicalBalance HistoricalBalance
	rlp.DecodeBytes(val, &historicalBalance)
	closer.Close()
	return &historicalBalance
}

func GenerateCurrentBatchTreeKey(account common.Address, layer uint64) string {
	return "user:" + account.Hex() + ":batch_tree:" + strconv.Itoa(int(layer))
}

// func StoreCurrentBatchTree(account common.Address, version uint64, batchTree *BatchTree, db *pebble.DB) {
// 	lastLeafNodeIdx := version - 1
// 	lxBatchIdx := func(layer uint64) uint64 {
// 		if layer == 0 || layer > MaxLayer {
// 			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
// 		}
// 		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
// 	}

//		for layer := uint64(1); layer <= MaxLayer; layer++ {
//			batchIdx := lxBatchIdx(uint64(layer))
//			key := GenerateCurrentBatchTreeKey(account, layer, batchIdx)
//			val, err := rlp.EncodeToBytes(batchTree[layer-1])
//			if err != nil {
//				panic(fmt.Errorf("failed to encode batch tree: %w", err))
//			}
//			err = db.Set([]byte(key), val, pebble.Sync)
//			if err != nil {
//				panic(fmt.Errorf("failed to store batch tree: %w", err))
//			}
//		}
//	}
func StoreCurrentBatchTree(account common.Address, batchTree *BatchTree, db *pebble.DB) {
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentBatchTreeKey(account, layer)
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

func GetCurrentBatchTree(account common.Address, db *pebble.DB) *BatchTree {
	var batchTree BatchTree
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentBatchTreeKey(account, layer)
		val, closer, err := db.Get([]byte(key))
		if err != nil {
			if err == pebble.ErrNotFound {
				panic(fmt.Errorf("batch tree not found for layer %d", layer))
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

func GenerateBatchCommitmentsKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_commitments:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

func StoreBatchCommitments(account common.Address, version uint64, batchCommitments *BatchCommitments, db *pebble.DB) {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(uint64(layer))
		key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
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

func GetBatchCommitments(account common.Address, version uint64, db *pebble.DB) *BatchCommitments {
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
		key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
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

func GetLxBatchCommitment(account common.Address, layer, batchIdx uint64, db *pebble.DB) gnark_kzg.Digest {
	key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
	val, closer, err := db.Get([]byte(key))
	if err != nil {
		if err == pebble.ErrNotFound {
			panic(fmt.Errorf("batch commitment not found for layer %d and batch index %d", layer, batchIdx))
		} else {
			panic(fmt.Errorf("failed to get batch commitments: %w", err))
		}
	}
	var commitment gnark_kzg.Digest
	rlp.DecodeBytes(val, &commitment)
	closer.Close()
	return commitment
}
