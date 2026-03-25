package proof

import (
	"fmt"
	"strconv"
	"time"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"
)

// RebuildSegmentTreeForProof builds only the required batch trees from stored data.
//
// Layer 1: fetches historical balances (leaves) from DB for each required batch,
//
//	inserts them as leaves, and computes intermediary nodes.
//
// Layer 2+: fetches stored batch roots from DB as leaf values,
//
//	computes intermediary nodes, and fills commitment hashes from stored commitments.
//
// Returns the tree batches map, all required historical balances, and a map of cached
// commitments keyed by "layer:batchIdx" to avoid redundant DB fetches downstream.
func RebuildSegmentTreeForProof(account common.Address, lxRequiredBatchIdxs map[uint64][]uint64, startingVersion uint64, endingVersion uint64, sdb *db.SamuraiStore, precomputedData *config.PrecomputedData) (map[string]tree.BatchTree, []*tree.HistoricalBalance, map[string]gnark_kzg.Digest) {
	cbInfo, err := tree.GetCurrentBalanceInfo(account, &sdb.StateDB)
	if err != nil {
		panic(err)
	}

	requiredTreeBatchesMap := make(map[string]tree.BatchTree)
	cachedCommitments := make(map[string]gnark_kzg.Digest)

	start := time.Now()

	// --- Fetch ALL historical balances in [startingVersion, endingVersion] once ---
	// Used for both the return value (verify path) and L1 tree building (dedup).
	requiredHBInfos := make([]*tree.HistoricalBalance, 0, endingVersion-startingVersion+1)
	for version := startingVersion; version <= endingVersion; version++ {
		hbInfo := tree.GetHistoricalBalance(account, version, &sdb.HistoryDB)
		requiredHBInfos = append(requiredHBInfos, hbInfo)
	}

	fmt.Printf("Time taken to fetch required HB infos: %v\n", time.Since(start))
	start = time.Now()

	// --- Layer 1: build each required L1 batch tree from historical balances ---
	for _, batchIdx := range lxRequiredBatchIdxs[1] {
		var batchTree tree.BatchTree

		versionStart := batchIdx * L1BatchSize
		versionEnd := (batchIdx+1)*L1BatchSize - 1
		// Clamp to the latest historical version
		if versionEnd >= cbInfo.Version {
			versionEnd = cbInfo.Version - 1
		}

		for version := versionStart; version <= versionEnd; version++ {
			// Reuse already-fetched HB if version falls in [startingVersion, endingVersion],
			// otherwise fetch individually (for versions outside the requested range).
			var hbInfo *tree.HistoricalBalance
			if version >= startingVersion && version <= endingVersion {
				hbInfo = requiredHBInfos[version-startingVersion]
			} else {
				hbInfo = tree.GetHistoricalBalance(account, version, &sdb.HistoryDB)
			}

			// Insert leaf at the correct offset position
			leafIdx := L1BatchSize - 1 + int(version%L1BatchSize)
			hbHash := hbInfo.Hash()
			updateBatchTree(&batchTree, uint64(leafIdx), hbHash)
		}

		key := fmt.Sprintf("1:%d", batchIdx)
		requiredTreeBatchesMap[key] = batchTree
	}

	fmt.Printf("Time taken to build L1 batch trees: %v\n", time.Since(start))

	// --- Layer 2+: build each required upper-layer batch tree from stored roots ---
	start = time.Now()
	for layer := uint64(2); layer <= MaxLayer; layer++ {
		// Latest batch index at the lower layer
		latestLowerBatchIdx := (cbInfo.Version - 1) / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-2))

		for _, batchIdx := range lxRequiredBatchIdxs[layer] {
			var batchTree tree.BatchTree

			// Determine range of lower-layer batches covered by this upper-layer batch
			lowerBatchStart := batchIdx * L2BatchSize
			lowerBatchEnd := lowerBatchStart + L2BatchSize - 1
			if lowerBatchEnd > latestLowerBatchIdx {
				lowerBatchEnd = latestLowerBatchIdx
			}

			// Fill leaf positions with stored batch roots from the lower layer
			for lowerBatchIdx := lowerBatchStart; lowerBatchIdx <= lowerBatchEnd; lowerBatchIdx++ {
				// Leaf offset in the segment tree for this lower-layer batch
				leafIdx := L2BatchSize - 1 + int(lowerBatchIdx%L2BatchSize)
				root := tree.GetBatchRoot(account, layer-1, lowerBatchIdx, sdb.StateDB)
				updateBatchTree(&batchTree, uint64(leafIdx), root)
			}

			// Fill commitment hash positions and cache the commitments
			InsertCommitmentHashes(layer, batchIdx, &batchTree, account, cbInfo.Version, sdb, cachedCommitments)

			key := fmt.Sprintf("%d:%d", layer, batchIdx)
			requiredTreeBatchesMap[key] = batchTree
		}
	}

	fmt.Printf("Time taken to build upper layer batch trees: %v\n", time.Since(start))

	return requiredTreeBatchesMap, requiredHBInfos, cachedCommitments
}

// updateBatchTree inserts a value at the given leaf index and propagates parent hashes upward.
func updateBatchTree(batchTree *tree.BatchTree, idx uint64, val common.Hash) {
	if (val == common.Hash{}) {
		return
	}
	batchTree[idx] = val
	for idx > 0 {
		parentIdx := tree.GetParent(idx)
		lChild := batchTree[2*parentIdx+1]
		rChild := batchTree[2*parentIdx+2]
		if (lChild == common.Hash{} || rChild == common.Hash{}) {
			break
		}
		batchTree[parentIdx] = hash.BytesToHash(lChild.Bytes(), rChild.Bytes())
		idx = parentIdx
	}
}

func InsertCommitmentHashes(layer uint64, batchIdx uint64, batchTree *tree.BatchTree, account common.Address, latestVersion uint64, sdb *db.SamuraiStore, cachedCommitments map[string]gnark_kzg.Digest) {
	if layer <= 1 || layer > MaxLayer {
		panic("layer" + fmt.Sprintf("%d", layer) + " is invalid")
	}
	latestLxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + fmt.Sprintf("%d", layer) + " is not supported")
		}
		return latestVersion / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-1))
	}

	lxm1BatchIdxStart := batchIdx * L2BatchSize
	lxm1BatchIdxEnd := min(lxm1BatchIdxStart+L2BatchSize-1, latestLxBatchIdx(layer-1))
	for bIdx := lxm1BatchIdxStart; bIdx <= lxm1BatchIdxEnd; bIdx++ {
		commitment := tree.GetBatchCommitment(account, layer-1, bIdx, sdb.StateDB)
		// Cache the commitment so GetNewProofRange can reuse it
		cacheKey := fmt.Sprintf("%d:%d", layer-1, bIdx)
		cachedCommitments[cacheKey] = commitment
		commitmentHash := hash.CommitmentToHash(commitment)
		treeIdx := bIdx - lxm1BatchIdxStart + (2 * L2BatchSize) - 1
		batchTree[treeIdx] = commitmentHash
	}
}

// AddLeafNode adds a leaf to the multi-layer tree (used by the verify path).
func AddLeafNode(accountInfo *tree.AccountInfo, leafNodeIdx uint64, leafNodeHash common.Hash) {
	// find which index to update for each layer
	lxBatchNodeIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		if layer == 1 {
			return leafNodeIdx % L1BatchSize
		} else {
			return leafNodeIdx / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-2)) % L2BatchSize
		}
	}
	lxBatchLeafNodeOffsetIdx := func(layer uint64) uint64 {
		idx := lxBatchNodeIdx(layer)
		if layer == 1 {
			return L1BatchSize - 1 + idx
		} else {
			return L2BatchSize - 1 + idx
		}
	}

	// Resetting for new batch
	for layer := 1; layer <= MaxLayer; layer++ {
		if (leafNodeIdx % (L1BatchSize * utils.PowUint64(L2BatchSize, uint64(layer)-1))) == 0 {
			accountInfo.CurrentLXBatchTree[layer-1] = tree.BatchTree{}
		}
	}
	lXm1RootHash := leafNodeHash
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		UpdateLXTree(accountInfo, lxBatchLeafNodeOffsetIdx(layer), lXm1RootHash, layer)
		lxRootHash := accountInfo.CurrentLXBatchTree[layer-1][0]
		lXm1RootHash = lxRootHash
	}
}

// UpdateLXTree updates a single layer of the batch tree (used by the verify path).
func UpdateLXTree(accountInfo *tree.AccountInfo, idx uint64, val common.Hash, layer uint64) {
	batchTree := &accountInfo.CurrentLXBatchTree[layer-1]

	// updating the tree
	if (val != common.Hash{}) {
		batchTree[idx] = val
		for idx > 0 {
			parentIdx := tree.GetParent(idx)

			lChild := batchTree[2*parentIdx+1]
			rChild := batchTree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			batchTree[parentIdx] = hash.BytesToHash(lChild.Bytes(), rChild.Bytes())

			idx = parentIdx
		}
	}
}
