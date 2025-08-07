package main

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime/pprof"
	"time"

	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/kzg"
	"github.com/nepal80m/samurai/polynomial"

	"github.com/nepal80m/samurai/segmenttree"
)

// var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")

// func verifyPolynomial(p polynomial.Polynomial, balanceStore []*big.Int) {
// 	cVInt := big.Int{}
// 	for i := range 2048 {
// 		expectedValue := balanceStore[i]
// 		var x fr.Element
// 		x.SetUint64(uint64(2047 + i))
// 		computedValue := p.Eval(&x)
// 		fmt.Println(expectedValue)
// 		fmt.Println(computedValue.BigInt(&cVInt))
// 		fmt.Println()
// 		if polynomial.HashToFieldElement(common.BigToHash(expectedValue)) != computedValue {
// 			log.Fatalf("Polynomial verification failed at index %d", i)
// 		}
// 	}
// }

func HashToFr(h common.Hash) fr.Element {
	var e fr.Element
	err := e.SetBytesCanonical(h[:])
	if err != nil {
		panic(err)
	}
	return e
}

func main() {
	// flag.Parse()
	// if *cpuprofile != "" {
	f, err := os.Create("cpu.prof")
	if err != nil {
		panic(err)
	}

	defer f.Close()
	pprof.StartCPUProfile(f)
	defer pprof.StopCPUProfile()
	// }
	// val1 := big.NewInt(1000000000000000000)
	// val2 := big.NewInt(2000000000000000000)

	// hash1 := common.BigToHash(val1)
	// hash2 := common.BigToHash(val2)

	// hash3 := crypto.Keccak256Hash(hash1.Bytes(), hash2.Bytes())

	// fmt.Println(hash1)
	// fmt.Println(hash2)
	// fmt.Println(hash3)

	// msg := append(hash1.Bytes()[:], hash2.Bytes()[:]...)
	// dst := []byte("pair-dst")
	// els, err := fr.Hash(msg, dst, 1)
	// elsBytes := els[0].Bytes()
	// if err != nil {
	// 	panic(err)
	// }
	// // fmt.Println(els)
	// fmt.Println(els[0])

	// elsHash := common.BytesToHash(elsBytes[:])
	// fmt.Println(elsHash)
	// var element fr.Element
	// element.SetBytes(elsHash.Bytes())
	// fmt.Println(element)
	// fmt.Println(segmenttree.BytesToPoseidonHash(hash1.Bytes()[:], hash2.Bytes()[:]))
	// h := poseidon2.NewMerkleDamgardHasher()
	// h.Write(hash1.Bytes()[:])
	// h.Write(hash2.Bytes()[:])
	// outBytes := h.Sum(nil)
	// outHash := common.BytesToHash(outBytes)
	// fmt.Println("Poseidon hash output:", outHash)
	// var out fr.Element
	// out.SetBytesCanonical(outBytes)
	// fmt.Println(out)
	// restoredOutFr := HashToFr(outHash)
	// fmt.Println(restoredOutFr)

	// Testing pairing check
	// V, weights := polynomial.LoadBarycentricData(polynomial.DataDir)
	// srs, err := kzg.SetupSRS(4096)
	// if err != nil {
	// 	panic(err)
	// }

	// start := time.Now()
	// poly := polynomial.Interpolate([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}, []fr.Element{fr.NewElement(0), fr.NewElement(1), fr.NewElement(2), fr.NewElement(3), fr.NewElement(4), fr.NewElement(5), fr.NewElement(6), fr.NewElement(7), fr.NewElement(8), fr.NewElement(9), fr.NewElement(10), fr.NewElement(11), fr.NewElement(12), fr.NewElement(13), fr.NewElement(14), fr.NewElement(15)}, V, weights)

	// nodesToInterpolate := []int{2, 8, 10}

	// pCommit, err := gnark_kzg.Commit(poly, srs.Inner.Pk)
	// if err != nil {
	// 	panic(err)
	// }

	// poseidon2.NewMerkleDamgardHasher()
	// pCommitBytes := pCommit.Bytes()
	// pCommitHash := common.BytesToHash(pCommitBytes[:])
	// fmt.Println(pCommitHash)

	// pCommitHash2 := segmenttree.CommitmentToHash(pCommit)

	// polynomial.HashToFieldElement(pCommitHash)
	// polynomial.HashToFieldElement(pCommitHash2)

	// Z := polynomial.VanishingPolynomial(nodesToInterpolate)
	// ZCommit, _ := kzg.CommitG2(Z, srs.G2Powers)

	// xs := make([]fr.Element, len(nodesToInterpolate))
	// ys := make([]fr.Element, len(nodesToInterpolate))
	// for i, nodeIdx := range nodesToInterpolate {
	// 	xs[i] = fr.NewElement(uint64(nodeIdx))
	// 	ys[i] = poly.Eval(&xs[i])
	// }
	// I := kzg.Interpolate(xs, ys)
	// // I := polynomial.Interpolate(nodesToInterpolate, ys, V, weights)
	// // I := bls_polynomial.InterpolateOnRange(ys)
	// ICommit, err := gnark_kzg.Commit(I, srs.Inner.Pk)
	// _ = ICommit
	// if err != nil {
	// 	panic(err)
	// }

	// diff := kzg.SubtractPolys(poly, I)
	// qPoly := kzg.PolyDiv(diff, Z)
	// qCommit, err := gnark_kzg.Commit(qPoly, srs.Inner.Pk)
	// if err != nil {
	// 	panic(err)
	// }
	// ok, err := proof.PairingCheck(pCommit, qCommit, ICommit, ZCommit, srs)
	// if err != nil {
	// 	panic(err)
	// }
	// if !ok {
	// 	panic("pairing check failed.")
	// } else {
	// 	fmt.Println("Pairing check passed.")
	// }
	// fmt.Println("Time taken to compute commitments", time.Since(start))

	main2()
}

func main2() {
	// fmt.Println("Starting Samurai...\n")

	start := time.Now()

	srs, err := kzg.SetupSRS(segmenttree.SegmentTreeSize)
	if err != nil {
		log.Fatalf("failed to setup SRS: %v", err)
	}
	// V, weights := polynomial.LoadBarycentricData(segmenttree.SegmentTreeSize)
	V, weights, weightCommits := kzg.LoadBarycentricData(segmenttree.SegmentTreeSize, srs)
	fmt.Println("Time taken to setup SRS", time.Since(start))
	_ = weightCommits
	start = time.Now()
	segmentTree := generateSegmentTreeAndCommitments(10000, V, weights, weightCommits, srs)
	fmt.Println("Time taken to generate segment tree and commitments", time.Since(start))
	_ = segmentTree
	start = time.Now()
	segmentTree.DumpStorage()
	fmt.Println("Time taken to dump storage", time.Since(start))

	// // storage := segmentTree.Storage

	// start = time.Now()
	// storage := segmenttree.LoadStorage()
	// fmt.Println("Time taken to load storage", time.Since(start))

	// testPolynomials(storage)

	// queryStartBlock := 20
	// queryEndBlock := 8049

	// start = time.Now()
	// rangeProofs := proof.GetRangeProofs(queryStartBlock, queryEndBlock, storage, V, weights, srs)
	// fmt.Println("Time taken to generate range proofs", time.Since(start))

	// start = time.Now()
	// proof.VerifyRangeProofs(queryStartBlock, queryEndBlock, rangeProofs, V, weights, srs, storage)
	// fmt.Println("Time taken to verify range proofs", time.Since(start))

	// _ = rangeProofs

}
func generateSegmentTreeAndCommitments(maxBlockNumber int, V polynomial.Polynomial, weights []fr.Element, weightCommits []gnark_kzg.Digest, srs *kzg.MultiSRS) *segmenttree.LayeredSegmentTree {

	segmentTree := segmenttree.NewLayeredSegmentTree(V, weights, weightCommits, srs)

	for blockNumber := range maxBlockNumber {
		// fmt.Println("Processing block", blockNumber, "...")
		// random balance
		balance := big.NewInt(1000000000000000000)
		balance.Add(balance, big.NewInt(int64(blockNumber)))
		// balance := big.NewInt(rand.Int63n(1000000000000000000))

		segmentTree.Update(blockNumber, balance)
	}

	return segmentTree
}

func testPolynomials(storage *segmenttree.Storage) {
	// Test the segment tree with some sample queries

	nodeIdx := 0
	nodeIdxFr := fr.NewElement(uint64(nodeIdx))

	P1 := storage.L1Polynomial[0]
	P2 := storage.L2Polynomial[0]
	P3 := storage.L3Polynomial[0]
	P4 := storage.L4Polynomial[0]

	eval1Fr := P1.Eval(&nodeIdxFr)
	eval2Fr := P2.Eval(&nodeIdxFr)
	eval3Fr := P3.Eval(&nodeIdxFr)
	eval4Fr := P4.Eval(&nodeIdxFr)

	eval1Hash := polynomial.FieldElementToHash(eval1Fr)
	eval2Hash := polynomial.FieldElementToHash(eval2Fr)
	eval3Hash := polynomial.FieldElementToHash(eval3Fr)
	eval4Hash := polynomial.FieldElementToHash(eval4Fr)

	fmt.Println(eval1Hash)
	fmt.Println(eval2Hash)
	fmt.Println(eval3Hash)
	fmt.Println(eval4Hash)

}
