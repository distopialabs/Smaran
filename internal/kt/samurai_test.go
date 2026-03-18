package kt

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

const testParamsDir = "./testdata/params"

func setupTestParamsDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(testParamsDir, 0755); err != nil {
		t.Fatalf("failed to create test params dir: %v", err)
	}
}

func newTestSamuraiServer(t *testing.T, batchSize uint64) *SamuraiKTServer {
	t.Helper()
	setupTestParamsDir(t)
	return NewSamuraiKTServer(batchSize, testParamsDir)
}

func TestSamuraiMPTReusableAfterHash(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

	s.Put([]byte("alice"), []byte("key-v1"))
	hash1 := s.GetCommitment()

	if bytes.Equal(hash1, types.EmptyRootHash.Bytes()) {
		t.Fatal("root should differ from empty trie after inserting an entry")
	}

	s.Put([]byte("alice"), []byte("key-v2"))
	hash2 := s.GetCommitment()

	if bytes.Equal(hash1, hash2) {
		t.Fatal("root should change after a new update")
	}

	hash3 := s.GetCommitment()
	if !bytes.Equal(hash2, hash3) {
		t.Fatalf("root should be stable: got %x then %x", hash2, hash3)
	}
}

func TestSamuraiMPTReusableAfterProve(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

	s.Put([]byte("bob"), []byte("pk-1"))
	_ = s.GetCommitment()

	result, err := s.Get([]byte("bob"))
	if err != nil {
		t.Fatalf("Get after first put: %v", err)
	}
	if result.CurrentVersion != 1 {
		t.Fatalf("expected version 1, got %d", result.CurrentVersion)
	}

	s.Put([]byte("bob"), []byte("pk-2"))
	h := s.GetCommitment()
	if len(h) == 0 {
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

func TestSamuraiMptProofVerifies(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

	s.Put([]byte("carol"), []byte("first-key"))
	commitment := s.GetCommitment()
	rootHash := common.BytesToHash(commitment)

	result, err := s.Get([]byte("carol"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	trieKey := makeSamuraiTrieKey([]byte("carol"))
	proofDB := memorydb.New()
	for _, node := range result.MptProof {
		h := crypto.Keccak256(node)
		if err := proofDB.Put(h, node); err != nil {
			t.Fatalf("proofDB.Put: %v", err)
		}
	}

	val, err := trie.VerifyProof(rootHash, trieKey, proofDB)
	if err != nil {
		t.Fatalf("VerifyProof failed: %v", err)
	}
	if val == nil {
		t.Fatal("expected non-nil leaf value for membership proof")
	}

	var decoded []byte
	if err := rlp.DecodeBytes(val, &decoded); err != nil {
		t.Fatalf("RLP decode failed: %v", err)
	}
	if !bytes.Equal(decoded, result.CommitmentHash) {
		t.Fatalf("commitment hash mismatch: proof returned %x, result has %x", decoded, result.CommitmentHash)
	}
}

func TestSamuraiGetUnknownUser(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

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
	if len(result.MptProof) == 0 {
		t.Fatal("expected non-empty MPT proof (non-membership)")
	}
	if len(result.SamuraiProofs) != 0 {
		t.Fatalf("expected 0 samurai proofs, got %d", len(result.SamuraiProofs))
	}
}

func TestSamuraiMultipleUsersIndependent(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

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

func TestSamuraiConcurrentPutGetCommitment(t *testing.T) {
	const (
		numWriters     = 4
		putsPerWriter  = 50
		usersPerWriter = 3
		numCommitReaders = 8
		readsPerReader   = 50
	)

	s := newTestSamuraiServer(t, 0)

	var wg sync.WaitGroup

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

	for r := 0; r < numCommitReaders; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < readsPerReader; i++ {
				commitment := s.GetCommitment()
				if len(commitment) == 0 {
					t.Error("GetCommitment returned empty slice")
				}
			}
		}()
	}

	wg.Wait()

	commitment := s.GetCommitment()
	if bytes.Equal(commitment, types.EmptyRootHash.Bytes()) {
		t.Fatal("final commitment should differ from empty trie")
	}

	for w := 0; w < numWriters; w++ {
		for u := 0; u < usersPerWriter; u++ {
			user := fmt.Sprintf("w%d-user%d", w, u)
			result, err := s.Get([]byte(user))
			if err != nil {
				t.Fatalf("final Get(%s): %v", user, err)
			}
			if result.CurrentVersion == 0 {
				t.Errorf("user %s: expected non-zero version count", user)
			}
		}
	}
}
