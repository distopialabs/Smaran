package ingest

import (
	"os"
	"testing"

	"github.com/nepal80m/samurai/internal/verkle/store"
	verkle "github.com/ethereum/go-verkle"
)

// TestSerializeDirtyPathsLeafSplit reproduces the moved-leaf hole: inserting a
// key whose stem shares a prefix with an existing leaf splits that leaf one
// level deeper. The old leaf's stem is not dirty in the splitting block, so
// the dirty-path walk alone never persists it at its new path, and
// resolveNodeAtBlock fails for every block from the split until the node's
// next write. serializeDirtyPaths must persist the re-parented leaf at the
// split block.
func TestSerializeDirtyPathsLeafSplit(t *testing.T) {
	dir, err := os.MkdirTemp("", "dirty-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	kv, err := store.OpenKVStore("pebble", dir+"/db")
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	ns := store.NewNodeStore(kv)

	// Two stems sharing a 3-byte prefix, diverging at byte 3.
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	prefix := []byte{0xf2, 0x02, 0x7e}
	copy(key1, prefix)
	copy(key2, prefix)
	key1[3] = 0x11
	key2[3] = 0xfb
	val := make([]byte, 32)
	val[31] = 1

	writtenPaths := make(map[string]struct{})

	// Block 1: insert key1 only — its leaf attaches at depth 1 (path f2).
	root := verkle.New()
	if err := root.Insert(key1, val, nil); err != nil {
		t.Fatal(err)
	}
	root.Commit()
	iroot := root.(*verkle.InternalNode)
	nodes, rc, err := serializeDirtyPaths(iroot, map[string]struct{}{string(key1[:31]): {}}, writtenPaths)
	if err != nil {
		t.Fatal(err)
	}
	if err := ns.SaveBlockState(nodes, 1, rc); err != nil {
		t.Fatal(err)
	}

	// Block 2: insert key2 — splits key1's leaf down to depth 4
	// (internals at f2, f202, f2027e; leaves at f2027e11 and f2027efb).
	if err := root.Insert(key2, val, nil); err != nil {
		t.Fatal(err)
	}
	root.Commit()
	nodes, rc, err = serializeDirtyPaths(iroot, map[string]struct{}{string(key2[:31]): {}}, writtenPaths)
	if err != nil {
		t.Fatal(err)
	}
	if err := ns.SaveBlockState(nodes, 2, rc); err != nil {
		t.Fatal(err)
	}

	// The moved leaf must resolve at its new path as of block 2.
	movedLeafPath := []byte{0xf2, 0x02, 0x7e, 0x11}
	got, err := ns.VersionedNodeResolverFn(2)(movedLeafPath)
	if err != nil {
		t.Fatalf("moved leaf not resolvable at block 2 (path %x): %v", movedLeafPath, err)
	}
	node, err := verkle.ParseNode(got, byte(len(movedLeafPath)))
	if err != nil {
		t.Fatalf("parse moved leaf: %v", err)
	}
	if _, ok := node.(*verkle.LeafNode); !ok {
		t.Fatalf("expected LeafNode at %x, got %T", movedLeafPath, node)
	}

	// The new leaf and the split-created internal must resolve too.
	for _, p := range [][]byte{{0xf2, 0x02, 0x7e, 0xfb}, {0xf2, 0x02, 0x7e}, {0xf2, 0x02}} {
		if _, err := ns.VersionedNodeResolverFn(2)(p); err != nil {
			t.Fatalf("path %x not resolvable at block 2: %v", p, err)
		}
	}

	// Pre-split state must be untouched: at block 1 the old leaf still
	// resolves at its original depth-1 path.
	if _, err := ns.VersionedNodeResolverFn(1)([]byte{0xf2}); err != nil {
		t.Fatalf("original leaf path f2 not resolvable at block 1: %v", err)
	}
}

// TestSerializeDirtyPathsLeafSplitAtLeafDepth covers the split where the two
// stems diverge at exactly the old leaf's resident depth: the split-created
// internal then REPLACES the leaf at its previously-written path, and the
// moved leaf hangs directly beneath it.
func TestSerializeDirtyPathsLeafSplitAtLeafDepth(t *testing.T) {
	dir, err := os.MkdirTemp("", "dirty-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	kv, err := store.OpenKVStore("pebble", dir+"/db")
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	ns := store.NewNodeStore(kv)

	// Shared first byte, divergence at byte 1 — the depth where the first
	// leaf sits after block 1.
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key1[0], key2[0] = 0xf2, 0xf2
	key1[1] = 0x11
	key2[1] = 0xfb
	val := make([]byte, 32)
	val[31] = 1

	writtenPaths := make(map[string]struct{})

	root := verkle.New()
	if err := root.Insert(key1, val, nil); err != nil {
		t.Fatal(err)
	}
	root.Commit()
	iroot := root.(*verkle.InternalNode)
	nodes, rc, err := serializeDirtyPaths(iroot, map[string]struct{}{string(key1[:31]): {}}, writtenPaths)
	if err != nil {
		t.Fatal(err)
	}
	if err := ns.SaveBlockState(nodes, 1, rc); err != nil {
		t.Fatal(err)
	}

	if err := root.Insert(key2, val, nil); err != nil {
		t.Fatal(err)
	}
	root.Commit()
	nodes, rc, err = serializeDirtyPaths(iroot, map[string]struct{}{string(key2[:31]): {}}, writtenPaths)
	if err != nil {
		t.Fatal(err)
	}
	if err := ns.SaveBlockState(nodes, 2, rc); err != nil {
		t.Fatal(err)
	}

	// Moved leaf at its new depth-2 path, and the internal that replaced it
	// at the old depth-1 path, must both resolve as of block 2.
	movedLeafPath := []byte{0xf2, 0x11}
	got, err := ns.VersionedNodeResolverFn(2)(movedLeafPath)
	if err != nil {
		t.Fatalf("moved leaf not resolvable at block 2 (path %x): %v", movedLeafPath, err)
	}
	node, err := verkle.ParseNode(got, byte(len(movedLeafPath)))
	if err != nil {
		t.Fatalf("parse moved leaf: %v", err)
	}
	if _, ok := node.(*verkle.LeafNode); !ok {
		t.Fatalf("expected LeafNode at %x, got %T", movedLeafPath, node)
	}
	got, err = ns.VersionedNodeResolverFn(2)([]byte{0xf2})
	if err != nil {
		t.Fatalf("split internal not resolvable at block 2: %v", err)
	}
	node, err = verkle.ParseNode(got, 1)
	if err != nil {
		t.Fatalf("parse split internal: %v", err)
	}
	if _, ok := node.(*verkle.InternalNode); !ok {
		t.Fatalf("expected InternalNode at f2 block 2, got %T", node)
	}
}
