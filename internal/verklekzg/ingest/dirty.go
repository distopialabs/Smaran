package ingest

import (
	"fmt"

	"github.com/nepal80m/samurai/internal/verklekzg/tree"
)

// serializeDirtyPaths serializes only the nodes along paths from the root to
// each modified stem. This is O(k * depth) where k = number of modified stems.
func serializeDirtyPaths(root *tree.InternalNode, dirtyStems map[string]struct{}) ([]tree.SerializedNode, []byte, error) {
	seen := make(map[string]struct{})
	var nodes []tree.SerializedNode

	rootSerialized, err := root.Serialize()
	if err != nil {
		return nil, nil, fmt.Errorf("serialize root: %w", err)
	}
	rootPath := []byte{}
	rootCommitment := tree.CommitmentBytes(root.Commitment())
	nodes = append(nodes, tree.SerializedNode{
		Path:            rootPath,
		SerializedBytes: rootSerialized,
	})
	seen[string(rootPath)] = struct{}{}

	children := root.Children()
	for stemStr := range dirtyStems {
		stem := []byte(stemStr)
		walkDirtyPath(children, stem, 0, seen, &nodes)
	}

	return nodes, rootCommitment, nil
}

func walkDirtyPath(children [tree.Width]tree.VerkleNode, stem []byte, depth byte, seen map[string]struct{}, nodes *[]tree.SerializedNode) {
	if int(depth) >= len(stem) {
		return
	}

	childIdx := stem[depth]
	child := children[childIdx]
	if child == nil {
		return
	}

	path := make([]byte, depth+1)
	copy(path, stem[:depth+1])
	pathStr := string(path)

	if _, ok := seen[pathStr]; ok {
		if inode, ok := child.(*tree.InternalNode); ok {
			walkDirtyPath(inode.Children(), stem, depth+1, seen, nodes)
		}
		return
	}

	serialized, err := child.Serialize()
	if err != nil {
		return
	}

	*nodes = append(*nodes, tree.SerializedNode{
		Path:            path,
		SerializedBytes: serialized,
	})
	seen[pathStr] = struct{}{}

	if inode, ok := child.(*tree.InternalNode); ok {
		walkDirtyPath(inode.Children(), stem, depth+1, seen, nodes)
	}
}
