// Package bench provides benchmarks comparing KZG (BLS12-381),
// IPA (Banderwagon/Pedersen via go-ipa), and Pedersen commitments
// on gnark-crypto's bls12-381/bandersnatch curve.
package bench

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/bandersnatch"
	bls_fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"

	verkle "github.com/ethereum/go-verkle"
)

// ---------------------------------------------------------------------------
// KZG helpers (BLS12-381 G1 multi-scalar multiplication)
// ---------------------------------------------------------------------------

func setupSRS(maxDegree int) (*gnark_kzg.SRS, []bls.G2Affine, error) {
	alphaBigHash := common.HexToHash("0x4259ec8d926b1afd827977927c1b0ba239fb3032e26ed1fdcbfb47ac193947c0")
	var alphaBig big.Int
	alphaBig.SetBytes(alphaBigHash.Bytes())

	srs, err := gnark_kzg.NewSRS(uint64(maxDegree+1), &alphaBig)
	if err != nil {
		return nil, nil, err
	}

	_, _, _, gen2 := bls.Generators()
	g2Powers := make([]bls.G2Affine, maxDegree+1)
	g2Powers[0] = gen2
	for i := 1; i <= maxDegree; i++ {
		g2Powers[i].ScalarMultiplication(&g2Powers[i-1], &alphaBig)
	}

	return srs, g2Powers, nil
}

func randomBLSPoly(n int) []bls_fr.Element {
	poly := make([]bls_fr.Element, n)
	for i := range poly {
		poly[i].SetRandom()
	}
	return poly
}

func randomVerklePoly(n int) []verkle.Fr {
	poly := make([]verkle.Fr, n)
	for i := range poly {
		poly[i].SetRandom()
	}
	return poly
}

func commitG2(coeffs []bls_fr.Element, g2Powers []bls.G2Affine) (bls.G2Affine, error) {
	var res bls.G2Affine
	if _, err := res.MultiExp(g2Powers[:len(coeffs)], coeffs, ecc.MultiExpConfig{}); err != nil {
		return bls.G2Affine{}, err
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// Bandersnatch helpers (gnark-crypto twisted Edwards on BLS12-381/Fr)
// ---------------------------------------------------------------------------

// setupBandersnatchBases generates n random base points on the bandersnatch
// curve by multiplying the generator by successive powers of a random scalar.
// This mimics a transparent CRS for Pedersen commitments.
func setupBandersnatchBases(n int) []bandersnatch.PointExtended {
	params := bandersnatch.GetEdwardsCurve()
	bases := make([]bandersnatch.PointExtended, n)

	var s bls_fr.Element
	s.SetRandom()
	var sBig big.Int
	s.BigInt(&sBig)

	var cur bandersnatch.PointExtended
	cur.FromAffine(&params.Base)
	for i := 0; i < n; i++ {
		bases[i].Set(&cur)
		cur.ScalarMultiplication(&cur, &sBig)
	}
	return bases
}

// bandersnatchMSM computes Σ scalars[i]·bases[i] using gnark-crypto's
// GLV-accelerated scalar multiplication and extended-coordinate additions.
func bandersnatchMSM(bases []bandersnatch.PointExtended, scalars []bls_fr.Element) bandersnatch.PointExtended {
	var acc, tmp bandersnatch.PointExtended
	// Identity on twisted Edwards: (0, 1, 1, 0)
	acc.X.SetZero()
	acc.Y.SetOne()
	acc.Z.SetOne()
	acc.T.SetZero()

	var sBig big.Int
	for i := range scalars {
		scalars[i].BigInt(&sBig)
		tmp.ScalarMultiplication(&bases[i], &sBig)
		acc.Add(&acc, &tmp)
	}
	return acc
}

// ---------------------------------------------------------------------------
// Benchmarks — KZG (BLS12-381)
// ---------------------------------------------------------------------------

func BenchmarkKZGCommitG1(b *testing.B) {
	for _, size := range []int{256, 4096} {
		srs, _, err := setupSRS(size)
		if err != nil {
			b.Fatal(err)
		}
		poly := randomBLSPoly(size)

		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := gnark_kzg.Commit(poly, srs.Pk); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkKZGCommitG2(b *testing.B) {
	for _, size := range []int{256, 4096} {
		_, g2Powers, err := setupSRS(size)
		if err != nil {
			b.Fatal(err)
		}
		poly := randomBLSPoly(size)

		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := commitG2(poly, g2Powers); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks — IPA (Banderwagon / Pedersen via go-verkle)
// ---------------------------------------------------------------------------

func BenchmarkIPACommit(b *testing.B) {
	cfg := verkle.GetConfig()

	for _, size := range []int{256} {
		poly := randomVerklePoly(size)

		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = cfg.CommitToPoly(poly[:], 0)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks — Pedersen (gnark-crypto bls12-381/bandersnatch)
// ---------------------------------------------------------------------------

func BenchmarkBandersnatchCommit(b *testing.B) {
	for _, size := range []int{256} {
		bases := setupBandersnatchBases(size)
		scalars := randomBLSPoly(size)

		b.Run(fmt.Sprintf("n=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = bandersnatchMSM(bases, scalars)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Head-to-head at n=256 (the Verkle tree node width)
// ---------------------------------------------------------------------------

func BenchmarkCommit256_KZG_G1(b *testing.B) {
	const n = 256
	srs, _, err := setupSRS(n)
	if err != nil {
		b.Fatal(err)
	}
	poly := randomBLSPoly(n)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := gnark_kzg.Commit(poly, srs.Pk); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCommit256_KZG_G2(b *testing.B) {
	const n = 256
	_, g2Powers, err := setupSRS(n)
	if err != nil {
		b.Fatal(err)
	}
	poly := randomBLSPoly(n)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := commitG2(poly, g2Powers); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCommit256_IPA(b *testing.B) {
	const n = 256
	cfg := verkle.GetConfig()
	poly := randomVerklePoly(n)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = cfg.CommitToPoly(poly[:], 0)
	}
}

func BenchmarkCommit256_Bandersnatch(b *testing.B) {
	const n = 256
	bases := setupBandersnatchBases(n)
	scalars := randomBLSPoly(n)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bandersnatchMSM(bases, scalars)
	}
}

// ---------------------------------------------------------------------------
// Incremental update: existing commitment + one new (scalar * base) point.
// This mirrors UpdateLXTree's hot path:
//   inc.ScalarMultiplication(&weightCommit, scalar)
//   commitment.Add(&commitment, &inc)
// ---------------------------------------------------------------------------

func BenchmarkIncUpdate_KZG_G1(b *testing.B) {
	srs, _, err := setupSRS(256)
	if err != nil {
		b.Fatal(err)
	}
	base := srs.Pk.G1[42]
	var acc bls.G1Affine
	acc.Set(&srs.Pk.G1[0])

	var scalar bls_fr.Element
	scalar.SetRandom()
	var sBig big.Int
	scalar.BigInt(&sBig)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var inc bls.G1Affine
		inc.ScalarMultiplication(&base, &sBig)
		acc.Add(&acc, &inc)
	}
}

func BenchmarkIncUpdate_Bandersnatch(b *testing.B) {
	bases := setupBandersnatchBases(2)
	acc := bases[0]
	base := bases[1]

	var scalar bls_fr.Element
	scalar.SetRandom()
	var sBig big.Int
	scalar.BigInt(&sBig)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var inc bandersnatch.PointExtended
		inc.ScalarMultiplication(&base, &sBig)
		acc.Add(&acc, &inc)
	}
}

func BenchmarkIncUpdate_Banderwagon(b *testing.B) {
	cfg := verkle.GetConfig()

	// Derive two distinct base points from the IPA config.
	var polyBase [256]verkle.Fr
	polyBase[0].SetOne()
	base := cfg.CommitToPoly(polyBase[:], 0)

	var polyAcc [256]verkle.Fr
	polyAcc[1].SetOne()
	accPoint := cfg.CommitToPoly(polyAcc[:], 0)
	acc := *accPoint

	var scalar verkle.Fr
	scalar.SetRandom()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var inc verkle.Point
		inc.ScalarMul(base, &scalar)
		acc.Add(&acc, &inc)
	}
}
