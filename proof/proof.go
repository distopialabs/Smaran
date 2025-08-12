package proof

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/big"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"
	"github.com/nepal80m/samurai/segmenttree"

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

func GetRangeProofs(account common.Address, startingBlock, endingBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS, blockOffset uint64) ([]*RangeProof, []*big.Int) {
	// lxCommitments := map[int]map[int]bls.G1Affine{
	// 	1: storage.L1Commitments,
	// 	2: storage.L2Commitments,
	// 	3: storage.L3Commitments,
	// 	4: storage.L4Commitments,
	// }

	// lxTrees := map[int]map[int][]common.Hash{
	// 	1: storage.L1Tree,
	// 	2: storage.L2Tree,
	// 	3: storage.L3Tree,
	// 	4: storage.L4Tree,
	// }

	// lxPolynomials := map[int]map[int]polynomial.Polynomial{
	// 	1: storage.L1Polynomial,
	// 	2: storage.L2Polynomial,
	// 	3: storage.L3Polynomial,
	// 	4: storage.L4Polynomial,
	// }
	// _ = lxPolynomials

	// TODO: find the commits required to prove the range
	// TODO: find relevant segment tree arrays (and fill them with commitment values)
	// TODO: generate polynomial from the filled segment tree arrays
	// TODO: generate proof for the polynomial
	balances, err := batchSingleUserBalances(account, uint64(startingBlock)+blockOffset, uint64(endingBlock)+blockOffset)
	if err != nil {
		panic(err)
	}

	allRangeProofs := make([]*RangeProof, 0)

	reqCommits := findCommitmentsCoveringRange(startingBlock, endingBlock)
	for _, reqCommit := range reqCommits {

		nodesToInterpolate := findNodesToInterpolate(reqCommit, true)

		fmt.Printf("\n\nlayer: %d, idx: %d, \n", reqCommit.layer, reqCommit.idx)
		if reqCommit.BlockRange == nil {
			fmt.Printf("Commitment is not covering any range.\n")
		} else {
			fmt.Printf("sb: %d, eb: %d\n", reqCommit.BlockRange.Start, reqCommit.BlockRange.End)
		}
		fmt.Printf("dependentCommitments: %v\n", reqCommit.dependentCommitments)
		fmt.Printf("nodesToInterpolate: %v\n", nodesToInterpolate)
	}
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
		_ = computedCommitment

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
		ZCommit, _ := kzg.CommitG2(Z, srs.G2Powers)

		zCommitBytes := ZCommit.Bytes()
		zCommitHash := common.BytesToHash(zCommitBytes[:])
		fmt.Printf("zCommitHash: %s\n", zCommitHash)

		xs := make([]fr.Element, len(nodesToInterpolate))
		ys := make([]fr.Element, len(nodesToInterpolate))
		for i, v := range nodesToInterpolate {
			xs[i] = fr.NewElement(uint64(v))

			fmt.Printf("xs[%d]: %v\n", i, v)
			fmt.Printf("ysHash[%d]: %v\n", i, tree[v])
			fmt.Printf("ysHashEval[%d]: %v\n", i, polynomial.FieldElementToHash(P.Eval(&xs[i])))

			// TODO: cannot recover fr from hash. modify this.
			// ys[i] = polynomial.HashToFieldElement(tree[v])
			// ys2 := P.Eval(&xs[i])

			ys[i] = polynomial.HashToFieldElement(tree[v])
			// ys2 := P.Eval(&xs[i])
			// if ys[i] != ys2 {
			// 	// panic("ys[i] != ys2")
			// 	fmt.Println("ys[i] != ys2 ❌ ")

			// } else {
			// 	// fmt.Printf("ys[%d]: %v\n", i, ys[i])
			// 	// fmt.Printf("ys2[%d]: %v\n", i, ys2)
			// 	fmt.Println("ys[i] == ys2 ✅")

			// }
		}

		// I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
		I := kzg.Interpolate(xs, ys)
		ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}
		iCommitBytes := ICommit.Bytes()
		iCommitHash := common.BytesToHash(iCommitBytes[:])
		fmt.Printf("iCommitmentHash: %s\n", iCommitHash)

		diff := kzg.SubtractPolys(P, I)
		Q := kzg.PolyDiv(diff, Z)
		QCommit, err := gnark_kzg.Commit(Q, srs.Inner.Pk)
		if err != nil {
			panic(err)
		}
		qCommitBytes := QCommit.Bytes()
		qCommitHash := common.BytesToHash(qCommitBytes[:])
		fmt.Printf("qCommitmentHash: %s\n", qCommitHash)

		ok, err := PairingCheck(computedCommitment, QCommit, ICommit, ZCommit, srs)
		if err != nil {
			panic(err)
		}
		if !ok {
			panic("pairing check failed.")
		} else {
			fmt.Println("Pairing check passed.✅")
		}

		rangeProof := &RangeProof{
			idx:                  idx,
			layer:                layer,
			Commitment:           computedCommitment,
			Proof:                QCommit,
			BlockRange:           reqCommit.BlockRange,
			dependentCommitments: reqCommit.dependentCommitments,
		}

		allRangeProofs = append(allRangeProofs, rangeProof)

		for i, v := range nodesToInterpolate {
			fmt.Printf("ys[%d]: %v\n", i, tree[v])
		}
		if QCommit == (bls.G1Affine{}) {
			fmt.Printf("I: %v\n", I)
			fmt.Printf("Z: %v\n", Z)
			panic("Proof is zero")
		}

	}

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

func batchSingleUserBalances(addr common.Address, startBlockNum, endBlockNum uint64) ([]*big.Int, error) {
	client, err := rpc.Dial("/mydata/erigon/mainnet/geth.ipc")
	if err != nil {
		log.Fatalf("Failed to connect to Erigon IPC: %v", err)
	}
	defer client.Close()
	elems := make([]rpc.BatchElem, endBlockNum-startBlockNum+1)
	for i := range endBlockNum - startBlockNum + 1 {
		var bal hexutil.Big
		elems[i] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args:   []any{addr, hexutil.Uint64(startBlockNum + uint64(i))},
			Result: &bal,
		}
	}

	if err := client.BatchCallContext(context.Background(), elems); err != nil {
		return nil, err
	}

	balances := make([]*big.Int, len(elems))
	for i, e := range elems {
		balances[i] = e.Result.(*hexutil.Big).ToInt()
	}
	return balances, nil
}
