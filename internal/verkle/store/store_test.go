package store

import (
	"os"
	"testing"

	verklelib "github.com/ethereum/go-verkle"
)

func testKVStore(t *testing.T, backend string) {
	dir, err := os.MkdirTemp("", "store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	kv, err := OpenKVStore(backend, dir+"/db")
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	// Test not found
	_, err = kv.Get([]byte("missing"))
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Test put and get
	if err := kv.Put([]byte("key1"), []byte("val1")); err != nil {
		t.Fatal(err)
	}
	val, err := kv.Get([]byte("key1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "val1" {
		t.Fatalf("expected val1, got %s", val)
	}

	// Test batch
	batch := kv.NewBatch()
	batch.Put([]byte("key2"), []byte("val2"))
	batch.Put([]byte("key3"), []byte("val3"))
	if err := batch.Commit(); err != nil {
		t.Fatal(err)
	}
	val, err = kv.Get([]byte("key2"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "val2" {
		t.Fatalf("expected val2, got %s", val)
	}

	// Test delete
	if err := kv.Delete([]byte("key1")); err != nil {
		t.Fatal(err)
	}
	_, err = kv.Get([]byte("key1"))
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestPebbleKVStore(t *testing.T) {
	testKVStore(t, "pebble")
}

func TestLevelDBKVStore(t *testing.T) {
	testKVStore(t, "leveldb")
}

func testNodeStoreMetadata(t *testing.T, backend string) {
	dir, err := os.MkdirTemp("", "ns-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	kv, err := OpenKVStore(backend, dir+"/db")
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	ns := NewNodeStore(kv)

	// Test last processed (unset)
	_, ok := ns.GetLastProcessed()
	if ok {
		t.Fatal("expected no last processed")
	}

	// Set and get
	if err := ns.SetLastProcessed(100); err != nil {
		t.Fatal(err)
	}
	last, ok := ns.GetLastProcessed()
	if !ok || last != 100 {
		t.Fatalf("expected 100, got %d", last)
	}

	// Root commitment
	root := []byte("test-root-commitment-32bytes!!")
	if err := ns.PutRootCommitment(100, root); err != nil {
		t.Fatal(err)
	}
	got, err := ns.GetRootCommitment(100)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(root) {
		t.Fatalf("root mismatch: %s vs %s", got, root)
	}

	// Start block
	if err := ns.SetStartBlock(18908895); err != nil {
		t.Fatal(err)
	}
	start, ok := ns.GetStartBlock()
	if !ok || start != 18908895 {
		t.Fatalf("expected 18908895, got %d", start)
	}
}

func TestNodeStoreMetadataPebble(t *testing.T) {
	testNodeStoreMetadata(t, "pebble")
}

func TestNodeStoreMetadataLevelDB(t *testing.T) {
	testNodeStoreMetadata(t, "leveldb")
}

func testNodeStoreSaveLoadRoundTrip(t *testing.T, backend string) {
	dir, err := os.MkdirTemp("", "ns-roundtrip-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	kv, err := OpenKVStore(backend, dir+"/db")
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()

	ns := NewNodeStore(kv)

	// Build a small Verkle tree
	tree := verklelib.New()
	for i := byte(1); i <= 5; i++ {
		key := [32]byte{}
		key[0] = i
		val := [32]byte{}
		val[0] = i * 10
		if err := tree.Insert(key[:], val[:], nil); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	tree.Commit()

	// Serialize and save
	inode, ok := tree.(*verklelib.InternalNode)
	if !ok {
		t.Fatal("root is not InternalNode")
	}
	serialized, err := inode.BatchSerialize()
	if err != nil {
		t.Fatalf("BatchSerialize: %v", err)
	}
	if len(serialized) == 0 {
		t.Fatal("BatchSerialize returned no nodes")
	}

	// Use root's CommitmentBytes as the block's root commitment (for metadata)
	rootCommitment := serialized[0].CommitmentBytes[:]
	blockNum := uint64(42)
	if err := ns.SaveBlockState(serialized, blockNum, rootCommitment); err != nil {
		t.Fatalf("SaveBlockState: %v", err)
	}

	// Load the tree back (root node is at the empty path)
	loaded, err := ns.LoadTree(blockNum)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}

	// Verify we can read values from the loaded tree via resolver
	resolver := ns.NodeResolverFn()
	for i := byte(1); i <= 5; i++ {
		key := [32]byte{}
		key[0] = i
		val, err := loaded.Get(key[:], resolver)
		if err != nil {
			t.Fatalf("Get key %d: %v", i, err)
		}
		if val == nil || val[0] != i*10 {
			t.Fatalf("value mismatch for key %d: got %v", i, val)
		}
	}

	// Verify last processed was updated
	last, ok := ns.GetLastProcessed()
	if !ok || last != blockNum {
		t.Fatalf("expected last processed %d, got %d (ok=%v)", blockNum, last, ok)
	}

	// Verify root commitment was stored
	storedRoot, err := ns.GetRootCommitment(blockNum)
	if err != nil {
		t.Fatalf("GetRootCommitment: %v", err)
	}
	if string(storedRoot) != string(rootCommitment) {
		t.Fatalf("root commitment mismatch")
	}
}

func TestNodeStoreSaveLoadPebble(t *testing.T) {
	testNodeStoreSaveLoadRoundTrip(t, "pebble")
}

func TestNodeStoreSaveLoadLevelDB(t *testing.T) {
	testNodeStoreSaveLoadRoundTrip(t, "leveldb")
}
