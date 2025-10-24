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
func BatchStoreCurrentBalanceInfo(account common.Address, currentBalance *CurrentBalance, b *pebble.Batch) {
	key := GenerateCurrentBalanceInfoKey(account)
	val, err := rlp.EncodeToBytes(currentBalance)
	if err != nil {
		panic(fmt.Errorf("failed to encode current balance info: %w", err))
	}
	b.Set([]byte(key), val, nil)
}

func GetCurrentBalanceInfo(account common.Address, db *pebble.DB) (*CurrentBalance, error) {
	key := GenerateCurrentBalanceInfoKey(account)
	val, closer, err := db.Get([]byte(key))
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, err
		} else {
			panic(err)
		}
		// return nil, err
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

func GenerateCurrentLXBatchTreeKey(account common.Address, layer uint64) string {
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
func StoreCurrentLXBatchTree(account common.Address, batchTree *LXBatchTree, db *pebble.DB) {
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentLXBatchTreeKey(account, layer)
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

func BatchStoreCurrentLXBatchTree(account common.Address, batchTree *LXBatchTree, b *pebble.Batch) {
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentLXBatchTreeKey(account, layer)
		val, err := rlp.EncodeToBytes(batchTree[layer-1])
		if err != nil {
			panic(fmt.Errorf("failed to encode batch tree: %w", err))
		}
		b.Set([]byte(key), val, nil)
	}
}

func GetCurrentLXBatchTree(account common.Address, db *pebble.DB) *LXBatchTree {
	var batchTree LXBatchTree
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentLXBatchTreeKey(account, layer)
		val, closer, err := db.Get([]byte(key))
		if err != nil {
			if err == pebble.ErrNotFound {
				panic(fmt.Errorf("batch tree not found for layer %d", layer))
			} else {
				panic(fmt.Errorf("failed to get batch tree: %w", err))
			}
		}
		var tree BatchTree
		rlp.DecodeBytes(val, &tree)
		batchTree[layer-1] = tree
		closer.Close()
	}
	return &batchTree
}

func GenerateBatchCommitmentsKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_commitments:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

func StoreLXBatchCommitments(account common.Address, version uint64, batchCommitments *LXBatchCommitment, db *pebble.DB) {
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

func BatchStoreLXBatchCommitments(account common.Address, version uint64, batchCommitments *LXBatchCommitment, b *pebble.Batch) {
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
		b.Set([]byte(key), val, nil)
	}
}

func GetLXBatchCommitments(account common.Address, version uint64, db *pebble.DB) *LXBatchCommitment {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	var batchCommitments LXBatchCommitment
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

func GetBatchCommitment(account common.Address, layer, batchIdx uint64, db *pebble.DB) gnark_kzg.Digest {
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
