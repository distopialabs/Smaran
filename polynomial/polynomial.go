package polynomial

import (
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/polynomial"
	"github.com/ethereum/go-ethereum/common"
	// "github.com/nepal80m/samurai/segmenttree"
)

type Polynomial = polynomial.Polynomial

// hashToFieldElement converts a common.Hash to a field element
func HashToFieldElement(hash common.Hash) fr.Element {
	var element fr.Element
	element.SetBytes(hash.Bytes())
	return element
}

func FieldElementToHash(element fr.Element) common.Hash {
	var hash common.Hash
	elementBytes := element.Bytes()
	hash.SetBytes(elementBytes[:])
	return hash
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

	poly := make(Polynomial, 4096)
	quot := make(Polynomial, 4096)
	var scale fr.Element

	for i, x := range xValues {
		SyntheticDivideInt(quot, V, x) // quot = V/(x-i)

		scale.Mul(&yValues[i], &weights[i]) // scale = y_i * w_i
		for k := range 4096 {
			var t fr.Element
			t.Mul(&quot[k], &scale)
			poly[k].Add(&poly[k], &t)
		}
	}

	return poly
}
