package main

import (
	"fmt"
	"log"
	"math/big"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
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

func main() {
	fmt.Println("Starting Samurai...\n")

	start := time.Now()

	V, weights := polynomial.LoadBarycentricData(polynomial.DataDir)
	srs, err := kzg.SetupSRS(4096)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}
	fmt.Println("Time taken to setup SRS", time.Since(start))

	start = time.Now()
	// segmentTree := generateSegmentTreeAndCommitments(2050, V, weights, srs)
	// _ = segmentTree
	fmt.Println("Time taken to generate segment tree and commitments", time.Since(start))

	// dump the storage
	start = time.Now()
	// segmentTree.DumpStorage()
	fmt.Println("Time taken to dump storage", time.Since(start))

	// storage := segmentTree.Storage
	start = time.Now()
	storage := segmenttree.LoadStorage()
	fmt.Println("Time taken to load storage", time.Since(start))

	queryStartBlock := 20
	queryEndBlock := 2051

	start = time.Now()
	rangeProofs := proof.GetRangeProofs(queryStartBlock, queryEndBlock, storage, V, weights, srs)
	fmt.Println("Time taken to get range proofs", time.Since(start))

	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, V, weights, srs)
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
