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
// Returns the serialized nodes and the root commitment bytes.
func serializeDirtyPaths(root *verkle.InternalNode, dirtyStems map[string]struct{}) ([]verkle.SerializedNode, []byte, error) {
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

	// Walk each dirty stem's path from root to leaf
	children := root.Children()
	for stemStr := range dirtyStems {
		stem := []byte(stemStr)
		walkDirtyPath(children, stem, 0, seen, &nodes)
	}

	return nodes, rootCommitment[:], nil
}

// walkDirtyPath walks the tree from a given level of children, following the
// stem bytes, serializing each node encountered using go-verkle's Serialize().
// Uses seen map to avoid serializing the same node twice when multiple dirty
// stems share a prefix.
func walkDirtyPath(children []verkle.VerkleNode, stem []byte, depth byte, seen map[string]struct{}, nodes *[]verkle.SerializedNode) {
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
			walkDirtyPath(inode.Children(), stem, depth+1, seen, nodes)
		}
		return
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

	// Recurse into internal node children
	if inode, ok := child.(*verkle.InternalNode); ok {
		walkDirtyPath(inode.Children(), stem, depth+1, seen, nodes)
	}
}
