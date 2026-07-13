// Package tree implements a 256-ary Verkle trie with KZG polynomial
// commitments on BLS12-381. Commitments are updated incrementally
// using precomputed Lagrange-basis KZG digests (WeightCommits).
package tree

import (
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
)

// Width is the branching factor of each trie node.
const Width = 256

// NodeResolverFn loads a serialized node from persistent storage given its
// path (sequence of child indices from the root). Mirrors the go-verkle
// resolver signature so the store layer stays interchangeable.
type NodeResolverFn func(path []byte) ([]byte, error)

// VerkleNode is the interface shared by every node type in the trie.
type VerkleNode interface {
	// Commitment returns the node's current KZG commitment.
	Commitment() gnark_kzg.Digest

	// Get returns the 32-byte value at the full key, or nil if absent.
	Get(key []byte, resolver NodeResolverFn) ([]byte, error)

	// Insert inserts or updates a value for the full key.
	Insert(key []byte, value []byte, resolver NodeResolverFn) error

	// Delete removes the value at the full key. Returns true if something
	// was actually deleted.
	Delete(key []byte, resolver NodeResolverFn) (bool, error)

	// Commit recomputes the KZG commitment incrementally (only dirty
	// sub-trees are touched). cfg supplies the SRS and WeightCommits.
	Commit(cfg *TreeConfig)

	// Serialize encodes the node to bytes for persistence.
	Serialize() ([]byte, error)

	// IsDirty reports whether the node (or any descendant) has uncommitted
	// changes.
	IsDirty() bool
}

// InternalNode is a 256-ary branch in the trie. Each child slot is indexed
// by the stem byte at the node's depth.
type InternalNode struct {
	children    [Width]VerkleNode
	commitment  gnark_kzg.Digest
	childHashes [Width]common.Hash // cached Hash(child.Commitment) for delta updates
	depth       byte
	dirty       bool
}

// LeafNode stores up to 256 values sharing the same 31-byte stem. The suffix
// byte of the key selects the value slot.
type LeafNode struct {
	stem       [31]byte
	values     [Width]*[32]byte
	commitment gnark_kzg.Digest
	// cachedFr tracks the field-element representation of each value for
	// incremental commitment deltas. Zero when the slot is empty.
	cachedSlotHashes [Width]common.Hash
	dirty            bool
}

// HashedNode is a lazy stub loaded from the DB. It knows its commitment but
// none of its children or values. Any read/write resolves it into a concrete
// InternalNode or LeafNode via the NodeResolverFn.
type HashedNode struct {
	commitment gnark_kzg.Digest
	path       []byte
}

// EmptyNode represents an absent child. It is a singleton — every nil child
// is logically an EmptyNode.
type EmptyNode struct{}

// SerializedNode pairs a trie path with its binary encoding. Used by
// persistence and dirty-path serialization, mirroring go-verkle's type.
type SerializedNode struct {
	Path            []byte
	SerializedBytes []byte
}

// ---------------------------------------------------------------------------
// Interface compliance & accessors
// ---------------------------------------------------------------------------

var (
	_ VerkleNode = (*InternalNode)(nil)
	_ VerkleNode = (*LeafNode)(nil)
	_ VerkleNode = (*HashedNode)(nil)
	_ VerkleNode = (*EmptyNode)(nil)
)

func (n *InternalNode) Commitment() gnark_kzg.Digest { return n.commitment }
func (n *InternalNode) IsDirty() bool                { return n.dirty }
func (n *InternalNode) Depth() byte                  { return n.depth }

// Children returns the raw child array (may contain nil entries).
func (n *InternalNode) Children() [Width]VerkleNode { return n.children }

// Child returns the child at index i (may be nil).
func (n *InternalNode) Child(i byte) VerkleNode { return n.children[i] }

// SetChild assigns a child at index i and marks the node dirty.
func (n *InternalNode) SetChild(i byte, child VerkleNode) {
	n.children[i] = child
	n.dirty = true
}

func (n *LeafNode) Commitment() gnark_kzg.Digest { return n.commitment }
func (n *LeafNode) IsDirty() bool                { return n.dirty }
func (n *LeafNode) Stem() [31]byte               { return n.stem }

// Value returns the value at suffix index i, or nil.
func (n *LeafNode) Value(i byte) *[32]byte { return n.values[i] }

// SetValue sets the value at suffix index i and marks the leaf dirty.
func (n *LeafNode) SetValue(i byte, v *[32]byte) {
	n.values[i] = v
	n.dirty = true
}

func (n *HashedNode) Commitment() gnark_kzg.Digest { return n.commitment }
func (n *HashedNode) IsDirty() bool                { return false }

func (n *EmptyNode) Commitment() gnark_kzg.Digest { return gnark_kzg.Digest{} }
func (n *EmptyNode) IsDirty() bool                { return false }

// NewInternalNode creates an empty internal node at the given depth.
func NewInternalNode(depth byte) *InternalNode {
	return &InternalNode{depth: depth}
}

// NewLeafNode creates a leaf with the given stem and no values.
func NewLeafNode(stem [31]byte) *LeafNode {
	return &LeafNode{stem: stem, dirty: true}
}

// NewHashedNode creates a lazy stub from a serialized commitment and path.
func NewHashedNode(commitment gnark_kzg.Digest, path []byte) *HashedNode {
	p := make([]byte, len(path))
	copy(p, path)
	return &HashedNode{commitment: commitment, path: p}
}
