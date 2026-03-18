package proof

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"

	"github.com/ethereum/go-ethereum/common"
)

// rebuilds the whole segment tree
// uses stored commitments to fill the commitHash part of the batch trees
// in this process, stores only the involved batch trees and returns them
func RebuildSegmentTreeForProof(account common.Address, lxRequiredBatchIdxs map[uint64][]uint64, startingVersion uint64, endingVersion uint64, db *db.SamuraiDB, precomputedData *config.PrecomputedData) (map[string]tree.BatchTree, []*tree.HistoricalBalance) {
	cbInfo, err := tree.GetCurrentBalanceInfo(account, db.StateDB)
	if err != nil {
		panic(err)
	}
	accountInfo := tree.NewAccountInfo(account, precomputedData)

	// # current balance info
	// TODO: do we need this value?
	// accountInfo.CurrentBalanceInfo = cbInfo

	// # batch tree data
	// initialize empty tree
	// var tree = new(tree.LXBatchTree)
	// for i := range tree.MaxLayer {
	// 	tree[i] = tree.BatchTree{}
	// }
	// accountInfo.CurrentLXBatchTree = new(tree.LXBatchTree)
	// add all historical balance info as leaf nodes

	requiredTreeBatchesMap := make(map[string]tree.BatchTree)
	requiredHBInfos := make([]*tree.HistoricalBalance, 0)

	// start := time.Now()

	dbFetchTime := time.Duration(0)
	leadAddTime := time.Duration(0)
	extraTime := time.Duration(0)

	for version := uint64(0); version < cbInfo.Version; version++ {
		fetchStart := time.Now()
		hbInfo := tree.GetHistoricalBalance(account, version, db.HistoryDB)
		dbFetchTime += time.Since(fetchStart)

		leafAddStart := time.Now()
		AddLeafNode(accountInfo, hbInfo.Version, hbInfo.Hash())
		leadAddTime += time.Since(leafAddStart)

		extraStart := time.Now()

		// check if this historical balance info is required
		if hbInfo.Version >= startingVersion && hbInfo.Version <= endingVersion {
			requiredHBInfos = append(requiredHBInfos, hbInfo)
		}

		// check if this batch is required; if yes, add to the requiredTreeBatchesMap
		nextVersion := version + 1
		for layer := uint64(1); layer <= MaxLayer; layer++ {
			batchSize := L1BatchSize * utils.PowUint64(L2BatchSize, layer-1)
			currentBatchIdx := version / batchSize
			nextBatchIdx := nextVersion / batchSize
			// copy only at the last version of this batch, or if this is the last version of the loop
			isLastInBatch := nextVersion >= cbInfo.Version || (nextBatchIdx != currentBatchIdx)
			if isLastInBatch && slices.Contains(lxRequiredBatchIdxs[layer], currentBatchIdx) {
				key := fmt.Sprintf("%d:%d", layer, currentBatchIdx)
				requiredTreeBatchesMap[key] = accountInfo.CurrentLXBatchTree[layer-1]
			}

		}

		extraTime += time.Since(extraStart)
	}

	// log.Infof("Time taken to add leaf nodes to segment tree: %v with %v db fetch time and %v leaf add time and %v extra time", time.Since(start), dbFetchTime, leadAddTime, extraTime)

	// start = time.Now()

	// fill in the commitHash part of the batch trees with stored commitments
	for layer := uint64(2); layer <= MaxLayer; layer++ {
		for _, batchIdx := range lxRequiredBatchIdxs[layer] {
			key := fmt.Sprintf("%d:%d", layer, batchIdx)
			treeBatch := requiredTreeBatchesMap[key]
			// fmt.Println("inserting commitment hashes for layer", layer, "batchIdx", batchIdx)
			InsertCommitmentHashes(layer, batchIdx, &treeBatch, account, cbInfo.Version, db)
			requiredTreeBatchesMap[key] = treeBatch
		}
	}

	// log.Infof("Time taken to insert commitment hashes into segment tree: %v", time.Since(start))

	return requiredTreeBatchesMap, requiredHBInfos
}

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
			// accountInfo.CurrentLXBatchCommitment[layer-1] = gnark_kzg.Digest{}
		}
	}
	// TODO: uncomment this and replace the below code with this.
	lXm1RootHash := leafNodeHash
	for layer := uint64(1); layer <= MaxLayer; layer++ {
		UpdateLXTree(accountInfo, lxBatchLeafNodeOffsetIdx(layer), lXm1RootHash, layer)
		lxRootHash := accountInfo.CurrentLXBatchTree[layer-1][0]
		lXm1RootHash = lxRootHash
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
			// batchTree[parentIdx] = hash.BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())
			batchTree[parentIdx] = hash.BytesToHash(lChild.Bytes(), rChild.Bytes())

			idx = parentIdx
		}
	}
}

func InsertCommitmentHashes(layer uint64, batchIdx uint64, batchTree *tree.BatchTree, account common.Address, latestVersion uint64, sdb *db.SamuraiDB) {
	if layer <= 1 || layer > MaxLayer {
		panic("layer" + strconv.Itoa(int(layer)) + " is invalid")
	}
	latestLxBatchIdx := func(layer uint64) uint64 {
		if layer == 0 || layer > MaxLayer {
			panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
		}
		return latestVersion / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-1))
	}

	lxm1BatchIdxStart := batchIdx * L2BatchSize
	lxm1BatchIdxEnd := min(lxm1BatchIdxStart+L2BatchSize-1, latestLxBatchIdx(layer-1))
	for bIdx := lxm1BatchIdxStart; bIdx <= lxm1BatchIdxEnd; bIdx++ {
		// fmt.Println("fetching commitment for layer", layer-1, "batchIdx", bIdx, "latestLxBatchIdx", latestLxBatchIdx(layer-1))
		commitment := tree.GetBatchCommitment(account, layer-1, bIdx, sdb.StateDB)
		// commitmentHash := hash.CommitmentToHash(commitment)
		commitmentHash := hash.CommitmentToHash(commitment)
		treeIdx := bIdx - lxm1BatchIdxStart + (2 * L2BatchSize) - 1
		batchTree[treeIdx] = commitmentHash
	}
}
