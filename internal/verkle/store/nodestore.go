package store

import (
	"encoding/binary"
	"fmt"

	verkle "github.com/ethereum/go-verkle"
)

// Key prefixes for metadata and versioned nodes.
var (
	prefixRoot  = []byte("meta:root:")
	prefixLast  = []byte("meta:last")
	prefixStart = []byte("meta:start")
	prefixVNode = []byte("vn:")
)

// versionedNodeKey creates a key for a specific version of a node.
// Format: "vn:" + pathLen(1 byte) + path + blockNum(8 bytes BE)
//
// This creates lexicographic ordering where:
//   - All versions of the same path are grouped together
//   - Within a path, versions sort by block number ascending
func versionedNodeKey(path []byte, blockNum uint64) []byte {
	key := make([]byte, 3+1+len(path)+8)
	copy(key, prefixVNode)
	key[3] = byte(len(path))
	copy(key[4:], path)
	binary.BigEndian.PutUint64(key[4+len(path):], blockNum)
	return key
}

// versionedNodePrefix returns the prefix for all versions of a given path.
// Format: "vn:" + pathLen(1 byte) + path
func versionedNodePrefix(path []byte) []byte {
	prefix := make([]byte, 3+1+len(path))
	copy(prefix, prefixVNode)
	prefix[3] = byte(len(path))
	copy(prefix[4:], path)
	return prefix
}

// NodeStore wraps a KVStore and provides Verkle tree node
// serialization/deserialization with versioned storage and metadata operations.
type NodeStore struct {
	KV KVStore
}

// NewNodeStore wraps a KVStore into a NodeStore.
func NewNodeStore(kv KVStore) *NodeStore {
	return &NodeStore{KV: kv}
}

// SaveBlockState atomically persists versioned tree nodes, root commitment,
// and last processed block number in a single batch write.
func (ns *NodeStore) SaveBlockState(nodes []verkle.SerializedNode, blockNum uint64, rootCommitment []byte) error {
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

// SaveNodes persists versioned serialized nodes from BatchSerialize.
func (ns *NodeStore) SaveNodes(nodes []verkle.SerializedNode, blockNum uint64) error {
	batch := ns.KV.NewBatch()
	for _, sn := range nodes {
		if err := batch.Put(versionedNodeKey(sn.Path, blockNum), sn.SerializedBytes); err != nil {
			return fmt.Errorf("nodestore: batch put: %w", err)
		}
	}
	return batch.Commit()
}

// resolveNodeAtBlock finds the latest version of a node at the given path
// that was written at or before the target block number.
// Uses SeekLT on key "vn:<pathLen><path><(blockNum+1)_BE>" to find
// the largest key strictly less than blockNum+1, then verifies the path prefix.
func (ns *NodeStore) resolveNodeAtBlock(path []byte, blockNum uint64) ([]byte, error) {
	// Build the upper bound: one past the target block
	upperBound := versionedNodeKey(path, blockNum+1)
	prefix := versionedNodePrefix(path)

	iter := ns.KV.NewIterator()
	defer iter.Close()

	if !iter.SeekLT(upperBound) {
		return nil, fmt.Errorf("node not found at path %x block %d", path, blockNum)
	}

	// Verify the found key has the expected path prefix
	key := iter.Key()
	if len(key) < len(prefix) {
		return nil, fmt.Errorf("node not found at path %x block %d", path, blockNum)
	}
	for i := 0; i < len(prefix); i++ {
		if key[i] != prefix[i] {
			return nil, fmt.Errorf("node not found at path %x block %d", path, blockNum)
		}
	}

	// Copy the value (iterator may invalidate on close)
	val := iter.Value()
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

// LoadTree loads the Verkle tree at a specific block number from versioned storage.
// Returns the root VerkleNode that will lazily load children via VersionedNodeResolverFn.
func (ns *NodeStore) LoadTree(blockNum uint64) (verkle.VerkleNode, error) {
	serialized, err := ns.resolveNodeAtBlock(nil, blockNum)
	if err != nil {
		return nil, fmt.Errorf("load root node for block %d: %w", blockNum, err)
	}

	root, err := verkle.ParseNode(serialized, 0)
	if err != nil {
		return nil, fmt.Errorf("parse root node for block %d: %w", blockNum, err)
	}

	return root, nil
}

// VersionedNodeResolverFn returns a resolver that reads the correct version
// of nodes for a specific block number. Each resolved node is the latest
// version written at or before the target block.
func (ns *NodeStore) VersionedNodeResolverFn(blockNum uint64) verkle.NodeResolverFn {
	return func(path []byte) ([]byte, error) {
		return ns.resolveNodeAtBlock(path, blockNum)
	}
}

// NodeResolverFn returns a resolver that reads the latest version of nodes
// (equivalent to VersionedNodeResolverFn with the highest block number).
// Used during ingestion where we always want the latest state.
func (ns *NodeStore) NodeResolverFn() verkle.NodeResolverFn {
	return ns.VersionedNodeResolverFn(^uint64(0) - 1) // max uint64 - 1
}

// --- Metadata helpers ---

func blockKey(blockNum uint64) []byte {
	key := make([]byte, len(prefixRoot)+8)
	copy(key, prefixRoot)
	binary.BigEndian.PutUint64(key[len(prefixRoot):], blockNum)
	return key
}

// PutRootCommitment stores the root commitment for a given block number.
func (ns *NodeStore) PutRootCommitment(blockNum uint64, root []byte) error {
	return ns.KV.Put(blockKey(blockNum), root)
}

// GetRootCommitment retrieves the root commitment for a given block number.
func (ns *NodeStore) GetRootCommitment(blockNum uint64) ([]byte, error) {
	return ns.KV.Get(blockKey(blockNum))
}

// SaveBlockMetadata atomically saves root commitment and last processed block.
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

// SetLastProcessed stores the last processed block number.
func (ns *NodeStore) SetLastProcessed(blockNum uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockNum)
	return ns.KV.Put(prefixLast, buf[:])
}

// GetLastProcessed returns the last processed block number, or false if unset.
func (ns *NodeStore) GetLastProcessed() (uint64, bool) {
	val, err := ns.KV.Get(prefixLast)
	if err != nil || len(val) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(val), true
}

// SetStartBlock stores the start block number.
func (ns *NodeStore) SetStartBlock(blockNum uint64) error {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], blockNum)
	return ns.KV.Put(prefixStart, buf[:])
}

// GetStartBlock returns the start block number, or false if unset.
func (ns *NodeStore) GetStartBlock() (uint64, bool) {
	val, err := ns.KV.Get(prefixStart)
	if err != nil || len(val) < 8 {
		return 0, false
	}
	return binary.BigEndian.Uint64(val), true
}
