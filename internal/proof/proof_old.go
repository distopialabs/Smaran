package proof

import (
	"fmt"
	"log"
	"sync"
	"time"

	// Added safe import
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/tree"
)

// GetNewProofRange generates range proofs for a given account and version range.
func OldGetProofRange(account common.Address, startingVersion, endingVersion uint64, precomputedData *config.PrecomputedData, db *db.SamuraiStore) ([]*RangeProof, []*tree.HistoricalBalance) {
	reqCommits := findCommitmentsCoveringRange(int(startingVersion), int(endingVersion))

	lxRequiredBatchIdxs := make(map[uint64][]uint64)
	for i := uint64(1); i <= tree.MaxLayer; i++ {
		lxRequiredBatchIdxs[i] = make([]uint64, 0)
	}
	// fmt.Println("Required Commits:")
	for _, reqCommit := range reqCommits {
		lxRequiredBatchIdxs[uint64(reqCommit.layer)] = append(lxRequiredBatchIdxs[uint64(reqCommit.layer)], uint64(reqCommit.idx))
		// fmt.Printf("layer: %d, idx: %d\n", reqCommit.layer, reqCommit.idx)
	}
	start := time.Now()
	requiredTreeBatchesMap, requiredHBInfos := OldRebuildSegmentTreeForProof(account, lxRequiredBatchIdxs, startingVersion, endingVersion, db, precomputedData)
	log.Printf("Time taken to rebuild segment tree: %dms", time.Since(start).Milliseconds())

	allRangeProofs := make([]*RangeProof, len(reqCommits))
	var wg sync.WaitGroup

	for i, reqCommit := range reqCommits {
		wg.Add(1)
		go func(i int, reqCommit RangeCommitment) {
			defer wg.Done()

			layer := reqCommit.layer
			idx := reqCommit.idx

			nodesToInterpolate := findNodesToInterpolate(reqCommit, true)

			treeKey := fmt.Sprintf("%d:%d", layer, idx)
			batchTree := requiredTreeBatchesMap[treeKey]

			xs1 := make([]int, len(batchTree))
			ys1 := make([]fr.Element, len(batchTree))
			for i, v := range batchTree {
				xs1[i] = i
				ys1[i] = polynomial.HashToFieldElement(v)
			}
			P := polynomial.Interpolate(xs1, ys1, precomputedData.V, precomputedData.Weights)

			storedCommitment := tree.GetBatchCommitment(account, uint64(layer), uint64(idx), db.StateDB)

			Z := polynomial.VanishingPolynomial(nodesToInterpolate)

			xs := make([]fr.Element, len(nodesToInterpolate))
			ys := make([]fr.Element, len(nodesToInterpolate))
			for i, v := range nodesToInterpolate {
				xs[i] = fr.NewElement(uint64(v))
				ys[i] = polynomial.HashToFieldElement(batchTree[v])
			}

			I := kzg.Interpolate(xs, ys)

			diff := kzg.SubtractPolys(P, I)
			Q := kzg.PolyDiv(diff, Z)
			QCommit, err := gnark_kzg.Commit(Q, precomputedData.SRS.Inner.Pk)
			if err != nil {
				panic(err)
			}

			rangeProof := &RangeProof{
				Idx:                  idx,
				Layer:                layer,
				Commitment:           storedCommitment,
				Proof:                QCommit,
				BlockRange:           reqCommit.BlockRange,
				DependentCommitments: reqCommit.dependentCommitments,
			}

			allRangeProofs[i] = rangeProof
		}(i, reqCommit)
	}
	wg.Wait()
	return allRangeProofs, requiredHBInfos
}
