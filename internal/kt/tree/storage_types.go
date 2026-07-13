package tree

import (
	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/crypto/polynomial"
)

// Storage holds tree data for storage operations.
type Storage struct {
	L1Tree map[int][]common.Hash
	L2Tree map[int][]common.Hash
	L3Tree map[int][]common.Hash
	L4Tree map[int][]common.Hash

	L1Polynomial map[int]polynomial.Polynomial
	L2Polynomial map[int]polynomial.Polynomial
	L3Polynomial map[int]polynomial.Polynomial
	L4Polynomial map[int]polynomial.Polynomial

	L1Commitments map[int]bls.G1Affine
	L2Commitments map[int]bls.G1Affine
	L3Commitments map[int]bls.G1Affine
	L4Commitments map[int]bls.G1Affine
}

// CachedData holds precomputed data for polynomial operations.
type CachedData struct {
	V       polynomial.Polynomial
	Weights []fr.Element
	SRS     *kzg.MultiSRS
}
