package proof

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/nepal80m/samurai/internal/math"

	"github.com/cockroachdb/pebble"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

// rebuilds the whole segment tree
// uses stored commitments to fill the commitHash part of the batch trees
// in this process, stores only the involved batch trees and returns them
func RebuildSegmentTreeForProof(account common.Address, lxRequiredBatchIdxs map[uint64][]uint64, startingVersion uint64, endingVersion uint64, db *pebble.DB, precomputedData *config.PrecomputedData) (map[string][]common.Hash, []*segmenttree.HistoricalBalance) {

	cbInfo, err := segmenttree.GetCurrentBalanceInfo(account, db)
	if err != nil {
		panic(err)
	}
	accountInfo := segmenttree.NewAccountInfo(account, precomputedData)

	// # current balance info
	accountInfo.CurrentBalanceInfo = cbInfo

	// # batch tree data
	// initialize empty tree
	var tree segmenttree.BatchTree
	for i := range segmenttree.MaxLayer {
		tree[i] = make([]common.Hash, segmenttree.SegmentTreeSize)
	}
	accountInfo.CurrentBatchTree = tree
	// add all historical balance info as leaf nodes

	requiredTreeBatchesMap := make(map[string][]common.Hash)
	requiredHBInfos := make([]*segmenttree.HistoricalBalance, 0)

	for version := uint64(0); version < cbInfo.Version; version++ {
		hbInfo := segmenttree.GetHistoricalBalance(account, version, db)

		AddLeafNode(accountInfo, hbInfo.Version, hbInfo.Hash())

		// check if this historical balance info is required
		if hbInfo.Version >= startingVersion && hbInfo.Version <= endingVersion {
			requiredHBInfos = append(requiredHBInfos, hbInfo)
		}

		// check if this batch is required, fill in the commitment hashes and add to the requiredTreeBatchesMap

		lxBatchIdx := func(layer uint64) uint64 {
			if layer == 0 || layer > MaxLayer {
				panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
			}
			return hbInfo.Version / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
		}
		for layer := uint64(1); layer <= MaxLayer; layer++ {
			if slices.Contains(lxRequiredBatchIdxs[layer], lxBatchIdx(layer)) {
				treeBatch := accountInfo.CurrentBatchTree[layer-1]
				key := fmt.Sprintf("%d:%d", layer, lxBatchIdx(layer))
				requiredTreeBatchesMap[key] = treeBatch
			}
		}
	}
	// fill in the commitHash part of the batch trees with stored commitments
	for layer := uint64(2); layer <= MaxLayer; layer++ {
		for _, batchIdx := range lxRequiredBatchIdxs[layer] {
			key := fmt.Sprintf("%d:%d", layer, batchIdx)
			treeBatch := requiredTreeBatchesMap[key]
			fmt.Println("inserting commitment hashes for layer", layer, "batchIdx", batchIdx)
			InsertCommitmentHashes(layer, batchIdx, treeBatch, account, cbInfo.Version, db)
		}
	}
	return requiredTreeBatchesMap, requiredHBInfos
}

func AddLeafNode(accountInfo *segmenttree.AccountInfo, leafNodeIdx uint64, leafNodeHash common.Hash) {

	// find which index to update for each layer
	lxBatchNodeIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		if layer == 1 {
			return leafNodeIdx % L1BatchSize

		} else {
			return leafNodeIdx / (L1BatchSize * math.Pow(L2BatchSize, layer-2)) % L2BatchSize
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
		if (leafNodeIdx % (L1BatchSize * math.Pow(L2BatchSize, uint64(layer)-1))) == 0 {
			accountInfo.CurrentBatchTree[layer-1] = make([]common.Hash, SegmentTreeSize)
			// accountInfo.CurrentBatchTreeCommitments[layer-1] = gnark_kzg.Digest{}
		}
	}
	// TODO: uncomment this and replace the below code with this.
	lXm1RootHash := leafNodeHash
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		UpdateLXTree(accountInfo, lxBatchLeafNodeOffsetIdx(layer), lXm1RootHash, layer)
		lXm1RootHash = accountInfo.CurrentBatchTree[layer-1][0]
	}

	// // updating layer 1 tree of current batch and calculate its commitment
	// UpdateLXTree(accountInfo, L1BatchSize-1+lxModIdx(1), leafNodeHash, 1)
	// l1RootHash := accountInfo.CurrentBatchTree[0][0]

	// // updating layer 2

	// UpdateLXTree(accountInfo, L2BatchSize-1+lxModIdx(2), l1RootHash, 2)
	// l2RootHash := accountInfo.CurrentBatchTree[1][0]

	// // updating layer 3

	// UpdateLXTree(accountInfo, L2BatchSize-1+lxModIdx(3), l2RootHash, 3)
	// l3RootHash := accountInfo.CurrentBatchTree[2][0]

	// // updating layer 4
	// UpdateLXTree(accountInfo, L2BatchSize-1+lxModIdx(4), l3RootHash, 4)
	// l4RootHash := accountInfo.CurrentBatchTree[3][0]
	// _ = l4RootHash

}

func UpdateLXTree(accountInfo *segmenttree.AccountInfo, idx uint64, val common.Hash, layer uint64) {

	tree := accountInfo.CurrentBatchTree[layer-1]

	// updating the tree
	// note: root hash of layer 1 is empty until the whole batch is filled. instead of updating the tree with empty hash everytime, we skip the tree update unless the root is filled. this is purely for optimization.
	if (val != common.Hash{}) {
		tree[idx] = val
		// TODO: switched from int to uint64; check if it creates a bug here. 2025-09-04
		for idx > 0 {
			parentIdx := segmenttree.GetParent(idx)

			lChild := tree[2*parentIdx+1]
			rChild := tree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			tree[parentIdx] = segmenttree.BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())

			idx = parentIdx

		}

	}

}

func InsertCommitmentHashes(layer uint64, batchIdx uint64, tree []common.Hash, account common.Address, latestVersion uint64, db *pebble.DB) {
	if layer <= 1 || layer > MaxLayer {
		panic("layer" + strconv.Itoa(int(layer)) + " is invalid")
	}
	latestLxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return latestVersion / (L1BatchSize * math.Pow(L2BatchSize, layer-1))
	}

	lxm1BatchIdxStart := batchIdx * L2BatchSize
	// lxm1BatchIdxEnd := lxm1BatchIdxStart + L2BatchSize - 1
	// last batch might not be full, so we need to take the min of the latest batch idx and the batch idx end
	lxm1BatchIdxEnd := min(lxm1BatchIdxStart+L2BatchSize-1, latestLxBatchIdx(layer-1))
	for batchIdx := lxm1BatchIdxStart; batchIdx <= lxm1BatchIdxEnd; batchIdx++ {
		fmt.Println("fetching commitment for layer", layer-1, "batchIdx", batchIdx, "latestLxBatchIdx", latestLxBatchIdx(layer-1))
		commitment := segmenttree.GetLxBatchCommitment(account, layer-1, batchIdx, db)
		commitmentHash := segmenttree.CommitmentToHash(commitment)
		treeIdx := batchIdx - lxm1BatchIdxStart + (2 * L2BatchSize) - 1
		tree[treeIdx] = commitmentHash
	}

}
