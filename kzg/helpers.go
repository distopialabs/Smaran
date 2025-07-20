package kzg

import (
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/nepal80m/samurai/polynomial"
)

// vanishingPolynomial returns Z(X) = ∏(X - xs[i])
func vanishingPolynomial(xs []fr.Element) polynomial.Polynomial {
	z := []fr.Element{{}}
	z[0].SetOne()
	for _, x := range xs {
		newZ := make([]fr.Element, len(z)+1)
		for i := range z {
			newZ[i+1].Add(&newZ[i+1], &z[i])
			var tmp fr.Element
			tmp.Mul(&z[i], &x).Neg(&tmp)
			newZ[i].Add(&newZ[i], &tmp)
		}
		z = newZ
	}
	return z
}

// interpolate constructs R(X) such that R(xs[i]) = ys[i]
func interpolate(xs, ys []fr.Element) []fr.Element {
	n := len(xs)
	res := make([]fr.Element, n)
	for i := 0; i < n; i++ {
		basis := []fr.Element{{}}
		basis[0].SetOne()
		var denom fr.Element
		denom.SetOne()
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			newB := make([]fr.Element, len(basis)+1)
			for k := range basis {
				newB[k+1].Add(&newB[k+1], &basis[k])
				var tmp fr.Element
				tmp.Mul(&basis[k], &xs[j]).Neg(&tmp)
				newB[k].Add(&newB[k], &tmp)
			}
			basis = newB
			var diff fr.Element
			diff.Sub(&xs[i], &xs[j])
			denom.Mul(&denom, &diff)
		}
		var invDen, scale fr.Element
		invDen.Inverse(&denom)
		scale.Mul(&ys[i], &invDen)
		for k := range basis {
			var tmp fr.Element
			tmp.Mul(&basis[k], &scale)
			res[k].Add(&res[k], &tmp)
		}
	}
	return res
}

// subtractPolys computes A - B
func subtractPolys(a, b []fr.Element) []fr.Element {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make([]fr.Element, n)
	for i := 0; i < n; i++ {
		var ai, bi fr.Element
		if i < len(a) {
			ai = a[i]
		}
		if i < len(b) {
			bi = b[i]
		}
		out[i].Sub(&ai, &bi)
	}
	return out
}

// polyDiv divides num by denom, returns quotient (exact division assumed)
func polyDiv(num, denom []fr.Element) []fr.Element {
	d := len(denom) - 1
	m := len(num) - 1
	rem := make([]fr.Element, len(num))
	copy(rem, num)
	q := make([]fr.Element, m-d+1)
	var invLead fr.Element
	invLead.Inverse(&denom[d])
	for i := m; i >= d; i-- {
		var coeff fr.Element
		coeff.Mul(&rem[i], &invLead)
		q[i-d] = coeff
		for j := 0; j <= d; j++ {
			var tmp fr.Element
			tmp.Mul(&coeff, &denom[j])
			rem[i-d+j].Sub(&rem[i-d+j], &tmp)
		}
	}
	return q
}
