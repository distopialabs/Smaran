package hash

import (
	"crypto/rand"
	"math/big"
	"testing"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/ethereum/go-ethereum/crypto"
)

func BenchmarkKeccakToG1AffineRoundtrip(b *testing.B) {
	input := make([]byte, 64)
	rand.Read(input)

	_, _, g1Generator, _ := bls.Generators()

	b.Run("Step1_Keccak256", func(b *testing.B) {
		for b.Loop() {
			crypto.Keccak256Hash(input)
		}
	})

	hash := crypto.Keccak256Hash(input)

	b.Run("Step2_HashToFr", func(b *testing.B) {
		var fe fr.Element
		for b.Loop() {
			fe.SetBytes(hash[:])
		}
	})

	var fe fr.Element
	fe.SetBytes(hash[:])
	scalar := fe.BigInt(new(big.Int))

	b.Run("Step3_FrToG1Affine", func(b *testing.B) {
		var point bls.G1Affine
		for b.Loop() {
			point.ScalarMultiplication(&g1Generator, scalar)
		}
	})

	var point bls.G1Affine
	point.ScalarMultiplication(&g1Generator, scalar)

	b.Run("Step4_G1AffineToKeccak", func(b *testing.B) {
		for b.Loop() {
			pointBytes := point.Marshal()
			crypto.Keccak256Hash(pointBytes)
		}
	})

	b.Run("Total", func(b *testing.B) {
		for b.Loop() {
			h := crypto.Keccak256Hash(input)
			var f fr.Element
			f.SetBytes(h[:])
			var p bls.G1Affine
			p.ScalarMultiplication(&g1Generator, f.BigInt(new(big.Int)))
			crypto.Keccak256Hash(p.Marshal())
		}
	})
}
