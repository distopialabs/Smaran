package polynomial

import (
	"fmt"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/polynomial"
	"github.com/ethereum/go-ethereum/common"
)

type Polynomial = polynomial.Polynomial

// hashToFieldElement converts a common.Hash to a field element
func HashToFieldElement(hash common.Hash) fr.Element {
	// return fr.NewElement(uint64(hash.Big().Uint64()))
	// var element fr.Element
	// element.SetBytes(hash.Bytes())
	// return element
	var e fr.Element
	err := e.SetBytesCanonical(hash[:])
	if err != nil {
		fmt.Println("Error in HashToFr:", err)

		panic(err)
	}
	return e
}

func FieldElementToHash(element fr.Element) common.Hash {
	var hash common.Hash
	elementBytes := element.Bytes()
	hash.SetBytes(elementBytes[:])
	return hash
}

func HashToFr(h common.Hash) fr.Element {
	var e fr.Element
	err := e.SetBytesCanonical(h[:])
	if err != nil {
		panic(err)
	}
	return e
}

// func NewFromSegmentTree(segmentTree segmenttree.SegmentTree, currentBlockNumber int, cachedPolynomial Polynomial, V Polynomial, weights []fr.Element) (Polynomial, error) {

// 	if len(segmentTree) != domainSize {
// 		panic("segment tree must have " + strconv.Itoa(domainSize) + " nodes")
// 	}

// 	yValues := make([]fr.Element, domainSize)
// 	for i := range domainSize {
// 		yValues[i] = HashToFieldElement((segmentTree)[i])
// 	}

// 	interPolynomial := Interpolate4096(yValues, currentBlockNumber, cachedPolynomial, V, weights)

// 	return interPolynomial, nil
// }

func Interpolate(xValues []int, yValues []fr.Element, V Polynomial, weights []fr.Element) Polynomial {
	domainSize := len(V) - 1
	poly := make(Polynomial, domainSize)
	quot := make(Polynomial, domainSize)
	var scale fr.Element

	for i, x := range xValues {
		SyntheticDivideInt(quot, V, x) // quot = V/(x-i)
		// TODO: should it be weights[i] or weights[x]? review this later
		scale.Mul(&yValues[i], &weights[x]) // scale = y_i * w_i
		// scale.Mul(&yValues[i], &weights[i]) // scale = y_i * w_i
		// TODO: check if it can be skipped for zero values
		for k := range domainSize {
			var t fr.Element
			t.Mul(&quot[k], &scale)
			poly[k].Add(&poly[k], &t)
		}
	}

	return poly
}

// VanishingPolynomial returns Z(X) = ∏(X - xs[i])
func VanishingPolynomial(xs []int) Polynomial {
	z := []fr.Element{{}}
	// z := make([]fr.Element, len(xs))
	z[0].SetOne()
	for _, x := range xs {
		newZ := make([]fr.Element, len(z)+1)
		for i := range z {
			newZ[i+1].Add(&newZ[i+1], &z[i])
			var tmp fr.Element
			xFr := fr.NewElement(uint64(x))
			tmp.Mul(&z[i], &xFr).Neg(&tmp)
			newZ[i].Add(&newZ[i], &tmp)
		}
		z = newZ

	}
	return z
}
