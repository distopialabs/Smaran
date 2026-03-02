// Package hash provides cryptographic hashing functions for Samurai.
package hash

import (
	"sync"

	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/poseidon2"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

// Singleton permutation for CommitmentToHash - initialized once, reused for all calls.
var (
	commitmentPermOnce sync.Once
	commitmentPerm     *poseidon2.Permutation
	bytesPermOnce      sync.Once
	bytesPerm          *poseidon2.Permutation
)

// CommitmentToHash converts a KZG commitment (G1Affine point) to a 32-byte hash
// using Poseidon2 compression. Thread-safe and uses a singleton permutation.
func CommitmentToHash(c gnark_kzg.Digest) common.Hash {
	// Initialize the permutation singleton once (thread-safe)
	commitmentPermOnce.Do(func() {
		pr := poseidon2.GetDefaultParameters()
		commitmentPerm = poseidon2.NewPermutation(2, pr.NbFullRounds, pr.NbPartialRounds)
	})

	var x, y fr.Element
	x.SetBytes(c.X.Marshal())
	y.SetBytes(c.Y.Marshal())

	// apply the permutation (Compress is stateless and thread-safe)
	digestBytes, err := commitmentPerm.Compress(x.Marshal(), y.Marshal())
	if err != nil {
		panic(err)
	}

	return common.BytesToHash(digestBytes[:])
}

// BytesToPoseidonHash computes a Poseidon2 hash over the concatenation of the given byte slices.
// func BytesToPoseidonHash(b ...[]byte) common.Hash {
// 	h := poseidon2.NewMerkleDamgardHasher()
// 	for _, bytes := range b {
// 		h.Write(bytes)
// 	}
// 	outBytes := h.Sum(nil)
// 	return common.BytesToHash(outBytes)
// }

func BytesToPoseidonHash(b ...[]byte) common.Hash {
	bytesPermOnce.Do(func() {
		pr := poseidon2.GetDefaultParameters()
		bytesPerm = poseidon2.NewPermutation(2, pr.NbFullRounds, pr.NbPartialRounds)
	})
	// For the common 2-input case (tree hashing), use Compress directly
	if len(b) == 2 && len(b[0]) == 32 && len(b[1]) == 32 {
		// Inputs are field elements (32 bytes each), use Compress directly
		digestBytes, err := bytesPerm.Compress(b[0], b[1])
		if err != nil {
			panic(err)
		}
		return common.BytesToHash(digestBytes)
	}
	// Fallback for other cases
	h := poseidon2.NewMerkleDamgardHasher()
	for _, bytes := range b {
		h.Write(bytes)
	}
	return common.BytesToHash(h.Sum(nil))
}
