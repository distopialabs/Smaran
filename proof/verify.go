package proof

import (
	"fmt"
	"math/big"
	"slices"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"
	"github.com/nepal80m/samurai/segmenttree"
)

const L1BatchSize = segmenttree.L1BatchSize

const L2BatchSize = segmenttree.L2BatchSize

const MaxLayer = segmenttree.MaxLayer

const SegmentTreeSize = L1BatchSize * 2

type RebuiltLayeredSegmentTree struct {
	Layer1Tree []common.Hash
	Layer2Tree []common.Hash
	Layer3Tree []common.Hash
	Layer4Tree []common.Hash
}

func NewRebuiltLayeredSegmentTree() *RebuiltLayeredSegmentTree {
	return &RebuiltLayeredSegmentTree{
		Layer1Tree: make([]common.Hash, SegmentTreeSize),
		Layer2Tree: make([]common.Hash, SegmentTreeSize),
		Layer3Tree: make([]common.Hash, SegmentTreeSize),
		Layer4Tree: make([]common.Hash, SegmentTreeSize),
	}
}

func VerifyRangeProofs(startingBlock, endingBlock int, rangeProofs []*RangeProof, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) {

	proofHashMap := make(map[string]*RangeProof, len(rangeProofs))
	for _, proof := range rangeProofs {
		key := fmt.Sprintf("%d:%d", proof.layer, proof.idx)
		proofHashMap[key] = proof
	}

	reqCommits := findCommitmentsCoveringRange(startingBlock, endingBlock)
	nodesValuesHashMap := RebuildSegmentTree(startingBlock, endingBlock, reqCommits, proofHashMap)

	// sort reqCommits by layer and idx
	slices.SortFunc(reqCommits, func(a, b RangeCommitment) int {
		if a.layer != b.layer {
			return a.layer - b.layer
		}
		return a.idx - b.idx
	})

	isVerified := make(map[string]bool, len(rangeProofs))

	// loop from last item to first item
	// fmt.Println("\n\nVerifying range proofs...")
	for i := len(reqCommits) - 1; i >= 0; i-- {
		reqCommit := reqCommits[i]

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)

		if proofHashMap[reqCommitKey] == nil {
			panic("This required commitment was not found in provided proofs.")
		}

		if reqCommit.layer != segmenttree.MaxLayer && !isVerified[reqCommitKey] {
			panic("This commitment is not verified.")
		}

		nodesToInterpolate := findNodesToInterpolate(reqCommit, true)
		rangeProof := proofHashMap[reqCommitKey]

		Commitment := rangeProof.Commitment
		// TODO: reconstruct tree using given balance values
		// tree := lxTrees[reqCommit.layer][reqCommit.idx]

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		ZCommit, err := gnark_kzg.Commit(Z, srs.Inner.Pk)
		_ = ZCommit
		if err != nil {
			panic(err)
		}

		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, nodeIdx := range nodesToInterpolate {
			nodeKey := fmt.Sprintf("%d:%d:%d", reqCommit.layer, reqCommit.idx, nodeIdx)
			ys[i] = polynomial.HashToFieldElement(nodesValuesHashMap[nodeKey])
		}

		I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
		ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
		_ = ICommit
		if err != nil {
			panic(err)
		}

		QCommit := rangeProof.Proof
		// TODO: Pairing check using G1 elements only
		ok, err := PairingCheck(Commitment, QCommit, ICommit, ZCommit, srs)
		if err != nil {
			panic(err)
		}
		if !ok {
			panic("pairing check failed: invalid proof")
		}
		for _, depCommitIdx := range reqCommit.dependentCommitments {
			depCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer-1, depCommitIdx)
			isVerified[depCommitKey] = true
		}

		// nodesToInterpolate := findNodesToInterpolate(rangeProof)

		// balance := big.NewInt(1000000000000000000)

		// Z := polynomial.VanishingPolynomial(nodesToInterpolate)

	}

}

type RequiredNode struct {
	layer     int
	commitIdx int
	nodeIdx   int
}

func (r *RequiredNode) GetKey() string {
	return fmt.Sprintf("%d:%d:%d", r.layer, r.commitIdx, r.nodeIdx)
}

func RebuildSegmentTree(startingBlock, endingBlock int, reqCommits []RangeCommitment, proofHashMap map[string]*RangeProof) map[string]common.Hash {

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

	// requiredNodesHashMap := make(map[string]bool)
	nodesValuesHashMap := make(map[string]common.Hash)
	requiredNodes := make([]RequiredNode, 0)
	// requiredCommitmentNodes := make([]RequiredNode, 0)

	for _, commit := range reqCommits {

		if commit.layer < segmenttree.MaxLayer {
			proofKey := fmt.Sprintf("%d:%d", commit.layer, commit.idx)
			commitment := proofHashMap[proofKey].Commitment
			commitmentBytes := commitment.Bytes()
			commitmentHash := common.Hash(commitmentBytes[:])

			modCommitIdx := 2*segmenttree.L2BatchSize - 1 + (commit.idx % segmenttree.L2BatchSize)
			parentCommitIdx := commit.idx / segmenttree.L2BatchSize
			nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer+1, parentCommitIdx, modCommitIdx)
			nodesValuesHashMap[nodeKey] = commitmentHash

		}

		// for _, dCommitIdx := range commit.dependentCommitments {
		// 	proofKey := fmt.Sprintf("%d:%d", commit.layer-1, dCommitIdx)
		// 	commitment := proofHashMap[proofKey].Commitment
		// 	commitmentBytes := commitment.Bytes()
		// 	commitmentHash := common.Hash(commitmentBytes[:])

		// 	modDepCommitIdx := 2*segmenttree.L2BatchSize - 1 + (dCommitIdx % segmenttree.L2BatchSize)
		// 	nodeKey := fmt.Sprintf("%d:%d:%d", commit.layer, commit.idx, modDepCommitIdx)
		// 	nodesValuesHashMap[nodeKey] = commitmentHash
		// }

		nodesToInterpolate := findNodesToInterpolate(commit, false)
		for _, nodeIdx := range nodesToInterpolate {
			// skipping dependent commitments nodes
			// if commit.layer > 1 && nodeIdx >= 2*segmenttree.L2BatchSize-1 {
			// 	commitIdx := commit.idx*segmenttree.L2BatchSize + nodeIdx - 2*segmenttree.L2BatchSize + 1
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

	segmentTree := NewRebuiltLayeredSegmentTree()

	for blockNumber := startingBlock; blockNumber <= endingBlock; blockNumber++ {
		balance := big.NewInt(1000000000000000000)
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

	// for _, node := range requiredNodes {
	// 	key := node.GetKey()
	// 	if nodesValuesHashMap[key] == (common.Hash{}) {
	// 		panic("required node value not found")
	// 	}

	// 	fmt.Printf("required node (layer: %d, idx: %d, node: %d) value: %s\n", node.layer, node.commitIdx, node.nodeIdx, nodesValuesHashMap[key])
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

	// l1CommitIndex := blockNumber / L1BatchSize
	// l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	// l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	// l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

}

func (segmentTree *RebuiltLayeredSegmentTree) UpdateLayerX(idx int, val common.Hash, layer int) {

	trees := map[int][]common.Hash{
		1: segmentTree.Layer1Tree,
		2: segmentTree.Layer2Tree,
		3: segmentTree.Layer3Tree,
		4: segmentTree.Layer4Tree,
	}
	// Update the tree

	tree := trees[layer]

	if (val != common.Hash{}) {
		// segmentTree.UpdateLayerX(idx, val, segmentTree.Layer2Tree, segmentTree.Layer2Polynomial)
		//  update value at idx and its ancestors in the tree

		tree[idx] = val

		updatedIndices := []int{idx}
		for idx > 0 {
			parentIdx := segmenttree.GetParent(idx)

			lChild := tree[2*parentIdx+1]
			rChild := tree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}

			tree[parentIdx] = crypto.Keccak256Hash(
				lChild.Bytes(),
				rChild.Bytes(),
			)

			updatedIndices = append(updatedIndices, parentIdx)

			idx = parentIdx

		}

	}

}

func PairingCheck(commit bls.G1Affine, proof bls.G1Affine, iCommit bls.G1Affine, zCommit bls.G1Affine, srs *kzg.MultiSRS) (bool, error) {
	return true, nil
}
