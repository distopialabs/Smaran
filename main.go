package main

import (
	"fmt"
	"log"
	"math/big"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"

	"github.com/nepal80m/samurai/proof"
	"github.com/nepal80m/samurai/segmenttree"
)

// func verifyPolynomial(p polynomial.Polynomial, balanceStore []*big.Int) {
// 	cVInt := big.Int{}
// 	for i := range 2048 {
// 		expectedValue := balanceStore[i]
// 		var x fr.Element
// 		x.SetUint64(uint64(2047 + i))
// 		computedValue := p.Eval(&x)
// 		fmt.Println(expectedValue)
// 		fmt.Println(computedValue.BigInt(&cVInt))
// 		fmt.Println()
// 		if polynomial.HashToFieldElement(common.BigToHash(expectedValue)) != computedValue {
// 			log.Fatalf("Polynomial verification failed at index %d", i)
// 		}
// 	}
// }

func HashToFr(h common.Hash) fr.Element {
	var e fr.Element
	err := e.SetBytesCanonical(h[:])
	if err != nil {
		panic(err)
	}
	return e
}
func main() {

	// V, weights := polynomial.LoadBarycentricData(polynomial.DataDir)
	// srs, err := kzg.SetupSRS(4096)
	// if err != nil {
	// 	panic(err)
	// }

	// poly := polynomial.Interpolate([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, []fr.Element{fr.NewElement(0), fr.NewElement(1), fr.NewElement(2), fr.NewElement(3), fr.NewElement(4), fr.NewElement(5), fr.NewElement(6), fr.NewElement(7), fr.NewElement(8), fr.NewElement(9), fr.NewElement(10), fr.NewElement(11), fr.NewElement(12), fr.NewElement(13), fr.NewElement(14), fr.NewElement(15)}, V, weights)

	// nodesToInterpolate := []int{2, 8, 10}

	// pCommit, err := gnark_kzg.Commit(poly, srs.Inner.Pk)
	// if err != nil {
	// 	panic(err)
	// }

	// Z := polynomial.VanishingPolynomial(nodesToInterpolate)
	// ZCommit := kzg.CommitG2(Z, srs.G2Powers)

	// xs := make([]fr.Element, len(nodesToInterpolate))
	// ys := make([]fr.Element, len(nodesToInterpolate))
	// for i, nodeIdx := range nodesToInterpolate {
	// 	xs[i] = fr.NewElement(uint64(nodeIdx))
	// 	ys[i] = poly.Eval(&xs[i])
	// }
	// I := kzg.Interpolate(xs, ys)
	// // I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
	// // I := bls_polynomial.InterpolateOnRange(ys)
	// ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
	// _ = ICommit
	// if err != nil {
	// 	panic(err)
	// }

	// diff := kzg.SubtractPolys(poly, I)
	// qPoly := kzg.PolyDiv(diff, Z)
	// qCommit, err := gnark_kzg.Commit(qPoly, srs.Inner.Pk)
	// if err != nil {
	// 	panic(err)
	// }
	// ok, err := proof.PairingCheck(pCommit, qCommit, ICommit, ZCommit, srs)
	// if err != nil {
	// 	panic(err)
	// }
	// if !ok {
	// 	panic("pairing check failed.")
	// }

	main2()
}

func main2() {
	fmt.Println("Starting Samurai...\n")

	start := time.Now()

	V, weights := polynomial.LoadBarycentricData(polynomial.DataDir)
	srs, err := kzg.SetupSRS(4096)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}
	fmt.Println("Time taken to setup SRS", time.Since(start))

	// start = time.Now()
	// segmentTree := generateSegmentTreeAndCommitments(2050, V, weights, srs)
	// fmt.Println("Time taken to generate segment tree and commitments", time.Since(start))

	// start = time.Now()
	// segmentTree.DumpStorage()
	// fmt.Println("Time taken to dump storage", time.Since(start))

	// storage := segmentTree.Storage

	start = time.Now()
	storage := segmenttree.LoadStorage()
	fmt.Println("Time taken to load storage", time.Since(start))

	queryStartBlock := 20
	queryEndBlock := 2049

	start = time.Now()
	rangeProofs := proof.GetRangeProofs(queryStartBlock, queryEndBlock, storage, V, weights, srs)
	fmt.Println("Time taken to generate range proofs", time.Since(start))

	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, V, weights, srs, storage)
	fmt.Println("Time taken to verify range proofs", time.Since(start))

	// _ = rangeProofs

}
func generateSegmentTreeAndCommitments(maxBlockNumber int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) *segmenttree.LayeredSegmentTree {

	segmentTree := segmenttree.NewLayeredSegmentTree(V, weights, srs)

	for blockNumber := range maxBlockNumber {
		// fmt.Println("Processing block", blockNumber, "...")
		balance := big.NewInt(1000000000000000000)
		segmentTree.Update(blockNumber, balance)
	}

	return segmentTree
}
