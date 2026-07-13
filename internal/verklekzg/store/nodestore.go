// Package store provides versioned persistence for the Verkle-KZG trie,
// re-using the KVStore / Batch / Iterator abstractions from internal/verkle/store.
package store

import (
	"encoding/binary"
	"fmt"

	"github.com/nepal80m/samurai/internal/verklekzg/tree"
	verklestore "github.com/nepal80m/samurai/internal/verkle/store"
)

// Re-export the generic store helpers so callers only need one import.
var (
	OpenKVStore = verklestore.OpenKVStore
	ErrNotFound = verklestore.ErrNotFound
)

// Type aliases.
type (
	KVStore  = verklestore.KVStore
	Batch    = verklestore.Batch
	Iterator = verklestore.Iterator
)

// Key prefixes.
var (
	prefixRoot  = []byte("meta:root:")
	prefixLast  = []byte("meta:last")
	prefixStart = []byte("meta:start")
	prefixVNode = []byte("vn:")
)

// NodeStore wraps a KVStore and provides Verkle-KZG node persistence
// with versioned storage and metadata operations.
type NodeStore struct {
	KV KVStore
}

func NewNodeStore(kv KVStore) *NodeStore {
	return &NodeStore{KV: kv}
}

func versionedNodeKey(path []byte, blockNum uint64) []byte {
	key := make([]byte, 3+1+len(path)+8)
	copy(key, prefixVNode)
	key[3] = byte(len(path))
	copy(key[4:], path)
	binary.BigEndian.PutUint64(key[4+len(path):], blockNum)
	return key
}

func versionedNodePrefix(path []byte) []byte {
	prefix := make([]byte, 3+1+len(path))
	copy(prefix, prefixVNode)
	prefix[3] = byte(len(path))
	copy(prefix[4:], path)
	return prefix
}

func (ns *NodeStore) SaveBlockState(nodes []tree.SerializedNode, blockNum uint64, rootCommitment []byte) error {
	batch := ns.KV.NewBatch()
	for _, sn := range nodes {
		if err := batch.Put(versionedNodeKey(sn.Path, blockNum), sn.SerializedBytes); err != nil {
			return fmt.Errorf("nodestore: batch put node: %w", err)
		}
	}
	if err := batch.Put(blockKey(blockNum), rootCommitment); err != nil {
		return fmt.Errorf("nodestore: batch put root: %w", err)
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockNum)
	if err := batch.Put(prefixLast, buf[:]); err != nil {
		return fmt.Errorf("nodestore: batch put last: %w", err)
	}
	return batch.Commit()
}

func (ns *NodeStore) SaveNodes(nodes []tree.SerializedNode, blockNum uint64) error {
	batch := ns.KV.NewBatch()
	for _, sn := range nodes {
		if err := batch.Put(versionedNodeKey(sn.Path, blockNum), sn.SerializedBytes); err != nil {
			return fmt.Errorf("nodestore: batch put: %w", err)
		}
	}
	return batch.Commit()
}

func (ns *NodeStore) resolveNodeAtBlock(path []byte, blockNum uint64) ([]byte, error) {
	upperBound := versionedNodeKey(path, blockNum+1)
	prefix := versionedNodePrefix(path)

	iter := ns.KV.NewIterator()
	defer iter.Close()

	if !iter.SeekLT(upperBound) {
		return nil, fmt.Errorf("node not found at path %x block %d", path, blockNum)
	}

	key := iter.Key()
	if len(key) < len(prefix) {
		return nil, fmt.Errorf("node not found at path %x block %d", path, blockNum)
	}
	for i := 0; i < len(prefix); i++ {
		if key[i] != prefix[i] {
			return nil, fmt.Errorf("node not found at path %x block %d", path, blockNum)
		}
	}

	val := iter.Value()
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (ns *NodeStore) LoadTree(blockNum uint64) (*tree.InternalNode, error) {
	serialized, err := ns.resolveNodeAtBlock(nil, blockNum)
	if err != nil {
		return nil, fmt.Errorf("load root for block %d: %w", blockNum, err)
	}
	node, err := tree.ParseNode(serialized, 0)
	if err != nil {
		return nil, fmt.Errorf("parse root for block %d: %w", blockNum, err)
	}
	root, ok := node.(*tree.InternalNode)
	if !ok {
		return nil, fmt.Errorf("root at block %d is not InternalNode", blockNum)
	}
	return root, nil
}

func (ns *NodeStore) VersionedNodeResolverFn(blockNum uint64) tree.NodeResolverFn {
	return func(path []byte) ([]byte, error) {
		return ns.resolveNodeAtBlock(path, blockNum)
	}
}

func (ns *NodeStore) NodeResolverFn() tree.NodeResolverFn {
	return ns.VersionedNodeResolverFn(^uint64(0) - 1)
}

func blockKey(blockNum uint64) []byte {
	key := make([]byte, len(prefixRoot)+8)
	copy(key, prefixRoot)
	binary.BigEndian.PutUint64(key[len(prefixRoot):], blockNum)
	return key
}

func (ns *NodeStore) PutRootCommitment(blockNum uint64, root []byte) error {
	return ns.KV.Put(blockKey(blockNum), root)
}

func (ns *NodeStore) GetRootCommitment(blockNum uint64) ([]byte, error) {
	return ns.KV.Get(blockKey(blockNum))
}

func (ns *NodeStore) SaveBlockMetadata(blockNum uint64, rootCommitment []byte) error {
	batch := ns.KV.NewBatch()
	if err := batch.Put(blockKey(blockNum), rootCommitment); err != nil {
		return fmt.Errorf("nodestore: batch put root: %w", err)
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockNum)
	if err := batch.Put(prefixLast, buf[:]); err != nil {
		return fmt.Errorf("nodestore: batch put last: %w", err)
	}
	return batch.Commit()
}

func (ns *NodeStore) SetLastProcessed(blockNum uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockNum)
	return ns.KV.Put(prefixLast, buf[:])
}

func (ns *NodeStore) GetLastProcessed() (uint64, bool) {
	val, err := ns.KV.Get(prefixLast)
	if err != nil || len(val) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(val), true
}

func (ns *NodeStore) SetStartBlock(blockNum uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockNum)
	return ns.KV.Put(prefixStart, buf[:])
}

func (ns *NodeStore) GetStartBlock() (uint64, bool) {
	val, err := ns.KV.Get(prefixStart)
	if err != nil || len(val) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(val), true
}
