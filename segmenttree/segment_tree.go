package segmenttree

import (
	"log"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"

	"github.com/nepal80m/samurai/kzg"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/polynomial"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
)

const L1BatchSize = 2048

// const L1BatchSize = 8

const L2BatchSize = 1365

// const L2BatchSize = 5

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

	// LXCommitmentsV3 map[int]map[int]bls.G1Affine

	L1Commitments map[int]bls.G1Affine
	L2Commitments map[int]bls.G1Affine
	L3Commitments map[int]bls.G1Affine
	L4Commitments map[int]bls.G1Affine
}

type CachedData struct {
	V             polynomial.Polynomial
	Weights       []fr.Element
	WeightCommits []gnark_kzg.Digest
	SRS           *kzg.MultiSRS
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

	prevL1CommitIncPoly polynomial.Polynomial
	prevL2CommitIncPoly polynomial.Polynomial
	prevL3CommitIncPoly polynomial.Polynomial

	LXTreeV3               map[int][]common.Hash
	LXPolynomialV3         map[int]polynomial.Polynomial
	LXCommitmentV3         map[int]gnark_kzg.Digest
	LXPrevCIncCommitmentV3 map[int]gnark_kzg.Digest

	// prevL1CommitV3 gnark_kzg.Digest

	// prevL2CommitV3 gnark_kzg.Digest

	// prevL3CommitV3 gnark_kzg.Digest

	CachedData *CachedData
	Storage    *Storage
}

func NewLayeredSegmentTree(V polynomial.Polynomial, weights []fr.Element, weightCommits []gnark_kzg.Digest, srs *kzg.MultiSRS) *LayeredSegmentTree {
	return &LayeredSegmentTree{
		Layer1Tree: make([]common.Hash, SegmentTreeSize),
		Layer2Tree: make([]common.Hash, SegmentTreeSize),
		Layer3Tree: make([]common.Hash, SegmentTreeSize),
		Layer4Tree: make([]common.Hash, SegmentTreeSize),

		// LXTreeV3: make(map[int][]common.Hash),
		LXTreeV3: map[int][]common.Hash{
			1: make([]common.Hash, SegmentTreeSize),
			2: make([]common.Hash, SegmentTreeSize),
			3: make([]common.Hash, SegmentTreeSize),
			4: make([]common.Hash, SegmentTreeSize),
		},
		LXPolynomialV3: map[int]polynomial.Polynomial{
			1: make(polynomial.Polynomial, SegmentTreeSize),
			2: make(polynomial.Polynomial, SegmentTreeSize),
			3: make(polynomial.Polynomial, SegmentTreeSize),
			4: make(polynomial.Polynomial, SegmentTreeSize),
		},
		LXCommitmentV3:         make(map[int]gnark_kzg.Digest),
		LXPrevCIncCommitmentV3: make(map[int]gnark_kzg.Digest),

		Layer1Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),
		prevL1CommitIncPoly: make(polynomial.Polynomial, SegmentTreeSize),
		Layer2Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),
		prevL2CommitIncPoly: make(polynomial.Polynomial, SegmentTreeSize),
		Layer3Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),
		prevL3CommitIncPoly: make(polynomial.Polynomial, SegmentTreeSize),
		Layer4Polynomial:    make(polynomial.Polynomial, SegmentTreeSize),

		CachedData: &CachedData{
			V:             V,
			Weights:       weights,
			WeightCommits: weightCommits,
			SRS:           srs,
		},
		Storage: &Storage{
			L1Commitments: make(map[int]bls.G1Affine),
			L2Commitments: make(map[int]bls.G1Affine),
			L3Commitments: make(map[int]bls.G1Affine),
			L4Commitments: make(map[int]bls.G1Affine),
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

	l1CommitIndex := blockNumber / L1BatchSize
	l2CommitIndex := blockNumber / (L1BatchSize * L2BatchSize)
	l3CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize)
	l4CommitIndex := blockNumber / (L1BatchSize * L2BatchSize * L2BatchSize * L2BatchSize)
	_ = l1CommitIndex
	_ = l2CommitIndex
	_ = l3CommitIndex
	_ = l4CommitIndex

	if blockNumber%L1BatchSize == 0 {
		// if idx0 == 0 {
		// fmt.Println("resetting layer 1 tree")
		segmentTree.Layer1Tree = make([]common.Hash, SegmentTreeSize)
		segmentTree.Layer1Polynomial = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.prevL1CommitIncPoly = make(polynomial.Polynomial, SegmentTreeSize)

		segmentTree.LXTreeV3[1] = make([]common.Hash, SegmentTreeSize)
		segmentTree.LXPolynomialV3[1] = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.LXCommitmentV3[1] = gnark_kzg.Digest{}
		segmentTree.LXPrevCIncCommitmentV3[2] = gnark_kzg.Digest{}
	}
	if blockNumber%(L1BatchSize*L2BatchSize) == 0 {
		// if idx1 == 0 && len(segmentTree.Layer2Tree) > 0 {
		// fmt.Println("resetting layer 2 tree")
		segmentTree.Layer2Tree = make([]common.Hash, SegmentTreeSize)
		segmentTree.Layer2Polynomial = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.prevL2CommitIncPoly = make(polynomial.Polynomial, SegmentTreeSize)

		segmentTree.LXTreeV3[2] = make([]common.Hash, SegmentTreeSize)
		segmentTree.LXPolynomialV3[2] = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.LXCommitmentV3[2] = gnark_kzg.Digest{}
		segmentTree.LXPrevCIncCommitmentV3[3] = gnark_kzg.Digest{}
	}
	if blockNumber%(L1BatchSize*L2BatchSize*L2BatchSize) == 0 {
		// if idx2 == 0 && len(segmentTree.Layer3Tree) > 0 {
		// fmt.Println("resetting layer 3 tree")
		segmentTree.Layer3Tree = make([]common.Hash, SegmentTreeSize)
		segmentTree.Layer3Polynomial = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.prevL3CommitIncPoly = make(polynomial.Polynomial, SegmentTreeSize)

		segmentTree.LXTreeV3[3] = make([]common.Hash, SegmentTreeSize)
		segmentTree.LXPolynomialV3[3] = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.LXCommitmentV3[3] = gnark_kzg.Digest{}
		segmentTree.LXPrevCIncCommitmentV3[4] = gnark_kzg.Digest{}
	}
	if blockNumber%(L1BatchSize*L2BatchSize*L2BatchSize*L2BatchSize) == 0 {
		// if idx3 == 0 && len(segmentTree.Layer4Tree) > 0 {
		// fmt.Println("resetting layer 4 tree")
		segmentTree.Layer4Tree = make([]common.Hash, SegmentTreeSize)
		segmentTree.LXPolynomialV3[4] = make(polynomial.Polynomial, SegmentTreeSize)
		segmentTree.Layer4Polynomial = make(polynomial.Polynomial, SegmentTreeSize)
		// segmentTree.prevL4CommitIncPoly = make(polynomial.Polynomial, SegmentTreeSize)

		segmentTree.LXTreeV3[4] = make([]common.Hash, SegmentTreeSize)
		segmentTree.LXCommitmentV3[4] = gnark_kzg.Digest{}
		// segmentTree.LXPrevCIncCommitmentV3[4] = gnark_kzg.Digest{}
	}

	// updating layer 1

	// // OPT 1
	// start := time.Now()
	segmentTree.UpdateLayerX(L1BatchSize-1+idx0, common.BigToHash(balance), common.Hash{}, 1)
	// l1CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer1Polynomial)
	// l1RootHash := segmentTree.Layer1Tree[0]
	// fmt.Println("Time taken to calculate commitment for layer 1, V1:", time.Since(start))
	// // OPT 1 END

	// OPT 2
	// _, incCommit := segmentTree.UpdateLayer1Tree(L1BatchSize-1+idx0, common.BigToHash(balance), common.Hash{}, 1)

	// l1Commit := segmentTree.Storage.L1Commitments[l1CommitIndex]

	// l1Commit.Add(&incCommit, &l1Commit)
	// segmentTree.Storage.L1Commitments[l1CommitIndex] = l1Commit

	// l1CommitHash := CommitmentToHash(l1Commit)

	// OPT 2 END

	// OPT 3
	// start := time.Now()
	l1CommitV3 := segmentTree.UpdateLayerXTreeV3(L1BatchSize-1+idx0, common.BigToHash(balance), common.Hash{}, 1)
	l1CommitHashV3 := CommitmentToHash(l1CommitV3)
	l1RootHashV3 := segmentTree.LXTreeV3[1][0]

	// if l1CommitHash != l1CommitHashV3 {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l1CommitHash:", l1CommitHash)
	// 	fmt.Println("l1CommitHashV3:", l1CommitHashV3)
	// 	panic("Commitment mismatch between OPT 2 and OPT 3")
	// }
	// if l1RootHash != l1RootHashV3 {
	// 	fmt.Println("l1RootHash:", l1RootHash)
	// 	fmt.Println("l1RootHashV3:", l1RootHashV3)
	// 	panic("Root mismatch between OPT 2 and OPT 3")
	// }
	// fmt.Println("Time taken to calculate commitment for layer 1, V3:", time.Since(start))
	// OPT 3 END

	// TODO: use loop to update all layers
	// updating layer 2
	// start = time.Now()
	segmentTree.UpdateLayerX(L2BatchSize-1+idx1, l1RootHashV3, l1CommitHashV3, 2)
	// l2CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer2Polynomial)
	// l2RootHash := segmentTree.Layer2Tree[0]
	// fmt.Println("Time taken to calculate commitment for layer 2, V1:", time.Since(start))

	// OPT 3
	// start = time.Now()
	l2CommitV3 := segmentTree.UpdateLayerXTreeV3(L2BatchSize-1+idx1, l1RootHashV3, l1CommitHashV3, 2)
	l2CommitHashV3 := CommitmentToHash(l2CommitV3)
	l2RootHashV3 := segmentTree.LXTreeV3[2][0]

	// if l2CommitHash != l2CommitHashV3 {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l2CommitHash:", l2CommitHash)
	// 	fmt.Println("l2CommitHashV3:", l2CommitHashV3)

	// 	panic("Commitment mismatch between OPT 2 and OPT 3 in layer 2")
	// }
	// if l2RootHash != l2RootHashV3 {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l2RootHash:", l2RootHash)
	// 	fmt.Println("l2RootHashV3:", l2RootHashV3)

	// 	panic("Root mismatch between OPT 2 and OPT 3 in layer 2")
	// }
	// fmt.Println("Time taken to calculate commitment for layer 2, V3:", time.Since(start))
	// OPT 3 END

	// updating layer 3
	// start = time.Now()
	segmentTree.UpdateLayerX(L2BatchSize-1+idx2, l2RootHashV3, l2CommitHashV3, 3)
	// l3CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer3Polynomial)
	// l3RootHash := segmentTree.Layer3Tree[0]
	// fmt.Println("Time taken to calculate commitment for layer 3, V1:", time.Since(start))
	// OPT 3
	// start = time.Now()
	l3CommitV3 := segmentTree.UpdateLayerXTreeV3(L2BatchSize-1+idx2, l2RootHashV3, l2CommitHashV3, 3)
	l3CommitHashV3 := CommitmentToHash(l3CommitV3)
	l3RootHashV3 := segmentTree.LXTreeV3[3][0]
	// if l3CommitHash != l3CommitHashV3 {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l3CommitHash:", l3CommitHash)
	// 	fmt.Println("l3CommitHashV3:", l3CommitHashV3)
	// 	panic("Commitment mismatch between OPT 2 and OPT 3 in layer 3")
	// }
	// if l3RootHash != l3RootHashV3 {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l3RootHash:", l3RootHash)
	// 	fmt.Println("l3RootHashV3:", l3RootHashV3)
	// 	panic("Root mismatch between OPT 2 and OPT 3 in layer 3")
	// }
	// fmt.Println("Time taken to calculate commitment for layer 3, V3:", time.Since(start))
	// OPT 3 END

	// updating layer 4
	// start = time.Now()
	segmentTree.UpdateLayerX(L2BatchSize-1+idx3, l3RootHashV3, l3CommitHashV3, 4)
	// l4CommitHash := segmentTree.CalculateCommitment(segmentTree.Layer4Polynomial)
	// l4RootHash := segmentTree.Layer4Tree[0]
	// fmt.Println("Time taken to calculate commitment for layer 4, V1:", time.Since(start))

	// OPT 3
	// start = time.Now()
	l4CommitV3 := segmentTree.UpdateLayerXTreeV3(L2BatchSize-1+idx3, l3RootHashV3, l3CommitHashV3, 4)
	l4CommitHashV3 := CommitmentToHash(l4CommitV3)
	l4RootHashV3 := segmentTree.LXTreeV3[4][0]
	_ = l4CommitHashV3
	_ = l4RootHashV3
	// if l4CommitHash != l4CommitHashV3 {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l4CommitHash:", l4CommitHash)
	// 	fmt.Println("l4CommitHashV3:", l4CommitHashV3)
	// 	panic("Commitment mismatch between OPT 2 and OPT 3 in layer 4")
	// }
	// if l4RootHashV3 != l4RootHash {
	// 	fmt.Println("BlockNumber", blockNumber)
	// 	fmt.Println("l4RootHash:", l4RootHash)
	// 	fmt.Println("l4RootHashV3:", l4RootHashV3)
	// 	panic("Root mismatch between OPT 2 and OPT 3 in layer 4")
	// }

	// fmt.Println("Time taken to calculate commitment for layer 4, V3:", time.Since(start))
	// OPT 3 END

	// start = time.Now()

	segmentTree.Storage.L1Commitments[l1CommitIndex] = l1CommitV3
	l1CommitV3.Bytes()
	segmentTree.Storage.L2Commitments[l2CommitIndex] = l2CommitV3
	segmentTree.Storage.L3Commitments[l3CommitIndex] = l3CommitV3
	segmentTree.Storage.L4Commitments[l4CommitIndex] = l4CommitV3

	segmentTree.Storage.L1Tree[l1CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L1Tree[l1CommitIndex], segmentTree.LXTreeV3[1])

	segmentTree.Storage.L2Tree[l2CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L2Tree[l2CommitIndex], segmentTree.LXTreeV3[2])

	segmentTree.Storage.L3Tree[l3CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L3Tree[l3CommitIndex], segmentTree.LXTreeV3[3])

	segmentTree.Storage.L4Tree[l4CommitIndex] = make([]common.Hash, SegmentTreeSize)
	copy(segmentTree.Storage.L4Tree[l4CommitIndex], segmentTree.LXTreeV3[4])

	segmentTree.Storage.L1Polynomial[l1CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L1Polynomial[l1CommitIndex], segmentTree.Layer1Polynomial)
	segmentTree.Storage.L2Polynomial[l2CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L2Polynomial[l2CommitIndex], segmentTree.Layer2Polynomial)
	segmentTree.Storage.L3Polynomial[l3CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L3Polynomial[l3CommitIndex], segmentTree.Layer3Polynomial)
	segmentTree.Storage.L4Polynomial[l4CommitIndex] = make(polynomial.Polynomial, SegmentTreeSize)
	copy(segmentTree.Storage.L4Polynomial[l4CommitIndex], segmentTree.Layer4Polynomial)
	// fmt.Println("Time taken to store data in storage", time.Since(start))

}

// TODO: move this to kzg package and take srs as argument
func (segmentTree *LayeredSegmentTree) CalculateCommitment(poly polynomial.Polynomial) common.Hash {

	commitment, err := gnark_kzg.Commit(poly, segmentTree.CachedData.SRS.Inner.Pk)
	if err != nil {
		log.Fatalf("failed to commit: %v", err)
	}
	return CommitmentToHash(commitment)
	// commitmentBytes := commitment.Bytes()
	// commitmentHash := common.BytesToHash(commitmentBytes[:])

	// return commitmentHash
}

func (segmentTree *LayeredSegmentTree) UpdateLayer1Tree(idx int, val common.Hash, l1CommitHash common.Hash, layer int) (polynomial.Polynomial, bls.G1Affine) {

	polysPointers := map[int]*polynomial.Polynomial{
		1: &segmentTree.Layer1Polynomial,
		2: &segmentTree.Layer2Polynomial,
		3: &segmentTree.Layer3Polynomial,
		4: &segmentTree.Layer4Polynomial,
	}
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
	// fmt.Printf("layer %d poly length: %d\n", layer, len(poly))
	// fmt.Printf("layer %d poly: %v\n", layer, poly)

	var incPoly1 polynomial.Polynomial
	var incPoly2 polynomial.Polynomial
	var prevCommitIncPoly polynomial.Polynomial
	var hasCommitValueAlready bool

	if layer > 1 {
		// updating lower layer commitment value and polynomial

		tree[L2BatchSize+idx] = l1CommitHash
		prevCommitIncPoly = prevCommitIncPolys[layer]

		hasCommitValueAlready = tree[L2BatchSize+idx] != common.Hash{}
		// if hasCommitValueAlready {
		// 	poly.Sub(poly, prevCommitIncPoly)
		// }

		incPoly1 = polynomial.Interpolate([]int{L2BatchSize + idx}, []fr.Element{polynomial.HashToFieldElement(l1CommitHash)}, segmentTree.CachedData.V, segmentTree.CachedData.Weights)
		// copy(prevCommitIncPoly, incPoly1)

		// poly.Add(poly, incPoly1)

		// polyPointer := polysPointers[layer]
		// *polyPointer = poly
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
			// tree[parentIdx] = GetParentHash(lChild, rChild)
			tree[parentIdx] = BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())
			// tree[parentIdx] = crypto.Keccak256Hash(
			// 	lChild.Bytes(),
			// 	rChild.Bytes(),
			// )

			updatedIndices = append(updatedIndices, parentIdx)

			idx = parentIdx

		}
		// fmt.Println("inc poly length", len(poly))

		// update the polynomial

		incPoly2 = GenerateIncrementalPolynomial(updatedIndices, segmentTree.CachedData.V, segmentTree.CachedData.Weights, tree)
		// fmt.Println("inc poly", incPoly)

		// poly.Add(poly, incPoly2)

		// polyPointer := polysPointers[layer]
		// *polyPointer = poly

	}

	if layer > 1 {
		if hasCommitValueAlready {
			poly.Sub(poly, prevCommitIncPoly)
		}
		copy(prevCommitIncPoly, incPoly1)

		poly.Add(poly, incPoly1)
	}
	var incCommitment bls.G1Affine
	if (val != common.Hash{}) {
		poly.Add(poly, incPoly2)
		var err error
		incCommitment, err = gnark_kzg.Commit(incPoly2, segmentTree.CachedData.SRS.Inner.Pk)
		if err != nil {
			panic(err)
		}

	}
	polyPointer := polysPointers[layer]
	*polyPointer = poly
	return incPoly1, incCommitment

}

func (segmentTree *LayeredSegmentTree) UpdateLayerXTreeV3(idx int, val common.Hash, lXm1CommitHash common.Hash, layer int) bls.G1Affine {

	tree := segmentTree.LXTreeV3[layer]
	prevCommit := segmentTree.LXCommitmentV3[layer]

	// prevCIncCommit := segmentTree.LXPrevCIncCommitmentV3[layer]

	var newCommit bls.G1Affine
	newCommit.Set(&prevCommit)
	// var newPrevCIncCommit bls.G1Affine

	if layer > 1 {
		prevCIncCommit := segmentTree.LXPrevCIncCommitmentV3[layer]
		// TODO: use tree[L2BatchSize+idx] to calculate prevCIncCommit and subtract it from newCommit
		// hasCommitValueAlready := tree[L2BatchSize+idx] != common.Hash{}

		if (prevCIncCommit != bls.G1Affine{}) {
			// if hasCommitValueAlready {
			newCommit.Sub(&newCommit, &prevCIncCommit)
		}

		tree[L2BatchSize+idx] = lXm1CommitHash

		var incCommit bls.G1Affine
		incCommit.ScalarMultiplication(&segmentTree.CachedData.WeightCommits[L2BatchSize+idx], lXm1CommitHash.Big())
		// ?: can we use multi expo here? ans: too much overhead
		// points := []bls.G1Affine{segmentTree.CachedData.WeightCommits[L2BatchSize+idx]}
		// scalars := []fr.Element{polynomial.HashToFieldElement(lXm1CommitHash)}
		// var incCommit bls.G1Affine
		// incCommit.MultiExp(points, scalars, ecc.MultiExpConfig{})

		newCommit.Add(&newCommit, &incCommit)
		segmentTree.LXPrevCIncCommitmentV3[layer] = incCommit
		// newPrevCIncCommit.Set(&incCommit)

	}

	if (val != common.Hash{}) {
		tree[idx] = val
		updatedIndices := []int{idx}
		updatedXs := []int{idx}
		updatedYs := []*big.Int{val.Big()}
		for idx > 0 {
			parentIdx := GetParent(idx)

			lChild := tree[2*parentIdx+1]
			rChild := tree[2*parentIdx+2]
			if (lChild == common.Hash{} || rChild == common.Hash{}) {
				break
			}
			tree[parentIdx] = BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())

			updatedIndices = append(updatedIndices, parentIdx)
			updatedXs = append(updatedXs, parentIdx)
			updatedYs = append(updatedYs, tree[parentIdx].Big())

			idx = parentIdx

		}
		if len(updatedIndices) > 7 {
			// Using multi expo for large number of updates
			points := make([]bls.G1Affine, len(updatedXs))
			scalars := make([]fr.Element, len(updatedXs))
			for i, idx := range updatedIndices {

				points[i] = segmentTree.CachedData.WeightCommits[idx]
				scalars[i] = polynomial.HashToFieldElement(tree[idx])
			}
			var tempIncCommit bls.G1Affine
			tempIncCommit.MultiExp(points, scalars, ecc.MultiExpConfig{})
			newCommit.Add(&newCommit, &tempIncCommit)
		} else {

			for i, idx := range updatedIndices {

				var incCommit bls.G1Affine
				incCommit.ScalarMultiplication(&segmentTree.CachedData.WeightCommits[idx], updatedYs[i])

				newCommit.Add(&newCommit, &incCommit)

			}
		}

	}
	segmentTree.LXCommitmentV3[layer] = newCommit

	return newCommit
}

func (segmentTree *LayeredSegmentTree) UpdateLayerX(idx int, val common.Hash, l1CommitHash common.Hash, layer int) {

	polysPointers := map[int]*polynomial.Polynomial{
		1: &segmentTree.Layer1Polynomial,
		2: &segmentTree.Layer2Polynomial,
		3: &segmentTree.Layer3Polynomial,
		4: &segmentTree.Layer4Polynomial,
	}
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

	// fmt.Println(tree)
	// fmt.Println(gnark_kzg.Commit(poly, segmentTree.CachedData.SRS.Inner.Pk))

	// fmt.Printf("layer %d poly length: %d\n", layer, len(poly))
	// fmt.Printf("layer %d poly: %v\n", layer, poly)

	var incPoly1 polynomial.Polynomial
	var incPoly2 polynomial.Polynomial
	var prevCommitIncPoly polynomial.Polynomial
	var hasCommitValueAlready bool

	if layer > 1 {
		// updating lower layer commitment value and polynomial

		tree[L2BatchSize+idx] = l1CommitHash
		prevCommitIncPoly = prevCommitIncPolys[layer]

		hasCommitValueAlready = tree[L2BatchSize+idx] != common.Hash{}
		// if hasCommitValueAlready {
		// 	poly.Sub(poly, prevCommitIncPoly)
		// }

		incPoly1 = polynomial.Interpolate([]int{L2BatchSize + idx}, []fr.Element{polynomial.HashToFieldElement(l1CommitHash)}, segmentTree.CachedData.V, segmentTree.CachedData.Weights)
		// copy(prevCommitIncPoly, incPoly1)

		// poly.Add(poly, incPoly1)

		// polyPointer := polysPointers[layer]
		// *polyPointer = poly
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
			// tree[parentIdx] = GetParentHash(lChild, rChild)
			tree[parentIdx] = BytesToPoseidonHash(lChild.Bytes(), rChild.Bytes())
			// tree[parentIdx] = crypto.Keccak256Hash(
			// 	lChild.Bytes(),
			// 	rChild.Bytes(),
			// )

			updatedIndices = append(updatedIndices, parentIdx)

			idx = parentIdx

		}
		// fmt.Println("inc poly length", len(poly))

		// update the polynomial

		incPoly2 = GenerateIncrementalPolynomial(updatedIndices, segmentTree.CachedData.V, segmentTree.CachedData.Weights, tree)
		// fmt.Println("inc poly", incPoly)

		// poly.Add(poly, incPoly2)

		// polyPointer := polysPointers[layer]
		// *polyPointer = poly

	}
	if layer > 1 {
		if hasCommitValueAlready {
			poly.Sub(poly, prevCommitIncPoly)
		}
		copy(prevCommitIncPoly, incPoly1)

		poly.Add(poly, incPoly1)
	}
	if (val != common.Hash{}) {
		poly.Add(poly, incPoly2)
	}
	polyPointer := polysPointers[layer]
	*polyPointer = poly

	// fmt.Println(tree)
	// fmt.Println(gnark_kzg.Commit(poly, segmentTree.CachedData.SRS.Inner.Pk))
	// fmt.Println(poly)
}

func GenerateIncrementalPolynomial(indexToProcess []int, V polynomial.Polynomial, weights []fr.Element, tree []common.Hash) polynomial.Polynomial {

	xValues := make([]int, len(indexToProcess))
	yValues := make([]fr.Element, len(indexToProcess))

	for i, index := range indexToProcess {
		xValues[i] = index
		yValues[i] = polynomial.HashToFieldElement(tree[index])
	}

	incPoly := polynomial.Interpolate(xValues, yValues, V, weights)

	return incPoly
}
