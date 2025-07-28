package proof

import (
	"fmt"
	"slices"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"
	"github.com/nepal80m/samurai/segmenttree"
)

func VerifyRangeProofs(startingBlock, endingBlock int, rangeProofs []*RangeProof, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS, storage *segmenttree.Storage) {
	lxTrees := map[int]map[int][]common.Hash{
		1: storage.L1Tree,
		2: storage.L2Tree,
		3: storage.L3Tree,
		4: storage.L4Tree,
	}

	RebuildSegmentTree(startingBlock, endingBlock, rangeProofs)

	proofHashMap := map[string]*RangeProof{}
	isVerified := map[string]bool{}
	for _, proof := range rangeProofs {
		key := fmt.Sprintf("%d:%d", proof.layer, proof.idx)
		proofHashMap[key] = proof
		isVerified[key] = false
	}

	reqCommits := findCommitmentsCoveringRange(startingBlock, endingBlock)

	// sort reqCommits by layer and idx
	slices.SortFunc(reqCommits, func(a, b RangeCommitment) int {
		if a.layer != b.layer {
			return a.layer - b.layer
		}
		return a.idx - b.idx
	})

	// loop from last item to first item
	fmt.Println("\n\nVerifying range proofs...")
	for i := len(reqCommits) - 1; i >= 0; i-- {
		reqCommit := reqCommits[i]
		fmt.Printf("reqCommit layer: %d, idx: %d\n", reqCommit.layer, reqCommit.idx)

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)

		if proofHashMap[reqCommitKey] == nil {
			panic("This required commitment was not found in provided proofs.")
		}

		if reqCommit.layer == segmenttree.MaxLayer || isVerified[reqCommitKey] {
			fmt.Println("Already verified this commitment.")

		} else {
			panic("This commitment is not verified.")
		}

		nodesToInterpolate := findNodesToInterpolate(reqCommit)
		rangeProof := proofHashMap[reqCommitKey]

		Commitment := rangeProof.Commitment
		// TODO: reconstruct tree using given balance values
		tree := lxTrees[reqCommit.layer][reqCommit.idx]

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		ZCommit, err := gnark_kzg.Commit(Z, srs.Inner.Pk)
		_ = ZCommit
		if err != nil {
			panic(err)
		}

		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, v := range nodesToInterpolate {
			ys[i] = polynomial.HashToFieldElement(tree[v])
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
		for _, depCommit := range reqCommit.dependentCommitments {
			depCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer-1, depCommit)
			isVerified[depCommitKey] = true
		}

		// nodesToInterpolate := findNodesToInterpolate(rangeProof)

		// balance := big.NewInt(1000000000000000000)

		// Z := polynomial.VanishingPolynomial(nodesToInterpolate)

	}

}

func RebuildSegmentTree(startingBlock, endingBlock int, rangeProofs []*RangeProof) {
	reqCommits := findCommitmentsCoveringRange(startingBlock, endingBlock)

	// sort reqCommits by layer and idx
	rangeCoveringCommits := make([]RangeCommitment, 0)
	for _, commit := range reqCommits {
		if commit.BlockRange != nil {
			rangeCoveringCommits = append(rangeCoveringCommits, commit)
			// fmt.Printf("range covering commit: %d:%d sb: %d eb: %d\n", commit.layer, commit.idx, commit.BlockRange.Start, commit.BlockRange.End)
		}
	}

	slices.SortFunc(rangeCoveringCommits, func(a, b RangeCommitment) int {
		return a.BlockRange.Start - b.BlockRange.Start
	})
	fmt.Println("\n")
	for _, commit := range rangeCoveringCommits {
		fmt.Printf("range covering commit: layer: %d, idx: %d, sb: %d, eb: %d\n", commit.layer, commit.idx, commit.BlockRange.Start, commit.BlockRange.End)
		nodesToInterpolate := findNodesToInterpolate(commit)
		nodesToFindValueOf := make([]int, 0)
		for _, node := range nodesToInterpolate {
			if commit.layer > 1 && node >= 2*segmenttree.L2BatchSize-1 {
				continue
			}
			nodesToFindValueOf = append(nodesToFindValueOf, node)
		}

		fmt.Printf("nodes to find value of: %v\n", nodesToFindValueOf)

	}
}

func PairingCheck(commit bls.G1Affine, proof bls.G1Affine, iCommit bls.G1Affine, zCommit bls.G1Affine, srs *kzg.MultiSRS) (bool, error) {
	return true, nil
}
