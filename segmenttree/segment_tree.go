package segmenttree

import (
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/nepal80m/samurai/kzg"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/nepal80m/samurai/polynomial"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

// const L1BatchSize = 2048

const L1BatchSize = 8

// const L2BatchSize = 1365

const L2BatchSize = 5

const MaxLayer = 4

const SegmentTreeSize = L1BatchSize * 2 //2048 * 2 = 4096

type Storage struct {
	L1Tree map[int][]common.Hash
	L2Tree map[int][]common.Hash
	L3Tree map[int][]common.Hash
	L4Tree map[int][]common.Hash

	L1Polynomial map[int]polynomial.Polynomial
	L2Polynomial map[int]polynomial.Polynomial
	L3Polynomial map[int]polynomial.Polynomial
	L4Polynomial map[int]polynomial.Polynomial

	L1Commitments map[int]common.Hash
	L2Commitments map[int]common.Hash
	L3Commitments map[int]common.Hash
	L4Commitments map[int]common.Hash
}

type CachedData struct {
	V       polynomial.Polynomial
	Weights []fr.Element
	SRS     *kzg.MultiSRS
}

type LayeredSegmentTree struct {
	Layer1Tree []common.Hash
	Layer2Tree []common.Hash
	Layer3Tree []common.Hash
	Layer4Tree []common.Hash

	Layer1Polynomial    polynomial.Polynomial
	prevL1CommitIncPoly polynomial.Polynomial
	Layer2Polynomial    polynomial.Polynomial
	prevL2CommitIncPoly polynomial.Polynomial
	Layer3Polynomial    polynomial.Polynomial
	prevL3CommitIncPoly polynomial.Polynomial
	Layer4Polynomial    polynomial.Polynomial

	CachedData *CachedData
	Storage    *Storage
}

func NewLayeredSegmentTree(V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) *LayeredSegmentTree {
	return &LayeredSegmentTree{
		Layer1Tree: make([]common.Hash, SegmentTreeSize),
		Layer2Tree: make([]common.Hash, SegmentTreeSize),
		Layer3Tree: make([]common.Hash, SegmentTreeSize),
		Layer4Tree: make([]common.Hash, SegmentTreeSize),

		Layer1Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),
		prevL1CommitIncPoly: make(polynomial.Polynomial, SegmentTreeSize),
		Layer2Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),
		prevL2CommitIncPoly: make(polynomial.Polynomial, SegmentTreeSize),
		Layer3Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),
		prevL3CommitIncPoly: make(polynomial.Polynomial, SegmentTreeSize),
		Layer4Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),

		CachedData: &CachedData{
			V:       V,
			Weights: weights,
			SRS:     srs,
		},
		Storage: &Storage{
			L1Commitments: make(map[int]common.Hash),
			L2Commitments: make(map[int]common.Hash),
			L3Commitments: make(map[int]common.Hash),
			L4Commitments: make(map[int]common.Hash),
			L1Tree:        make(map[int][]common.Hash),
			L2Tree:        make(map[int][]common.Hash),
			L3Tree:        make(map[int][]common.Hash),
			L4Tree:        make(map[int][]common.Hash),
			L1Polynomial:  make(map[int]polynomial.Polynomial),
			L2Polynomial:  make(map[int]polynomial.Polynomial),
			L3Polynomial:  make(map[int]polynomial.Polynomial),
			L4Polynomial:  make(map[int]polynomial.Polynomial),
		},
	}
}

func (segmentTree *LayeredSegmentTree) Update(blockNumber int, balance *big.Int) {

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
	segmentTree.UpdateLayerX(L1BatchSize-1+idx0, common.BigToHash(balance), common.Hash{}, 1)
	l1CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer1Polynomial)
	l1RootHash := segmentTree.Layer1Tree[0]

	// TODO: use loop to update all layers
	// updating layer 2
	segmentTree.UpdateLayerX(L2BatchSize-1+idx1, l1RootHash, l1CommitHash, 2)
	l2CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer2Polynomial)
	l2RootHash := segmentTree.Layer2Tree[0]

	// updating layer 3
	segmentTree.UpdateLayerX(L2BatchSize-1+idx2, l2RootHash, l2CommitHash, 3)
	l3CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer3Polynomial)
	l3RootHash := segmentTree.Layer3Tree[0]

	// updating layer 4
	segmentTree.UpdateLayerX(L2BatchSize-1+idx3, l3RootHash, l3CommitHash, 4)
	l4CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer4Polynomial)
	_ = l4CommitHash

	l1CommitIndex := blockNumber / L1BatchSize
	l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)

	segmentTree.Storage.L1Commitments[l1CommitIndex] = l1CommitHash
	segmentTree.Storage.L2Commitments[l2CommitIndex] = l2CommitHash
	segmentTree.Storage.L3Commitments[l3CommitIndex] = l3CommitHash
	segmentTree.Storage.L4Commitments[l4CommitIndex] = l4CommitHash

	segmentTree.Storage.L1Tree[l1CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L1Tree[l1CommitIndex], segmentTree.Layer1Tree)

	segmentTree.Storage.L2Tree[l2CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L2Tree[l2CommitIndex], segmentTree.Layer2Tree)

	segmentTree.Storage.L3Tree[l3CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L3Tree[l3CommitIndex], segmentTree.Layer3Tree)

	segmentTree.Storage.L4Tree[l4CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L4Tree[l4CommitIndex], segmentTree.Layer4Tree)

	segmentTree.Storage.L1Polynomial[l1CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L1Polynomial[l1CommitIndex], segmentTree.Layer1Polynomial)
	segmentTree.Storage.L2Polynomial[l2CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L2Polynomial[l2CommitIndex], segmentTree.Layer2Polynomial)
	segmentTree.Storage.L3Polynomial[l3CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L3Polynomial[l3CommitIndex], segmentTree.Layer3Polynomial)
	segmentTree.Storage.L4Polynomial[l4CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L4Polynomial[l4CommitIndex], segmentTree.Layer4Polynomial)

}

func (segmentTree *LayeredSegmentTree) CalculateCommitment(poly polynomial.Polynomial) common.Hash {

	commitment, err := gnark_kzg.Commit(poly, segmentTree.CachedData.SRS.Inner.Pk)
	if err != nil {
		log.Fatalf("failed to commit: %v", err)
	}
	commitmentBytes := commitment.Bytes()
	commitmentHash := common.BytesToHash(commitmentBytes[:])
	return commitmentHash
}

func (segmentTree *LayeredSegmentTree) UpdateLayerX(idx int, val common.Hash, l1CommitHash common.Hash, layer int) {

	polys := map[int]polynomial.Polynomial{
		1: segmentTree.Layer1Polynomial,
		2: segmentTree.Layer2Polynomial,
		3: segmentTree.Layer3Polynomial,
		4: segmentTree.Layer4Polynomial,
	}
	prevCommitIncPolys := map[int]polynomial.Polynomial{
		2: segmentTree.prevL1CommitIncPoly,
		3: segmentTree.prevL2CommitIncPoly,
		4: segmentTree.prevL3CommitIncPoly,
	}
	trees := map[int][]common.Hash{
		1: segmentTree.Layer1Tree,
		2: segmentTree.Layer2Tree,
		3: segmentTree.Layer3Tree,
		4: segmentTree.Layer4Tree,
	}
	// Update the tree

	tree := trees[layer]
	poly := polys[layer]

	if layer > 1 {
		// updating lower layer commitment value and polynomial

		tree[L2BatchSize+idx] = l1CommitHash
		prevCommitIncPoly := prevCommitIncPolys[layer]

		hasCommitValueAlready := tree[L2BatchSize+idx] != common.Hash{}
		if hasCommitValueAlready {
			poly.Sub(poly, prevCommitIncPoly)
		}

		incPoly := polynomial.Interpolate([]int{L2BatchSize + idx}, []fr.Element{polynomial.HashToFieldElement(l1CommitHash)}, segmentTree.CachedData.V, segmentTree.CachedData.Weights)
		copy(prevCommitIncPoly, incPoly)
		// prevCommitIncPoly = incPoly
		poly.Add(poly, incPoly)
	}

	if (val != common.Hash{}) {
		// segmentTree.UpdateLayerX(idx, val, segmentTree.Layer2Tree, segmentTree.Layer2Polynomial)
		//  update value at idx and its ancestors in the tree

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

		incPoly := GenerateIncrementalPolynomial(updatedIndices, segmentTree.CachedData.V, segmentTree.CachedData.Weights, tree)

		poly.Add(poly, incPoly)

	}

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

func (segmentTree *LayeredSegmentTree) DumpTrees() {

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
	err = os.WriteFile("l1Tree.json", l1TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l1Tree to file: %v", err)
	}

	l2TreeJSON, err := json.Marshal(l2Tree)
	if err != nil {
		log.Fatalf("failed to marshal l2Tree: %v", err)
	}
	err = os.WriteFile("l2Tree.json", l2TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l2Tree to file: %v", err)
	}

	l3TreeJSON, err := json.Marshal(l3Tree)
	if err != nil {
		log.Fatalf("failed to marshal l3Tree: %v", err)
	}
	err = os.WriteFile("l3Tree.json", l3TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l3Tree to file: %v", err)
	}

	l4TreeJSON, err := json.Marshal(l4Tree)
	if err != nil {
		log.Fatalf("failed to marshal l4Tree: %v", err)
	}
	err = os.WriteFile("l4Tree.json", l4TreeJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l4Tree to file: %v", err)
	}

	fmt.Println("Dumped trees to json files")

}
func (segmentTree *LayeredSegmentTree) DumpCommitments() {

	// dump commitments to a json file
	l1Commitments := segmentTree.Storage.L1Commitments
	l2Commitments := segmentTree.Storage.L2Commitments
	l3Commitments := segmentTree.Storage.L3Commitments
	l4Commitments := segmentTree.Storage.L4Commitments

	// store in separate json files
	l1CommitmentsJSON, err := json.Marshal(l1Commitments)
	if err != nil {
		log.Fatalf("failed to marshal l1Commitments: %v", err)
	}
	err = os.WriteFile("l1Commitments.json", l1CommitmentsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l1Commitments to file: %v", err)
	}

	l2CommitmentsJSON, err := json.Marshal(l2Commitments)
	if err != nil {
		log.Fatalf("failed to marshal l2Commitments: %v", err)
	}
	err = os.WriteFile("l2Commitments.json", l2CommitmentsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l2Commitments to file: %v", err)
	}

	l3CommitmentsJSON, err := json.Marshal(l3Commitments)
	if err != nil {
		log.Fatalf("failed to marshal l3Commitments: %v", err)
	}
	err = os.WriteFile("l3Commitments.json", l3CommitmentsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l3Commitments to file: %v", err)
	}

	l4CommitmentsJSON, err := json.Marshal(l4Commitments)
	if err != nil {
		log.Fatalf("failed to marshal l4Commitments: %v", err)
	}
	err = os.WriteFile("l4Commitments.json", l4CommitmentsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l4Commitments to file: %v", err)
	}

}

func (segmentTree *LayeredSegmentTree) DumpPolynomials() {

	// dump polynomials to a json file
	l1Polynomials := segmentTree.Storage.L1Polynomial
	l2Polynomials := segmentTree.Storage.L2Polynomial
	l3Polynomials := segmentTree.Storage.L3Polynomial
	l4Polynomials := segmentTree.Storage.L4Polynomial

	// store in separate json files
	l1PolynomialsJSON, err := json.Marshal(l1Polynomials)
	if err != nil {
		log.Fatalf("failed to marshal l1Polynomials: %v", err)
	}
	err = os.WriteFile("l1Polynomials.json", l1PolynomialsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l1Polynomials to file: %v", err)
	}

	l2PolynomialsJSON, err := json.Marshal(l2Polynomials)
	if err != nil {
		log.Fatalf("failed to marshal l2Polynomials: %v", err)
	}
	err = os.WriteFile("l2Polynomials.json", l2PolynomialsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l2Polynomials to file: %v", err)
	}

	l3PolynomialsJSON, err := json.Marshal(l3Polynomials)
	if err != nil {
		log.Fatalf("failed to marshal l3Polynomials: %v", err)
	}
	err = os.WriteFile("l3Polynomials.json", l3PolynomialsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l3Polynomials to file: %v", err)
	}

	l4PolynomialsJSON, err := json.Marshal(l4Polynomials)
	if err != nil {
		log.Fatalf("failed to marshal l4Polynomials: %v", err)
	}
	err = os.WriteFile("l4Polynomials.json", l4PolynomialsJSON, 0644)
	if err != nil {
		log.Fatalf("failed to write l4Polynomials to file: %v", err)
	}

	fmt.Println("Dumped polynomials to json files")

}

func (segmentTree *LayeredSegmentTree) DumpStorage() {
	segmentTree.DumpTrees()
	segmentTree.DumpCommitments()
	segmentTree.DumpPolynomials()
}

func (storage *Storage) LoadTrees() {

	l1TreeJSON, err := os.ReadFile("l1Tree.json")
	if err != nil {
		log.Fatalf("failed to read l1Tree from file: %v", err)
	}
	err = json.Unmarshal(l1TreeJSON, &storage.L1Tree)
	if err != nil {
		log.Fatalf("failed to unmarshal l1Tree: %v", err)
	}

	l2TreeJSON, err := os.ReadFile("l2Tree.json")
	if err != nil {
		log.Fatalf("failed to read l2Tree from file: %v", err)
	}
	err = json.Unmarshal(l2TreeJSON, &storage.L2Tree)
	if err != nil {
		log.Fatalf("failed to unmarshal l2Tree: %v", err)
	}

	l3TreeJSON, err := os.ReadFile("l3Tree.json")
	if err != nil {
		log.Fatalf("failed to read l3Tree from file: %v", err)
	}
	err = json.Unmarshal(l3TreeJSON, &storage.L3Tree)
	if err != nil {
		log.Fatalf("failed to unmarshal l3Tree: %v", err)
	}

	l4TreeJSON, err := os.ReadFile("l4Tree.json")
	if err != nil {
		log.Fatalf("failed to read l4Tree from file: %v", err)
	}
	err = json.Unmarshal(l4TreeJSON, &storage.L4Tree)
	if err != nil {
		log.Fatalf("failed to unmarshal l4Tree: %v", err)
	}

}

func (storage *Storage) LoadCommitments() {
	l1CommitmentsJSON, err := os.ReadFile("l1Commitments.json")
	if err != nil {
		log.Fatalf("failed to read l1Commitments from file: %v", err)
	}
	err = json.Unmarshal(l1CommitmentsJSON, &storage.L1Commitments)
	if err != nil {
		log.Fatalf("failed to unmarshal l1Commitments: %v", err)
	}

	l2CommitmentsJSON, err := os.ReadFile("l2Commitments.json")
	if err != nil {
		log.Fatalf("failed to read l2Commitments from file: %v", err)
	}
	err = json.Unmarshal(l2CommitmentsJSON, &storage.L2Commitments)
	if err != nil {
		log.Fatalf("failed to unmarshal l2Commitments: %v", err)
	}

	l3CommitmentsJSON, err := os.ReadFile("l3Commitments.json")
	if err != nil {
		log.Fatalf("failed to read l3Commitments from file: %v", err)
	}
	err = json.Unmarshal(l3CommitmentsJSON, &storage.L3Commitments)
	if err != nil {
		log.Fatalf("failed to unmarshal l3Commitments: %v", err)
	}

	l4CommitmentsJSON, err := os.ReadFile("l4Commitments.json")
	if err != nil {
		log.Fatalf("failed to read l4Commitments from file: %v", err)
	}
	err = json.Unmarshal(l4CommitmentsJSON, &storage.L4Commitments)
	if err != nil {
		log.Fatalf("failed to unmarshal l4Commitments: %v", err)
	}

}

func (storage *Storage) LoadPolynomials() {

	l1PolynomialsJSON, err := os.ReadFile("l1Polynomials.json")
	if err != nil {
		log.Fatalf("failed to read l1Polynomials from file: %v", err)
	}
	err = json.Unmarshal(l1PolynomialsJSON, &storage.L1Polynomial)
	if err != nil {
		log.Fatalf("failed to unmarshal l1Polynomials: %v", err)
	}

	l2PolynomialsJSON, err := os.ReadFile("l2Polynomials.json")
	if err != nil {
		log.Fatalf("failed to read l2Polynomials from file: %v", err)
	}
	err = json.Unmarshal(l2PolynomialsJSON, &storage.L2Polynomial)
	if err != nil {
		log.Fatalf("failed to unmarshal l2Polynomials: %v", err)
	}

	l3PolynomialsJSON, err := os.ReadFile("l3Polynomials.json")
	if err != nil {
		log.Fatalf("failed to read l3Polynomials from file: %v", err)
	}
	err = json.Unmarshal(l3PolynomialsJSON, &storage.L3Polynomial)
	if err != nil {
		log.Fatalf("failed to unmarshal l3Polynomials: %v", err)
	}

	l4PolynomialsJSON, err := os.ReadFile("l4Polynomials.json")
	if err != nil {
		log.Fatalf("failed to read l4Polynomials from file: %v", err)
	}
	err = json.Unmarshal(l4PolynomialsJSON, &storage.L4Polynomial)
	if err != nil {
		log.Fatalf("failed to unmarshal l4Polynomials: %v", err)
	}

}

func LoadStorage() *Storage {
	storage := &Storage{}
	storage.LoadTrees()
	storage.LoadCommitments()
	storage.LoadPolynomials()
	return storage
}

// Depricated
func (segmentTree *LayeredSegmentTree) OldUpdateLayer1(idx int, val common.Hash) {

	segmentTree.OldUpdateLayerX(idx, val, segmentTree.Layer1Tree, segmentTree.Layer1Polynomial)
}

// Depricated
func (segmentTree *LayeredSegmentTree) OldUpdateLayerX(idx int, val common.Hash, tree []common.Hash, poly polynomial.Polynomial) {
	//  update value at idx and its ancestors in the tree
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

	incPoly := GenerateIncrementalPolynomial(updatedIndices, segmentTree.CachedData.V, segmentTree.CachedData.Weights, tree)

	poly.Add(poly, incPoly)

}
