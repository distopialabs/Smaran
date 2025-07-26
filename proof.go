package main

import (
	"fmt"
	"math"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"
	"github.com/nepal80m/samurai/segmenttree"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

type BlockRange struct {
	Start, End int
}

type NewRangeProof struct {
	BlockRange  *BlockRange
	lXm1Commits []int
	Commitment  gnark_kzg.Digest
	Proof       bls.G1Affine
}
type RangeProof struct {
	startingBlock *int
	endingBlock   *int
	Commitment    gnark_kzg.Digest
	Proof         bls.G1Affine
	// ZCommit       bls.G1Affine
	// ICommit       bls.G1Affine

	onlyInterpolatesLXm1Commit bool
	lXm1CommitToInterpolate    []int
}

func getRangeProof(startingBlock, endingBlock int, storage *segmenttree.Storage, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) []*RangeProof {
	// lxCommitments := map[int]map[int]common.Hash{
	// 	1: storage.L1Commitments,
	// 	2: storage.L2Commitments,
	// 	3: storage.L3Commitments,
	// 	4: storage.L4Commitments,
	// }

	lxTrees := map[int]map[int][]common.Hash{
		1: storage.L1Tree,
		2: storage.L2Tree,
		3: storage.L3Tree,
		4: storage.L4Tree,
	}

	lxPolynomials := map[int]map[int]polynomial.Polynomial{
		1: storage.L1Polynomial,
		2: storage.L2Polynomial,
		3: storage.L3Polynomial,
		4: storage.L4Polynomial,
	}

	allRangeProofs := make([]*RangeProof, 0)

	reqCommits := findRequiredCommitments(startingBlock, endingBlock, 1, []RequiredCommitment{})
	for _, reqCommit := range reqCommits {
		fmt.Printf("idx: %d, layer: %d, sb: %d, eb: %d\n", reqCommit.idx, reqCommit.layer, reqCommit.sb, reqCommit.eb)
	}

	for _, reqCommit := range reqCommits {
		layer := reqCommit.layer
		idx := reqCommit.idx
		sb := reqCommit.sb
		eb := reqCommit.eb

		fmt.Printf("\n\nlayer: %d, idx: %d, sb: %d, eb: %d\n", layer, idx, sb, eb)

		l0BatchSize := segmenttree.L1BatchSize * Pow(segmenttree.L2BatchSize, layer-1)
		l0BatchStartIdx := idx * l0BatchSize
		l0BatchEndIdx := l0BatchStartIdx + l0BatchSize - 1

		lXm1BatchSize := l0BatchSize // this should match the size of the tree
		lXm1BatchStartIdx := l0BatchStartIdx
		lXm1BatchEndIdx := l0BatchEndIdx
		lXm1SB := sb
		lXm1EB := eb

		if layer > 1 {
			denom := segmenttree.L1BatchSize * Pow(segmenttree.L2BatchSize, layer-2)
			lXm1BatchSize /= denom // this should match the size of the tree
			lXm1BatchStartIdx /= denom
			lXm1BatchEndIdx /= denom
			lXm1SB /= denom
			lXm1EB /= denom
		}

		treeNodes := collectNodes(lXm1BatchSize, lXm1SB-lXm1BatchStartIdx, lXm1EB-lXm1BatchStartIdx)

		// commitment := lxCommitments[layer][idx]
		tree := lxTrees[layer][idx]
		P := lxPolynomials[layer][idx]

		commitment, err := gnark_kzg.Commit(P, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}

		Z := polynomial.VanishingPolynomial(treeNodes)
		ZCommit, err := gnark_kzg.Commit(Z, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}

		ys := make([]fr.Element, len(treeNodes))
		for i, v := range treeNodes {
			ys[i] = polynomial.HashToFieldElement(tree[v])
		}

		I := polynomial.Interpolate(treeNodes, ys, V, weights)
		ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}

		diff := kzg.SubtractPolys(P, I)
		Q := kzg.PolyDiv(diff, Z)
		QCommit, err := gnark_kzg.Commit(Q, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}

		rangeProof := &RangeProof{
			startingBlock: sb,
			endingBlock:   eb,
			Commitment:    commitment,
			Proof:         QCommit,
			ZCommit:       ZCommit,
			ICommit:       ICommit,
		}

		allRangeProofs = append(allRangeProofs, rangeProof)

	}

	return allRangeProofs

}

func Pow(base, exponent int) int {
	return int(math.Pow(float64(base), float64(exponent)))
}

type RequiredCommitment struct {
	idx   int
	layer int
	sb    int
	eb    int
}

// Given a starting block, ending block, and layer, get all the commitments required to prove it.
// layer is passed for recursion. check getRequiredCommitments.py for more details.
// returns a list of commitments required to prove the range.
// each commitment is a list of [idx, sb, eb, layer]
// idx is index of commitment in the storage.
// layer is the layer of the commitment.
// this commitment should cover the range of block of index sb to eb.
func findRequiredCommitments(sb, eb, layer int, commitments []RequiredCommitment) []RequiredCommitment {

	l0BatchSize := segmenttree.L1BatchSize * Pow(segmenttree.L2BatchSize, layer-1)

	hasLeftFragment := sb%(l0BatchSize) != 0
	hasRightFragment := eb%(l0BatchSize) != l0BatchSize-1

	leftCommitIndex := sb / (l0BatchSize)
	rightCommitIndex := eb / (l0BatchSize)

	if leftCommitIndex == rightCommitIndex && (hasLeftFragment || hasRightFragment) {
		// commitments = append(commitments, []int{leftCommitIndex, sb, eb, layer})
		commitments = append(commitments, RequiredCommitment{
			idx:   leftCommitIndex,
			layer: layer,
			sb:    sb,
			eb:    eb,
		})
		fmt.Printf("Only need upper layer commit for commit of layer: %d, idx: %d, sb: %d, eb: %d\n", layer, leftCommitIndex, sb, eb)
		return commitments
	}

	if hasLeftFragment {
		leftFragmentStart := sb
		leftFragmentEnd := (leftCommitIndex+1)*l0BatchSize - 1
		// commitments = append(commitments, []int{leftCommitIndex, leftFragmentStart, leftFragmentEnd, layer})

		commitments = append(commitments, RequiredCommitment{
			idx:   leftCommitIndex,
			layer: layer,
			sb:    leftFragmentStart,
			eb:    leftFragmentEnd,
		})
		fmt.Printf("Need upper layer commit for commit of layer: %d, idx: %d, sb: %d, eb: %d\n", layer, leftCommitIndex, leftFragmentStart, leftFragmentEnd)

		sb = leftFragmentEnd + 1
	}
	if hasRightFragment {

		rightFragmentStart := rightCommitIndex * l0BatchSize
		rightFragmentEnd := eb
		// commitments = append(commitments, []int{rightCommitIndex, rightFragmentStart, rightFragmentEnd, layer})

		commitments = append(commitments, RequiredCommitment{
			idx:   rightCommitIndex,
			layer: layer,
			sb:    rightFragmentStart,
			eb:    rightFragmentEnd,
		})
		fmt.Printf("Need upper layer commit for commit of layer: %d, idx: %d, sb: %d, eb: %d\n", layer, rightCommitIndex, rightFragmentStart, rightFragmentEnd)
		eb = rightFragmentStart - 1
	}
	if sb < eb {
		commitments = findRequiredCommitments(sb, eb, layer+1, commitments)
	}

	return commitments

}

func collectNodes(N, L, R int) []int {

	base := N - 1
	l := L + base
	r := R + base
	out := make([]int, 0)

	for l <= r {
		if l%2 == 0 {
			out = append(out, l)
			l += 1
		}
		if r%2 == 1 {
			out = append(out, r)
			r -= 1
		}
		l = int(math.Floor(float64(l-1) / 2))
		r = int(math.Floor(float64(r-1) / 2))
	}

	return out
}
