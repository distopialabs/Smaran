package proof

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/logging"
	"github.com/nepal80m/samurai/internal/tree"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

var log = logging.GetLogger("proof")

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
	Idx                  int
	Layer                int
	BlockRange           *BlockRange
	DependentCommitments []int
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
func BinarySearchVersionByBlockNumber(blockNumber uint64, searchStart uint64, searchEnd uint64, account common.Address, db *db.SamuraiDB) (uint64, error) {
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
func BlockRangeToVersionRange(account common.Address, startingBlock uint64, endingBlock uint64, config *config.Config, db *db.SamuraiDB) (uint64, uint64, error) {

	cbInfo, err := tree.GetCurrentBalanceInfo(account, db.StateDB)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrAccountNotFound, err)
	}

	if cbInfo.Version == 0 {
		return 0, 0, fmt.Errorf("no historical balances available to prove for this account")
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
		return 0, 0, fmt.Errorf("starting block %d is beyond the latest historical version", startingBlock)
	} else {
		startingVersion, err = BinarySearchVersionByBlockNumber(startingBlock, 0, endingVersion, account, db)
		if err != nil {
			return 0, 0, fmt.Errorf("%w: starting block %d: %v", ErrVersionNotFound, startingBlock, err)
		}
	}

	return startingVersion, endingVersion, nil
}

// GetNewProofRange generates range proofs for a given account and version range.
func GetNewProofRange(account common.Address, startingVersion, endingVersion uint64, precomputedData *config.PrecomputedData, db *db.SamuraiDB) ([]*RangeProof, []*tree.HistoricalBalance) {
	reqCommits := FindCommitmentsCoveringRange(int(startingVersion), int(endingVersion))

	lxRequiredBatchIdxs := make(map[uint64][]uint64)
	for i := uint64(1); i <= tree.MaxLayer; i++ {
		lxRequiredBatchIdxs[i] = make([]uint64, 0)
	}
	for _, reqCommit := range reqCommits {
		lxRequiredBatchIdxs[uint64(reqCommit.Layer)] = append(lxRequiredBatchIdxs[uint64(reqCommit.Layer)], uint64(reqCommit.Idx))
	}
	start := time.Now()
	requiredTreeBatchesMap, requiredHBInfos := RebuildSegmentTreeForProof(account, lxRequiredBatchIdxs, startingVersion, endingVersion, db, precomputedData)
	log.Infof("Time taken to rebuild segment tree: %dms", time.Since(start).Milliseconds())

	allRangeProofs := make([]*RangeProof, len(reqCommits))
	var wg sync.WaitGroup

	needVerify := false

	for i, reqCommit := range reqCommits {
		wg.Add(1)
		go func(i int, reqCommit RangeCommitment) {
			defer wg.Done()

			layer := reqCommit.Layer
			idx := reqCommit.Idx

			nodesToInterpolate := FindNodesToInterpolate(reqCommit, true)

		log.Debugf("layer: %d, idx: %d", reqCommit.Layer, reqCommit.Idx)
		if reqCommit.BlockRange == nil {
			log.Debugf("Commitment is not covering any range.")
		} else {
			log.Debugf("sb: %d, eb: %d", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		}
		log.Debugf("DependentCommitments: %v", reqCommit.DependentCommitments)
		log.Debugf("nodesToInterpolate: %v", nodesToInterpolate)

			treeKey := fmt.Sprintf("%d:%d", layer, idx)
			batchTree := requiredTreeBatchesMap[treeKey]

			xs1 := make([]int, len(batchTree))
			ys1 := make([]fr.Element, len(batchTree))
			for i, v := range batchTree {
				xs1[i] = i
				ys1[i] = polynomial.HashToFieldElement(v)
				// fmt.Printf("xs1[%d] = %d, ys1[%d] = %s\n", i, i, i, ys1[i].String())
			}
			P := polynomial.Interpolate(xs1, ys1, precomputedData.V, precomputedData.Weights)

			storedCommitment := tree.GetBatchCommitment(account, uint64(layer), uint64(idx), db.StateDB)

			Z := polynomial.VanishingPolynomial(nodesToInterpolate)

			xs := make([]fr.Element, len(nodesToInterpolate))
			ys := make([]fr.Element, len(nodesToInterpolate))
			for i, v := range nodesToInterpolate {
				xs[i] = fr.NewElement(uint64(v))
				ys[i] = polynomial.HashToFieldElement(batchTree[v])
				// fmt.Printf("xs[%d] = %d, ys[%d] = %s\n", i, v, i, ys[i].String())
			}

			I := kzg.Interpolate(xs, ys)

			diff := kzg.SubtractPolys(P, I)
			Q := kzg.PolyDiv(diff, Z)
			QCommit, err := gnark_kzg.Commit(Q, precomputedData.SRS.Inner.Pk)
			if err != nil {
				panic(err)
			}
			if needVerify {
				// Always compare stored vs rebuilt commitment
				pCommit, _ := gnark_kzg.Commit(P, precomputedData.SRS.Inner.Pk)
				storedBytes := storedCommitment.Bytes()
				rebuiltBytes := pCommit.Bytes()
				commitMatch := storedCommitment.Equal(&pCommit)
				log.Debugf("DEBUG layer %d idx %d: stored=%x rebuilt=%x match=%v", layer, idx, storedBytes[:8], rebuiltBytes[:8], commitMatch)

				ZCommit, _ := kzg.CommitG2(Z, precomputedData.SRS.G2Powers)
				ICommit, err := gnark_kzg.Commit(I, precomputedData.SRS.Inner.Pk)
				if err != nil {
					panic(err)
				}

				ok, err := PairingCheck(storedCommitment, QCommit, ICommit, ZCommit, precomputedData.SRS)
				if err != nil {
				}
				if !ok {
					// Recompute commitment from P
					pCommit, _ := gnark_kzg.Commit(P, precomputedData.SRS.Inner.Pk)
				log.Debugf("Stored Commitment: %x", storedCommitment.Bytes())
				log.Debugf("Rebuilt Tree Commitment: %x", pCommit.Bytes())

				// Load the stored tree from DB for comparison
				storedLXTree := tree.GetCurrentLXBatchTree(account, db.TreeDB)
				storedBatchTree := storedLXTree[layer-1]

				log.Debugf("--- Comparing Rebuilt vs Stored BatchTree ---")
				diffCount := 0
				for i := 0; i < len(batchTree); i++ {
					rebuilt := batchTree[i]
					stored := storedBatchTree[i]
					if rebuilt != stored {
						diffCount++
						if diffCount <= 50 { // limit output
							log.Debugf("DIFF Idx %d: rebuilt=%s stored=%s", i, rebuilt.Hex(), stored.Hex())
						}
					}
				}
				log.Debugf("Total differing indices: %d", diffCount)

				// Also dump non-zero entries of rebuilt tree
				nonZeroCount := 0
				for i, h := range batchTree {
					if h != (common.Hash{}) {
						nonZeroCount++
						if nonZeroCount <= 30 {
							log.Debugf("Rebuilt NonZero Idx %d: %s", i, h.Hex())
						}
					}
				}
				log.Debugf("Total non-zero entries in rebuilt tree: %d", nonZeroCount)

				// Dump non-zero entries of stored tree
				storedNonZeroCount := 0
				for i, h := range storedBatchTree {
					if h != (common.Hash{}) {
						storedNonZeroCount++
						if storedNonZeroCount <= 30 {
							log.Debugf("Stored NonZero Idx %d: %s", i, h.Hex())
						}
					}
				}
				log.Debugf("Total non-zero entries in stored tree: %d", storedNonZeroCount)
				log.Debugf("--- End Comparison ---")

					panic("pairing check failed.")
				} else {
					log.Infof("Pairing check passed.")
				}
			}

			rangeProof := &RangeProof{
				Idx:                  idx,
				Layer:                layer,
				Commitment:           storedCommitment,
				Proof:                QCommit,
				BlockRange:           reqCommit.BlockRange,
				DependentCommitments: reqCommit.DependentCommitments,
			}

			allRangeProofs[i] = rangeProof
		}(i, reqCommit)
	}
	wg.Wait()
	return allRangeProofs, requiredHBInfos
}

// FindCommitmentsCoveringRange finds all commitments needed to cover the given block range.
func FindCommitmentsCoveringRange(sb, eb int) []RangeCommitment {
	rcCommitments := findRangeCoveringCommitments(sb, eb, 1)
	reqCommitments := addDepencencyCommitments(rcCommitments)

	return reqCommitments

}

// addDepencencyCommitments adds dependency commitments for upper layers.
func addDepencencyCommitments(dependentCommitments []RangeCommitment) []RangeCommitment {

	commitHashMap := make(map[string]*RangeCommitment)

	depQueue := Queue[RangeCommitment]{}
	for _, dCommit := range dependentCommitments {
		key := fmt.Sprintf("%d:%d", dCommit.Layer, dCommit.Idx)
		commitHashMap[key] = &dCommit
		depQueue.Enqueue(dCommit)
	}

	for !depQueue.IsEmpty() {
		dCommit, err := depQueue.Dequeue()
		if err != nil {
			panic(err)
		}

		if dCommit.Layer == tree.MaxLayer {
			continue
		}

		reqCommitIdx := dCommit.Idx / tree.L2BatchSize
		reqCommitLayer := dCommit.Layer + 1

		reqCommitKey := fmt.Sprintf("%d:%d", reqCommitLayer, reqCommitIdx)

		_, exists := commitHashMap[reqCommitKey]
		if exists {
			commitHashMap[reqCommitKey].DependentCommitments = append(commitHashMap[reqCommitKey].DependentCommitments, dCommit.Idx)
		} else {
			newCommit := RangeCommitment{
				Idx:                  reqCommitIdx,
				Layer:                reqCommitLayer,
				DependentCommitments: []int{dCommit.Idx},
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
			Idx:        leftCommitIndex,
			Layer:      layer,
			BlockRange: &BlockRange{Start: sb, End: eb},
		})

		return reqCommitments
	}

	if hasLeftFragment {
		leftFragmentStart := sb
		leftFragmentEnd := (leftCommitIndex+1)*l0BatchSize - 1

		reqCommitments = append(reqCommitments, RangeCommitment{
			Idx:        leftCommitIndex,
			Layer:      layer,
			BlockRange: &BlockRange{Start: leftFragmentStart, End: leftFragmentEnd},
		})

		sb = leftFragmentEnd + 1
	}
	if hasRightFragment {

		rightFragmentStart := rightCommitIndex * l0BatchSize
		rightFragmentEnd := eb

		reqCommitments = append(reqCommitments, RangeCommitment{
			Idx:        rightCommitIndex,
			Layer:      layer,
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

// FindNodesToInterpolate finds the tree nodes that need to be interpolated for a commitment.
func FindNodesToInterpolate(commitment RangeCommitment, includeDependentCommitments bool) []int {

	layer := commitment.Layer
	idx := commitment.Idx

	nodesToInterpolate := make([]int, 0)
	if includeDependentCommitments {
		for _, depCommitIdx := range commitment.DependentCommitments {
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

// DumpNewProofsAndBalances writes proofs and historical balances to JSON files.
func DumpNewProofsAndBalances(proofs []*RangeProof, balances []*tree.HistoricalBalance) {
	// Create the storage/proofs directory if it doesn't exist
	err := os.MkdirAll(fmt.Sprintf("storage/proofs/"), 0755)
	if err != nil {
		panic(err)
	}

	proofFile, err := os.Create(fmt.Sprintf("storage/proofs/proofs.json"))
	if err != nil {
		panic(err)
	}
	defer proofFile.Close()

	balanceFile, err := os.Create(fmt.Sprintf("storage/proofs/historical_balances.json"))
	if err != nil {
		panic(err)
	}
	defer balanceFile.Close()

	json.NewEncoder(proofFile).Encode(proofs)
	json.NewEncoder(balanceFile).Encode(balances)

}
