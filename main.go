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
	"github.com/nepal80m/samurai/segmenttree"
)

func verifyPolynomial(p polynomial.Polynomial, balanceStore []*big.Int) {
	cVInt := big.Int{}
	for i := range 2048 {
		expectedValue := balanceStore[i]
		var x fr.Element
		x.SetUint64(uint64(2047 + i))
		computedValue := p.Eval(&x)
		fmt.Println(expectedValue)
		fmt.Println(computedValue.BigInt(&cVInt))
		fmt.Println()
		if polynomial.HashToFieldElement(common.BigToHash(expectedValue)) != computedValue {
			log.Fatalf("Polynomial verification failed at index %d", i)
		}
	}
}

func main() {
	fmt.Println("Starting Samurai...")
	// cachedPolynomial := make(polynomial.Polynomial, 4096)
	// var cachedCommitment gnark_kzg.Digest
	V, weights := polynomial.LoadBarycentricData(polynomial.DataDir)

	srs, err := kzg.SetupSRS(4096)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}

	segmentTree := segmenttree.NewLayeredSegmentTree(V, weights, srs)

	totalStart := time.Now()
	for blockNumber := range 1365 {
		// fmt.Println("Processing block", blockNumber, "...")
		balance := big.NewInt(1000000000000000000)
		segmentTree.Update(blockNumber, balance)
		// FROM HERE -------------------------
		// incPoly := polynomial.GenerateIncrementalPolynomial(updatedIndices, V, weights, segmentTree.Layer1Tree)

		// cachedPolynomial.Add(cachedPolynomial, incPoly)

		// incCommitment, err := gnark_kzg.Commit(incPoly, srs.Inner.Pk)
		// incCommitment := kzg.CommitG1(incPoly, srs.Inner.Pk.G1)
		// if err != nil {
		// 	log.Fatalf("failed to commit: %v", err)
		// }
		// cachedCommitment.Add(&cachedCommitment, &incCommitment)

		// cachedCommitmentBytes := cachedCommitment.Bytes()
		// cachedCommitmentHash := common.BytesToHash(cachedCommitmentBytes[:])
		// _ = cachedCommitmentHash
		// TO HERE -------------------------

		// segmentTreePolynomial, err := polynomial.NewFromSegmentTree(segmentTree, i, cachedPolynomial, V, weights)
		// if err != nil {
		// 	log.Fatalf("Failed to create polynomial from segment tree: %v", err)
		// }
		// cachedPolynomial = segmentTreePolynomial
		// _ = segmentTreePolynomial

		// // Build an SRS large enough for the polynomial degree.

		// commitment, err := gnark_kzg.Commit(cachedPolynomial, srs.Inner.Pk)
		// if err != nil {
		// 	log.Fatalf("failed to commit: %v", err)
		// }
		// commitmentBytes := commitment.Bytes()
		// commitmentHash := common.BytesToHash(commitmentBytes[:])
		// fmt.Println(commitmentHash)
		// fmt.Println(cachedCommitmentHash)
		// fmt.Println(commitmentHash == cachedCommitmentHash)
		// fmt.Println()
		// if commitmentHash != cachedCommitmentHash {
		// 	log.Fatalf("commitment hash mismatch")
		// }

		// _ = commitmentHash

		// // Select a handful of evaluation points to open.
		// pointInts := []int{0, 100, 2050}
		// var xs []fr.Element
		// for _, v := range pointInts {
		// 	var x fr.Element
		// 	x.SetInt64(int64(v))
		// 	xs = append(xs, x)
		// }

		// // Convert polynomial coefficients to fr.Element slice.

		// mp, err := kzg.ProveMultiPoints(segmentTreePolynomial, xs, srs)
		// if err != nil {
		// 	log.Fatalf("failed to create multiproof: %v", err)
		// }
		// fmt.Println("Multiproof created for", len(xs), "evaluation points")

		// if err := mp.Verify(srs); err != nil {
		// 	log.Fatalf("multiproof verification failed: %v", err)
		// }
		// fmt.Println("Multiproof verification succeeded ✔️")

		// // === KZG multiproof section removed to simplify and resolve compilation issues ===
	}
	// verify the polynomial
	// verifyPolynomial(cachedPolynomial, balanceStore)
	elapsed := time.Since(totalStart)
	fmt.Println("Total time:", elapsed)

	// data structure of storing commitment of many accounts per block

}
