package tree

import (
	"bytes"
	"fmt"
	"math/big"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	fr "github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/crypto/hash"
)

// New creates a fresh empty root internal node at depth 0.
func New() *InternalNode {
	return NewInternalNode(0)
}

// ---------------------------------------------------------------------------
// InternalNode — Get / Insert / Delete / Commit
// ---------------------------------------------------------------------------

func (n *InternalNode) Get(key []byte, resolver NodeResolverFn) ([]byte, error) {
	stem := key[:31]
	childIdx := stem[n.depth]
	child := n.children[childIdx]

	if child == nil {
		return nil, fmt.Errorf("key not found")
	}

	child, err := resolveIfHashed(child, key, n.depth, resolver)
	if err != nil {
		return nil, err
	}
	n.children[childIdx] = child

	return child.Get(key, resolver)
}

func (n *InternalNode) Insert(key []byte, value []byte, resolver NodeResolverFn) error {
	stem := key[:31]
	suffix := key[31]
	childIdx := stem[n.depth]
	child := n.children[childIdx]

	if child == nil {
		// Empty slot — create a leaf directly.
		var s [31]byte
		copy(s[:], stem)
		leaf := NewLeafNode(s)
		var v [32]byte
		copy(v[:], value)
		leaf.SetValue(suffix, &v)
		n.children[childIdx] = leaf
		n.dirty = true
		return nil
	}

	// Resolve hashed stubs before modifying.
	child, err := resolveIfHashed(child, key, n.depth, resolver)
	if err != nil {
		return err
	}
	n.children[childIdx] = child

	switch c := child.(type) {
	case *InternalNode:
		if err := c.Insert(key, value, resolver); err != nil {
			return err
		}
		n.dirty = true
		return nil

	case *LeafNode:
		if bytes.Equal(c.stem[:], stem) {
			// Same stem — update value in existing leaf.
			var v [32]byte
			copy(v[:], value)
			c.SetValue(suffix, &v)
			n.dirty = true
			return nil
		}
		// Stem collision: split the leaf into deeper internal nodes.
		n.children[childIdx] = splitLeaf(c, key, value, n.depth+1)
		n.dirty = true
		return nil

	default:
		return fmt.Errorf("unexpected child type %T at depth %d index %d", child, n.depth, childIdx)
	}
}

func (n *InternalNode) Delete(key []byte, resolver NodeResolverFn) (bool, error) {
	stem := key[:31]
	suffix := key[31]
	childIdx := stem[n.depth]
	child := n.children[childIdx]

	if child == nil {
		return false, nil
	}

	child, err := resolveIfHashed(child, key, n.depth, resolver)
	if err != nil {
		return false, err
	}
	n.children[childIdx] = child

	switch c := child.(type) {
	case *InternalNode:
		deleted, err := c.Delete(key, resolver)
		if deleted {
			n.dirty = true
		}
		return deleted, err

	case *LeafNode:
		if !bytes.Equal(c.stem[:], stem) {
			return false, nil
		}
		if c.values[suffix] == nil {
			return false, nil
		}
		c.SetValue(suffix, nil)
		n.dirty = true
		return true, nil

	default:
		return false, fmt.Errorf("unexpected child type %T", child)
	}
}

// Commit incrementally recomputes the KZG commitment using the delta pattern
// from internal/tree/types.go UpdateLXTree:
//
//	delta = Hash(new_child_commitment) - cached_old_hash
//	commitment += delta * WeightCommit[childIdx]
func (n *InternalNode) Commit(cfg *TreeConfig) {
	if !n.dirty {
		return
	}

	for i := 0; i < Width; i++ {
		child := n.children[i]
		if child == nil {
			continue
		}
		if !child.IsDirty() {
			continue
		}

		// Recurse: commit the child first.
		child.Commit(cfg)

		newHash := hash.CommitmentToHash(child.Commitment())
		oldHash := n.childHashes[i]

		if newHash == oldHash {
			continue
		}

		// delta = Fr(newHash) - Fr(oldHash)
		var deltaFr fr.Element
		deltaFr.SetBigInt(newHash.Big())
		if (oldHash != common.Hash{}) {
			var oldFr fr.Element
			oldFr.SetBigInt(oldHash.Big())
			deltaFr.Sub(&deltaFr, &oldFr)
		}

		// commitment += delta * WeightCommit[i]
		var inc bls.G1Affine
		var deltaBig big.Int
		deltaFr.BigInt(&deltaBig)
		inc.ScalarMultiplication(&cfg.WeightCommits[i], &deltaBig)
		n.commitment.Add(&n.commitment, &inc)

		n.childHashes[i] = newHash
	}

	n.dirty = false
}

// BatchSerialize serializes every reachable node in the sub-tree rooted at n.
// Used for periodic full-flush to DB.
func (n *InternalNode) BatchSerialize() ([]SerializedNode, error) {
	var nodes []SerializedNode
	return n.batchSerializeRec(nil, &nodes)
}

func (n *InternalNode) batchSerializeRec(path []byte, out *[]SerializedNode) ([]SerializedNode, error) {
	ser, err := n.Serialize()
	if err != nil {
		return *out, fmt.Errorf("serialize internal at path %x: %w", path, err)
	}
	*out = append(*out, SerializedNode{Path: copyBytes(path), SerializedBytes: ser})

	for i := 0; i < Width; i++ {
		child := n.children[i]
		if child == nil {
			continue
		}
		childPath := append(copyBytes(path), byte(i))
		switch c := child.(type) {
		case *InternalNode:
			if _, err := c.batchSerializeRec(childPath, out); err != nil {
				return *out, err
			}
		case *LeafNode:
			ser, err := c.Serialize()
			if err != nil {
				return *out, fmt.Errorf("serialize leaf at path %x: %w", childPath, err)
			}
			*out = append(*out, SerializedNode{Path: childPath, SerializedBytes: ser})
		case *HashedNode:
			// Hashed nodes are already persisted; skip.
		}
	}
	return *out, nil
}

// ---------------------------------------------------------------------------
// LeafNode — Get / Insert / Delete / Commit
// ---------------------------------------------------------------------------

func (n *LeafNode) Get(key []byte, _ NodeResolverFn) ([]byte, error) {
	stem := key[:31]
	suffix := key[31]
	if !bytes.Equal(n.stem[:], stem) {
		return nil, fmt.Errorf("key not found")
	}
	v := n.values[suffix]
	if v == nil {
		return nil, fmt.Errorf("key not found")
	}
	out := make([]byte, 32)
	copy(out, v[:])
	return out, nil
}

func (n *LeafNode) Insert(key []byte, value []byte, _ NodeResolverFn) error {
	stem := key[:31]
	suffix := key[31]
	if !bytes.Equal(n.stem[:], stem) {
		return fmt.Errorf("stem mismatch in leaf insert (should have been split)")
	}
	var v [32]byte
	copy(v[:], value)
	n.SetValue(suffix, &v)
	return nil
}

func (n *LeafNode) Delete(key []byte, _ NodeResolverFn) (bool, error) {
	stem := key[:31]
	suffix := key[31]
	if !bytes.Equal(n.stem[:], stem) {
		return false, nil
	}
	if n.values[suffix] == nil {
		return false, nil
	}
	n.SetValue(suffix, nil)
	return true, nil
}

// Commit incrementally updates the leaf's KZG commitment. The polynomial
// evaluates to Fr(value[i]) at omega^i, with slot 255 reserved for the
// stem hash to bind the stem into the commitment.
func (n *LeafNode) Commit(cfg *TreeConfig) {
	if !n.dirty {
		return
	}

	for i := 0; i < Width; i++ {
		var newSlotHash common.Hash
		if i == Width-1 {
			// Slot 255 encodes the stem.
			newSlotHash = hash.BytesToHash(n.stem[:])
		} else if n.values[i] != nil {
			newSlotHash = common.BytesToHash(n.values[i][:])
		}
		// else: zero hash for empty slot

		oldHash := n.cachedSlotHashes[i]
		if newSlotHash == oldHash {
			continue
		}

		var deltaFr fr.Element
		deltaFr.SetBigInt(newSlotHash.Big())
		if (oldHash != common.Hash{}) {
			var oldFr fr.Element
			oldFr.SetBigInt(oldHash.Big())
			deltaFr.Sub(&deltaFr, &oldFr)
		}

		var inc bls.G1Affine
		var deltaBig big.Int
		deltaFr.BigInt(&deltaBig)
		inc.ScalarMultiplication(&cfg.WeightCommits[i], &deltaBig)
		n.commitment.Add(&n.commitment, &inc)

		n.cachedSlotHashes[i] = newSlotHash
	}

	n.dirty = false
}

// ---------------------------------------------------------------------------
// HashedNode stubs
// ---------------------------------------------------------------------------

func (n *HashedNode) Get(key []byte, _ NodeResolverFn) ([]byte, error) {
	return nil, fmt.Errorf("cannot Get through unresolved HashedNode at path %x", n.path)
}

func (n *HashedNode) Insert(key []byte, _ []byte, _ NodeResolverFn) error {
	return fmt.Errorf("cannot Insert through unresolved HashedNode at path %x", n.path)
}

func (n *HashedNode) Delete(key []byte, _ NodeResolverFn) (bool, error) {
	return false, fmt.Errorf("cannot Delete through unresolved HashedNode at path %x", n.path)
}

func (n *HashedNode) Commit(_ *TreeConfig) {}

func (n *HashedNode) Serialize() ([]byte, error) {
	return nil, fmt.Errorf("cannot serialize unresolved HashedNode")
}

// ---------------------------------------------------------------------------
// EmptyNode stubs
// ---------------------------------------------------------------------------

func (n *EmptyNode) Get([]byte, NodeResolverFn) ([]byte, error)      { return nil, fmt.Errorf("key not found") }
func (n *EmptyNode) Insert([]byte, []byte, NodeResolverFn) error     { return fmt.Errorf("cannot insert into EmptyNode directly") }
func (n *EmptyNode) Delete([]byte, NodeResolverFn) (bool, error)     { return false, nil }
func (n *EmptyNode) Commit(*TreeConfig)                              {}
func (n *EmptyNode) Serialize() ([]byte, error)                      { return nil, fmt.Errorf("cannot serialize EmptyNode") }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// splitLeaf creates a chain of internal nodes to resolve a stem collision
// between an existing leaf and a new key with a different stem.
func splitLeaf(existing *LeafNode, newKey []byte, newValue []byte, startDepth byte) VerkleNode {
	newStem := newKey[:31]
	newSuffix := newKey[31]
	oldStem := existing.stem[:]

	// Find the first byte where the stems diverge.
	depth := startDepth
	for depth < 31 && oldStem[depth] == newStem[depth] {
		depth++
	}

	if depth >= 31 {
		// Stems are identical — shouldn't happen (caller checks), but handle gracefully.
		var v [32]byte
		copy(v[:], newValue)
		existing.SetValue(newSuffix, &v)
		return existing
	}

	// Build chain of internal nodes from startDepth to depth.
	var buildChain func(d byte) *InternalNode
	buildChain = func(d byte) *InternalNode {
		node := NewInternalNode(d)
		if d == depth {
			// Place both leaves at the divergence point.
			node.children[oldStem[d]] = existing
			var ns [31]byte
			copy(ns[:], newStem)
			newLeaf := NewLeafNode(ns)
			var v [32]byte
			copy(v[:], newValue)
			newLeaf.SetValue(newSuffix, &v)
			node.children[newStem[d]] = newLeaf
		} else {
			// Shared prefix byte: one child leads deeper.
			node.children[oldStem[d]] = buildChain(d + 1)
		}
		node.dirty = true
		return node
	}

	return buildChain(startDepth)
}

// resolveIfHashed replaces a HashedNode child with a fully parsed node using
// the resolver. The path passed to the resolver is derived from the key being
// walked: key[:parentDepth+1] gives the sequence of child indices from root to
// this child, which matches the path format used by the NodeStore.
func resolveIfHashed(child VerkleNode, key []byte, parentDepth byte, resolver NodeResolverFn) (VerkleNode, error) {
	if _, ok := child.(*HashedNode); !ok {
		return child, nil
	}
	if resolver == nil {
		return nil, fmt.Errorf("resolver is nil, cannot resolve HashedNode at depth %d", parentDepth)
	}
	// The path to this child is the stem bytes from index 0 through parentDepth
	// (inclusive), i.e. key[0..parentDepth]. key[parentDepth] is the child index
	// at the parent.
	path := key[:parentDepth+1]
	data, err := resolver(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %x: %w", path, err)
	}
	node, err := ParseNode(data, parentDepth+1)
	if err != nil {
		return nil, fmt.Errorf("parse resolved node at path %x: %w", path, err)
	}
	return node, nil
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// BuildFullCommitment recomputes a node's KZG commitment from scratch (non-incremental).
// Useful after deserialization when cached hashes are not available.
func (n *InternalNode) BuildFullCommitment(cfg *TreeConfig) {
	var poly [DomainSize]fr.Element
	for i := 0; i < Width; i++ {
		child := n.children[i]
		if child == nil {
			continue
		}
		h := hash.CommitmentToHash(child.Commitment())
		poly[i].SetBigInt(h.Big())
		n.childHashes[i] = h
	}
	c, _ := gnark_kzg.Commit(poly[:], cfg.SRS.Inner.Pk)
	n.commitment = c
	n.dirty = false
}

// BuildFullCommitment recomputes a leaf's KZG commitment from scratch.
func (n *LeafNode) BuildFullCommitment(cfg *TreeConfig) {
	var poly [DomainSize]fr.Element
	for i := 0; i < Width; i++ {
		var slotHash common.Hash
		if i == Width-1 {
			slotHash = hash.BytesToHash(n.stem[:])
		} else if n.values[i] != nil {
			slotHash = common.BytesToHash(n.values[i][:])
		}
		poly[i].SetBigInt(slotHash.Big())
		n.cachedSlotHashes[i] = slotHash
	}
	c, _ := gnark_kzg.Commit(poly[:], cfg.SRS.Inner.Pk)
	n.commitment = c
	n.dirty = false
}
