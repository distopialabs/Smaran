package ingest

import (
	"fmt"

	verkle "github.com/ethereum/go-verkle"
)

// serializeDirtyPaths serializes only the nodes along paths from root to each
// modified stem. This is O(k * tree_depth) where k = number of modified stems,
// instead of O(n) for a full BatchSerialize.
//
// It walks the tree using go-verkle's public Children() method, serializing
// each internal node and leaf encountered along the dirty paths using
// go-verkle's Serialize() method.
//
// writtenPaths is the session-lifetime set of every path this ingest has
// persisted so far; the walk records what it serializes there. It exists to
// catch leaf splits: when an insert splits an existing leaf, go-verkle creates
// internal node(s) and re-parents the OLD leaf one level deeper. That leaf's
// stem is not in dirtyStems (its value did not change), so no dirty path
// covers its new location — without extra handling it is never persisted
// there, and resolveNodeAtBlock fails with "node not found" for every block
// between the split and the node's next write (next flush or next update).
// The walk therefore serializes, at every on-path internal node, any resident
// leaf child whose own path has never been written: after a split that is
// exactly the re-parented leaf (the split-created internal is on the dirty
// stem's path, since the two stems share the prefix up to the divergence).
//
// Returns the serialized nodes and the root commitment bytes.
func serializeDirtyPaths(root *verkle.InternalNode, dirtyStems map[string]struct{}, writtenPaths map[string]struct{}) ([]verkle.SerializedNode, []byte, error) {
	seen := make(map[string]struct{})
	var nodes []verkle.SerializedNode

	// Always serialize the root node (it changes every block)
	rootSerialized, err := root.Serialize()
	if err != nil {
		return nil, nil, fmt.Errorf("serialize root: %w", err)
	}
	rootPath := []byte{}
	rootCommitment := root.Commitment().Bytes()
	nodes = append(nodes, verkle.SerializedNode{
		Path:            rootPath,
		SerializedBytes: rootSerialized,
	})
	seen[string(rootPath)] = struct{}{}
	writtenPaths[string(rootPath)] = struct{}{}

	// Walk each dirty stem's path from root to leaf
	children := root.Children()
	for stemStr := range dirtyStems {
		stem := []byte(stemStr)
		walkDirtyPath(children, stem, 0, seen, writtenPaths, &nodes)
	}

	return nodes, rootCommitment[:], nil
}

// walkDirtyPath walks the tree from a given level of children, following the
// stem bytes, serializing each node encountered using go-verkle's Serialize().
// Uses seen map to avoid serializing the same node twice when multiple dirty
// stems share a prefix.
func walkDirtyPath(children []verkle.VerkleNode, stem []byte, depth byte, seen, writtenPaths map[string]struct{}, nodes *[]verkle.SerializedNode) {
	if int(depth) >= len(stem) {
		return
	}

	childIdx := stem[depth]
	child := children[childIdx]
	if child == nil {
		return
	}

	// Build the path for this child
	path := make([]byte, depth+1)
	copy(path, stem[:depth+1])
	pathStr := string(path)

	// Skip if already serialized
	if _, ok := seen[pathStr]; ok {
		// Still need to descend if it's an internal node, because
		// deeper nodes on the same prefix might not be serialized yet
		if inode, ok := child.(*verkle.InternalNode); ok {
			walkDirtyPath(inode.Children(), stem, depth+1, seen, writtenPaths, nodes)
		}
		return
	}

	// A leaf split re-parents the pre-existing leaf one level deeper, off the
	// dirty stem's own path, without dirtying its stem. Serialize any resident
	// leaf child of this internal node that has never been persisted at its
	// current path — after a split that is exactly the moved leaf (see the
	// serializeDirtyPaths comment).
	inode, isInternal := child.(*verkle.InternalNode)
	if isInternal {
		grandchildren := inode.Children()
		for idx, gc := range grandchildren {
			leaf, isLeaf := gc.(*verkle.LeafNode)
			if !isLeaf {
				continue
			}
			leafPath := make([]byte, depth+2)
			copy(leafPath, path)
			leafPath[depth+1] = byte(idx)
			leafPathStr := string(leafPath)
			if _, ok := writtenPaths[leafPathStr]; ok {
				continue
			}
			if _, ok := seen[leafPathStr]; ok {
				continue
			}
			leafSerialized, err := leaf.Serialize()
			if err != nil {
				continue
			}
			*nodes = append(*nodes, verkle.SerializedNode{
				Path:            leafPath,
				SerializedBytes: leafSerialized,
			})
			seen[leafPathStr] = struct{}{}
			writtenPaths[leafPathStr] = struct{}{}
		}
	}

	// Serialize this node using go-verkle's Serialize()
	serialized, err := child.Serialize()
	if err != nil {
		// Skip nodes that can't be serialized (e.g., Empty, HashedNode)
		return
	}

	*nodes = append(*nodes, verkle.SerializedNode{
		Path:            path,
		SerializedBytes: serialized,
	})
	seen[pathStr] = struct{}{}
	writtenPaths[pathStr] = struct{}{}

	// Recurse into internal node children
	if isInternal {
		walkDirtyPath(inode.Children(), stem, depth+1, seen, writtenPaths, nodes)
	}
}
