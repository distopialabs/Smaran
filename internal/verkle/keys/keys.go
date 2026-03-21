// Package keys implements EIP-6800 Verkle tree key derivation and
// basic-data packing/unpacking for account balance proofs.
package keys

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"sync"

	verkle "github.com/ethereum/go-verkle"
)

// BasicDataLeafKey is the sub-index for basic account data per EIP-6800.
const BasicDataLeafKey byte = 0

// Cached Verkle IPA config (initialized once at package load).
var verkleConfig = verkle.GetConfig()

// treeKeyCache caches GetTreeKeyForBasicData results to avoid
// recomputing the expensive Pedersen hash for repeated addresses.
var treeKeyCache sync.Map // [20]byte → [32]byte

// Address20To32 converts a 20-byte EVM address to a 32-byte Verkle address
// by prepending 12 zero bytes, per EIP-6800.
func Address20To32(addr [20]byte) [32]byte {
	var addr32 [32]byte
	copy(addr32[12:], addr[:])
	return addr32
}

// GetTreeKey computes a Verkle tree key per EIP-6800:
//
//	key = pedersen_hash(addr32 ++ treeIndex_LE_32) → take first 31 bytes → append subIndex
//
// The Pedersen hash is computed via go-verkle's IPA config by building
// a 256-element polynomial from 16-byte LE chunks of the input, then
// committing and hashing the result.
func GetTreeKey(addr32 [32]byte, treeIndex uint64, subIndex byte) [32]byte {
	cfg := verkleConfig

	// Build the input polynomial as specified in EIP-6800:
	//   poly[0] = 2 + 256*64 = 16386
	//   poly[1..2] = addr32 as two 16-byte LE field elements
	//   poly[3..4] = treeIndex as 32 bytes split into two 16-byte LE field elements
	var poly [256]verkle.Fr

	// poly[0] = 2 + 256*64
	poly[0].SetUint64(2 + 256*64)

	// Split addr32 into two 16-byte LE chunks → poly[1..2]
	for i := 0; i < 2; i++ {
		var chunk [16]byte
		copy(chunk[:], addr32[i*16:(i+1)*16])
		if err := verkle.FromLEBytes(&poly[1+i], chunk[:]); err != nil {
			panic(fmt.Sprintf("keys: FromLEBytes failed: %v", err))
		}
	}

	// treeIndex as 32-byte LE → split into two 16-byte LE chunks → poly[3..4]
	var treeIndexLE [32]byte
	binary.LittleEndian.PutUint64(treeIndexLE[0:8], treeIndex)

	for i := 0; i < 2; i++ {
		var chunk [16]byte
		copy(chunk[:], treeIndexLE[i*16:(i+1)*16])
		if err := verkle.FromLEBytes(&poly[3+i], chunk[:]); err != nil {
			panic(fmt.Sprintf("keys: FromLEBytes failed: %v", err))
		}
	}

	// Commit to polynomial → point → hash to 32 bytes
	point := cfg.CommitToPoly(poly[:], 0)
	h := verkle.HashPointToBytes(point)

	// Take first 31 bytes of hash, set byte 31 = subIndex
	var key [32]byte
	copy(key[:31], h[:31])
	key[31] = subIndex

	return key
}

// GetTreeKeyForBasicData returns the Verkle tree key for an account's
// basic data (version, code_size, nonce, balance) per EIP-6800.
// Results are cached since the Pedersen hash is deterministic for a given address.
func GetTreeKeyForBasicData(addr [20]byte) [32]byte {
	if cached, ok := treeKeyCache.Load(addr); ok {
		return cached.([32]byte)
	}
	addr32 := Address20To32(addr)
	key := GetTreeKey(addr32, 0, BasicDataLeafKey)
	treeKeyCache.Store(addr, key)
	return key
}

// PackBasicData packs a balance into the 32-byte EIP-6800 basic-data value:
//
//	offset 0, size 1:  version   (0)
//	offset 5, size 3:  code_size (0)
//	offset 8, size 8:  nonce     (0)
//	offset 16, size 16: balance  (big-endian)
//
// Returns error if balance exceeds 128 bits.
func PackBasicData(balance *big.Int) ([32]byte, error) {
	var val [32]byte
	if balance == nil || balance.Sign() == 0 {
		return val, nil // all zeros
	}
	if balance.BitLen() > 128 {
		return val, fmt.Errorf("balance overflow: %d bits (max 128)", balance.BitLen())
	}
	if balance.Sign() < 0 {
		return val, fmt.Errorf("negative balance: %s", balance.String())
	}
	// Write balance big-endian into bytes 16..31
	b := balance.Bytes() // big-endian, minimal
	copy(val[32-len(b):], b)
	return val, nil
}

// UnpackBalance extracts the balance from a 32-byte EIP-6800 basic-data value.
func UnpackBalance(val [32]byte) *big.Int {
	return new(big.Int).SetBytes(val[16:32])
}
