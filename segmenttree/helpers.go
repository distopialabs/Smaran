package segmenttree

import (
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/poseidon2"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

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

func GetAncestors(nodeIdx int) []int {
	ancestors := []int{}
	for nodeIdx > 0 {
		parentNodeIdx := GetParent(nodeIdx)
		ancestors = append(ancestors, parentNodeIdx)
		nodeIdx = parentNodeIdx
	}
	return ancestors
}

func GetParent(nodeIdx int) int {
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
