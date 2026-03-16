package kt

import (
	"bytes"
	"fmt"
	"sync"
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
	s := NewOptiksServer(1)

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
	s := NewOptiksServer(1)

	s.Put([]byte("bob"), []byte("pk-1"))
	_ = s.GetCommitment()

	// Get generates proofs internally via Prove.
	result, err := s.Get([]byte("bob"), false)
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

	result2, err := s.Get([]byte("bob"), false)
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
	s := NewOptiksServer(1)

	s.Put([]byte("carol"), []byte("first-key"))
	commitment := s.GetCommitment()
	rootHash := common.BytesToHash(commitment)

	result, err := s.Get([]byte("carol"), false)
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
	s := NewOptiksServer(1)

	s.Put([]byte("dave"), []byte("v1-data"))
	s.Put([]byte("dave"), []byte("v2-data"))
	s.Put([]byte("dave"), []byte("v3-data"))

	commitment := s.GetCommitment()
	rootHash := common.BytesToHash(commitment)

	result, err := s.Get([]byte("dave"), false)
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
	s := NewOptiksServer(1)

	// Insert something so the trie is non-empty.
	s.Put([]byte("existing"), []byte("data"))
	_ = s.GetCommitment()

	result, err := s.Get([]byte("unknown"), false)
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
	s := NewOptiksServer(1)

	s.Put([]byte("alice"), []byte("a1"))
	s.Put([]byte("bob"), []byte("b1"))
	s.Put([]byte("alice"), []byte("a2"))

	_ = s.GetCommitment()

	rAlice, err := s.Get([]byte("alice"), false)
	if err != nil {
		t.Fatalf("Get alice: %v", err)
	}
	if rAlice.CurrentVersion != 2 {
		t.Fatalf("alice: expected version 2, got %d", rAlice.CurrentVersion)
	}

	rBob, err := s.Get([]byte("bob"), false)
	if err != nil {
		t.Fatalf("Get bob: %v", err)
	}
	if rBob.CurrentVersion != 1 {
		t.Fatalf("bob: expected version 1, got %d", rBob.CurrentVersion)
	}
}

// TestConcurrentPutGetCommitment exercises the RWMutex under high concurrency.
// Multiple writers call Put for distinct users while many readers call Get and
// GetCommitment in parallel. The test checks structural invariants on every
// read result and should be run with -race.
func TestConcurrentPutGetCommitment(t *testing.T) {
	const (
		numWriters       = 8
		numReaders       = 16
		putsPerWriter    = 200
		readsPerReader   = 300
		usersPerWriter   = 5
	)

	s := NewOptiksServer(0)

	var wg sync.WaitGroup

	// Writers: each goroutine owns a disjoint set of users and puts
	// multiple versions for each.
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < putsPerWriter; i++ {
				userIdx := i % usersPerWriter
				user := fmt.Sprintf("w%d-user%d", writerID, userIdx)
				key := fmt.Sprintf("key-w%d-u%d-v%d", writerID, userIdx, i)
				s.Put([]byte(user), []byte(key))
			}
		}(w)
	}

	// Readers: each goroutine reads random users and checks invariants.
	for r := 0; r < numReaders; r++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for i := 0; i < readsPerReader; i++ {
				writerID := i % numWriters
				userIdx := i % usersPerWriter
				user := fmt.Sprintf("w%d-user%d", writerID, userIdx)

				if i%3 == 0 {
					commitment := s.GetCommitment()
					if len(commitment) == 0 {
						t.Error("GetCommitment returned empty slice")
					}
					continue
				}

				useCaching := i%2 == 0
				result, err := s.Get([]byte(user), useCaching)
				if err != nil {
					t.Errorf("reader %d: Get(%s): %v", readerID, user, err)
					continue
				}

				n := result.CurrentVersion

				if n > 0 && result.Value == nil {
					t.Errorf("reader %d: user %s version %d but Value is nil", readerID, user, n)
				}

				if uint64(len(result.VersionProofs)) != n {
					t.Errorf("reader %d: user %s expected %d VersionProofs, got %d",
						readerID, user, n, len(result.VersionProofs))
				}

				if n > 0 && len(result.NextVersionNonMembershipProof) == 0 {
					t.Errorf("reader %d: user %s version %d has empty non-membership proof",
						readerID, user, n)
				}

				if n > 0 && result.CurrentVersionEpoch == 0 {
					t.Errorf("reader %d: user %s version %d but CurrentVersionEpoch is 0",
						readerID, user, n)
				}

				if useCaching {
					if len(result.OldVersions) != 0 {
						t.Errorf("reader %d: useCaching=true but OldVersions has %d entries",
							readerID, len(result.OldVersions))
					}
					if len(result.OldVersionEpochs) != 0 {
						t.Errorf("reader %d: useCaching=true but OldVersionEpochs has %d entries",
							readerID, len(result.OldVersionEpochs))
					}
				} else {
					if len(result.OldVersions) != len(result.OldVersionEpochs) {
						t.Errorf("reader %d: user %s OldVersions len %d != OldVersionEpochs len %d",
							readerID, user, len(result.OldVersions), len(result.OldVersionEpochs))
					}
					if n > 0 && uint64(len(result.OldVersions)) != n-1 {
						t.Errorf("reader %d: user %s version %d expected %d old versions, got %d",
							readerID, user, n, n-1, len(result.OldVersions))
					}
				}
			}
		}(r)
	}

	wg.Wait()

	// After all writers finish, flush and verify final state is consistent.
	commitment := s.GetCommitment()
	if bytes.Equal(commitment, types.EmptyRootHash.Bytes()) {
		t.Fatal("final commitment should differ from empty trie")
	}

	// Every user that was written should have the expected version count.
	for w := 0; w < numWriters; w++ {
		for u := 0; u < usersPerWriter; u++ {
			user := fmt.Sprintf("w%d-user%d", w, u)
			result, err := s.Get([]byte(user), false)
			if err != nil {
				t.Fatalf("final Get(%s): %v", user, err)
			}

			expectedVersions := uint64(putsPerWriter / usersPerWriter)
			if result.CurrentVersion != expectedVersions {
				t.Errorf("user %s: expected %d versions, got %d", user, expectedVersions, result.CurrentVersion)
			}

			if uint64(len(result.OldVersions)) != expectedVersions-1 {
				t.Errorf("user %s: expected %d old versions, got %d",
					user, expectedVersions-1, len(result.OldVersions))
			}

			// Epochs must be monotonically non-decreasing across old → current.
			for j := 1; j < len(result.OldVersionEpochs); j++ {
				if result.OldVersionEpochs[j] < result.OldVersionEpochs[j-1] {
					t.Errorf("user %s: old epoch[%d]=%d < epoch[%d]=%d",
						user, j, result.OldVersionEpochs[j], j-1, result.OldVersionEpochs[j-1])
				}
			}
			if len(result.OldVersionEpochs) > 0 {
				lastOld := result.OldVersionEpochs[len(result.OldVersionEpochs)-1]
				if result.CurrentVersionEpoch < lastOld {
					t.Errorf("user %s: current epoch %d < last old epoch %d",
						user, result.CurrentVersionEpoch, lastOld)
				}
			}
		}
	}
}
