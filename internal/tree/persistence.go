package tree

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"google.golang.org/protobuf/proto"

	"github.com/nepal80m/samurai/internal/db"
	treepb "github.com/nepal80m/samurai/internal/tree/pb"
	"github.com/nepal80m/samurai/internal/utils"
)

// Key generation functions

func GenerateCurrentBalanceInfoKey(account common.Address) string {
	return "user:" + account.Hex() + ":current_balance_info"
}

func GenerateHistoricalBalanceKey(account common.Address, version uint64) string {
	return "user:" + account.Hex() + ":historical_balance_info:" + strconv.Itoa(int(version))
}

func GenerateCurrentLXBatchTreeKey(account common.Address, layer uint64) string {
	return "user:" + account.Hex() + ":batch_tree:" + strconv.Itoa(int(layer))
}

func GenerateBatchTreeChunkKey(account common.Address, layer uint64, chunkIdx int) string {
	return "user:" + account.Hex() + ":batch_tree:" + strconv.Itoa(int(layer)) + ":chunk:" + strconv.Itoa(chunkIdx)
}

func GenerateBatchCommitmentsKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_commitments:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

func GenerateBatchRootKey(account common.Address, layer, batchIdx uint64) string {
	return "user:" + account.Hex() + ":batch_root:" + strconv.Itoa(int(layer)) + ":" + strconv.Itoa(int(batchIdx))
}

// Store/Get CurrentBalanceInfo

func StoreCurrentBalanceInfo(account common.Address, cb *CurrentBalance, d *db.DB) {
	key := GenerateCurrentBalanceInfoKey(account)
	val, err := proto.Marshal(protoFromCurrentBalance(cb))
	if err != nil {
		panic(fmt.Errorf("failed to encode current balance info: %w", err))
	}
	if err := (*d).Set([]byte(key), val, false); err != nil {
		panic(fmt.Errorf("failed to store current balance info: %w", err))
	}
}

func GetCurrentBalanceInfo(account common.Address, d *db.DB) (*CurrentBalance, error) {
	key := GenerateCurrentBalanceInfoKey(account)
	val, err := (*d).Get([]byte(key))
	if err != nil {
		if err == db.ErrNotFound {
			return nil, err
		}
		panic(err)
	}
	pb := &treepb.CurrentBalance{}
	if err := proto.Unmarshal(val, pb); err != nil {
		panic(fmt.Errorf("failed to decode current balance info: %w", err))
	}
	return currentBalanceFromProto(pb), nil
}

// Store/Get LXBatchTree

func StoreCurrentLXBatchTree(account common.Address, batchTree *LXBatchTree, dirtyChunks *[MaxLayer]map[int]bool, d *db.DB) {
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		if dirtyChunks == nil {
			totalChunks := SegmentTreeSize / ChunkSize
			for chunkIdx := 0; chunkIdx < totalChunks; chunkIdx++ {
				key := GenerateBatchTreeChunkKey(account, layer, chunkIdx)
				startIdx := chunkIdx * ChunkSize
				endIdx := startIdx + ChunkSize
				chunkData := make([]byte, (endIdx-startIdx)*common.HashLength)
				for i := startIdx; i < endIdx; i++ {
					copy(chunkData[(i-startIdx)*common.HashLength:], batchTree[layer-1][i][:])
				}
				if err := (*d).Set([]byte(key), chunkData, false); err != nil {
					panic(fmt.Errorf("failed to store batch tree chunk: %w", err))
				}
			}
		} else {
			chunksToUpdate := dirtyChunks[layer-1]
			for chunkIdx := range chunksToUpdate {
				key := GenerateBatchTreeChunkKey(account, layer, chunkIdx)
				startIdx := chunkIdx * ChunkSize
				endIdx := startIdx + ChunkSize
				if endIdx > SegmentTreeSize {
					endIdx = SegmentTreeSize
				}

				// Check if the chunk is entirely empty (all zero hashes)
				isEmpty := true
				for i := startIdx; i < endIdx; i++ {
					if (batchTree[layer-1][i] != common.Hash{}) {
						isEmpty = false
						break
					}
				}

				if isEmpty {
					// Delete stale chunk from DB to reclaim space
					if err := (*d).Delete([]byte(key), false); err != nil {
						panic(fmt.Errorf("failed to delete empty batch tree chunk: %w", err))
					}
				} else {
					chunkData := make([]byte, (endIdx-startIdx)*common.HashLength)
					for i := startIdx; i < endIdx; i++ {
						copy(chunkData[(i-startIdx)*common.HashLength:], batchTree[layer-1][i][:])
					}
					if err := (*d).Set([]byte(key), chunkData, false); err != nil {
						panic(fmt.Errorf("failed to store batch tree chunk: %w", err))
					}
				}
			}
		}
	}
}

func GetCurrentLXBatchTree(account common.Address, d *db.DB) *LXBatchTree {
	var batchTree LXBatchTree
	pdb, ok := (*d).(*db.PebbleDB)
	if !ok {
		panic("GetCurrentLXBatchTree requires PebbleDB for iteration")
	}

	for layer := uint64(1); layer <= MaxLayer; layer++ {
		prefix := []byte(GenerateCurrentLXBatchTreeKey(account, layer) + ":chunk:")

		iterOpts := &pebble.IterOptions{
			LowerBound: prefix,
			UpperBound: func() []byte {
				ub := make([]byte, len(prefix))
				copy(ub, prefix)
				for i := len(ub) - 1; i >= 0; i-- {
					ub[i]++
					if ub[i] != 0 {
						break
					}
				}
				return ub
			}(),
		}

		iter, err := pdb.InnerDB().NewIter(iterOpts)
		if err != nil {
			panic(fmt.Errorf("failed to create iterator: %w", err))
		}
		defer iter.Close()

		for iter.First(); iter.Valid(); iter.Next() {
			key := string(iter.Key())
			parts := strings.Split(key, ":")
			chunkIdxStr := parts[len(parts)-1]
			chunkIdx, err := strconv.Atoi(chunkIdxStr)
			if err != nil {
				panic(fmt.Errorf("invalid chunk index in key %s: %w", key, err))
			}

			val := iter.Value()
			startIdx := chunkIdx * ChunkSize
			for i := 0; i < len(val); i += common.HashLength {
				treeIdx := startIdx + (i / common.HashLength)
				if treeIdx < SegmentTreeSize {
					copy(batchTree[layer-1][treeIdx][:], val[i:i+common.HashLength])
				}
			}
		}

		if err := iter.Error(); err != nil {
			panic(fmt.Errorf("iterator error: %w", err))
		}
	}
	return &batchTree
}

// Store/Get LXBatchCommitments

func StoreLXBatchCommitments(account common.Address, version uint64, bc *LXBatchCommitment, d *db.DB) {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		return lastLeafNodeIdx / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-1))
	}

	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(layer)
		key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
		valBytes := bc[layer-1].Bytes()
		val := make([]byte, len(valBytes))
		copy(val, valBytes[:])
		if err := (*d).Set([]byte(key), val, false); err != nil {
			panic(fmt.Errorf("failed to store batch commitments: %w", err))
		}
	}
}

func GetLXBatchCommitments(account common.Address, version uint64, d *db.DB) *LXBatchCommitment {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		return lastLeafNodeIdx / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-1))
	}

	var bc LXBatchCommitment
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(layer)
		key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
		val, err := (*d).Get([]byte(key))
		if err != nil {
			if err == db.ErrNotFound {
				panic(fmt.Errorf("batch commitments not found for layer %d batch %d", layer, batchIdx))
			}
			panic(fmt.Errorf("failed to get batch commitments: %w", err))
		}
		var commitment gnark_kzg.Digest
		if _, err := commitment.SetBytes(val); err != nil {
			panic(fmt.Errorf("failed to decode commitment: %w", err))
		}
		bc[layer-1] = commitment
	}
	return &bc
}

func GetBatchCommitment(account common.Address, layer, batchIdx uint64, d db.DB) gnark_kzg.Digest {
	key := GenerateBatchCommitmentsKey(account, layer, batchIdx)
	val, err := d.Get([]byte(key))
	if err != nil {
		panic(fmt.Errorf("failed to get batch commitment: %w", err))
	}
	var commitment gnark_kzg.Digest
	if _, err := commitment.SetBytes(val); err != nil {
		panic(fmt.Errorf("failed to decode commitment: %w", err))
	}
	return commitment
}

// Store/Get LXBatchRoots

func StoreLXBatchRoots(account common.Address, version uint64, batchTree *LXBatchTree, d *db.DB) {
	lastLeafNodeIdx := version - 1
	lxBatchIdx := func(layer uint64) uint64 {
		return lastLeafNodeIdx / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-1))
	}

	for layer := uint64(1); layer <= MaxLayer; layer++ {
		batchIdx := lxBatchIdx(layer)
		key := GenerateBatchRootKey(account, layer, batchIdx)
		root := batchTree[layer-1][0]
		if err := (*d).Set([]byte(key), root[:], false); err != nil {
			panic(fmt.Errorf("failed to store batch root: %w", err))
		}
	}
}

func GetBatchRoot(account common.Address, layer, batchIdx uint64, d db.DB) common.Hash {
	key := GenerateBatchRootKey(account, layer, batchIdx)
	val, err := d.Get([]byte(key))
	if err != nil {
		panic(fmt.Errorf("failed to get batch root: %w", err))
	}
	return common.BytesToHash(val)
}

// Store/Get HistoricalBalance

func StoreHistoricalBalance(account common.Address, hb *HistoricalBalance, d db.DB) {
	key := GenerateHistoricalBalanceKey(account, hb.Version)
	val, err := proto.Marshal(protoFromHistoricalBalance(hb))
	if err != nil {
		panic(fmt.Errorf("failed to encode historical balance: %w", err))
	}
	if err := d.Set([]byte(key), val, false); err != nil {
		panic(fmt.Errorf("failed to store historical balance: %w", err))
	}
}

func GetHistoricalBalance(account common.Address, version uint64, d *db.DB) *HistoricalBalance {
	key := GenerateHistoricalBalanceKey(account, version)
	val, err := (*d).Get([]byte(key))
	if err != nil {
		panic(fmt.Errorf("failed to get historical balance for version %d: %w", version, err))
	}
	pb := &treepb.HistoricalBalance{}
	if err := proto.Unmarshal(val, pb); err != nil {
		panic(fmt.Errorf("failed to decode historical balance: %w", err))
	}
	return historicalBalanceFromProto(pb)
}
