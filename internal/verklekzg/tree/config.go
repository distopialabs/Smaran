package tree

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/fft"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/polynomial"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
)

const (
	// DomainSize is the polynomial degree / FFT domain size used for each
	// node's KZG commitment. Matches the branching factor Width = 256.
	DomainSize = Width

	fieldBytes  = 32
	digestBytes = 48
)

// TreeConfig holds the SRS and precomputed data needed for incremental
// KZG commitment operations at every trie node.
type TreeConfig struct {
	SRS           *kzg.MultiSRS
	WeightCommits [DomainSize]gnark_kzg.Digest
	Weights       [DomainSize]fr.Element
	V             polynomial.Polynomial // vanishing polynomial over the domain
	Omega         fr.Element            // primitive 256-th root of unity
}

// NewTreeConfig creates or loads the precomputed data required for
// incremental KZG operations over a 256-point roots-of-unity domain.
// paramsDir is a directory where cached binary files are stored.
func NewTreeConfig(paramsDir string) (*TreeConfig, error) {
	srs, err := kzg.SetupSRS(DomainSize)
	if err != nil {
		return nil, fmt.Errorf("setup SRS: %w", err)
	}

	domain := fft.NewDomain(uint64(DomainSize))

	cfg := &TreeConfig{
		SRS:   srs,
		Omega: domain.Generator,
	}

	weightsFile := filepath.Join(paramsDir, fmt.Sprintf("bary_weights_%d.bin", DomainSize))
	wcFile := filepath.Join(paramsDir, fmt.Sprintf("bary_weights_commits_%d.bin", DomainSize))
	vFile := filepath.Join(paramsDir, fmt.Sprintf("vanishing_poly_%d.bin", DomainSize))

	_, wErr := os.Stat(weightsFile)
	_, wcErr := os.Stat(wcFile)
	_, vErr := os.Stat(vFile)

	if wErr != nil || vErr != nil || wcErr != nil {
		if err := precomputeBarycentricData(DomainSize, weightsFile, vFile); err != nil {
			return nil, fmt.Errorf("precompute barycentric data: %w", err)
		}
		if err := precomputeBarycentricCommits(DomainSize, wcFile, srs); err != nil {
			return nil, fmt.Errorf("precompute barycentric commits: %w", err)
		}
	}

	start := time.Now()
	weights := readFieldSlice(weightsFile, DomainSize)
	wc := readDigestSlice(wcFile, DomainSize)
	V := readFieldSlice(vFile, DomainSize+1)

	copy(cfg.Weights[:], weights)
	copy(cfg.WeightCommits[:], wc)
	cfg.V = V

	fmt.Printf("[verklekzg] barycentric data loaded in %v\n", time.Since(start))
	return cfg, nil
}

// OmegaPow returns omega^i.
func (cfg *TreeConfig) OmegaPow(i int) fr.Element {
	var e fr.Element
	e.Exp(cfg.Omega, new(big.Int).SetInt64(int64(i)))
	return e
}

// ---------------------------------------------------------------------------
// Precomputation (mirrors internal/crypto/kzg/barycentric4096.go for domain 256)
// ---------------------------------------------------------------------------

func precomputeBarycentricData(domainSize int, wPath, vPath string) error {
	domain := fft.NewDomain(uint64(domainSize))
	omega := domain.Generator

	V := make(polynomial.Polynomial, 1)
	V[0].SetOne()

	for i := range domainSize {
		deg := len(V)
		tmpV := make(polynomial.Polynomial, deg+1)
		copy(tmpV[1:], V)

		var omegaI fr.Element
		omegaI.Exp(omega, new(big.Int).SetInt64(int64(i)))
		omegaI.Neg(&omegaI)

		for k := range deg {
			var t fr.Element
			t.Mul(&V[k], &omegaI)
			tmpV[k].Add(&tmpV[k], &t)
		}
		V = tmpV
	}

	Vprime := make(polynomial.Polynomial, len(V)-1)
	for i := 1; i < len(V); i++ {
		var iFr fr.Element
		iFr.SetInt64(int64(i))
		Vprime[i-1].Mul(&V[i], &iFr)
	}

	weights := make([]fr.Element, domainSize)
	var omegaI fr.Element
	for i := range domainSize {
		omegaI.Exp(omega, new(big.Int).SetInt64(int64(i)))
		weights[i] = Vprime.Eval(&omegaI)
	}
	weights = fr.BatchInvert(weights)

	if err := dumpFieldSlice(wPath, weights); err != nil {
		return err
	}
	return dumpFieldSlice(vPath, V)
}

func precomputeBarycentricCommits(domainSize int, wcPath string, srs *kzg.MultiSRS) error {
	domain := fft.NewDomain(uint64(domainSize))
	omega := domain.Generator

	V := make(polynomial.Polynomial, 1)
	V[0].SetOne()
	for i := range domainSize {
		deg := len(V)
		tmpV := make(polynomial.Polynomial, deg+1)
		copy(tmpV[1:], V)
		var omegaI fr.Element
		omegaI.Exp(omega, new(big.Int).SetInt64(int64(i)))
		omegaI.Neg(&omegaI)
		for k := range deg {
			var t fr.Element
			t.Mul(&V[k], &omegaI)
			tmpV[k].Add(&tmpV[k], &t)
		}
		V = tmpV
	}

	Vprime := make(polynomial.Polynomial, len(V)-1)
	for i := 1; i < len(V); i++ {
		var iFr fr.Element
		iFr.SetInt64(int64(i))
		Vprime[i-1].Mul(&V[i], &iFr)
	}
	weights := make([]fr.Element, domainSize)
	var omegaI fr.Element
	for i := range domainSize {
		omegaI.Exp(omega, new(big.Int).SetInt64(int64(i)))
		weights[i] = Vprime.Eval(&omegaI)
	}
	weights = fr.BatchInvert(weights)

	weightCommits := make([]gnark_kzg.Digest, domainSize)
	quot := make(polynomial.Polynomial, domainSize)
	for i := range domainSize {
		omegaI.Exp(omega, new(big.Int).SetInt64(int64(i)))
		kzg.SyntheticDivide(quot, V, &omegaI)
		for k := range domainSize {
			quot[k].Mul(&quot[k], &weights[i])
		}
		weightCommits[i], _ = gnark_kzg.Commit(quot, srs.Inner.Pk)
	}

	return dumpDigestSlice(wcPath, weightCommits)
}

// ---------------------------------------------------------------------------
// Binary I/O helpers
// ---------------------------------------------------------------------------

func dumpFieldSlice(path string, s []fr.Element) error {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for i := range s {
		b := s[i].Bytes()
		if len(b) != fieldBytes {
			return errors.New("unexpected field element length")
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
		panic(fmt.Sprintf("readFieldSlice %s: %v", path, err))
	}
	if len(data) != count*fieldBytes {
		panic(fmt.Sprintf("corrupted file %s: got %d bytes, want %d", path, len(data), count*fieldBytes))
	}
	out := make([]fr.Element, count)
	for i := range count {
		out[i].SetBytes(data[i*fieldBytes : (i+1)*fieldBytes])
	}
	return out
}

func dumpDigestSlice(path string, s []gnark_kzg.Digest) error {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for i := range s {
		b := s[i].Bytes()
		if len(b) != digestBytes {
			return errors.New("unexpected digest length")
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
		panic(fmt.Sprintf("readDigestSlice %s: %v", path, err))
	}
	if len(data) != count*digestBytes {
		panic(fmt.Sprintf("corrupted file %s: got %d bytes, want %d", path, len(data), count*digestBytes))
	}
	out := make([]gnark_kzg.Digest, count)
	for i := range count {
		out[i].SetBytes(data[i*digestBytes : (i+1)*digestBytes])
	}
	return out
}
