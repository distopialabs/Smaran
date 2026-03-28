package proof

import (
	"fmt"
	"log"
	"slices"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/fft"

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

func VerifyNewRangeProofs(account common.Address, startingVersion, endingVersion uint64, rangeProofs []*RangeProof, balanceInfos []*tree.HistoricalBalance, precomputedData *config.PrecomputedData, mptProofNodes [][]byte, stateRoot common.Hash, currentBalance *tree.CurrentBalance) error {
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

	// Step 1: Verify MPT proof to establish trust in the top-layer commitment.
	if stateRoot != (common.Hash{}) && len(mptProofNodes) == 0 {
		return fmt.Errorf("state root provided but server sent no MPT proof nodes — cannot verify trust anchor")
	}
	if len(mptProofNodes) > 0 && stateRoot == (common.Hash{}) {
		return fmt.Errorf("MPT proof nodes received but no --state-root given — cannot verify trust anchor")
	}
	if len(mptProofNodes) > 0 {
		exists, mptBalance, err := VerifyMPTProof(stateRoot, account, mptProofNodes)
		if err != nil {
			return fmt.Errorf("MPT proof verification failed: %w", err)
		}
		if !exists {
			return fmt.Errorf("MPT proof verification failed: account does not exist in state trie")
		}

		// Find the top-layer commitment from the provided range proofs.
		// The top-layer commitment hash is computed from the proof's commitment digest.
		var topLayerCommitmentHash common.Hash
		for _, reqCommit := range reqCommits {
			if reqCommit.layer == tree.MaxLayer {
				proofKey := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)
				rp := proofHashMap[proofKey]
				if rp == nil {
					return fmt.Errorf("top-layer commitment not found in provided proofs")
				}
				topLayerCommitmentHash = hash.CommitmentToHash(rp.Commitment)
				break
			}
		}

		// Verify the final commitment hash matches: hash(currentBalance, topLayerCommitmentHash) == mptBalance
		if !VerifyFinalCommitmentHash(mptBalance, currentBalance, topLayerCommitmentHash) {
			return fmt.Errorf("final commitment hash mismatch: MPT balance does not match hash(currentBalance, topLayerCommitment)")
		}
		log.Printf("MPT proof verification passed")
	}

	// Step 2: Rebuild segment tree and verify range proofs (existing logic).
	// TODO: Rebuild partial tree
	// start := time.Now()
	requiredTreeBatchesMap := RebuildSegmentTreeForVerify(account, lxRequiredBatchIdxs, startingVersion, endingVersion, balanceInfos, proofHashMap, reqCommits, precomputedData)
	// log.Printf("Time taken to rebuild segment tree: %v", time.Since(start))

	slices.SortFunc(reqCommits, func(a, b RangeCommitment) int {
		if a.layer != b.layer {
			return a.layer - b.layer
		}
		return a.idx - b.idx
	})
	isVerified := make(map[string]bool, len(rangeProofs))
	// verifyStart := time.Now()

	// loop from last item to first item
	for i := len(reqCommits) - 1; i >= 0; i-- {

		// innerVerifyStart := time.Now()
		reqCommit := reqCommits[i]

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)

		if proofHashMap[reqCommitKey] == nil {
			return fmt.Errorf("required commitment %s was not found in provided proofs", reqCommitKey)
		}

		if reqCommit.layer != tree.MaxLayer && !isVerified[reqCommitKey] {
			return fmt.Errorf("commitment %s is not verified", reqCommitKey)
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

		// Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		domain := fft.NewDomain(uint64(len(precomputedData.V) - 1))
		Z := polynomial.VanishingPolynomial(nodesToInterpolate, &domain.Generator)
		ZCommit, _ := kzg.CommitG2(Z, precomputedData.SRS.G2Powers)
		// ZCommit, err := gnark_kzg.Commit(Z, srs.Inner.Pk)
		// ZCommit, _ := kzg.CommitG2(Z, precomputedData.SRS.G2Powers)

		// zCommitBytes := ZCommit.Bytes()
		// zCommitHash := common.BytesToHash(zCommitBytes[:])
		// fmt.Printf("zCommitmentHash: %s\n", zCommitHash)

		// _ = ZCommit
		// if err != nil {
		// 	panic(err)
		// }

		key := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)
		I := make(polynomial.Polynomial, int(domain.Cardinality))
		for _, nodeIdx := range nodesToInterpolate {
			I[nodeIdx] = polynomial.HashToFieldElement(requiredTreeBatchesMap[key][nodeIdx])
		}
		// fft.BitReverse(I)
		domain.FFTInverse(I, fft.DIF)
		fft.BitReverse(I)

		ICommit, err := gnark_kzg.Commit(I, precomputedData.SRS.Inner.Pk)
		if err != nil {
			panic(err)
		}
		if err != nil {
			return fmt.Errorf("commit I polynomial for layer %d idx %d: %w", reqCommit.layer, reqCommit.idx, err)
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
			return fmt.Errorf("pairing check failed for layer %d idx %d: %w", reqCommit.layer, reqCommit.idx, err)
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
	// log.Printf("Time taken to verify range proofs: %v", time.Since(verifyStart))
	return nil
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
