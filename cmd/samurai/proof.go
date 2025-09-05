package main

import (
	"fmt"
	"time"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/math/polynomial"
	"github.com/nepal80m/samurai/internal/proof"
)

func generateProofs(addr common.Address, queryStartBlock int, queryEndBlock int, V polynomial.Polynomial, weights []fr.Element, srs *kzg.MultiSRS, config *config.Config) {
	// 0x0000000000000000000000000000000000000027
	start := time.Now()
	fmt.Println("Generating range proofs for account", addr.Hex())
	rangeProofs, balances := proof.GetRangeProofs(addr, queryStartBlock, queryEndBlock, V, weights, srs, config.StartingBlockNumber)
	_ = rangeProofs
	_ = balances
	fmt.Println("Time taken to generate range proofs", time.Since(start))
	start = time.Now()
	proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, balances, V, weights, srs)
	fmt.Println("Time taken to verify range proofs", time.Since(start))

	// dump proof and balances to file
	proof.DumpProofsAndBalances(rangeProofs, balances)

}
