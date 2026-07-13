package tree

import (
	"encoding/binary"
	"fmt"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Node type tags used in the serialized format.
const (
	tagInternal byte = 0x01
	tagLeaf     byte = 0x02
)

// Commitment size (compressed G1 point on BLS12-381).
const commitSize = 48

// ---------------------------------------------------------------------------
// InternalNode serialization
//
// Format:
//   [tag 1B][depth 1B][commitment 48B][child_bitmap 32B][child_commitments...]
//
// child_bitmap: 256 bits (32 bytes), bit i is set if children[i] is non-nil.
// child_commitments: for each set bit, 48 bytes of the child's commitment.
// ---------------------------------------------------------------------------

func (n *InternalNode) Serialize() ([]byte, error) {
	var bitmap [32]byte
	count := 0
	for i := 0; i < Width; i++ {
		if n.children[i] != nil {
			bitmap[i/8] |= 1 << (uint(i) % 8)
			count++
		}
	}

	size := 1 + 1 + commitSize + 32 + count*commitSize
	buf := make([]byte, size)
	off := 0

	buf[off] = tagInternal
	off++
	buf[off] = n.depth
	off++

	cb := n.commitment.Bytes()
	copy(buf[off:off+commitSize], cb[:])
	off += commitSize

	copy(buf[off:off+32], bitmap[:])
	off += 32

	for i := 0; i < Width; i++ {
		if n.children[i] == nil {
			continue
		}
		childC := n.children[i].Commitment()
		cb := childC.Bytes()
		copy(buf[off:off+commitSize], cb[:])
		off += commitSize
	}

	return buf, nil
}

// ---------------------------------------------------------------------------
// LeafNode serialization
//
// Format:
//   [tag 1B][stem 31B][commitment 48B][value_bitmap 32B][values...]
//
// value_bitmap: 256 bits (32 bytes), bit i is set if values[i] is non-nil.
// values: for each set bit, 32 bytes.
// ---------------------------------------------------------------------------

func (n *LeafNode) Serialize() ([]byte, error) {
	var bitmap [32]byte
	count := 0
	for i := 0; i < Width; i++ {
		if n.values[i] != nil {
			bitmap[i/8] |= 1 << (uint(i) % 8)
			count++
		}
	}

	size := 1 + 31 + commitSize + 32 + count*32
	buf := make([]byte, size)
	off := 0

	buf[off] = tagLeaf
	off++

	copy(buf[off:off+31], n.stem[:])
	off += 31

	cb := n.commitment.Bytes()
	copy(buf[off:off+commitSize], cb[:])
	off += commitSize

	copy(buf[off:off+32], bitmap[:])
	off += 32

	for i := 0; i < Width; i++ {
		if n.values[i] == nil {
			continue
		}
		copy(buf[off:off+32], n.values[i][:])
		off += 32
	}

	return buf, nil
}

// ---------------------------------------------------------------------------
// ParseNode deserializes a node from bytes produced by Serialize.
// depth is the depth of the *caller* (parent); it is used to derive child
// paths for the HashedNode stubs produced by parseInternalNode.
// ---------------------------------------------------------------------------

func ParseNode(data []byte, depth byte) (VerkleNode, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("empty serialized node")
	}
	switch data[0] {
	case tagInternal:
		return parseInternalNode(data)
	case tagLeaf:
		return parseLeafNode(data)
	default:
		return nil, fmt.Errorf("unknown node tag 0x%02x", data[0])
	}
}

func parseInternalNode(data []byte) (*InternalNode, error) {
	minSize := 1 + 1 + commitSize + 32
	if len(data) < minSize {
		return nil, fmt.Errorf("internal node too short: %d < %d", len(data), minSize)
	}

	off := 1
	nodeDepth := data[off]
	off++

	var commitment gnark_kzg.Digest
	if _, err := commitment.SetBytes(data[off : off+commitSize]); err != nil {
		return nil, fmt.Errorf("parse internal commitment: %w", err)
	}
	off += commitSize

	var bitmap [32]byte
	copy(bitmap[:], data[off:off+32])
	off += 32

	node := &InternalNode{
		depth:      nodeDepth,
		commitment: commitment,
	}

	for i := 0; i < Width; i++ {
		if bitmap[i/8]&(1<<(uint(i)%8)) == 0 {
			continue
		}
		if off+commitSize > len(data) {
			return nil, fmt.Errorf("internal node truncated at child %d", i)
		}

		var childCommit gnark_kzg.Digest
		if _, err := childCommit.SetBytes(data[off : off+commitSize]); err != nil {
			return nil, fmt.Errorf("parse child %d commitment: %w", i, err)
		}
		off += commitSize

		// HashedNode stubs get a nil path; the NodeStore sets the full
		// path when it constructs the resolver.
		node.children[i] = NewHashedNode(childCommit, nil)

		node.childHashes[i] = Keccak256Commitment(childCommit)
	}

	return node, nil
}

func parseLeafNode(data []byte) (*LeafNode, error) {
	minSize := 1 + 31 + commitSize + 32
	if len(data) < minSize {
		return nil, fmt.Errorf("leaf node too short: %d < %d", len(data), minSize)
	}

	off := 1
	var stem [31]byte
	copy(stem[:], data[off:off+31])
	off += 31

	var commitment gnark_kzg.Digest
	if _, err := commitment.SetBytes(data[off : off+commitSize]); err != nil {
		return nil, fmt.Errorf("parse leaf commitment: %w", err)
	}
	off += commitSize

	var bitmap [32]byte
	copy(bitmap[:], data[off:off+32])
	off += 32

	node := &LeafNode{
		stem:       stem,
		commitment: commitment,
	}

	for i := 0; i < Width; i++ {
		if bitmap[i/8]&(1<<(uint(i)%8)) == 0 {
			continue
		}
		if off+32 > len(data) {
			return nil, fmt.Errorf("leaf node truncated at value %d", i)
		}
		var v [32]byte
		copy(v[:], data[off:off+32])
		off += 32
		node.values[i] = &v
		node.cachedSlotHashes[i] = common.BytesToHash(v[:])
	}

	// Slot 255 always caches the stem hash.
	node.cachedSlotHashes[Width-1] = crypto.Keccak256Hash(stem[:])

	return node, nil
}

// ---------------------------------------------------------------------------
// Exported helpers
// ---------------------------------------------------------------------------

// Keccak256Commitment hashes a KZG digest via Keccak256(X || Y) — same as
// internal/crypto/hash.CommitmentToKeccak256Hash.
func Keccak256Commitment(c gnark_kzg.Digest) common.Hash {
	var input []byte
	input = append(input, c.X.Marshal()...)
	input = append(input, c.Y.Marshal()...)
	return crypto.Keccak256Hash(input)
}

// CommitmentBytes returns the serialized (compressed) commitment.
func CommitmentBytes(c gnark_kzg.Digest) []byte {
	b := (&c).Bytes()
	return b[:]
}

// EncodeUint64BE encodes a uint64 as 8 big-endian bytes.
func EncodeUint64BE(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

// OccupiedChildCount returns the number of non-nil child slots.
func (n *InternalNode) OccupiedChildCount() int {
	c := 0
	for i := range n.children {
		if n.children[i] != nil {
			c++
		}
	}
	return c
}
