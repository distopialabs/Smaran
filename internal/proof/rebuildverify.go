package proof

import (
	"fmt"

	"github.com/nepal80m/samurai/internal/math"

	"slices"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

func RebuildSegmentTreeForVerify(account common.Address, lxRequiredBatchIdxs map[uint64][]uint64, startingVersion, endingVersion uint64, balanceInfos []*segmenttree.HistoricalBalance, proofHashMap map[string]*RangeProof, reqCommits []RangeCommitment, precomputedData *config.PrecomputedData) map[string][]common.Hash {

	accountInfo := segmenttree.NewAccountInfo(account, precomputedData)
	var tree segmenttree.BatchTree
	for i := range MaxLayer {
		tree[i] = make([]common.Hash, SegmentTreeSize)
	}
	accountInfo.CurrentBatchTree = tree
	var commitments segmenttree.BatchCommitments
	accountInfo.CurrentBatchTreeCommitments = commitments

	requiredTreeBatchesMap := make(map[string][]common.Hash)

	for _, hbInfo := range balanceInfos {
		AddLeafNode(accountInfo, hbInfo.Version, hbInfo.Hash())

		// check if this batch is required; if yes, add to the requiredTreeBatchesMap
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
	// fill in the commitHash part of the batch trees with commitments provided from prover.
	for _, commit := range reqCommits {
		if commit.layer < segmenttree.MaxLayer {
			proofKey := fmt.Sprintf("%d:%d", commit.layer, commit.idx)
			commitment := proofHashMap[proofKey].Commitment
			commitmentHash := segmenttree.CommitmentToHash(commitment)

			parentLayer := commit.layer + 1
			parentBatchIdx := commit.idx / L2BatchSize
			batchNodeIdx := commit.idx % L2BatchSize
			batchNodeOffsetIdx := 2*L2BatchSize - 1 + batchNodeIdx

			treeKey := fmt.Sprintf("%d:%d", parentLayer, parentBatchIdx)
			requiredTreeBatchesMap[treeKey][batchNodeOffsetIdx] = commitmentHash
		}

	}
	return requiredTreeBatchesMap

}
