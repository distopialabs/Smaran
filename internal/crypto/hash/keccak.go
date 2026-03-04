package hash

import (
	"github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// BytesToKeccak256Hash computes a Keccak256 hash over the concatenation of the given byte slices.
func BytesToKeccak256Hash(b ...[]byte) common.Hash {
	var input []byte
	for _, bytes := range b {
		input = append(input, bytes...)
	}
	return crypto.Keccak256Hash(input)
}

// CommitmentToKeccak256Hash converts a KZG commitment to a 32-byte hash
// using Keccak256 over the X and Y coordinates.
func CommitmentToKeccak256Hash(c kzg.Digest) common.Hash {
	var input []byte
	input = append(input, c.X.Marshal()...)
	input = append(input, c.Y.Marshal()...)
	return crypto.Keccak256Hash(input)
}
