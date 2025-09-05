package main

import (
	"fmt"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
	"github.com/nepal80m/samurai/internal/proof"
)

func verifyProofs(queryStartBlock int, queryEndBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) {
	numBlocks := queryEndBlock - queryStartBlock + 1
	start := time.Now()
	proofs, balances, err := proof.ReadProofsAndBalances(numBlocks)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Time taken to read proofs and balances: %v\n", time.Since(start))
	// TODO: FIX REBUILD PARTIAL TREE. issue is in commitment json marsharling and unmarshaling
	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, proofs, balances, V, weights, srs)
	fmt.Println("Time taken to verify range proofs", time.Since(start))

	// start = time.Now()
	// proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
	// fmt.Println("Time taken to verify range proofs", time.Since(start))

}
