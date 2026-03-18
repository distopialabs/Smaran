package proof

import (
	"fmt"
	"slices"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"
)

func RebuildSegmentTreeForVerify(account common.Address, lxRequiredBatchIdxs map[uint64][]uint64, startingVersion, endingVersion uint64, balanceInfos []*tree.HistoricalBalance, proofHashMap map[string]*RangeProof, reqCommits []RangeCommitment, precomputedData *config.PrecomputedData) map[string]*tree.BatchTree {

	accountInfo := tree.NewAccountInfo(account, precomputedData)
	// var tree tree.BatchTree
	// for i := range MaxLayer {
	// 	tree[i] = make([]common.Hash, SegmentTreeSize)
	// }
	// accountInfo.CurrentBatchTree = tree
	// var commitments tree.BatchCommitments
	// accountInfo.CurrentBatchTreeCommitments = commitments

	// requiredTreeBatchesMap := make(map[string][]common.Hash)
	requiredTreeBatchesMap := make(map[string]*tree.BatchTree)

	start := time.Now()

	for i, hbInfo := range balanceInfos {
		AddLeafNode(accountInfo, hbInfo.Version, hbInfo.Hash())

		// check if this batch is required; if yes, add to the requiredTreeBatchesMap
		// Only save at batch boundaries (or the very last version) to avoid
		// copying the 128KB BatchTree on every single iteration.
		nextVersion := hbInfo.Version + 1
		for layer := uint64(1); layer <= MaxLayer; layer++ {
			batchSize := L1BatchSize * utils.PowUint64(L2BatchSize, layer-1)
			currentBatchIdx := hbInfo.Version / batchSize
			nextBatchIdx := nextVersion / batchSize
			isLastInBatch := i == len(balanceInfos)-1 || (nextBatchIdx != currentBatchIdx)
			if isLastInBatch && slices.Contains(lxRequiredBatchIdxs[layer], currentBatchIdx) {
				treeBatch := accountInfo.CurrentLXBatchTree[layer-1]
				key := fmt.Sprintf("%d:%d", layer, currentBatchIdx)
				requiredTreeBatchesMap[key] = &treeBatch
			}
		}
	}

	log.Infof("Time taken to add leaf nodes in segment tree: %v", time.Since(start))
	start = time.Now()
	// fill in the commitHash part of the batch trees with commitments provided from prover.
	for _, commit := range reqCommits {
		if commit.Layer < tree.MaxLayer {
			proofKey := fmt.Sprintf("%d:%d", commit.Layer, commit.Idx)
			if proofHashMap[proofKey] == nil {
				panic(fmt.Sprintf("Required proof for key %s not found in provided proofs", proofKey))
			}
			commitment := proofHashMap[proofKey].Commitment
			commitmentHash := hash.CommitmentToHash(commitment)

			parentLayer := commit.Layer + 1
			parentBatchIdx := commit.Idx / L2BatchSize
			batchNodeIdx := commit.Idx % L2BatchSize
			batchNodeOffsetIdx := 2*L2BatchSize - 1 + batchNodeIdx

			treeKey := fmt.Sprintf("%d:%d", parentLayer, parentBatchIdx)

			if requiredTreeBatchesMap[treeKey] != nil {
				requiredTreeBatchesMap[treeKey][batchNodeOffsetIdx] = commitmentHash
			}
		}

	}
	log.Infof("Time taken to fill in the commitHash part of the batch trees: %v", time.Since(start))
	return requiredTreeBatchesMap

}
