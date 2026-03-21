package commands

import (
	"fmt"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/proof"
)

// RunVerify executes the proof verification mode.
func RunVerify(queryStartBlock int, queryEndBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS) {
	numBlocks := queryEndBlock - queryStartBlock + 1

	start := time.Now()
	proofs, balances, err := proof.ReadProofsAndBalances(numBlocks)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Time taken to read proofs and balances: %v\n", time.Since(start))

	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, proofs, balances, V, weights, srs)
	fmt.Println("Time taken to verify range proofs", time.Since(start))
}
