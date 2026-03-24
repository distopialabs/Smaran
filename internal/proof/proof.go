package proof

import (
	"errors"
	"fmt"
	"log"
	"math"
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

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

// BlockRange represents a contiguous range of blocks.
type BlockRange struct {
	Start, End int
}

// Sentinel errors for BlockRangeToVersionRange
var (
	// ErrAccountNotFound is returned when the account does not exist in the database.
	ErrAccountNotFound = errors.New("account not found")
	// ErrBlockRangeOutOfBounds is returned when the query block range ends before the account's first recorded block.
	ErrBlockRangeOutOfBounds = errors.New("block range outside account's recorded history")
	// ErrVersionNotFound is returned when a version cannot be found for the given block number.
	ErrVersionNotFound = errors.New("version not found for block")
)

// RangeCommitment represents a commitment required to prove a block range.
type RangeCommitment struct {
	idx                  int
	layer                int
	BlockRange           *BlockRange
	dependentCommitments []int
}

// RangeProof represents a proof for a range of blocks.
type RangeProof struct {
	Idx                  int
	Layer                int
	Commitment           gnark_kzg.Digest
	Proof                bls.G1Affine
	BlockRange           *BlockRange
	DependentCommitments []int
}

// BinarySearchVersionByBlockNumber finds the version for a given block number using binary search.
func BinarySearchVersionByBlockNumber(blockNumber uint64, searchStart uint64, searchEnd uint64, account common.Address, db *db.SamuraiStore) (uint64, error) {
	L := searchStart
	R := searchEnd
	for L <= R {
		m := (L + R) / 2
		hbInfo := tree.GetHistoricalBalance(account, m, db.HistoryDB)
		if hbInfo.StartBlock <= blockNumber && blockNumber <= hbInfo.EndBlock {
			return m, nil
		} else if blockNumber < hbInfo.StartBlock {
			if m == 0 {
				return 0, errors.New("version not found")
			}
			R = m - 1
		} else {
			L = m + 1
		}

	}
	return 0, errors.New("version not found")
}

// BlockRangeToVersionRange converts a block range to a version range for a given account.
// It handles edge cases:
// - If endingBlock < account's first recorded block: returns error (no data available)
// - If startingBlock < account's first recorded block: clamps to version 0
func BlockRangeToVersionRange(account common.Address, startingBlock uint64, endingBlock uint64, db *db.SamuraiStore) (uint64, uint64, error) {

	cbInfo, err := tree.GetCurrentBalanceInfo(account, db.StateDB)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrAccountNotFound, err)
	}

	if cbInfo.Version == 0 {
		return 0, 0, fmt.Errorf("%w: no historical balances available", ErrAccountNotFound)
	}

	// Get the first recorded version to check bounds
	firstHbInfo := tree.GetHistoricalBalance(account, 0, db.HistoryDB)

	// Case 1: Ending block is before account's first recorded block - no data available
	if endingBlock < firstHbInfo.StartBlock {
		return 0, 0, fmt.Errorf("%w: query ends at block %d, first recorded block is %d", ErrBlockRangeOutOfBounds, endingBlock, firstHbInfo.StartBlock)
	}

	// Determine ending version
	var endingVersion uint64
	if endingBlock >= cbInfo.StartBlock {
		endingVersion = cbInfo.Version - 1
	} else {
		endingVersion, err = BinarySearchVersionByBlockNumber(endingBlock, 0, cbInfo.Version-1, account, db)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: ending block %d: %v", ErrVersionNotFound, endingBlock, err)
		}
	}

	// Determine starting version
	var startingVersion uint64
	// Case 2: Starting block is before account's first recorded block - clamp to version 0
	if startingBlock < firstHbInfo.StartBlock {
		startingVersion = 0
	} else if startingBlock >= cbInfo.StartBlock {
		return 0, 0, fmt.Errorf("%w: starting block %d is at or beyond the latest historical version (block %d)", ErrBlockRangeOutOfBounds, startingBlock, cbInfo.StartBlock)
	} else {
		startingVersion, err = BinarySearchVersionByBlockNumber(startingBlock, 0, endingVersion, account, db)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: starting block %d: %v", ErrVersionNotFound, startingBlock, err)
		}
	}

	return startingVersion, endingVersion, nil
}

// GetNewProofRange generates range proofs for a given account and version range.
func GetNewProofRange(account common.Address, startingVersion, endingVersion uint64, precomputedData *config.PrecomputedData, db *db.SamuraiStore) ([]*RangeProof, []*tree.HistoricalBalance) {
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
	requiredTreeBatchesMap, requiredHBInfos, cachedCommitments := RebuildSegmentTreeForProof(account, lxRequiredBatchIdxs, startingVersion, endingVersion, db, precomputedData)
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

			// fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
			// if reqCommit.BlockRange == nil {
			// 	fmt.Printf("Commitment is not covering any range.\n")
			// } else {
			// 	fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
			// }
			// fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
			// fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)

			treeKey := fmt.Sprintf("%d:%d", layer, idx)
			batchTree := requiredTreeBatchesMap[treeKey]

			xs1 := make([]int, len(batchTree))
			ys1 := make([]fr.Element, len(batchTree))
			var zeroHash common.Hash
			for i, v := range batchTree {
				xs1[i] = i
				if v == zeroHash {
					// Skip HashToFieldElement for zero hashes — result is zero element
					continue
				}
				ys1[i] = polynomial.HashToFieldElement(v)
			}
			P := polynomial.Interpolate(xs1, ys1, precomputedData.V, precomputedData.Weights)

			// Use cached commitment from tree rebuild instead of re-fetching from DB
			commitKey := fmt.Sprintf("%d:%d", layer, idx)
			storedCommitment, ok := cachedCommitments[commitKey]
			if !ok {
				// Fallback to DB fetch if not cached (e.g., L1 commitments)
				storedCommitment = tree.GetBatchCommitment(account, uint64(layer), uint64(idx), db.StateDB)
			}

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

// findCommitmentsCoveringRange finds all commitments needed to cover the given block range.
func findCommitmentsCoveringRange(sb, eb int) []RangeCommitment {
	rcCommitments := findRangeCoveringCommitments(sb, eb, 1)
	reqCommitments := addDepencencyCommitments(rcCommitments)

	return reqCommitments

}

// addDepencencyCommitments adds dependency commitments for upper layers.
func addDepencencyCommitments(dependentCommitments []RangeCommitment) []RangeCommitment {

	commitHashMap := make(map[string]*RangeCommitment)

	depQueue := Queue[RangeCommitment]{}
	for _, dCommit := range dependentCommitments {
		key := fmt.Sprintf("%d:%d", dCommit.layer, dCommit.idx)
		commitHashMap[key] = &dCommit
		depQueue.Enqueue(dCommit)
	}

	for !depQueue.IsEmpty() {
		dCommit, err := depQueue.Dequeue()
		if err != nil {
			panic(err)
		}

		if dCommit.layer == tree.MaxLayer {
			continue
		}

		reqCommitIdx := dCommit.idx / tree.L2BatchSize
		reqCommitLayer := dCommit.layer + 1

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommitLayer, reqCommitIdx)

		_, exists := commitHashMap[reqCommitKey]
		if exists {
			commitHashMap[reqCommitKey].dependentCommitments = append(commitHashMap[reqCommitKey].dependentCommitments, dCommit.idx)
		} else {
			newCommit := RangeCommitment{
				idx:                  reqCommitIdx,
				layer:                reqCommitLayer,
				dependentCommitments: []int{dCommit.idx},
			}
			commitHashMap[reqCommitKey] = &newCommit
			depQueue.Enqueue(newCommit)
		}
	}

	reqCommitments := make([]RangeCommitment, 0)
	for _, reqCommit := range commitHashMap {
		reqCommitments = append(reqCommitments, *reqCommit)
	}
	return reqCommitments
}

// findRangeCoveringCommitments finds all commitments required to cover a block range at a given layer.
// Returns a list of RangeCommitment where each commitment covers a portion of the range.
func findRangeCoveringCommitments(sb, eb int, layer int) []RangeCommitment {
	reqCommitments := make([]RangeCommitment, 0)

	l0BatchSize := tree.L1BatchSize * Pow(tree.L2BatchSize, layer-1)

	hasLeftFragment := sb%(l0BatchSize) != 0
	hasRightFragment := eb%(l0BatchSize) != l0BatchSize-1

	leftCommitIndex := sb / (l0BatchSize)
	rightCommitIndex := eb / (l0BatchSize)

	if leftCommitIndex == rightCommitIndex && (hasLeftFragment || hasRightFragment) {
		reqCommitments = append(reqCommitments, RangeCommitment{
			idx:        leftCommitIndex,
			layer:      layer,
			BlockRange: &BlockRange{Start: sb, End: eb},
		})

		return reqCommitments
	}

	if hasLeftFragment {
		leftFragmentStart := sb
		leftFragmentEnd := (leftCommitIndex+1)*l0BatchSize - 1

		reqCommitments = append(reqCommitments, RangeCommitment{
			idx:        leftCommitIndex,
			layer:      layer,
			BlockRange: &BlockRange{Start: leftFragmentStart, End: leftFragmentEnd},
		})

		sb = leftFragmentEnd + 1
	}
	if hasRightFragment {

		rightFragmentStart := rightCommitIndex * l0BatchSize
		rightFragmentEnd := eb

		reqCommitments = append(reqCommitments, RangeCommitment{
			idx:        rightCommitIndex,
			layer:      layer,
			BlockRange: &BlockRange{Start: rightFragmentStart, End: rightFragmentEnd},
		})
		eb = rightFragmentStart - 1
	}
	if sb < eb && layer < tree.MaxLayer {
		upperLayerCommitments := findRangeCoveringCommitments(sb, eb, layer+1)
		reqCommitments = append(reqCommitments, upperLayerCommitments...)
	}

	return reqCommitments

}

// findNodesToInterpolate finds the tree nodes that need to be interpolated for a commitment.
func findNodesToInterpolate(commitment RangeCommitment, includeDependentCommitments bool) []int {

	layer := commitment.layer
	idx := commitment.idx

	nodesToInterpolate := make([]int, 0)
	if includeDependentCommitments {
		for _, depCommitIdx := range commitment.dependentCommitments {
			if layer <= 1 {
				panic("layer1 cannot have dependents")
			}
			modDepCommitIdx := 2*tree.L2BatchSize - 1 + (depCommitIdx % tree.L2BatchSize)
			nodesToInterpolate = append(nodesToInterpolate, modDepCommitIdx)
		}
	}

	if commitment.BlockRange == nil {
		return nodesToInterpolate
	}

	sb := commitment.BlockRange.Start
	eb := commitment.BlockRange.End

	l0BatchSize := tree.L1BatchSize * Pow(tree.L2BatchSize, layer-1)
	l0BatchStartIdx := idx * l0BatchSize
	l0BatchEndIdx := l0BatchStartIdx + l0BatchSize - 1

	lXm1BatchSize := l0BatchSize // this should match the size of the lXtree
	lXm1BatchStartIdx := l0BatchStartIdx
	lXm1BatchEndIdx := l0BatchEndIdx
	lXm1SB := sb
	lXm1EB := eb

	if layer > 1 {
		denom := tree.L1BatchSize * Pow(tree.L2BatchSize, layer-2)
		lXm1BatchSize /= denom // this should match the size of the lXtree
		lXm1BatchStartIdx /= denom
		lXm1BatchEndIdx /= denom
		lXm1SB /= denom
		lXm1EB /= denom
	}

	coveringNodes := findCoveringNodes(lXm1BatchSize, lXm1SB-lXm1BatchStartIdx, lXm1EB-lXm1BatchStartIdx)

	nodesToInterpolate = append(nodesToInterpolate, coveringNodes...)

	return nodesToInterpolate

}

// findCoveringNodes finds the segment tree nodes that cover a range [L, R] in a tree of N leaves.
func findCoveringNodes(N, L, R int) []int {

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
