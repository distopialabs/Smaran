package proof

import (
	"fmt"
	"slices"
	"strconv"
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

	for _, hbInfo := range balanceInfos {
		// innerStart := time.Now()
		AddLeafNode(accountInfo, hbInfo.Version, hbInfo.Hash())
		// fmt.Println("Time taken to add leaf node for version", hbInfo.Version, time.Since(innerStart))

		// check if this batch is required; if yes, add to the requiredTreeBatchesMap
		lxBatchIdx := func(layer uint64) uint64 {
			if layer == 0 || layer > MaxLayer {
				panic("layer" + strconv.Itoa(int(layer)) + " is not supported")
			}
			return hbInfo.Version / (L1BatchSize * utils.PowUint64(L2BatchSize, layer-1))
		}
		// innerStart = time.Now()
		for layer := uint64(1); layer <= MaxLayer; layer++ {
			if slices.Contains(lxRequiredBatchIdxs[layer], lxBatchIdx(layer)) {
				treeBatch := accountInfo.CurrentLXBatchTree[layer-1]
				key := fmt.Sprintf("%d:%d", layer, lxBatchIdx(layer))
				requiredTreeBatchesMap[key] = &treeBatch
			}
		}
		// fmt.Println("Time taken to check if batch is required", time.Since(innerStart))
	}

	fmt.Println("Time taken to add leaf nodes in segment tree", time.Since(start))
	start = time.Now()
	// fill in the commitHash part of the batch trees with commitments provided from prover.
	for _, commit := range reqCommits {
		if commit.layer < tree.MaxLayer {
			proofKey := fmt.Sprintf("%d:%d", commit.layer, commit.idx)
			if proofHashMap[proofKey] == nil {
				panic(fmt.Sprintf("Required proof for key %s not found in provided proofs", proofKey))
			}
			commitment := proofHashMap[proofKey].Commitment
			commitmentHash := hash.CommitmentToHash(commitment)

			parentLayer := commit.layer + 1
			parentBatchIdx := commit.idx / L2BatchSize
			batchNodeIdx := commit.idx % L2BatchSize
			batchNodeOffsetIdx := 2*L2BatchSize - 1 + batchNodeIdx

			treeKey := fmt.Sprintf("%d:%d", parentLayer, parentBatchIdx)

			if requiredTreeBatchesMap[treeKey] != nil {
				requiredTreeBatchesMap[treeKey][batchNodeOffsetIdx] = commitmentHash
			}
		}

	}
	fmt.Println("Time taken to fill in the commitHash part of the batch trees", time.Since(start))
	return requiredTreeBatchesMap

}
