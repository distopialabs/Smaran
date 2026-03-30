package hash

import (
	"crypto/rand"
	"math/big"
	"testing"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/ethereum/go-ethereum/crypto"
)

func BenchmarkBLS12381Pairing(b *testing.B) {
	_, _, g1Aff, g2Aff := bls.Generators()

	P := []bls.G1Affine{g1Aff}
	Q := []bls.G2Affine{g2Aff}

	b.Run("Step1_MillerLoop", func(b *testing.B) {
		for b.Loop() {
			bls.MillerLoop(P, Q)
		}
	})

	mlResult, _ := bls.MillerLoop(P, Q)

	b.Run("Step2_FinalExponentiation", func(b *testing.B) {
		for b.Loop() {
			bls.FinalExponentiation(&mlResult)
		}
	})

	b.Run("Step3_Pair", func(b *testing.B) {
		for b.Loop() {
			bls.Pair(P, Q)
		}
	})

	b.Run("Step4_PairingCheck_1Pair", func(b *testing.B) {
		for b.Loop() {
			bls.PairingCheck(P, Q)
		}
	})

	P2 := []bls.G1Affine{g1Aff, g1Aff}
	Q2 := []bls.G2Affine{g2Aff, g2Aff}

	b.Run("Step5_PairingCheck_2Pairs", func(b *testing.B) {
		for b.Loop() {
			bls.PairingCheck(P2, Q2)
		}
	})
}

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
