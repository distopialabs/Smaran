package hash

import (
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

// HashType represents the supported hashing algorithms.
type HashType string

const (
	SHA256    HashType = "sha256"
	Keccak256 HashType = "keccak"
	Poseidon  HashType = "poseidon"
)

// currentHashType stores the globally active hashing algorithm.
// Hardcoded to Keccak256 as the default for the Samurai application.
var currentHashType HashType = Keccak256

// BytesToHash hashes the given byte slices using the active HashType.
func BytesToHash(b ...[]byte) common.Hash {
	switch currentHashType {
	case SHA256:
		return BytesToSHA256Hash(b...)
	case Keccak256:
		return BytesToKeccak256Hash(b...)
	case Poseidon:
		return BytesToPoseidonHash(b...)
	default:
		return BytesToKeccak256Hash(b...)
	}
}

// CommitmentToHash hashes a KZG commitment using the active HashType.
func CommitmentToHash(c gnark_kzg.Digest) common.Hash {
	switch currentHashType {
	case SHA256:
		return CommitmentToSHA256Hash(c)
	case Keccak256:
		return CommitmentToKeccak256Hash(c)
	case Poseidon:
		return CommitmentToPoseidonHash(c) // Use the existing Poseidon logic for commitments
	default:
		return CommitmentToKeccak256Hash(c)
	}
}
