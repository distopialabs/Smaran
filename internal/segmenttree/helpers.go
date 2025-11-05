package segmenttree

import (
	"encoding/json"
	"fmt"
	"time"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/poseidon2"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

func logBlockedTime(name string, d time.Duration) chan struct{} {
	start := time.Now()
	ticker := time.NewTicker(d)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				fmt.Println("🚨", name, "blocked for", time.Since(start))
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	return quit
}

func MarshalG1AffineMap(g1AffineMap map[int]bls.G1Affine) ([]byte, error) {
	g1affinebytesMap := make(map[int][]byte)
	for k, v := range g1AffineMap {
		g1affinebytesMap[k] = v.Marshal()
	}
	return json.Marshal(g1affinebytesMap)
}

func UnmarshalG1AffineMap(data []byte) (map[int]bls.G1Affine, error) {
	var g1affinebytesMap map[int][]byte
	err := json.Unmarshal(data, &g1affinebytesMap)
	if err != nil {
		return nil, err
	}
	g1AffineMap := make(map[int]bls.G1Affine)
	for k, v := range g1affinebytesMap {
		var tempG1Affine bls.G1Affine
		tempG1Affine.Unmarshal(v)
		g1AffineMap[k] = tempG1Affine
	}
	return g1AffineMap, nil
}

// func GetParentHash(left, right common.Hash) common.Hash {
// 	h := poseidon2.NewMerkleDamgardHasher()
// 	h.Write(left.Bytes())
// 	h.Write(right.Bytes())
// 	outBytes := h.Sum(nil)
// 	return common.BytesToHash(outBytes)
// }

func CommitmentToHash(c gnark_kzg.Digest) common.Hash {

	// cBytes := c.Bytes()
	// return BytesToPoseidonHash(cBytes[:])
	var x, y fr.Element
	// xBytes := c.X.Bytes()
	// yBytes := c.Y.Bytes()
	x.SetBytes(c.X.Marshal())
	y.SetBytes(c.Y.Marshal())

	// elems := []fr.Element{x, y}

	// create a 2‑width permutation
	pr := poseidon2.GetDefaultParameters()
	perm := poseidon2.NewPermutation(2, pr.NbFullRounds, pr.NbPartialRounds)

	// apply the permutation in place
	digestBytes, err := perm.Compress(x.Marshal(), y.Marshal())
	if err != nil {
		panic(err)
	}
	// if err := perm.Permutation(elems); err != nil {
	// 	panic(err)
	// }

	// now elems[0] holds your hash as an fr.Element
	// hashScalar := elems[0]
	// hashBytes := hashScalar.Bytes()

	return common.BytesToHash(digestBytes[:])
}

func BytesToPoseidonHash(b ...[]byte) common.Hash {
	h := poseidon2.NewMerkleDamgardHasher()
	for _, b := range b {
		h.Write(b)
	}
	outBytes := h.Sum(nil)
	return common.BytesToHash(outBytes)
}

func GetAncestors(nodeIdx uint64) []uint64 {
	ancestors := []uint64{}
	for nodeIdx > 0 {
		parentNodeIdx := GetParent(nodeIdx)
		ancestors = append(ancestors, parentNodeIdx)
		nodeIdx = parentNodeIdx
	}
	return ancestors
}

func GetParent(nodeIdx uint64) uint64 {
	if nodeIdx == 0 {
		panic("root has no parent.")
	}
	if nodeIdx&1 == 0 { // if even, node is right child
		return (nodeIdx - 2) / 2
	} else { // if odd, node is left child
		return (nodeIdx - 1) / 2
	}
}

func getLeftChild(nodeIdx int) int {
	return 2*nodeIdx + 1
}

func getRightChild(nodeIdx int) int {
	return 2*nodeIdx + 2
}

func getSibling(nodeIdx int) int {
	if nodeIdx == 0 {
		return 0
	}
	if nodeIdx&1 == 0 {
		return nodeIdx - 1
	} else {
		return nodeIdx + 1
	}
}

// func (segmentTree *SegmentTree) DumpTrees() {

// 	// dump trees to a json file
// 	l1Tree := segmentTree.Storage.L1Tree
// 	l2Tree := segmentTree.Storage.L2Tree
// 	l3Tree := segmentTree.Storage.L3Tree
// 	l4Tree := segmentTree.Storage.L4Tree

// 	// dump trees to a json file
// 	l1TreeJSON, err := json.Marshal(l1Tree)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l1Tree: %v", err)
// 	}
// 	err = os.WriteFile("l1Tree.json", l1TreeJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l1Tree to file: %v", err)
// 	}

// 	l2TreeJSON, err := json.Marshal(l2Tree)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l2Tree: %v", err)
// 	}
// 	err = os.WriteFile("l2Tree.json", l2TreeJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l2Tree to file: %v", err)
// 	}

// 	l3TreeJSON, err := json.Marshal(l3Tree)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l3Tree: %v", err)
// 	}
// 	err = os.WriteFile("l3Tree.json", l3TreeJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l3Tree to file: %v", err)
// 	}

// 	l4TreeJSON, err := json.Marshal(l4Tree)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l4Tree: %v", err)
// 	}
// 	err = os.WriteFile("l4Tree.json", l4TreeJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l4Tree to file: %v", err)
// 	}

// 	fmt.Println("Dumped trees to json files")

// }
// func (segmentTree *SegmentTree) DumpCommitments() {

// 	// dump commitments to a json file
// 	l1Commitments := segmentTree.Storage.L1Commitments
// 	l2Commitments := segmentTree.Storage.L2Commitments
// 	l3Commitments := segmentTree.Storage.L3Commitments
// 	l4Commitments := segmentTree.Storage.L4Commitments

// 	// store in separate json files
// 	l1CommitmentsJSON, err := MarshalG1AffineMap(l1Commitments)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l1Commitments: %v", err)
// 	}
// 	err = os.WriteFile("l1Commitments.json", l1CommitmentsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l1Commitments to file: %v", err)
// 	}

// 	l2CommitmentsJSON, err := MarshalG1AffineMap(l2Commitments)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l2Commitments: %v", err)
// 	}
// 	err = os.WriteFile("l2Commitments.json", l2CommitmentsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l2Commitments to file: %v", err)
// 	}

// 	l3CommitmentsJSON, err := MarshalG1AffineMap(l3Commitments)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l3Commitments: %v", err)
// 	}
// 	err = os.WriteFile("l3Commitments.json", l3CommitmentsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l3Commitments to file: %v", err)
// 	}

// 	l4CommitmentsJSON, err := MarshalG1AffineMap(l4Commitments)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l4Commitments: %v", err)
// 	}
// 	err = os.WriteFile("l4Commitments.json", l4CommitmentsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l4Commitments to file: %v", err)
// 	}

// }

// func (segmentTree *SegmentTree) DumpPolynomials() {

// 	// dump polynomials to a json file
// 	l1Polynomials := segmentTree.Storage.L1Polynomial
// 	l2Polynomials := segmentTree.Storage.L2Polynomial
// 	l3Polynomials := segmentTree.Storage.L3Polynomial
// 	l4Polynomials := segmentTree.Storage.L4Polynomial

// 	// store in separate json files
// 	l1PolynomialsJSON, err := json.Marshal(l1Polynomials)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l1Polynomials: %v", err)
// 	}
// 	err = os.WriteFile("l1Polynomials.json", l1PolynomialsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l1Polynomials to file: %v", err)
// 	}

// 	l2PolynomialsJSON, err := json.Marshal(l2Polynomials)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l2Polynomials: %v", err)
// 	}
// 	err = os.WriteFile("l2Polynomials.json", l2PolynomialsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l2Polynomials to file: %v", err)
// 	}

// 	l3PolynomialsJSON, err := json.Marshal(l3Polynomials)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l3Polynomials: %v", err)
// 	}
// 	err = os.WriteFile("l3Polynomials.json", l3PolynomialsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l3Polynomials to file: %v", err)
// 	}

// 	l4PolynomialsJSON, err := json.Marshal(l4Polynomials)
// 	if err != nil {
// 		log.Fatalf("failed to marshal l4Polynomials: %v", err)
// 	}
// 	err = os.WriteFile("l4Polynomials.json", l4PolynomialsJSON, 0644)
// 	if err != nil {
// 		log.Fatalf("failed to write l4Polynomials to file: %v", err)
// 	}

// 	fmt.Println("Dumped polynomials to json files")

// }

// func (segmentTree *SegmentTree) DumpStorage() {
// 	segmentTree.DumpTrees()
// 	segmentTree.DumpCommitments()
// 	segmentTree.DumpPolynomials()
// }

// func (storage *Storage) LoadTrees() {

// 	l1TreeJSON, err := os.ReadFile("l1Tree.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l1Tree from file: %v", err)
// 	}
// 	err = json.Unmarshal(l1TreeJSON, &storage.L1Tree)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l1Tree: %v", err)
// 	}

// 	l2TreeJSON, err := os.ReadFile("l2Tree.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l2Tree from file: %v", err)
// 	}
// 	err = json.Unmarshal(l2TreeJSON, &storage.L2Tree)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l2Tree: %v", err)
// 	}

// 	l3TreeJSON, err := os.ReadFile("l3Tree.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l3Tree from file: %v", err)
// 	}
// 	err = json.Unmarshal(l3TreeJSON, &storage.L3Tree)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l3Tree: %v", err)
// 	}

// 	l4TreeJSON, err := os.ReadFile("l4Tree.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l4Tree from file: %v", err)
// 	}
// 	err = json.Unmarshal(l4TreeJSON, &storage.L4Tree)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l4Tree: %v", err)
// 	}

// }

// func (storage *Storage) LoadCommitments() {
// 	l1CommitmentsJSON, err := os.ReadFile("l1Commitments.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l1Commitments from file: %v", err)
// 	}
// 	storage.L1Commitments, err = UnmarshalG1AffineMap(l1CommitmentsJSON)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l1Commitments: %v", err)
// 	}

// 	l2CommitmentsJSON, err := os.ReadFile("l2Commitments.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l2Commitments from file: %v", err)
// 	}
// 	storage.L2Commitments, err = UnmarshalG1AffineMap(l2CommitmentsJSON)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l2Commitments: %v", err)
// 	}

// 	l3CommitmentsJSON, err := os.ReadFile("l3Commitments.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l3Commitments from file: %v", err)
// 	}
// 	storage.L3Commitments, err = UnmarshalG1AffineMap(l3CommitmentsJSON)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l3Commitments: %v", err)
// 	}

// 	l4CommitmentsJSON, err := os.ReadFile("l4Commitments.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l4Commitments from file: %v", err)
// 	}
// 	storage.L4Commitments, err = UnmarshalG1AffineMap(l4CommitmentsJSON)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l4Commitments: %v", err)
// 	}

// }

// func (storage *Storage) LoadPolynomials() {

// 	l1PolynomialsJSON, err := os.ReadFile("l1Polynomials.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l1Polynomials from file: %v", err)
// 	}
// 	err = json.Unmarshal(l1PolynomialsJSON, &storage.L1Polynomial)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l1Polynomials: %v", err)
// 	}

// 	l2PolynomialsJSON, err := os.ReadFile("l2Polynomials.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l2Polynomials from file: %v", err)
// 	}
// 	err = json.Unmarshal(l2PolynomialsJSON, &storage.L2Polynomial)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l2Polynomials: %v", err)
// 	}

// 	l3PolynomialsJSON, err := os.ReadFile("l3Polynomials.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l3Polynomials from file: %v", err)
// 	}
// 	err = json.Unmarshal(l3PolynomialsJSON, &storage.L3Polynomial)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l3Polynomials: %v", err)
// 	}

// 	l4PolynomialsJSON, err := os.ReadFile("l4Polynomials.json")
// 	if err != nil {
// 		log.Fatalf("failed to read l4Polynomials from file: %v", err)
// 	}
// 	err = json.Unmarshal(l4PolynomialsJSON, &storage.L4Polynomial)
// 	if err != nil {
// 		log.Fatalf("failed to unmarshal l4Polynomials: %v", err)
// 	}

// }

// func LoadStorage() *Storage {
// 	storage := &Storage{}
// 	storage.LoadTrees()
// 	storage.LoadCommitments()
// 	storage.LoadPolynomials()
// 	return storage
// }
