package proof

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
	"github.com/nepal80m/samurai/internal/segmenttree"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

type BlockRange struct {
	Start, End int
}

type RangeCommitment struct {
	idx                  int
	layer                int
	BlockRange           *BlockRange
	dependentCommitments []int
}

type RangeProof struct {
	idx                  int
	layer                int
	Commitment           gnark_kzg.Digest
	Proof                bls.G1Affine
	BlockRange           *BlockRange
	dependentCommitments []int
}

func BinarySearchVersionByBlockNumber(blockNumber uint64, searchStart uint64, searchEnd uint64, account common.Address, db segmenttree.DB) (uint64, error) {
	L := searchStart
	R := searchEnd
	for L <= R {
		m := (L + R) / 2
		hbInfo := segmenttree.GetHistoricalBalance(account, m, db)
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
func BlockRangeToVersionRange(account common.Address, startingBlock uint64, endingBlock uint64, config *config.Config, db segmenttree.DB) (uint64, uint64) {

	cbInfo, err := segmenttree.GetCurrentBalanceInfo(account, db)
	if err != nil {
		fmt.Printf("Error getting current balance info for account %s: %v\n", account.Hex(), err)
		panic(err)
	}
	// for ending block
	var endingVersion uint64
	if endingBlock >= cbInfo.StartBlock {
		endingVersion = cbInfo.Version
	} else {
		endingVersion, err = BinarySearchVersionByBlockNumber(endingBlock, 0, cbInfo.Version-1, account, db)
		if err != nil {
			panic(err)
		}
	}

	// for starting block
	var startingVersion uint64
	if endingVersion == cbInfo.Version && startingBlock >= cbInfo.StartBlock {
		startingVersion = cbInfo.Version
	} else {
		startingVersion, err = BinarySearchVersionByBlockNumber(startingBlock, 0, endingVersion, account, db)
		if err != nil {
			panic(err)
		}
	}
	return startingVersion, endingVersion

}

func GetNewProofRange(account common.Address, startingVersion, endingVersion uint64, precomputedData *config.PrecomputedData, blockOffset uint64, db segmenttree.DB) ([]*RangeProof, []*segmenttree.HistoricalBalance) {
	// TODO: find the commits required to prove the range
	reqCommits := findCommitmentsCoveringRange(int(startingVersion), int(endingVersion))

	lxRequiredBatchIdxs := make(map[uint64][]uint64)
	for i := uint64(1); i <= segmenttree.MaxLayer; i++ {
		lxRequiredBatchIdxs[i] = make([]uint64, 0)
	}
	fmt.Println("Required Commits:")
	for _, reqCommit := range reqCommits {
		lxRequiredBatchIdxs[uint64(reqCommit.layer)] = append(lxRequiredBatchIdxs[uint64(reqCommit.layer)], uint64(reqCommit.idx))
		fmt.Printf("layer: %d, idx: %d\n", reqCommit.layer, reqCommit.idx)
	}
	start := time.Now()
	requiredTreeBatchesMap, requiredHBInfos := RebuildSegmentTreeForProof(account, lxRequiredBatchIdxs, startingVersion, endingVersion, db, precomputedData)
	fmt.Println("Time taken to rebuild segment tree", time.Since(start))

	// ------------------------------------------------------------
	allRangeProofs := make([]*RangeProof, 0)
	for _, reqCommit := range reqCommits {
		layer := reqCommit.layer
		idx := reqCommit.idx

		nodesToInterpolate := findNodesToInterpolate(reqCommit, true)

		fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
		if reqCommit.BlockRange == nil {
			fmt.Printf("Commitment is not covering any range.\n")
		} else {
			fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		}
		fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
		fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)

		treeKey := fmt.Sprintf("%d:%d", layer, idx)
		tree := requiredTreeBatchesMap[treeKey]

		xs1 := make([]int, len(tree))
		ys1 := make([]fr.Element, len(tree))
		for i, v := range tree {
			xs1[i] = i
			ys1[i] = polynomial.HashToFieldElement(v)
		}
		start = time.Now()
		P := polynomial.Interpolate(xs1, ys1, precomputedData.V, precomputedData.Weights)
		fmt.Println("Time taken to interpolate polynomial", time.Since(start))

		start = time.Now()
		storedCommitment := segmenttree.GetLxBatchCommitment(account, uint64(layer), uint64(idx), db)
		fmt.Println("Time taken to get stored commitment", time.Since(start))

		// start = time.Now()
		// computedCommitment, err := gnark_kzg.Commit(P, precomputedData.SRS.Inner.Pk)
		// if err != nil {
		// 	panic(err)
		// }
		// fmt.Println("Time taken to commit polynomial", time.Since(start))
		// fmt.Printf("computedCommitment: %v\n", computedCommitment)
		// fmt.Printf("storedCommitment: %v\n", storedCommitment)
		// _ = computedCommitment

		// if computedCommitment != storedCommitment {
		// 	panic("commitment calculated from polynomial does not match with the stored commitment")
		// }

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		// ZCommit, _ := kzg.CommitG2(Z, precomputedData.SRS.G2Powers)

		xs := make([]fr.Element, len(nodesToInterpolate))
		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, v := range nodesToInterpolate {
			xs[i] = fr.NewElement(uint64(v))
			ys[i] = polynomial.HashToFieldElement(tree[v])

		}

		I := kzg.Interpolate(xs, ys)
		// ICommit, err := gnark_kzg.Commit(I, precomputedData.SRS.Inner.Pk)
		// if err != nil {
		// 	panic(err)
		// }
		diff := kzg.SubtractPolys(P, I)
		Q := kzg.PolyDiv(diff, Z)
		QCommit, err := gnark_kzg.Commit(Q, precomputedData.SRS.Inner.Pk)
		if err != nil {
			panic(err)
		}
		// qCommitBytes := QCommit.Bytes()
		// qCommitHash := common.BytesToHash(qCommitBytes[:])
		// fmt.Printf("qCommitmentHash: %s\n", qCommitHash)

		// ok, err := PairingCheck(storedCommitment, QCommit, ICommit, ZCommit, precomputedData.SRS)
		// if err != nil {
		// 	panic(err)
		// }
		// if !ok {
		// 	panic("pairing check failed.")
		// } else {
		// 	fmt.Println("Pairing check passed.✅")
		// }

		rangeProof := &RangeProof{
			idx:                  idx,
			layer:                layer,
			Commitment:           storedCommitment,
			Proof:                QCommit,
			BlockRange:           reqCommit.BlockRange,
			dependentCommitments: reqCommit.dependentCommitments,
		}

		allRangeProofs = append(allRangeProofs, rangeProof)

		// for i, v := range nodesToInterpolate {
		// 	fmt.Printf("ys[%d]: %v\n", i, tree[v])
		// }
		// if QCommit == (bls.G1Affine{}) {
		// 	fmt.Printf("I: %v\n", I)
		// 	fmt.Printf("Z: %v\n", Z)
		// 	panic("Proof is zero")
		// }

	}
	return allRangeProofs, requiredHBInfos
}

func GetRangeProofs(account common.Address, startingBlock, endingBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS, blockOffset uint64) ([]*RangeProof, []*big.Int) {

	// TODO: find the commits required to prove the range

	// TODO: find relevant segment tree arrays (and fill them with commitment values)
	// TODO: generate polynomial from the filled segment tree arrays
	// TODO: generate proof for the polynomial
	start := time.Now()
	balances, err := batchSingleUserBalances(account, uint64(startingBlock)+blockOffset, uint64(endingBlock)+blockOffset)
	if err != nil {
		panic(err)
	}
	balanceFetchTime := time.Since(start)
	fmt.Printf("Time taken to get balances: %v\n", balanceFetchTime)

	allRangeProofs := make([]*RangeProof, 0)

	reqCommits := findCommitmentsCoveringRange(startingBlock, endingBlock)
	// for _, reqCommit := range reqCommits {

	// 	nodesToInterpolate := findNodesToInterpolate(reqCommit, true)

	// 	fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
	// 	if reqCommit.BlockRange == nil {
	// 		fmt.Printf("Commitment is not covering any range.\n")
	// 	} else {
	// 		fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
	// 	}
	// 	fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
	// 	fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)
	// }
	for _, reqCommit := range reqCommits {
		layer := reqCommit.layer
		idx := reqCommit.idx

		nodesToInterpolate := findNodesToInterpolate(reqCommit, true)

		fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
		if reqCommit.BlockRange == nil {
			fmt.Printf("Commitment is not covering any range.\n")
		} else {
			fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		}
		fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
		fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)

		// storedCommitment := lxCommitments[layer][idx]
		tree, err := segmenttree.ReadTreeSegment(segmenttree.StoragePath, account, layer, idx)
		if err != nil {
			panic(err)
		}

		// P := lxPolynomials[layer][idx]
		xs1 := make([]int, len(tree))
		ys1 := make([]fr.Element, len(tree))
		for i, v := range tree {
			xs1[i] = i
			ys1[i] = polynomial.HashToFieldElement(v)
		}
		P := polynomial.Interpolate(xs1, ys1, V, weights)

		computedCommitment, err := gnark_kzg.Commit(P, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}
		// _ = computedCommitment

		// if computedCommitment != storedCommitment {
		// 	panic("commitment calculated from polynomial does not match with the stored commitment")
		// }

		// computedCommitmentHash := segmenttree.CommitmentToHash(computedCommitment)
		// commitmentBytes := commitment.Bytes()
		// commitmentHash := common.BytesToHash(commitmentBytes[:])
		// fmt.Printf("pCommitHash: %v\n", computedCommitmentHash)

		// if layer < segmenttree.MaxLayer {
		// 	modDepCommitIdx := 2*segmenttree.L2BatchSize - 1 + (idx % segmenttree.L2BatchSize)
		// 	storedCommitmentHash := lxTrees[layer+1][idx/segmenttree.L2BatchSize][modDepCommitIdx]
		// 	fmt.Printf("storedCommitmentHash: %v\n", storedCommitmentHash)
		// 	if storedCommitmentHash != computedCommitmentHash {
		// 		panic("commitment hash does not match")
		// 	} else {
		// 		fmt.Println("commitment hash matches with the commitment stored in upper layer storage")
		// 	}
		// }

		Z := polynomial.VanishingPolynomial(nodesToInterpolate)
		// ZCommit, _ := kzg.CommitG2(Z, srs.G2Powers)

		// zCommitBytes := ZCommit.Bytes()
		// zCommitHash := common.BytesToHash(zCommitBytes[:])
		// fmt.Printf("zCommitHash: %s\n", zCommitHash)

		xs := make([]fr.Element, len(nodesToInterpolate))
		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, v := range nodesToInterpolate {
			xs[i] = fr.NewElement(uint64(v))

			// fmt.Printf("xs[%d]: %v\n", i, v)
			// fmt.Printf("ysHash[%d]: %v\n", i, tree[v])
			// fmt.Printf("ysHashEval[%d]: %v\n", i, polynomial.FieldElementToHash(P.Eval(&xs[i])))

			ys[i] = polynomial.HashToFieldElement(tree[v])

		}

		// I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
		I := kzg.Interpolate(xs, ys)
		// ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
		// if err != nil {
		// 	panic(err)
		// }
		// iCommitBytes := ICommit.Bytes()
		// iCommitHash := common.BytesToHash(iCommitBytes[:])
		// fmt.Printf("iCommitmentHash: %s\n", iCommitHash)

		diff := kzg.SubtractPolys(P, I)
		Q := kzg.PolyDiv(diff, Z)
		QCommit, err := gnark_kzg.Commit(Q, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}
		// qCommitBytes := QCommit.Bytes()
		// qCommitHash := common.BytesToHash(qCommitBytes[:])
		// fmt.Printf("qCommitmentHash: %s\n", qCommitHash)

		// ok, err := PairingCheck(computedCommitment, QCommit, ICommit, ZCommit, srs)
		// if err != nil {
		// 	panic(err)
		// }
		// if !ok {
		// 	panic("pairing check failed.")
		// } else {
		// 	fmt.Println("Pairing check passed.✅")
		// }

		rangeProof := &RangeProof{
			idx:                  idx,
			layer:                layer,
			Commitment:           computedCommitment,
			Proof:                QCommit,
			BlockRange:           reqCommit.BlockRange,
			dependentCommitments: reqCommit.dependentCommitments,
		}

		allRangeProofs = append(allRangeProofs, rangeProof)

		// for i, v := range nodesToInterpolate {
		// 	fmt.Printf("ys[%d]: %v\n", i, tree[v])
		// }
		// if QCommit == (bls.G1Affine{}) {
		// 	fmt.Printf("I: %v\n", I)
		// 	fmt.Printf("Z: %v\n", Z)
		// 	panic("Proof is zero")
		// }

	}
	fmt.Println("Number of range proofs:", len(allRangeProofs))
	fmt.Printf("Time taken to get balances: %v\n", balanceFetchTime)

	return allRangeProofs, balances

}

func findCommitmentsCoveringRange(sb, eb int) []RangeCommitment {
	rcCommitments := findRangeCoveringCommitments(sb, eb, 1)
	reqCommitments := addDepencencyCommitments(rcCommitments)

	return reqCommitments

}

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

		if dCommit.layer == segmenttree.MaxLayer {
			continue
		}

		reqCommitIdx := dCommit.idx / segmenttree.L2BatchSize
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

// Given a starting block, ending block, and layer, get all the commitments required to prove it.
// layer is passed for recursion. check getRequiredCommitments.py for more details.
// returns a list of commitments required to prove the range.
// each commitment is a list of [idx, sb, eb, layer]
// idx is index of commitment in the storage.
// layer is the layer of the commitment.
// this commitment should cover the range of block of index sb to eb.
func findRangeCoveringCommitments(sb, eb int, layer int) []RangeCommitment {
	reqCommitments := make([]RangeCommitment, 0)

	l0BatchSize := segmenttree.L1BatchSize * Pow(segmenttree.L2BatchSize, layer-1)

	hasLeftFragment := sb%(l0BatchSize) != 0
	hasRightFragment := eb%(l0BatchSize) != l0BatchSize-1

	leftCommitIndex := sb / (l0BatchSize)
	rightCommitIndex := eb / (l0BatchSize)

	if leftCommitIndex == rightCommitIndex && (hasLeftFragment || hasRightFragment) {
		// commitments = append(commitments, []int{leftCommitIndex, sb, eb, layer})
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
		// commitments = append(commitments, []int{leftCommitIndex, leftFragmentStart, leftFragmentEnd, layer})

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
		// commitments = append(commitments, []int{rightCommitIndex, rightFragmentStart, rightFragmentEnd, layer})

		reqCommitments = append(reqCommitments, RangeCommitment{
			idx:        rightCommitIndex,
			layer:      layer,
			BlockRange: &BlockRange{Start: rightFragmentStart, End: rightFragmentEnd},
		})
		eb = rightFragmentStart - 1
	}
	if sb < eb && layer < segmenttree.MaxLayer {
		upperLayerCommitments := findRangeCoveringCommitments(sb, eb, layer+1)
		reqCommitments = append(reqCommitments, upperLayerCommitments...)
	}

	return reqCommitments

}

func findNodesToInterpolate(commitment RangeCommitment, includeDependentCommitments bool) []int {

	layer := commitment.layer
	idx := commitment.idx

	nodesToInterpolate := make([]int, 0)
	if includeDependentCommitments {
		for _, depCommitIdx := range commitment.dependentCommitments {
			if layer <= 1 {
				panic("layer1 cannot have dependents")
			}
			modDepCommitIdx := 2*segmenttree.L2BatchSize - 1 + (depCommitIdx % segmenttree.L2BatchSize)
			nodesToInterpolate = append(nodesToInterpolate, modDepCommitIdx)
		}
	}

	if commitment.BlockRange == nil {
		return nodesToInterpolate
	}

	sb := commitment.BlockRange.Start
	eb := commitment.BlockRange.End

	l0BatchSize := segmenttree.L1BatchSize * Pow(segmenttree.L2BatchSize, layer-1)
	l0BatchStartIdx := idx * l0BatchSize
	l0BatchEndIdx := l0BatchStartIdx + l0BatchSize - 1

	lXm1BatchSize := l0BatchSize // this should match the size of the lXtree
	lXm1BatchStartIdx := l0BatchStartIdx
	lXm1BatchEndIdx := l0BatchEndIdx
	lXm1SB := sb
	lXm1EB := eb

	if layer > 1 {
		denom := segmenttree.L1BatchSize * Pow(segmenttree.L2BatchSize, layer-2)
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

// N is number of leaves in the segment tree, L is starting index, R is ending index of the range.
// returns a list of nodes that covers the range.
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

// func DumpProofsAndBalances(proofs []*RangeProof, balances []*big.Int) {
// 	// Create the storage/proofs directory if it doesn't exist
// 	err := os.MkdirAll(fmt.Sprintf("storage/proofs/%d", len(balances)), 0755)
// 	if err != nil {
// 		panic(err)
// 	}

// 	proofFile, err := os.Create(fmt.Sprintf("storage/proofs/%d/proofs.json", len(balances)))
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer proofFile.Close()

// 	balanceFile, err := os.Create(fmt.Sprintf("storage/proofs/%d/balances.json", len(balances)))
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer balanceFile.Close()

// 	json.NewEncoder(proofFile).Encode(proofs)
// 	json.NewEncoder(balanceFile).Encode(balances)

//		fmt.Println("Proofs and balances dumped to storage/proofs/proofs.json and storage/proofs/balances.json", len(balances))
//	}
func DumpNewProofsAndBalances(proofs []*RangeProof, balances []*segmenttree.HistoricalBalance) {
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

	fmt.Println("Proofs and balances dumped to storage/proofs/proofs.json and storage/proofs/historical_balances.json", len(balances))
}
