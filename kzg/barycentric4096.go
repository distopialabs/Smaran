package kzg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/polynomial"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
)

const (
	// domainSize      = 4096
	// domainSize = 16
	DataDir = "polynomial/data"
	// weightsFileName = "bary_weights_4096.bin"
	// vanishFileName  = "vanishing_poly_4096.bin"
	weightsFileNamePlaceholder       = "bary_weights_%d.bin"
	weightCommitsFileNamePlaceholder = "bary_weights_commits_%d.bin"
	vanishFileNamePlaceholder        = "vanishing_poly_%d.bin"
	fieldBytes                       = 32 // size of a serialized fr.Element
	digestBytes                      = 48 // size of a serialized fr.Element
)

// Computes the vanishing polynomial and the barycentric weights
// and stores them as raw binary files (32 bytes per field element).
func PrecomputeBarycentricData(domainSize int, wPath string, vPath string) error {
	fmt.Println("[barycentric] precomputing vanishing polynomial and weights …")

	// Build vanishing polynomial V(x) incrementally.
	V := make(polynomial.Polynomial, 1)
	V[0].SetOne()

	for i := range domainSize {
		// multiply V by (X - i)
		deg := len(V)
		tmpV := make(polynomial.Polynomial, deg+1)

		// multiply by x; shift coefficients right by 1 position
		// tmpV[k+1] = V[k]
		copy(tmpV[1:], V)

		// negI = -i
		var negI fr.Element
		negI.SetInt64(int64(-i))

		// tmpV[k] += -i * V[k]
		for k := range deg {
			var t fr.Element
			t.Mul(&V[k], &negI)
			tmpV[k].Add(&tmpV[k], &t)
		}
		V = tmpV
	}

	// Compute V' (derivative), needed for weights.
	Vprime := make(polynomial.Polynomial, len(V)-1)
	for i := 1; i < len(V); i++ {
		var iFr fr.Element
		iFr.SetInt64(int64(i))
		Vprime[i-1].Mul(&V[i], &iFr)
	}

	// Compute weights.
	weights := make([]fr.Element, domainSize)
	var xi fr.Element
	for i := range domainSize {
		xi.SetInt64(int64(i))
		weights[i] = Vprime.Eval(&xi)
	}
	weights = fr.BatchInvert(weights)

	// Write to files.
	if err := dumpFieldSlice(wPath, weights); err != nil {
		return err
	}
	if err := dumpFieldSlice(vPath, V); err != nil {
		return err
	}
	fmt.Println("[barycentric] precomputation done, data saved to", wPath, vPath)
	return nil
}

func PrecomputeBarycentricCommits(domainSize int, wcPath string, srs *MultiSRS) error {

	fmt.Println("[barycentric] precomputing vanishing polynomial and weights …")

	// Build vanishing polynomial V(x) incrementally.
	V := make(polynomial.Polynomial, 1)
	V[0].SetOne()

	for i := range domainSize {
		// multiply V by (X - i)
		deg := len(V)
		tmpV := make(polynomial.Polynomial, deg+1)

		// multiply by x; shift coefficients right by 1 position
		// tmpV[k+1] = V[k]
		copy(tmpV[1:], V)

		// negI = -i
		var negI fr.Element
		negI.SetInt64(int64(-i))

		// tmpV[k] += -i * V[k]
		for k := range deg {
			var t fr.Element
			t.Mul(&V[k], &negI)
			tmpV[k].Add(&tmpV[k], &t)
		}
		V = tmpV
	}

	// Compute V' (derivative), needed for weights.
	Vprime := make(polynomial.Polynomial, len(V)-1)
	for i := 1; i < len(V); i++ {
		var iFr fr.Element
		iFr.SetInt64(int64(i))
		Vprime[i-1].Mul(&V[i], &iFr)
	}

	// Compute weights.
	weights := make([]fr.Element, domainSize)
	var xi fr.Element
	for i := range domainSize {
		xi.SetInt64(int64(i))
		weights[i] = Vprime.Eval(&xi)
	}
	weights = fr.BatchInvert(weights)

	weightCommits := make([]gnark_kzg.Digest, domainSize)

	quot := make(polynomial.Polynomial, domainSize)
	for i := range domainSize {
		SyntheticDivideInt(quot, V, i)
		for k := range domainSize {
			quot[k].Mul(&quot[k], &weights[i])
		}
		weightCommits[i], _ = gnark_kzg.Commit(quot, srs.Inner.Pk)

	}

	// Write to files.
	if err := dumpDigestSlice(wcPath, weightCommits); err != nil {
		return err
	}
	fmt.Println("[barycentric] precomputation done, data saved to", wcPath)
	return nil
}

func LoadBarycentricData(domainSize int, srs *MultiSRS) (V polynomial.Polynomial, weights []fr.Element, weightCommits []gnark_kzg.Digest) {
	weightsFileName := fmt.Sprintf(weightsFileNamePlaceholder, domainSize)
	weightCommitsFileName := fmt.Sprintf(weightCommitsFileNamePlaceholder, domainSize)
	vanishFileName := fmt.Sprintf(vanishFileNamePlaceholder, domainSize)

	wPath := filepath.Join(DataDir, weightsFileName)
	wcPath := filepath.Join(DataDir, weightCommitsFileName)
	vPath := filepath.Join(DataDir, vanishFileName)
	// Skip if files already exist.

	_, wErr := os.Stat(wPath)
	_, wcErr := os.Stat(wcPath)
	_, vErr := os.Stat(vPath)

	if wErr != nil || vErr != nil || wcErr != nil {
		if err := PrecomputeBarycentricData(domainSize, wPath, vPath); err != nil {
			panic(err)
		}
		if err := PrecomputeBarycentricCommits(domainSize, wcPath, srs); err != nil {
			panic(err)
		}
	}

	start := time.Now()
	weights = readFieldSlice(wPath, domainSize)
	weightCommits = readDigestSlice(wcPath, domainSize)
	V = readFieldSlice(vPath, domainSize+1)

	elapsed := time.Since(start)
	fmt.Println("Barycentric data loading time:", elapsed)
	return
}

func Interpolate4096(yValues []fr.Element, currentBlockNumber int, cachedPolynomial polynomial.Polynomial, V polynomial.Polynomial, weights []fr.Element, domainSize int) polynomial.Polynomial {
	if len(yValues) != domainSize {
		panic(fmt.Sprint("Interpolate4096 expects exactly", domainSize, "y-values"))
	}

	// V, weights := loadBarycentricData(dataDir)

	// result polynomial (degree 4095)
	res := make(polynomial.Polynomial, domainSize)
	copy(res, cachedPolynomial)

	bn := 2047 + currentBlockNumber
	indexToProcess := []int{bn}
	for bn > 0 {
		if bn&1 == 0 {
			bn = (bn - 2) / 2
		} else {
			bn = (bn - 1) / 2
		}
		if yValues[bn].IsZero() {
			break
		}

		indexToProcess = append(indexToProcess, bn)
	}
	// temp polynomial for the quotient V/(X - i)
	quot := make(polynomial.Polynomial, domainSize)

	var scale fr.Element

	for _, index := range indexToProcess {

		// fmt.Println("Processing index", index)
		SyntheticDivideInt(quot, V, index)
		scale.Mul(&yValues[index], &weights[index])
		// res += scale * quotient

		for k := range domainSize {
			var t fr.Element
			t.Mul(&quot[k], &scale)
			res[k].Add(&res[k], &t)
		}
	}

	// for i := range domainSize {
	// 	if yValues[i].IsZero() {
	// 		continue
	// 	}

	// 	// calculate V/(x-i)
	// 	syntheticDivideInt(quot, V, i)

	// 	// scale = y_i * w_i
	// 	scale.Mul(&yValues[i], &weights[i])

	// 	// res += scale * quotient
	// 	for k := range domainSize {
	// 		var t fr.Element
	// 		t.Mul(&quot[k], &scale)
	// 		res[k].Add(&res[k], &t)
	// 	}
	// }
	return res
}

// syntheticDivideInt computes quotient = P / (X - a) where a is small int.
// P degree N, quotient length N.  P is unchanged.
func SyntheticDivideInt(quot, P polynomial.Polynomial, a int) {
	deg := len(P) - 1
	if len(quot) != deg {
		panic("quot slice has wrong length")
	}
	var aElem fr.Element
	aElem.SetInt64(int64(a))

	// Highest degree coefficient
	quot[deg-1] = P[deg]
	for i := deg - 2; i >= 0; i-- {
		// quot[i] = P[i+1] + quot[i+1]*a
		var tmp fr.Element
		tmp.Mul(&quot[i+1], &aElem)
		quot[i].Add(&P[i+1], &tmp)
	}
}

func dumpFieldSlice(path string, s []fr.Element) error {
	// create directory if it doesn't exist
	os.MkdirAll(filepath.Dir(path), 0755)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := range s {
		b := s[i].Bytes()
		if len(b) != fieldBytes {
			return errors.New("unexpected element length")
		}
		if _, err := f.Write(b[:]); err != nil {
			return err
		}
	}
	return nil
}
func readFieldSlice(path string, count int) []fr.Element {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	expected := count * fieldBytes
	if len(data) != expected {
		panic(fmt.Sprintf("corrupted file %s", path))
	}
	out := make([]fr.Element, count)
	for i := 0; i < count; i++ {
		out[i].SetBytes(data[i*fieldBytes : (i+1)*fieldBytes])
	}
	return out
}
func dumpDigestSlice(path string, s []gnark_kzg.Digest) error {
	// create directory if it doesn't exist
	os.MkdirAll(filepath.Dir(path), 0755)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := range s {
		b := s[i].Bytes()
		if len(b) != digestBytes {
			return errors.New("unexpected element length")
		}
		if _, err := f.Write(b[:]); err != nil {
			return err
		}
	}
	return nil
}

func readDigestSlice(path string, count int) []gnark_kzg.Digest {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	expected := count * digestBytes
	if len(data) != expected {
		panic(fmt.Sprintf("corrupted file %s", path))
	}
	out := make([]gnark_kzg.Digest, count)
	for i := 0; i < count; i++ {
		out[i].SetBytes(data[i*digestBytes : (i+1)*digestBytes])
	}
	return out
}
