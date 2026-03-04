package proof

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"slices"
	"time"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/hash"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/tree"
)

const L1BatchSize = tree.L1BatchSize

const L2BatchSize = tree.L2BatchSize

const MaxLayer = tree.MaxLayer

const SegmentTreeSize = L1BatchSize * 2

type RebuiltLayeredSegmentTree struct {
	Layer1Tree []common.Hash
	Layer2Tree []common.Hash
	Layer3Tree []common.Hash
	Layer4Tree []common.Hash

	Storage *tree.Storage
}

func NewRebuiltLayeredSegmentTree() *RebuiltLayeredSegmentTree {
	return &RebuiltLayeredSegmentTree{
		Layer1Tree: make([]common.Hash, SegmentTreeSize),
		Layer2Tree: make([]common.Hash, SegmentTreeSize),
		Layer3Tree: make([]common.Hash, SegmentTreeSize),
		Layer4Tree: make([]common.Hash, SegmentTreeSize),
		Storage: &tree.Storage{
			L1Tree: make(map[int][]common.Hash),
			L2Tree: make(map[int][]common.Hash),
			L3Tree: make(map[int][]common.Hash),
			L4Tree: make(map[int][]common.Hash),
		},
	}
}

func VerifyNewRangeProofs(account common.Address, startingVersion, endingVersion uint64, rangeProofs []*RangeProof, balanceInfos []*tree.HistoricalBalance, precomputedData *config.PrecomputedData) {
	// fmt.Println("\n\nVerifying range proofs...")

	proofHashMap := make(map[string]*RangeProof, len(rangeProofs))
	for _, proof := range rangeProofs {
		key := fmt.Sprintf("%d:%d", proof.Layer, proof.Idx)
		proofHashMap[key] = proof
	}

	reqCommits := findCommitmentsCoveringRange(int(startingVersion), int(endingVersion))

	lxRequiredBatchIdxs := make(map[uint64][]uint64)
	for i := uint64(1); i <= tree.MaxLayer; i++ {
		lxRequiredBatchIdxs[i] = make([]uint64, 0)
	}
	for _, reqCommit := range reqCommits {
		lxRequiredBatchIdxs[uint64(reqCommit.layer)] = append(lxRequiredBatchIdxs[uint64(reqCommit.layer)], uint64(reqCommit.idx))
	}

	// TODO: Rebuild partial tree
	start := time.Now()
	requiredTreeBatchesMap := RebuildSegmentTreeForVerify(account, lxRequiredBatchIdxs, startingVersion, endingVersion, balanceInfos, proofHashMap, reqCommits, precomputedData)
	fmt.Println("Time taken to rebuild segment tree", time.Since(start))

	slices.SortFunc(reqCommits, func(a, b RangeCommitment) int {
		if a.layer != b.layer {
			return a.layer - b.layer
		}
		return a.idx - b.idx
	})
	isVerified := make(map[string]bool, len(rangeProofs))
	verifyStart := time.Now()

	// loop from last item to first item
	for i := len(reqCommits) - 1; i >= 0; i-- {

		// innerVerifyStart := time.Now()
		reqCommit := reqCommits[i]

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)

		if proofHashMap[reqCommitKey] == nil {
			panic("This required commitment was not found in provided proofs.")
		}

		if reqCommit.layer != tree.MaxLayer && !isVerified[reqCommitKey] {
			panic("This commitment is not verified.")
		}

		nodesToInterpolate := findNodesToInterpolate(reqCommit, true)
		rangeProof := proofHashMap[reqCommitKey]

		// fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
		// if reqCommit.BlockRange == nil {
		// 	fmt.Printf("Commitment is not covering any range.\n")
		// } else {
		// 	fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		// }
		// fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
		// fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)
		// for i, nodeIdx := range nodesToInterpolate {
		// 	key := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)
		// 	fmt.Printf("ys[%d]: %v\n", i, requiredTreeBatchesMap[key][nodeIdx])
		// }
		Commitment := rangeProof.Commitment

		// pCommitBytes := Commitment.Bytes()
		// pCommitHash := common.BytesToHash(pCommitBytes[:])
		// fmt.Printf("pCommitmentHash: %s\n", pCommitHash)

		// TODO: reconstruct tree using given balance values
		// tree := lxTrees[reqCommit.layer][reqCommit.idx]

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		// ZCommit, err := gnark_kzg.Commit(Z, srs.Inner.Pk)
		ZCommit, _ := kzg.CommitG2(Z, precomputedData.SRS.G2Powers)

		// zCommitBytes := ZCommit.Bytes()
		// zCommitHash := common.BytesToHash(zCommitBytes[:])
		// fmt.Printf("zCommitmentHash: %s\n", zCommitHash)

		// _ = ZCommit
		// if err != nil {
		// 	panic(err)
		// }

		xs := make([]fr.Element, len(nodesToInterpolate))
		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, nodeIdx := range nodesToInterpolate {
			xs[i] = fr.NewElement(uint64(nodeIdx))
			key := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)
			ys[i] = polynomial.HashToFieldElement(requiredTreeBatchesMap[key][nodeIdx])
		}

		// I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
		I := kzg.Interpolate(xs, ys)
		ICommit, err := gnark_kzg.Commit(I, precomputedData.SRS.Inner.Pk)
		if err != nil {
			panic(err)
		}

		// iCommitBytes := ICommit.Bytes()
		// iCommitHash := common.BytesToHash(iCommitBytes[:])
		// fmt.Printf("iCommitmentHash: %s\n", iCommitHash)

		QCommit := rangeProof.Proof
		// qCommitBytes := QCommit.Bytes()
		// qCommitHash := common.BytesToHash(qCommitBytes[:])
		// fmt.Printf("qCommitmentHash: %s\n", qCommitHash)

		// fmt.Printf("Commitment: %v\n", Commitment)
		// fmt.Printf("Proof: %v\n", QCommit)
		// fmt.Printf("ys: %v\n", ys)

		// fmt.Printf("I: %v\n", I)
		// fmt.Printf("Z: %v\n", Z)

		// TODO: Pairing check using G1 elements only
		_, err = PairingCheck(Commitment, QCommit, ICommit, ZCommit, precomputedData.SRS)
		if err != nil {
			panic(err)
			// fmt.Printf("pairing check failed: invalid proof\n")
			// panic("pairing check failed: invalid proof")
		} else {
			// fmt.Println("pairing check passed✅")
			for _, depCommitIdx := range reqCommit.dependentCommitments {
				depCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer-1, depCommitIdx)
				isVerified[depCommitKey] = true
			}
		}

		// nodesToInterpolate := findNodesToInterpolate(rangeProof)

		// balance := big.NewInt(1000000000000000000)

		// Z := polynomial.VanishingPolynomial(nodesToInterpolate)

		// fmt.Printf("Time taken to verify range proof %d:%d: %v\n", reqCommit.layer, reqCommit.idx, time.Since(innerVerifyStart))
	}
	fmt.Println("Time taken to verify range proofs", time.Since(verifyStart))
}

func VerifyRangeProofs(startingBlock, endingBlock int, rangeProofs []*RangeProof, balances []*big.Int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) {
	// fmt.Println("\n\nVerifying range proofs...")

	proofHashMap := make(map[string]*RangeProof, len(rangeProofs))
	for _, proof := range rangeProofs {
		key := fmt.Sprintf("%d:%d", proof.Layer, proof.Idx)
		proofHashMap[key] = proof
	}

	reqCommits := findCommitmentsCoveringRange(startingBlock, endingBlock)
	start := time.Now()
	nodesValuesHashMap := RebuildPartialSegmentTree(startingBlock, endingBlock, reqCommits, proofHashMap, balances)
	treeRebuildTime := time.Since(start)
	// fmt.Printf("Time taken to rebuild partial segment tree: %v\n", treeRebuildTime)

	// sort reqCommits by layer and idx
	slices.SortFunc(reqCommits, func(a, b RangeCommitment) int {
		if a.layer != b.layer {
			return a.layer - b.layer
		}
		return a.idx - b.idx
	})

	isVerified := make(map[string]bool, len(rangeProofs))

	// loop from last item to first item
	for i := len(reqCommits) - 1; i >= 0; i-- {
		reqCommit := reqCommits[i]

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)

		if proofHashMap[reqCommitKey] == nil {
			panic("This required commitment was not found in provided proofs.")
		}

		if reqCommit.layer != tree.MaxLayer && !isVerified[reqCommitKey] {
			panic("This commitment is not verified.")
		}

		nodesToInterpolate := findNodesToInterpolate(reqCommit, true)
		rangeProof := proofHashMap[reqCommitKey]

		fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
		if reqCommit.BlockRange == nil {
			fmt.Printf("Commitment is not covering any range.\n")
		} else {
			fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		}
		fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
		fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)
		for i, nodeIdx := range nodesToInterpolate {
			nodeKey := fmt.Sprintf("%d:%d:%d", reqCommit.layer, reqCommit.idx, nodeIdx)
			fmt.Printf("ys[%d]: %v\n", i, nodesValuesHashMap[nodeKey])
		}
		Commitment := rangeProof.Commitment

		pCommitBytes := Commitment.Bytes()
		pCommitHash := common.BytesToHash(pCommitBytes[:])
		fmt.Printf("pCommitmentHash: %s\n", pCommitHash)

		// TODO: reconstruct tree using given balance values
		// tree := lxTrees[reqCommit.layer][reqCommit.idx]

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		// ZCommit, err := gnark_kzg.Commit(Z, srs.Inner.Pk)
		ZCommit, _ := kzg.CommitG2(Z, srs.G2Powers)

		zCommitBytes := ZCommit.Bytes()
		zCommitHash := common.BytesToHash(zCommitBytes[:])
		fmt.Printf("zCommitmentHash: %s\n", zCommitHash)

		// _ = ZCommit
		// if err != nil {
		// 	panic(err)
		// }

		xs := make([]fr.Element, len(nodesToInterpolate))
		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, nodeIdx := range nodesToInterpolate {
			xs[i] = fr.NewElement(uint64(nodeIdx))
			nodeKey := fmt.Sprintf("%d:%d:%d", reqCommit.layer, reqCommit.idx, nodeIdx)
			ys[i] = polynomial.HashToFieldElement(nodesValuesHashMap[nodeKey])
		}

		// I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
		I := kzg.Interpolate(xs, ys)
		ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}

		iCommitBytes := ICommit.Bytes()
		iCommitHash := common.BytesToHash(iCommitBytes[:])
		fmt.Printf("iCommitmentHash: %s\n", iCommitHash)

		QCommit := rangeProof.Proof
		qCommitBytes := QCommit.Bytes()
		qCommitHash := common.BytesToHash(qCommitBytes[:])
		fmt.Printf("qCommitmentHash: %s\n", qCommitHash)

		// fmt.Printf("Commitment: %v\n", Commitment)
		// fmt.Printf("Proof: %v\n", QCommit)
		// fmt.Printf("ys: %v\n", ys)

		// fmt.Printf("I: %v\n", I)
		// fmt.Printf("Z: %v\n", Z)

		// TODO: Pairing check using G1 elements only
		_, err = PairingCheck(Commitment, QCommit, ICommit, ZCommit, srs)
		if err != nil {
			panic(err)
			// fmt.Printf("pairing check failed: invalid proof\n")
			// panic("pairing check failed: invalid proof")
		} else {
			fmt.Println("pairing check passed✅")
			for _, depCommitIdx := range reqCommit.dependentCommitments {
				depCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer-1, depCommitIdx)
				isVerified[depCommitKey] = true
			}
		}

		// nodesToInterpolate := findNodesToInterpolate(rangeProof)

		// balance := big.NewInt(1000000000000000000)

		// Z := polynomial.VanishingPolynomial(nodesToInterpolate)

	}
	fmt.Printf("Time taken to rebuild partial segment tree: %v\n", treeRebuildTime)

}

type RequiredNode struct {
	layer     int
	commitIdx int
	nodeIdx   int
}

func (r *RequiredNode) GetKey() string {
	return fmt.Sprintf("%d:%d:%d", r.layer, r.commitIdx, r.nodeIdx)
}

func RebuildPartialSegmentTree(startingBlock, endingBlock int, reqCommits []RangeCommitment, proofHashMap map[string]*RangeProof, balances []*big.Int) map[string]common.Hash {

	// sort reqCommits by layer and idx
	// rangeCoveringCommits := make([]RangeCommitment, 0)
	// for _, commit := range reqCommits {
	// 	if commit.BlockRange != nil {
	// 		rangeCoveringCommits = append(rangeCoveringCommits, commit)
	// 	}
	// }
	// slices.SortFunc(rangeCoveringCommits, func(a, b RangeCommitment) int {
	// 	return a.BlockRange.Start - b.BlockRange.Start
	// })
	segmentTree := NewRebuiltLayeredSegmentTree()

	// requiredNodesHashMap := make(map[string]bool)
	nodesValuesHashMap := make(map[string]common.Hash)
	requiredNodes := make([]RequiredNode, 0)
	// requiredCommitmentNodes := make([]RequiredNode, 0)

	for _, commit := range reqCommits {

		if commit.layer < tree.MaxLayer {
			proofKey := fmt.Sprintf("%d:%d", commit.layer, commit.idx)
			commitment := proofHashMap[proofKey].Commitment
			// commitmentHash := hash.CommitmentToHash(commitment)
			commitmentHash := hash.CommitmentToHash(commitment)
			// commitmentBytes := commitment.Bytes()
			// commitmentHash := common.BytesToHash(commitmentBytes[:])

			modCommitIdx := 2*tree.L2BatchSize - 1 + (commit.idx % tree.L2BatchSize)
			parentCommitIdx := commit.idx / tree.L2BatchSize
			nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer+1, parentCommitIdx, modCommitIdx)
			nodesValuesHashMap[nodeKey] = commitmentHash

			// if commit.layer == 1 {
			// 	storage.L2Tree[parentCommitIdx][modCommitIdx] = commitmentHash
			// }
			// if commit.layer == 2 {
			// 	storage.L3Tree[parentCommitIdx][modCommitIdx] = commitmentHash
			// }
			// if commit.layer == 3 {
			// 	storage.L4Tree[parentCommitIdx][modCommitIdx] = commitmentHash
			// }

		}

		// for _, dCommitIdx := range commit.dependentCommitments {
		// 	proofKey := fmt.Sprintf("%d:%d", commit.layer-1, dCommitIdx)
		// 	commitment := proofHashMap[proofKey].Commitment
		// 	commitmentBytes := commitment.Bytes()
		// 	commitmentHash := common.Hash(commitmentBytes[:])

		// 	modDepCommitIdx := 2*tree.L2BatchSize - 1 + (dCommitIdx % tree.L2BatchSize)
		// 	nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer, commit.idx, modDepCommitIdx)
		// 	nodesValuesHashMap[nodeKey] = commitmentHash
		// }

		nodesToInterpolate := findNodesToInterpolate(commit, false)
		for _, nodeIdx := range nodesToInterpolate {
			// skipping dependent commitments nodes
			// if commit.layer > 1 && nodeIdx >= 2*tree.L2BatchSize-1 {
			// 	commitIdx := commit.idx*tree.L2BatchSize + nodeIdx - 2*tree.L2BatchSize + 1
			// 	proofKey := fmt.Sprintf("%d:%d", commit.layer-1, commitIdx)
			// 	commitment := proofHashMap[proofKey].Commitment
			// 	commitmentBytes := commitment.Bytes()
			// 	commitmentHash := common.Hash(commitmentBytes[:])
			// 	nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer, commit.idx, nodeIdx)
			// 	requiredNodesValues[nodeKey] = commitmentHash

			// } else {
			requiredNodes = append(requiredNodes, RequiredNode{
				layer:     commit.layer,
				commitIdx: commit.idx,
				nodeIdx:   nodeIdx,
			})
			// }
		}

	}

	for blockNumber := startingBlock; blockNumber <= endingBlock; blockNumber++ {
		balance := balances[blockNumber-startingBlock]
		segmentTree.Update(blockNumber, balance)
		l1CommitIndex := blockNumber / L1BatchSize
		l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
		l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
		l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

		for _, node := range requiredNodes {
			key := node.GetKey()

			if nodesValuesHashMap[key] != (common.Hash{}) {
				continue
			}
			if node.layer == 1 && node.commitIdx == l1CommitIndex && segmentTree.Layer1Tree[node.nodeIdx] != (common.Hash{}) {
				nodesValuesHashMap[key] = segmentTree.Layer1Tree[node.nodeIdx]
			}
			if node.layer == 2 && node.commitIdx == l2CommitIndex && segmentTree.Layer2Tree[node.nodeIdx] != (common.Hash{}) {
				nodesValuesHashMap[key] = segmentTree.Layer2Tree[node.nodeIdx]
			}
			if node.layer == 3 && node.commitIdx == l3CommitIndex && segmentTree.Layer3Tree[node.nodeIdx] != (common.Hash{}) {
				nodesValuesHashMap[key] = segmentTree.Layer3Tree[node.nodeIdx]
			}
			if node.layer == 4 && node.commitIdx == l4CommitIndex && segmentTree.Layer4Tree[node.nodeIdx] != (common.Hash{}) {
				nodesValuesHashMap[key] = segmentTree.Layer4Tree[node.nodeIdx]
			}
		}

	}

	for _, commit := range reqCommits {

		if commit.layer < tree.MaxLayer {
			proofKey := fmt.Sprintf("%d:%d", commit.layer, commit.idx)
			commitment := proofHashMap[proofKey].Commitment
			commitmentBytes := commitment.Bytes()
			// commitmentHash := common.Hash(commitmentBytes[:])
			commitmentHash := common.BytesToHash(commitmentBytes[:])

			modCommitIdx := 2*tree.L2BatchSize - 1 + (commit.idx % tree.L2BatchSize)
			parentCommitIdx := commit.idx / tree.L2BatchSize
			// nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer+1, parentCommitIdx, modCommitIdx)
			// nodesValuesHashMap[nodeKey] = commitmentHash

			if commit.layer == 1 {
				segmentTree.Storage.L2Tree[parentCommitIdx][modCommitIdx] = commitmentHash
			}
			if commit.layer == 2 {
				segmentTree.Storage.L3Tree[parentCommitIdx][modCommitIdx] = commitmentHash
			}
			if commit.layer == 3 {
				segmentTree.Storage.L4Tree[parentCommitIdx][modCommitIdx] = commitmentHash
			}

		}
	}

	// segmentTree.DumpStorage()

	// for _, node := range requiredNodes {
	// 	key := node.GetKey()
	// 	if nodesValuesHashMap[key] == (common.Hash{}) {
	// 		panic("required node value not found")
	// 	}

	// 	fmt.Printf("required node (layer: %d, idx: %d, node: %d) value: %s\n", node.layer, node.commitIdx, node.nodeIdx, nodesValuesHashMap[key])
	// }

	// for _, commit := range reqCommits {
	// 	nodesToInterpolate := findNodesToInterpolate(commit, true)
	// 	for _, nodeIdx := range nodesToInterpolate {
	// 		nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer, commit.idx, nodeIdx)
	// 		calculatedValue := nodesValuesHashMap[nodeKey]

	// 		lxTrees := map[int]map[int][]common.Hash{
	// 			1: storage.L1Tree,
	// 			2: storage.L2Tree,
	// 			3: storage.L3Tree,
	// 			4: storage.L4Tree,
	// 		}
	// 		tree := lxTrees[commit.layer][commit.idx]

	// 		actualValue := tree[nodeIdx]

	// 		if calculatedValue != actualValue {
	// 			fmt.Printf("layer: %d, idx: %d, node: %d\n", commit.layer, commit.idx, nodeIdx)
	// 			fmt.Printf("calculatedValue: %s\n", calculatedValue)
	// 			fmt.Printf("actualValue: %s\n", actualValue)
	// 			panic("calculated value does not match actual value")
	// 		}
	// 	}
	// }

	return nodesValuesHashMap

}
func (segmentTree *RebuiltLayeredSegmentTree) Update(blockNumber int, balance *big.Int) {

	if balance == nil {
		panic("balance cannot be nil")
	}

	// find which index to update for each layer
	idx0 := blockNumber % L1BatchSize
	idx1 := blockNumber / L1BatchSize % L2BatchSize
	idx2 := blockNumber / (L1BatchSize * L2BatchSize) % L2BatchSize
	idx3 := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize) % L2BatchSize

	if blockNumber%L1BatchSize == 0 {
		// if idx0 == 0 {
		// fmt.Println("resetting layer 1 tree")
		segmentTree.Layer1Tree = make([]common.Hash, SegmentTreeSize)
	}
	if blockNumber%(L1BatchSize*L2BatchSize) == 0 {
		// if idx1 == 0 && len(segmentTree.Layer2Tree) > 0 {
		// fmt.Println("resetting layer 2 tree")
		segmentTree.Layer2Tree = make([]common.Hash, SegmentTreeSize)
	}
	if blockNumber%(L1BatchSize*L2BatchSize*L2BatchSize) == 0 {
		// if idx2 == 0 && len(segmentTree.Layer3Tree) > 0 {
		// fmt.Println("resetting layer 3 tree")
		segmentTree.Layer3Tree = make([]common.Hash, SegmentTreeSize)
	}
	if blockNumber%(L1BatchSize*L2BatchSize*L2BatchSize*L2BatchSize) == 0 {
		// if idx3 == 0 && len(segmentTree.Layer4Tree) > 0 {
		// fmt.Println("resetting layer 4 tree")
		segmentTree.Layer4Tree = make([]common.Hash, SegmentTreeSize)
	}

	// updating layer 1

	// segmentTree.UpdateLayer1(L1BatchSize-1+idx0, common.BigToHash(balance))
	segmentTree.UpdateLayerX(L1BatchSize-1+idx0, common.BigToHash(balance), 1)
	l1RootHash := segmentTree.Layer1Tree[0]

	// TODO: use loop to update all layers
	// updating layer 2
	segmentTree.UpdateLayerX(L2BatchSize-1+idx1, l1RootHash, 2)
	l2RootHash := segmentTree.Layer2Tree[0]

	// updating layer 3
	segmentTree.UpdateLayerX(L2BatchSize-1+idx2, l2RootHash, 3)
	l3RootHash := segmentTree.Layer3Tree[0]

	// updating layer 4
	segmentTree.UpdateLayerX(L2BatchSize-1+idx3, l3RootHash, 4)
	// _ = l4CommitHash

	l1CommitIndex := blockNumber / L1BatchSize
	l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

	segmentTree.Storage.L1Tree[l1CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L1Tree[l1CommitIndex], segmentTree.Layer1Tree)

	segmentTree.Storage.L2Tree[l2CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L2Tree[l2CommitIndex], segmentTree.Layer2Tree)

	segmentTree.Storage.L3Tree[l3CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L3Tree[l3CommitIndex], segmentTree.Layer3Tree)

	segmentTree.Storage.L4Tree[l4CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L4Tree[l4CommitIndex], segmentTree.Layer4Tree)
}

func (segmentTree *RebuiltLayeredSegmentTree) UpdateLayerX(idx int, val common.Hash, layer int) {

	trees := map[int][]common.Hash{
		1: segmentTree.Layer1Tree,
		2: segmentTree.Layer2Tree,
		3: segmentTree.Layer3Tree,
		4: segmentTree.Layer4Tree,
	}

	batchTree := trees[layer]

	if (val != common.Hash{}) {
		batchTree[idx] = val

		updatedIndices := []int{idx}
		for idx > 0 {
			parentIdx := tree.GetParent(uint64(idx))

			lChild := batchTree[2*parentIdx+1]
			rChild := batchTree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			// batchTree[parentIdx] = hash.BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())
			batchTree[parentIdx] = hash.BytesToHash(lChild.Bytes(), rChild.Bytes())

			updatedIndices = append(updatedIndices, int(parentIdx))
			idx = int(parentIdx)
		}
	}
}

func PairingCheck(commit bls.G1Affine, proof bls.G1Affine, iCommit bls.G1Affine, zCommit bls.G2Affine, srs *kzg.MultiSRS) (bool, error) {

	var lhsG1 bls.G1Affine
	lhsG1.Sub(&commit, &iCommit)

	lhsNegZ := zCommit
	lhsNegZ.Neg(&lhsNegZ)

	P := []bls.G1Affine{lhsG1, proof}
	// Q := make([]bls.G2Affine, 2)
	Q := []bls.G2Affine{srs.G2Powers[0], lhsNegZ}

	ok, err := bls.PairingCheck(P, Q)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("pairing check failed: invalid multiproof")
	}
	return true, nil
}

func (segmentTree *RebuiltLayeredSegmentTree) DumpTrees() {

	// dump trees to a json file
	l1Tree := segmentTree.Storage.L1Tree
	l2Tree := segmentTree.Storage.L2Tree
	l3Tree := segmentTree.Storage.L3Tree
	l4Tree := segmentTree.Storage.L4Tree

	// dump trees to a json file
	l1TreeJSON, err := json.Marshal(l1Tree)
	if err != nil {
		log.Fatalf("failed to marshal l1Tree: %v", err)
	}
	err = os.WriteFile("l1TreeRebuilt.json", l1TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l1Tree to file: %v", err)
	}

	l2TreeJSON, err := json.Marshal(l2Tree)
	if err != nil {
		log.Fatalf("failed to marshal l2Tree: %v", err)
	}
	err = os.WriteFile("l2TreeRebuilt.json", l2TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l2Tree to file: %v", err)
	}

	l3TreeJSON, err := json.Marshal(l3Tree)
	if err != nil {
		log.Fatalf("failed to marshal l3Tree: %v", err)
	}
	err = os.WriteFile("l3TreeRebuilt.json", l3TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l3Tree to file: %v", err)
	}

	l4TreeJSON, err := json.Marshal(l4Tree)
	if err != nil {
		log.Fatalf("failed to marshal l4Tree: %v", err)
	}
	err = os.WriteFile("l4TreeRebuilt.json", l4TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l4Tree to file: %v", err)
	}

	fmt.Println("Dumped trees to json files")

}

func (segmentTree *RebuiltLayeredSegmentTree) DumpStorage() {
	segmentTree.DumpTrees()
}

func ReadProofsAndBalances(numBlocks int) ([]*RangeProof, []*big.Int, error) {
	proofsFile, err := os.Open(fmt.Sprintf("storage/proofs/%d/proofs.json", numBlocks))
	if err != nil {
		return nil, nil, err
	}
	defer proofsFile.Close()

	balancesFile, err := os.Open(fmt.Sprintf("storage/proofs/%d/balances.json", numBlocks))
	if err != nil {
		return nil, nil, err
	}
	defer balancesFile.Close()

	proofs := []*RangeProof{}
	balances := []*big.Int{}

	proofsJSON, err := io.ReadAll(proofsFile)
	if err != nil {
		return nil, nil, err
	}

	json.Unmarshal(proofsJSON, &proofs)

	balancesJSON, err := io.ReadAll(balancesFile)
	if err != nil {
		return nil, nil, err
	}

	json.Unmarshal(balancesJSON, &balances)

	return proofs, balances, nil
}
