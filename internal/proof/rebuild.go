package proof

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
	"github.com/nepal80m/samurai/internal/segmenttree"
)

type ProofSegmentTree struct {
	LXTree       map[int][]common.Hash
	LXPolynomial map[int]polynomial.Polynomial
	LXCommitment map[int]gnark_kzg.Digest
	CachedData   *segmenttree.CachedData
	Storage      *segmenttree.Storage
}

func RebuildProofSegmentTree(startingBlock, endingBlock int, storage *segmenttree.Storage, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) {
	segmentTree := NewProofSegmentTree(V, weights, srs)
	_ = segmentTree

	for blockNumber := startingBlock; blockNumber <= endingBlock; blockNumber++ {
		balance := big.NewInt(1000000000000000000)
		balance.Add(balance, big.NewInt(int64(blockNumber)))
		segmentTree.Update(blockNumber, balance)

	}
}

func NewProofSegmentTree(V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) *ProofSegmentTree {
	return &ProofSegmentTree{
		LXTree: map[int][]common.Hash{
			1: make([]common.Hash, SegmentTreeSize),
			2: make([]common.Hash, SegmentTreeSize),
			3: make([]common.Hash, SegmentTreeSize),
			4: make([]common.Hash, SegmentTreeSize),
		},
		LXPolynomial: map[int]polynomial.Polynomial{
			1: make(polynomial.Polynomial, SegmentTreeSize),
			2: make(polynomial.Polynomial, SegmentTreeSize),
			3: make(polynomial.Polynomial, SegmentTreeSize),
			4: make(polynomial.Polynomial, SegmentTreeSize),
		},
		LXCommitment: make(map[int]gnark_kzg.Digest),

		CachedData: &segmenttree.CachedData{
			V:       V,
			Weights: weights,
			SRS:     srs,
		},
		Storage: &segmenttree.Storage{
			L1Tree: make(map[int][]common.Hash),
			L2Tree: make(map[int][]common.Hash),
			L3Tree: make(map[int][]common.Hash),
			L4Tree: make(map[int][]common.Hash),
		},
	}
}
func (segmentTree *ProofSegmentTree) Update(blockNumber int, balance *big.Int) {

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
		segmentTree.LXTree[1] = make([]common.Hash, SegmentTreeSize)
	}
	if blockNumber%(L1BatchSize*L2BatchSize) == 0 {
		// if idx1 == 0 && len(segmentTree.Layer2Tree) > 0 {
		// fmt.Println("resetting layer 2 tree")
		segmentTree.LXTree[2] = make([]common.Hash, SegmentTreeSize)
	}
	if blockNumber%(L1BatchSize*L2BatchSize*L2BatchSize) == 0 {
		// if idx2 == 0 && len(segmentTree.Layer3Tree) > 0 {
		// fmt.Println("resetting layer 3 tree")
		segmentTree.LXTree[3] = make([]common.Hash, SegmentTreeSize)
	}
	if blockNumber%(L1BatchSize*L2BatchSize*L2BatchSize*L2BatchSize) == 0 {
		// if idx3 == 0 && len(segmentTree.Layer4Tree) > 0 {
		// fmt.Println("resetting layer 4 tree")
		segmentTree.LXTree[4] = make([]common.Hash, SegmentTreeSize)
	}

	// updating layer 1

	// segmentTree.UpdateLayer1(L1BatchSize-1+idx0, common.BigToHash(balance))
	segmentTree.UpdateLayerX(L1BatchSize-1+idx0, common.BigToHash(balance), 1)
	l1RootHash := segmentTree.LXTree[1][0]

	// TODO: use loop to update all layers
	// updating layer 2
	segmentTree.UpdateLayerX(L2BatchSize-1+idx1, l1RootHash, 2)
	l2RootHash := segmentTree.LXTree[2][0]

	// updating layer 3
	segmentTree.UpdateLayerX(L2BatchSize-1+idx2, l2RootHash, 3)
	l3RootHash := segmentTree.LXTree[3][0]

	// updating layer 4
	segmentTree.UpdateLayerX(L2BatchSize-1+idx3, l3RootHash, 4)
	// _ = l4CommitHash

	l1CommitIndex := blockNumber / L1BatchSize
	l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

	segmentTree.Storage.L1Tree[l1CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L1Tree[l1CommitIndex], segmentTree.LXTree[1])

	segmentTree.Storage.L2Tree[l2CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L2Tree[l2CommitIndex], segmentTree.LXTree[2])

	segmentTree.Storage.L3Tree[l3CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L3Tree[l3CommitIndex], segmentTree.LXTree[3])

	segmentTree.Storage.L4Tree[l4CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L4Tree[l4CommitIndex], segmentTree.LXTree[4])
}

func (segmentTree *ProofSegmentTree) UpdateLayerX(idx int, val common.Hash, layer int) {

	trees := map[int][]common.Hash{
		1: segmentTree.LXTree[1],
		2: segmentTree.LXTree[2],
		3: segmentTree.LXTree[3],
		4: segmentTree.LXTree[4],
	}
	// Update the tree

	tree := trees[layer]

	if (val != common.Hash{}) {
		// segmentTree.UpdateLayerX(idx, val, segmentTree.Layer2Tree, segmentTree.Layer2Polynomial)
		//  update value at idx and its ancestors in the tree

		tree[idx] = val

		updatedIndices := []int{idx}
		for idx > 0 {
			parentIdx := segmenttree.GetParent(uint64(idx))

			lChild := tree[2*parentIdx+1]
			rChild := tree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			tree[parentIdx] = segmenttree.BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())
			// tree[parentIdx] = segmenttree.GetParentHash(lChild, rChild)
			// tree[parentIdx] = crypto.Keccak256Hash(
			// 	lChild.Bytes(),
			// 	rChild.Bytes(),
			// )

			updatedIndices = append(updatedIndices, int(parentIdx))

			idx = int(parentIdx)

		}

	}

}

func (segmentTree *ProofSegmentTree) DumpStorage() {
	segmentTree.DumpTrees()
}
func (segmentTree *ProofSegmentTree) DumpTrees() {

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
