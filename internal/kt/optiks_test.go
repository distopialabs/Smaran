package kt

import (
	"bytes"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/trie"
)

// TestMPTReusableAfterHash verifies that calling Hash() on the trie does not
// destroy it — subsequent Updates, Gets, and Hash calls still work.
// See KT.md § "Code structure": "Add a test to check if the Mpt is reusable."
func TestMPTReusableAfterHash(t *testing.T) {
	s := NewOptiksServer()

	// Insert first entry and flush.
	s.Put([]byte("alice"), []byte("key-v1"))
	hash1 := s.GetCommitment()

	if bytes.Equal(hash1, types.EmptyRootHash.Bytes()) {
		t.Fatal("root should differ from empty trie after inserting an entry")
	}

	// Insert a second entry — the trie must still be usable.
	s.Put([]byte("alice"), []byte("key-v2"))
	hash2 := s.GetCommitment()

	if bytes.Equal(hash1, hash2) {
		t.Fatal("root should change after a new update")
	}

	// A third call to GetCommitment with no new puts should return the same hash.
	hash3 := s.GetCommitment()
	if !bytes.Equal(hash2, hash3) {
		t.Fatalf("root should be stable: got %x then %x", hash2, hash3)
	}
}

// TestMPTReusableAfterProve verifies that calling Prove (via Get) does not
// destroy the trie — subsequent updates and proofs still succeed.
func TestMPTReusableAfterProve(t *testing.T) {
	s := NewOptiksServer()

	s.Put([]byte("bob"), []byte("pk-1"))
	_ = s.GetCommitment()

	// Get generates proofs internally via Prove.
	result, err := s.Get([]byte("bob"))
	if err != nil {
		t.Fatalf("Get after first put: %v", err)
	}
	if result.CurrentVersion != 1 {
		t.Fatalf("expected version 1, got %d", result.CurrentVersion)
	}

	// Trie must still accept updates after proving.
	s.Put([]byte("bob"), []byte("pk-2"))
	hash := s.GetCommitment()
	if len(hash) == 0 {
		t.Fatal("commitment should not be empty")
	}

	result2, err := s.Get([]byte("bob"))
	if err != nil {
		t.Fatalf("Get after second put: %v", err)
	}
	if result2.CurrentVersion != 2 {
		t.Fatalf("expected version 2, got %d", result2.CurrentVersion)
	}
	if !bytes.Equal(result2.Value, []byte("pk-2")) {
		t.Fatalf("expected value pk-2, got %s", result2.Value)
	}
}

// TestNonMembershipProofVerifies checks that the non-membership proof for
// version (n+1) can be verified using trie.VerifyProof.
func TestNonMembershipProofVerifies(t *testing.T) {
	s := NewOptiksServer()

	s.Put([]byte("carol"), []byte("first-key"))
	commitment := s.GetCommitment()
	rootHash := common.BytesToHash(commitment)

	result, err := s.Get([]byte("carol"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// The non-membership proof is for version 2 (n=1, so n+1=2).
	nonExistKey := makeTrieKey([]byte("carol"), result.CurrentVersion+1)

	// Reconstruct the proof DB for verification.
	proofDB := memorydb.New()
	for _, node := range result.NextVersionNonMembershipProof {
		h := crypto.Keccak256(node)
		if err := proofDB.Put(h, node); err != nil {
			t.Fatalf("proofDB.Put: %v", err)
		}
	}

	// VerifyProof returns (nil, nil) for a valid non-membership proof.
	val, err := trie.VerifyProof(rootHash, nonExistKey, proofDB)
	if err != nil {
		t.Fatalf("VerifyProof for non-membership failed: %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil value for non-membership proof, got %x", val)
	}
}

// TestMembershipProofVerifies checks that each version's membership proof can
// be verified using trie.VerifyProof.
func TestMembershipProofVerifies(t *testing.T) {
	s := NewOptiksServer()

	s.Put([]byte("dave"), []byte("v1-data"))
	s.Put([]byte("dave"), []byte("v2-data"))
	s.Put([]byte("dave"), []byte("v3-data"))

	commitment := s.GetCommitment()
	rootHash := common.BytesToHash(commitment)

	result, err := s.Get([]byte("dave"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if result.CurrentVersion != 3 {
		t.Fatalf("expected version 3, got %d", result.CurrentVersion)
	}
	if len(result.VersionProofs) != 3 {
		t.Fatalf("expected 3 version proofs, got %d", len(result.VersionProofs))
	}

	for i, proof := range result.VersionProofs {
		version := uint64(i + 1)
		trieKey := makeTrieKey([]byte("dave"), version)

		proofDB := memorydb.New()
		for _, node := range proof {
			h := crypto.Keccak256(node)
			if err := proofDB.Put(h, node); err != nil {
				t.Fatalf("version %d: proofDB.Put: %v", version, err)
			}
		}

		val, err := trie.VerifyProof(rootHash, trieKey, proofDB)
		if err != nil {
			t.Fatalf("version %d: VerifyProof failed: %v", version, err)
		}
		if val == nil {
			t.Fatalf("version %d: expected non-nil leaf value", version)
		}
	}
}

// TestGetUnknownUser verifies that querying a non-existent user returns
// version 0, nil value, and a valid non-membership proof.
func TestGetUnknownUser(t *testing.T) {
	s := NewOptiksServer()

	// Insert something so the trie is non-empty.
	s.Put([]byte("existing"), []byte("data"))
	_ = s.GetCommitment()

	result, err := s.Get([]byte("unknown"))
	if err != nil {
		t.Fatalf("Get unknown user: %v", err)
	}
	if result.CurrentVersion != 0 {
		t.Fatalf("expected version 0, got %d", result.CurrentVersion)
	}
	if result.Value != nil {
		t.Fatalf("expected nil value, got %x", result.Value)
	}
	if len(result.NextVersionNonMembershipProof) == 0 {
		t.Fatal("expected non-empty non-membership proof")
	}
	if len(result.VersionProofs) != 0 {
		t.Fatalf("expected 0 version proofs, got %d", len(result.VersionProofs))
	}
}

// TestMultipleUsersIndependent verifies that version tracking is per-user.
func TestMultipleUsersIndependent(t *testing.T) {
	s := NewOptiksServer()

	s.Put([]byte("alice"), []byte("a1"))
	s.Put([]byte("bob"), []byte("b1"))
	s.Put([]byte("alice"), []byte("a2"))

	_ = s.GetCommitment()

	rAlice, err := s.Get([]byte("alice"))
	if err != nil {
		t.Fatalf("Get alice: %v", err)
	}
	if rAlice.CurrentVersion != 2 {
		t.Fatalf("alice: expected version 2, got %d", rAlice.CurrentVersion)
	}

	rBob, err := s.Get([]byte("bob"))
	if err != nil {
		t.Fatalf("Get bob: %v", err)
	}
	if rBob.CurrentVersion != 1 {
		t.Fatalf("bob: expected version 1, got %d", rBob.CurrentVersion)
	}
}
