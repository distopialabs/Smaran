package segmenttree

import (
	"fmt"
	"log"
	"math/big"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/nepal80m/samurai/kzg"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/nepal80m/samurai/polynomial"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

const SegmentTreeSize = 4096

type CachedData struct {
	V       polynomial.Polynomial
	weights []fr.Element
	srs     *kzg.MultiSRS
}

type LayeredSegmentTree struct {
	Layer1Tree []common.Hash
	Layer2Tree []common.Hash
	Layer3Tree []common.Hash
	Layer4Tree []common.Hash

	Layer1Polynomial polynomial.Polynomial
	Layer2Polynomial polynomial.Polynomial
	Layer3Polynomial polynomial.Polynomial
	Layer4Polynomial polynomial.Polynomial

	cachedData CachedData
}

func NewLayeredSegmentTree(V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) *LayeredSegmentTree {
	return &LayeredSegmentTree{
		Layer1Tree: make([]common.Hash, 4096),
		Layer2Tree: make([]common.Hash, 4096),
		Layer3Tree: make([]common.Hash, 4096),
		Layer4Tree: make([]common.Hash, 4096),

		Layer1Polynomial: make(polynomial.Polynomial, 4096),
		Layer2Polynomial: make(polynomial.Polynomial, 4096),
		Layer3Polynomial: make(polynomial.Polynomial, 4096),
		Layer4Polynomial: make(polynomial.Polynomial, 4096),

		cachedData: CachedData{
			V:       V,
			weights: weights,
			srs:     srs,
		},
	}
}

func (segmentTree *LayeredSegmentTree) Update(blockNumber int, balance *big.Int) {

	if balance == nil {
		panic("balance cannot be nil")
	}

	// find appropriate index for each layer
	idx0 := blockNumber % 2048
	idx1 := blockNumber / 2048 % 1365
	idx2 := blockNumber / (2048 * 1365) % 1365
	idx3 := blockNumber / (2048 * 1365 * 1365) % 1365

	if idx0 == 0 {
		fmt.Println("resetting layer 1 tree")
		segmentTree.Layer1Tree = make([]common.Hash, 4095)
	}
	if idx1 == 0 && len(segmentTree.Layer2Tree) > 0 {
		segmentTree.Layer2Tree = make([]common.Hash, 4095)
	}
	if idx2 == 0 && len(segmentTree.Layer3Tree) > 0 {
		segmentTree.Layer3Tree = make([]common.Hash, 4095)
	}
	if idx3 == 0 && len(segmentTree.Layer4Tree) > 0 {
		segmentTree.Layer4Tree = make([]common.Hash, 4095)
	}

	// updating layer 1
	layer1CommitmentHash := segmentTree.UpdateLayer1(1364+idx0, common.BigToHash(balance))
	_ = layer1CommitmentHash
	layer1RootHash := segmentTree.Layer1Tree[0]

	if blockNumber >= 1363 {
		// fmt.Println("layer 1 tree", segmentTree.Layer1Tree)
		fmt.Println("0", segmentTree.Layer1Tree[0])
		fmt.Println("1", segmentTree.Layer1Tree[1])
		fmt.Println("2", segmentTree.Layer1Tree[2])
		fmt.Println("3", segmentTree.Layer1Tree[3])
		fmt.Println("4", segmentTree.Layer1Tree[4])
		fmt.Println("5", segmentTree.Layer1Tree[5])
		fmt.Println("6", segmentTree.Layer1Tree[6])
	}

	// updating layer 2

	segmentTree.Layer2Tree[2729+idx1] = layer1CommitmentHash
	incPoly := polynomial.Interpolate([]int{2729 + idx1}, []fr.Element{polynomial.HashToFieldElement(layer1CommitmentHash)}, segmentTree.cachedData.V, segmentTree.cachedData.weights)
	segmentTree.Layer2Polynomial.Add(segmentTree.Layer2Polynomial, incPoly)

	if (layer1RootHash != common.Hash{}) {
		fmt.Println("updating layer 2 at index", 1364+idx1, layer1RootHash, layer1CommitmentHash)
		layer2CommitmentHash := segmentTree.UpdateLayer2(1364+idx1, layer1RootHash)
		_ = layer2CommitmentHash
	}

	// updating layer 3

	// updating layer 4

}

func (segmentTree *LayeredSegmentTree) UpdateLayerX(idx int, val common.Hash, tree []common.Hash, poly polynomial.Polynomial) common.Hash {
	//  update the tree

	tree[idx] = val

	updatedIndices := []int{idx}
	for idx > 0 {
		parentIdx := GetParent(idx)

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
	// update the polynomial

	incPoly := GenerateIncrementalPolynomial(updatedIndices, segmentTree.cachedData.V, segmentTree.cachedData.weights, tree)

	poly.Add(poly, incPoly)

	commitment, err := gnark_kzg.Commit(poly, segmentTree.cachedData.srs.Inner.Pk)
	if err != nil {
		log.Fatalf("failed to commit: %v", err)
	}
	commitmentBytes := commitment.Bytes()
	commitmentHash := common.BytesToHash(commitmentBytes[:])
	// fmt.Println(commitmentHash)
	return commitmentHash

}

func (segmentTree *LayeredSegmentTree) UpdateLayer2(idx int, val common.Hash) common.Hash {

	return segmentTree.UpdateLayerX(idx, val, segmentTree.Layer2Tree, segmentTree.Layer2Polynomial)

}

func (segmentTree *LayeredSegmentTree) UpdateLayer1(idx int, val common.Hash) common.Hash {

	return segmentTree.UpdateLayerX(idx, val, segmentTree.Layer1Tree, segmentTree.Layer1Polynomial)
}

func GenerateIncrementalPolynomial(indexToProcess []int, V polynomial.Polynomial, weights []fr.Element, segmentTree SegmentTree) polynomial.Polynomial {

	xValues := make([]int, len(indexToProcess))
	yValues := make([]fr.Element, len(indexToProcess))

	for i, index := range indexToProcess {
		xValues[i] = index
		yValues[i] = polynomial.HashToFieldElement(segmentTree[index])
	}

	incPoly := polynomial.Interpolate(xValues, yValues, V, weights)

	return incPoly
}
