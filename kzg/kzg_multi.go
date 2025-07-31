package kzg

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

type MultiSRS struct {
	Inner    *kzg.SRS
	G2Powers []bls.G2Affine // [α⁰]G2, [α¹]G2, ...
}

func SetupSRS(maxDegree int) (*MultiSRS, error) {
	var alpha fr.Element
	if _, err := alpha.SetRandom(); err != nil {
		return nil, err
	}

	var alphaBig big.Int
	alpha.BigInt(&alphaBig)

	// TODO: remove 2 lines below this
	alphaBigHash := common.HexToHash("0x4259ec8d926b1afd827977927c1b0ba239fb3032e26ed1fdcbfb47ac193947c0")
	alphaBig.SetBytes(alphaBigHash.Bytes())

	inner, err := kzg.NewSRS(uint64(maxDegree+1), &alphaBig)
	if err != nil {
		return nil, err
	}
	// TODO: skip computing G2 powers
	_, _, _, gen2 := bls.Generators()
	g2Powers := make([]bls.G2Affine, maxDegree+1)
	g2Powers[0] = gen2

	for i := 1; i <= maxDegree; i++ {
		g2Powers[i].ScalarMultiplication(&g2Powers[i-1], &alphaBig)
	}

	return &MultiSRS{Inner: inner, G2Powers: g2Powers}, nil
}

func polynomialEval(poly []fr.Element, point fr.Element) fr.Element {
	var acc fr.Element
	if len(poly) == 0 {
		return acc // zero
	}
	acc.Set(&poly[len(poly)-1])
	for i := len(poly) - 2; i >= 0; i-- {
		acc.Mul(&acc, &point).Add(&acc, &poly[i])
	}
	return acc
}

// func CommitG1(coeffs []fr.Element, g1Powers []bls.G1Affine) bls.G1Affine {
// 	var acc bls.G1Jac
// 	for i, c := range coeffs {
// 		if c.IsZero() {
// 			continue
// 		}
// 		var term bls.G1Affine
// 		var bigc big.Int
// 		c.BigInt(&bigc)
// 		term.ScalarMultiplication(&g1Powers[i], &bigc)

// 		var termJac bls.G1Jac
// 		termJac.FromAffine(&term)
// 		acc.AddAssign(&termJac)
// 	}
// 	var res bls.G1Affine
// 	res.FromJacobian(&acc)
// 	return res
// }

func OldCommitG2(coeffs []fr.Element, g2Powers []bls.G2Affine) bls.G2Affine {
	var acc bls.G2Jac
	for i, c := range coeffs {
		if c.IsZero() {
			continue
		}
		var term bls.G2Affine
		var bigc big.Int
		c.BigInt(&bigc)
		term.ScalarMultiplication(&g2Powers[i], &bigc)

		var termJac bls.G2Jac
		termJac.FromAffine(&term)
		acc.AddAssign(&termJac)
	}
	var res bls.G2Affine
	res.FromJacobian(&acc)
	return res
}

func CommitG2(coeffs []fr.Element, g2Powers []bls.G2Affine) (bls.G2Affine, error) {

	if len(coeffs) == 0 || len(coeffs) > len(g2Powers) {
		return bls.G2Affine{}, fmt.Errorf("invalid coefficients length: %d, expected between 1 and %d", len(coeffs), len(g2Powers))
	}

	var res bls.G2Affine

	config := ecc.MultiExpConfig{}

	if _, err := res.MultiExp(g2Powers[:len(coeffs)], coeffs, config); err != nil {
		return bls.G2Affine{}, err
	}
	return res, nil

	// var acc bls.G2Jac
	// for i, c := range coeffs {
	// 	if c.IsZero() {
	// 		continue
	// 	}
	// 	var term bls.G2Affine
	// 	var bigc big.Int
	// 	c.BigInt(&bigc)
	// 	term.ScalarMultiplication(&g2Powers[i], &bigc)

	// 	var termJac bls.G2Jac
	// 	termJac.FromAffine(&term)
	// 	acc.AddAssign(&termJac)
	// }
	// var res bls.G2Affine
	// res.FromJacobian(&acc)
	// return res
}

func ProveMultiPoints(P []fr.Element, xs []fr.Element, srs *MultiSRS) (*KZGMultiProof, error) {
	if len(xs) == 0 {
		return nil, fmt.Errorf("no evaluation points provided")
	}

	C, err := kzg.Commit(P, srs.Inner.Pk)
	if err != nil {
		return nil, err
	}

	ys := make([]fr.Element, len(xs))
	for i, x := range xs {
		ys[i] = polynomialEval(P, x)
	}

	Z := VanishingPolynomial(xs)
	ZCommit, err := kzg.Commit(Z, srs.Inner.Pk)
	if err != nil {
		return nil, err
	}

	I := Interpolate(xs, ys)
	ICommit, err := kzg.Commit(I, srs.Inner.Pk)
	if err != nil {
		return nil, err
	}

	diff := SubtractPolys(P, I)
	Q := PolyDiv(diff, Z)

	proofG2, _ := CommitG2(Q, srs.G2Powers)

	return &KZGMultiProof{
		Commitment: C,
		Xs:         xs,
		Ys:         ys,
		ZCommit:    ZCommit,
		ICommit:    ICommit,
		ProofG2:    proofG2,
	}, nil
}

type KZGMultiProof struct {
	Commitment bls.G1Affine

	Xs []fr.Element
	Ys []fr.Element

	ZCommit bls.G1Affine
	ICommit bls.G1Affine

	ProofG2 bls.G2Affine
}

func (mp *KZGMultiProof) Verify(srs *MultiSRS) error {

	recomputedZ := VanishingPolynomial(mp.Xs)
	recomputedI := Interpolate(mp.Xs, mp.Ys)

	zc, err := kzg.Commit(recomputedZ, srs.Inner.Pk)
	if err != nil {
		return err
	}
	ic, err := kzg.Commit(recomputedI, srs.Inner.Pk)
	if err != nil {
		return err
	}
	if !zc.Equal(&mp.ZCommit) || !ic.Equal(&mp.ICommit) {
		return fmt.Errorf("provided auxiliary commitments are invalid")
	}

	var lhsG1 bls.G1Affine
	lhsG1.Sub(&mp.Commitment, &mp.ICommit)

	lhsNegZ := mp.ZCommit
	lhsNegZ.Neg(&lhsNegZ)

	P := []bls.G1Affine{lhsG1, lhsNegZ}
	Q := make([]bls.G2Affine, 2)
	Q[0] = srs.G2Powers[0]
	Q[1] = mp.ProofG2

	ok, err := bls.PairingCheck(P, Q)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("pairing check failed: invalid multiproof")
	}
	return nil
}
