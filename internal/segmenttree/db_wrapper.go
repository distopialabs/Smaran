package segmenttree

import (
	"fmt"
	"strconv"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"google.golang.org/protobuf/proto"

	"github.com/nepal80m/samurai/internal/math"
	segmenttreepb "github.com/nepal80m/samurai/internal/segmenttree/pb"
)

type DBEncoding int

const (
	EncodingRLP DBEncoding = iota
	EncodingProto
)

var dbEncoding DBEncoding = EncodingProto

func SetDBEncoding(enc DBEncoding) {
	dbEncoding = enc
}

func GenerateCurrentBalanceInfoKey(account common.Address) string {
	// key := "current_balance_info:" + account.Hex()
	return "user:" + account.Hex() + ":current_balance_info"
}

func StoreCurrentBalanceInfo(account common.Address, currentBalance *CurrentBalance, db DB) {
	key := GenerateCurrentBalanceInfoKey(account)
	var val []byte
	var err error
	if dbEncoding == EncodingProto {
		val, err = proto.Marshal(protoFromCurrentBalance(currentBalance))
	} else {
		val, err = rlp.EncodeToBytes(currentBalance)
	}
	if err != nil {
		panic(fmt.Errorf("failed to encode current balance info: %w", err))
	}
	err = db.Set([]byte(key), val, false)
	if err != nil {
		panic(fmt.Errorf("failed to store current balance info: %w", err))
	}
}
func BatchStoreCurrentBalanceInfo(account common.Address, currentBalance *CurrentBalance, b Batch) {
	key := GenerateCurrentBalanceInfoKey(account)
	var val []byte
	var err error
	if dbEncoding == EncodingProto {
		val, err = proto.Marshal(protoFromCurrentBalance(currentBalance))
	} else {
		val, err = rlp.EncodeToBytes(currentBalance)
	}
	if err != nil {
		panic(fmt.Errorf("failed to encode current balance info: %w", err))
	}
	b.Set([]byte(key), val, false)
}

func GetCurrentBalanceInfo(account common.Address, db DB) (*CurrentBalance, error) {
	key := GenerateCurrentBalanceInfoKey(account)
	val, err := db.Get([]byte(key))
	if err != nil {
		if err == ErrNotFound {
			return nil, err
		} else {
			panic(err)
		}
		// return nil, err
	}
	var cbInfo *CurrentBalance
	if dbEncoding == EncodingProto {
		pb := &segmenttreepb.CurrentBalance{}
		if err := proto.Unmarshal(val, pb); err != nil {
			panic(fmt.Errorf("failed to decode current balance info: %w", err))
		}
		cbInfo = currentBalanceFromProto(pb)
	} else {
		var x CurrentBalance
		if err := rlp.DecodeBytes(val, &x); err != nil {
			panic(fmt.Errorf("failed to decode current balance info: %w", err))
		}
		cbInfo = &x
	}
	return cbInfo, nil
}

func GenerateHistoricalBalanceByHashKey(hbHash common.Hash) string {
	return "historical_balance_info:" + hbHash.Hex()
}
func GenerateHistoricalBalanceKey(account common.Address, version uint64) string {
	return "user:" + account.Hex() + ":historical_balance_info:" + strconv.Itoa(int(version))
}

func StoreHistoricalBalanceByHash(historicalBalance *HistoricalBalance, db DB) {
	var hbBytes []byte
	var err error
	if dbEncoding == EncodingProto {
		hbBytes, err = proto.Marshal(protoFromHistoricalBalance(historicalBalance))
	} else {
		hbBytes, err = rlp.EncodeToBytes(historicalBalance)
	}
	if err != nil {
		panic(fmt.Errorf("failed to encode historical balance: %w", err))
	}
	hbHash := BytesToPoseidonHash(hbBytes)
	key := GenerateHistoricalBalanceByHashKey(hbHash)
	err = db.Set([]byte(key), hbBytes, true)
	if err != nil {
		panic(fmt.Errorf("failed to store historical balance: %w", err))
	}
}

func StoreHistoricalBalance(account common.Address, historicalBalance *HistoricalBalance, db DB) {
	key := GenerateHistoricalBalanceKey(account, historicalBalance.Version)
	var hbBytes []byte
	var err error
	if dbEncoding == EncodingProto {
		hbBytes, err = proto.Marshal(protoFromHistoricalBalance(historicalBalance))
	} else {
		hbBytes, err = rlp.EncodeToBytes(historicalBalance)
	}
	if err != nil {
		panic(fmt.Errorf("failed to encode historical balance: %w", err))
	}
	err = db.Set([]byte(key), hbBytes, false)
	if err != nil {
		panic(fmt.Errorf("failed to store historical balance: %w", err))
	}
}

func GetHistoricalBalanceByHash(hbHash common.Hash, db DB) *HistoricalBalance {
	key := GenerateHistoricalBalanceByHashKey(hbHash)
	val, err := db.Get([]byte(key))
	if err != nil {
		if err == ErrNotFound {
			panic(fmt.Errorf("key %s not found", key))
		} else {
			panic(fmt.Errorf("failed to get historical balance: %w", err))
		}
	}
	var historicalBalance *HistoricalBalance
	if dbEncoding == EncodingProto {
		pb := &segmenttreepb.HistoricalBalance{}
		if err := proto.Unmarshal(val, pb); err != nil {
			panic(fmt.Errorf("failed to decode historical balance: %w", err))
		}
		historicalBalance = historicalBalanceFromProto(pb)
	} else {
		var x HistoricalBalance
		if err := rlp.DecodeBytes(val, &x); err != nil {
			panic(fmt.Errorf("failed to decode historical balance: %w", err))
		}
		historicalBalance = &x
	}
	return historicalBalance
}

func GetHistoricalBalance(account common.Address, version uint64, db DB) *HistoricalBalance {
	key := GenerateHistoricalBalanceKey(account, version)
	val, err := db.Get([]byte(key))
	if err != nil {
		panic(fmt.Errorf("failed to get historical balance for version %d: %w", version, err))
	}
	var historicalBalance *HistoricalBalance
	if dbEncoding == EncodingProto {
		pb := &segmenttreepb.HistoricalBalance{}
		if err := proto.Unmarshal(val, pb); err != nil {
			panic(fmt.Errorf("failed to decode historical balance: %w", err))
		}
		historicalBalance = historicalBalanceFromProto(pb)
	} else {
		var x HistoricalBalance
		if err := rlp.DecodeBytes(val, &x); err != nil {
			panic(fmt.Errorf("failed to decode historical balance: %w", err))
		}
		historicalBalance = &x
	}
	return historicalBalance
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
func StoreCurrentLXBatchTree(account common.Address, batchTree *LXBatchTree, db DB) {
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentLXBatchTreeKey(account, layer)
		var err error
		val := batchTree[layer-1].MarshalBinary()
		// if dbEncoding == EncodingProto {
		// 	bt := (*BatchTree)(&batchTree[layer-1])
		// 	val, err = proto.Marshal(protoFromBatchTree(bt))
		// } else {
		// 	val, err = rlp.EncodeToBytes(batchTree[layer-1])
		// }
		// if err != nil {
		// 	panic(fmt.Errorf("failed to encode batch tree: %w", err))
		// }
		err = db.Set([]byte(key), val, false)
		if err != nil {
			panic(fmt.Errorf("failed to store batch tree: %w", err))
		}
	}
}

func BatchStoreCurrentLXBatchTree(account common.Address, batchTree *LXBatchTree, b Batch) {
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentLXBatchTreeKey(account, layer)
		// var val []byte
		// var err error
		val := batchTree[layer-1].MarshalBinary()
		// if dbEncoding == EncodingProto {
		// 	bt := (*BatchTree)(&batchTree[layer-1])
		// 	val, err = proto.Marshal(protoFromBatchTree(bt))
		// } else {
		// 	val, err = rlp.EncodeToBytes(batchTree[layer-1])
		// }
		// if err != nil {
		// 	panic(fmt.Errorf("failed to encode batch tree: %w", err))
		// }
		b.Set([]byte(key), val, false)
	}
}

func GetCurrentLXBatchTree(account common.Address, db DB) *LXBatchTree {
	var batchTree LXBatchTree
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		key := GenerateCurrentLXBatchTreeKey(account, layer)
		val, err := db.Get([]byte(key))
		if err != nil {
			if err == ErrNotFound {
				panic(fmt.Errorf("batch tree not found for layer %d", layer))
			} else {
				panic(fmt.Errorf("failed to get batch tree: %w", err))
			}
		}
		var tree BatchTree
		if err := tree.UnmarshalBinary(val); err != nil {
			panic(fmt.Errorf("failed to decode batch tree: %w", err))
		}
		batchTree[layer-1] = tree
		// if dbEncoding == EncodingProto {
		// 	pb := &segmenttreepb.BatchTree{}
		// 	if err := proto.Unmarshal(val, pb); err != nil {
		// 		panic(fmt.Errorf("failed to decode batch tree: %w", err))
		// 	}
		// 	tree := batchTreeFromProto(pb)
		// 	batchTree[layer-1] = tree
		// } else {
		// 	var tree BatchTree
		// 	if err := rlp.DecodeBytes(val, &tree); err != nil {
		// 		panic(fmt.Errorf("failed to decode batch tree: %w", err))
		// 	}
		// 	batchTree[layer-1] = tree
		// }
	}
	return &batchTree
}

// GetCurrentLXBatchTreeAndCommitments is a Pebble-specific implementation using iterators
// It was found to be slower than separate Get calls, so it's not currently used
// Keeping it here for reference but commented out since it requires Pebble-specific APIs
/*
func GetCurrentLXBatchTreeAndCommitments(account common.Address, version uint64, db *pebble.DB) (*LXBatchTree, *LXBatchCommitment) {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return lastLeafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	prefix := []byte("user:" + account.Hex() + ":") //TODO: add ":batch_" to prefix
	iterOpts := &pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xFF),
	}

	iter, err := db.NewIter(iterOpts)
	if err != nil {
		panic(fmt.Errorf("failed to create iterator: %w", err))
	}
	defer iter.Close()

	var batchTree LXBatchTree
	var batchCommitments LXBatchCommitment

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if strings.Contains(key, ":batch_tree:") {
			parts := strings.Split(key, ":")
			if len(parts) >= 4 {
				layer, err := strconv.ParseUint(parts[3], 10, 64)
				if err != nil {
					panic(fmt.Errorf("failed to parse layer: %w", err))
				}
				if layer > MaxLayer || layer < 1 {
					panic(fmt.Errorf("layer %d is not supported", layer))
				}
				var tree BatchTree
				if err := tree.UnmarshalBinary(iter.Value()); err != nil {
					panic(fmt.Errorf("failed to decode batch tree: %w", err))
				}
				batchTree[layer-1] = tree
			}
		} else if strings.Contains(key, ":batch_commitments:") {
			parts := strings.Split(key, ":")
			if len(parts) >= 5 {
				layer, err := strconv.ParseUint(parts[3], 10, 64)
				if err != nil {
					panic(fmt.Errorf("failed to parse layer: %w", err))
				}
				if layer > MaxLayer || layer < 1 {
					panic(fmt.Errorf("layer %d is not supported", layer))
				}
				batchIdx, err := strconv.ParseUint(parts[4], 10, 64)
				if err != nil {
					panic(fmt.Errorf("failed to parse batch index: %w", err))
				}
				// TODO: uncomment this, and check if invalid batch index used when storing.
				// if batchIdx > L1BatchSize*math.Pow(L2BatchSize, layer-1) {
				// 	panic(fmt.Errorf("batch index %d is not supported", batchIdx))
				// }
				expectedBatchIdx := lxBatchIdx(layer)
				if expectedBatchIdx == batchIdx {
					var commitment gnark_kzg.Digest
					_, err := commitment.SetBytes(iter.Value())
					if err != nil {
						panic(fmt.Errorf("failed to set commitment: %w", err))
					}
					batchCommitments[layer-1] = commitment
				}
			}
		}
	}
	if err := iter.Error(); err != nil {
		panic(fmt.Errorf("failed to iterate: %w", err))
	}
	return &batchTree, &batchCommitments
}
*/

func GenerateBatchCommitmentsKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_commitments:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

func StoreLXBatchCommitments(account common.Address, version uint64, batchCommitments *LXBatchCommitment, db DB) {
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
		valBytes := batchCommitments[layer-1].Bytes()
		val := make([]byte, len(valBytes))
		copy(val, valBytes[:])
		err := db.Set([]byte(key), val, false)
		if err != nil {
			panic(fmt.Errorf("failed to store batch commitments: %w", err))
		}
	}
}

func BatchStoreLXBatchCommitments(account common.Address, version uint64, batchCommitments *LXBatchCommitment, b Batch) {
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
		valBytes := batchCommitments[layer-1].Bytes()
		val := make([]byte, len(valBytes))
		copy(val, valBytes[:])
		b.Set([]byte(key), val, false)
	}
}

func GetLXBatchCommitments(account common.Address, version uint64, db DB) *LXBatchCommitment {
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
		val, err := db.Get([]byte(key))
		if err != nil {
			if err == ErrNotFound {
				panic(fmt.Errorf("batch commitments not found for layer %d and batch index %d", layer, batchIdx))
			} else {
				panic(fmt.Errorf("failed to get batch commitments: %w", err))
			}
		}

		commitmentBytes := make([]byte, len(val))
		copy(commitmentBytes, val[:])
		var commitment gnark_kzg.Digest
		_, err = commitment.SetBytes(commitmentBytes)
		if err != nil {
			panic(fmt.Errorf("failed to set commitment: %w", err))
		}
		batchCommitments[layer-1] = commitment
	}
	return &batchCommitments
}

func GetBatchCommitment(account common.Address, layer, batchIdx uint64, db DB) gnark_kzg.Digest {
	key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
	val, err := db.Get([]byte(key))
	if err != nil {
		if err == ErrNotFound {
			panic(fmt.Errorf("batch commitment not found for layer %d and batch index %d", layer, batchIdx))
		} else {
			panic(fmt.Errorf("failed to get batch commitments: %w", err))
		}
	}

	commitmentBytes := make([]byte, len(val))
	copy(commitmentBytes, val[:])
	var commitment gnark_kzg.Digest
	_, err = commitment.SetBytes(commitmentBytes)
	if err != nil {
		panic(fmt.Errorf("failed to set commitment: %w", err))
	}

	return commitment
}
