package proof

import (
	"fmt"
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

		// Determine the top-layer commitment hash based on response case.
		var topLayerCommitmentHash common.Hash

		if currentBalance.Version == 0 {
			// Case: version==0, no historical data. topLayerCommitmentHash stays empty (zero hash).
		} else if len(balanceInfos) == 0 {
			// Case: top-layer commitment only (current balance covers query range).
			// The server sends a single RangeProof at MaxLayer containing the commitment.
			if len(rangeProofs) != 1 || rangeProofs[0].Layer != tree.MaxLayer {
				return fmt.Errorf("expected single top-layer range proof for commitment-only response")
			}
			topLayerCommitmentHash = hash.CommitmentToHash(rangeProofs[0].Commitment)
		} else {
			// Case: full range proofs. Find top-layer commitment from range proofs.
			for _, rp := range rangeProofs {
				if rp.Layer == tree.MaxLayer {
					topLayerCommitmentHash = hash.CommitmentToHash(rp.Commitment)
					break
				}
			}
		}

		if !VerifyFinalCommitmentHash(mptBalance, currentBalance, topLayerCommitmentHash) {
			return fmt.Errorf("final commitment hash mismatch: MPT balance does not match hash(currentBalance, topLayerCommitment)")
		}
		// log.Printf("MPT proof verification passed")
	}

	// Cases with no balance infos (version==0 or top-layer-only) need no KZG verification.
	if len(balanceInfos) == 0 {
		return nil
	}

	// Step 2: Full range proof verification — rebuild segment tree and verify KZG proofs.
	proofHashMap := make(map[string]*RangeProof, len(rangeProofs))
	for _, rp := range rangeProofs {
		key := fmt.Sprintf("%d:%d", rp.Layer, rp.Idx)
		proofHashMap[key] = rp
	}

	reqCommits := findCommitmentsCoveringRange(int(startingVersion), int(endingVersion))

	lxRequiredBatchIdxs := make(map[uint64][]uint64)
	for i := uint64(1); i <= tree.MaxLayer; i++ {
		lxRequiredBatchIdxs[i] = make([]uint64, 0)
	}
	for _, reqCommit := range reqCommits {
		lxRequiredBatchIdxs[uint64(reqCommit.layer)] = append(lxRequiredBatchIdxs[uint64(reqCommit.layer)], uint64(reqCommit.idx))
	}

	requiredTreeBatchesMap := RebuildSegmentTreeForVerify(account, lxRequiredBatchIdxs, startingVersion, endingVersion, balanceInfos, proofHashMap, reqCommits, precomputedData)

	slices.SortFunc(reqCommits, func(a, b RangeCommitment) int {
		if a.layer != b.layer {
			return a.layer - b.layer
		}
		return a.idx - b.idx
	})
	isVerified := make(map[string]bool, len(rangeProofs))

	for i := len(reqCommits) - 1; i >= 0; i-- {
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

		Commitment := rangeProof.Commitment

		domain := fft.NewDomain(uint64(len(precomputedData.V) - 1))
		Z := polynomial.VanishingPolynomial(nodesToInterpolate, &domain.Generator)
		ZCommit, _ := kzg.CommitG2(Z, precomputedData.SRS.G2Powers)

		key := fmt.Sprintf("%d:%d", reqCommit.layer, reqCommit.idx)
		I := make(polynomial.Polynomial, int(domain.Cardinality))
		for _, nodeIdx := range nodesToInterpolate {
			I[nodeIdx] = polynomial.HashToFieldElement(requiredTreeBatchesMap[key][nodeIdx])
		}
		domain.FFTInverse(I, fft.DIF)
		fft.BitReverse(I)

		ICommit, err := gnark_kzg.Commit(I, precomputedData.SRS.Inner.Pk)
		if err != nil {
			return fmt.Errorf("commit I polynomial for layer %d idx %d: %w", reqCommit.layer, reqCommit.idx, err)
		}

		QCommit := rangeProof.Proof

		_, err = PairingCheck(Commitment, QCommit, ICommit, ZCommit, precomputedData.SRS)
		if err != nil {
			return fmt.Errorf("pairing check failed for layer %d idx %d: %w", reqCommit.layer, reqCommit.idx, err)
		}

		for _, depCommitIdx := range reqCommit.dependentCommitments {
			depCommitKey := fmt.Sprintf("%d:%d", reqCommit.layer-1, depCommitIdx)
			isVerified[depCommitKey] = true
		}
	}

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
