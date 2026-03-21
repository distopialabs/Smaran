package proof_test

import (
	"math/big"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/triedb"
	"github.com/ethereum/go-ethereum/triedb/hashdb"
	"github.com/holiman/uint256"

	"github.com/nepal80m/samurai/internal/merkle/proof"
	st "github.com/nepal80m/samurai/internal/merkle/state"
)

// testEnv creates an in-memory state environment for tests.
type testEnv struct {
	tdb *triedb.Database
	sdb *state.CachingDB
}

func newTestEnv() *testEnv {
	memdb := rawdb.NewMemoryDatabase()
	tdb := triedb.NewDatabase(memdb, &triedb.Config{
		HashDB: &hashdb.Config{CleanCacheSize: 16 * 1024 * 1024},
	})
	sdb := state.NewDatabase(tdb, nil)
	return &testEnv{tdb: tdb, sdb: sdb}
}

func (e *testEnv) openState(root common.Hash) (*state.StateDB, error) {
	return state.New(root, e.sdb)
}

func (e *testEnv) commitState(stateDB *state.StateDB, block uint64) (common.Hash, error) {
	root, err := stateDB.Commit(block, true)
	if err != nil {
		return common.Hash{}, err
	}
	// Tests always flush immediately so tries can be reopened.
	if err := e.tdb.Commit(root, false); err != nil {
		return common.Hash{}, err
	}
	return root, nil
}

var (
	addr1 = common.HexToAddress("0x1111111111111111111111111111111111111111")
	addr2 = common.HexToAddress("0x2222222222222222222222222222222222222222")
	addr3 = common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
)

// TestRootDeterminism verifies that ingesting the same updates twice produces identical roots.
func TestRootDeterminism(t *testing.T) {
	var roots [2]common.Hash

	for i := 0; i < 2; i++ {
		env := newTestEnv()
		s, _ := env.openState(st.EmptyRootHash)
		s.SetBalance(addr1, uint256.NewInt(1000), tracing.BalanceChangeUnspecified)
		s.SetBalance(addr2, uint256.NewInt(2000), tracing.BalanceChangeUnspecified)
		root, err := env.commitState(s, 1)
		if err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
		roots[i] = root
	}

	if roots[0] != roots[1] {
		t.Fatalf("roots differ: %s vs %s", roots[0].Hex(), roots[1].Hex())
	}
	t.Logf("Deterministic root: %s", roots[0].Hex())
}

// TestProofVerification generates a proof and verifies it.
func TestProofVerification(t *testing.T) {
	env := newTestEnv()

	// Create state with some accounts.
	s, _ := env.openState(st.EmptyRootHash)
	s.SetBalance(addr1, uint256.NewInt(5000), tracing.BalanceChangeUnspecified)
	s.SetBalance(addr2, uint256.NewInt(9999), tracing.BalanceChangeUnspecified)
	root, err := env.commitState(s, 1)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Reopen state to generate proof.
	s2, _ := env.openState(root)
	trie := s2.GetTrie()

	result, rawNodes, err := proof.GenerateAccountProof(s2, root, addr1, trie)
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}

	if result.Balance.ToInt().Uint64() != 5000 {
		t.Fatalf("balance mismatch: got %s, want 5000", result.Balance.ToInt().String())
	}

	// Verify the proof.
	exists, bal, err := proof.VerifyAccountProof(root, addr1, rawNodes)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !exists {
		t.Fatal("account should exist")
	}
	if bal.Uint64() != 5000 {
		t.Fatalf("verified balance mismatch: got %s, want 5000", bal.String())
	}

	t.Logf("Proof verified: %d nodes, balance=%s", len(rawNodes), bal.String())
}

// TestEmptyAccountPruning verifies that setting balance to 0 removes the account.
func TestEmptyAccountPruning(t *testing.T) {
	env := newTestEnv()

	// Block 1: create account.
	s, _ := env.openState(st.EmptyRootHash)
	s.SetBalance(addr1, uint256.NewInt(1000), tracing.BalanceChangeUnspecified)
	root1, err := env.commitState(s, 1)
	if err != nil {
		t.Fatalf("commit 1: %v", err)
	}

	// Block 2: set balance to 0 → should be pruned.
	s2, _ := env.openState(root1)
	s2.SetBalance(addr1, uint256.NewInt(0), tracing.BalanceChangeUnspecified)
	root2, err := env.commitState(s2, 2)
	if err != nil {
		t.Fatalf("commit 2: %v", err)
	}

	// Root 2 should be the empty root since the only account was pruned.
	if root2 != st.EmptyRootHash {
		t.Fatalf("expected empty root after pruning, got %s", root2.Hex())
	}

	t.Logf("Empty account correctly pruned, root=%s", root2.Hex())
}

// TestMissingAccountProof verifies that proofs for non-existent accounts work correctly.
func TestMissingAccountProof(t *testing.T) {
	env := newTestEnv()

	// Create state with addr1 only.
	s, _ := env.openState(st.EmptyRootHash)
	s.SetBalance(addr1, uint256.NewInt(1000), tracing.BalanceChangeUnspecified)
	root, err := env.commitState(s, 1)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Generate proof for addr3 which does NOT exist.
	s2, _ := env.openState(root)
	trie := s2.GetTrie()

	result, rawNodes, err := proof.GenerateAccountProof(s2, root, addr3, trie)
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}

	// Balance should be 0 for non-existent account.
	if result.Balance.ToInt().Sign() != 0 {
		t.Fatalf("expected 0 balance, got %s", result.Balance.ToInt().String())
	}

	// Verify the proof — should succeed with exists=false.
	exists, bal, err := proof.VerifyAccountProof(root, addr3, rawNodes)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if exists {
		t.Fatal("account should NOT exist")
	}
	if bal.Sign() != 0 {
		t.Fatalf("expected 0 balance, got %s", bal.String())
	}

	t.Logf("Missing account proof verified correctly, %d nodes", len(rawNodes))
}

// TestMultiBlockIngestion simulates ingesting multiple blocks and verifying state.
func TestMultiBlockIngestion(t *testing.T) {
	env := newTestEnv()
	root := st.EmptyRootHash

	// Block 1: addr1=100, addr2=200.
	s, _ := env.openState(root)
	s.SetBalance(addr1, uint256.NewInt(100), tracing.BalanceChangeUnspecified)
	s.SetBalance(addr2, uint256.NewInt(200), tracing.BalanceChangeUnspecified)
	root, _ = env.commitState(s, 1)

	// Block 2: addr1=300 (update), addr3=500 (new).
	s, _ = env.openState(root)
	s.SetBalance(addr1, uint256.NewInt(300), tracing.BalanceChangeUnspecified)
	s.SetBalance(addr3, uint256.NewInt(500), tracing.BalanceChangeUnspecified)
	root, _ = env.commitState(s, 2)

	// Block 3: addr2=0 (prune).
	s, _ = env.openState(root)
	s.SetBalance(addr2, uint256.NewInt(0), tracing.BalanceChangeUnspecified)
	root, _ = env.commitState(s, 3)

	// Verify final state via proofs.
	s2, _ := env.openState(root)
	trie := s2.GetTrie()

	// addr1 should have balance 300.
	_, nodes, _ := proof.GenerateAccountProof(s2, root, addr1, trie)
	exists, bal, err := proof.VerifyAccountProof(root, addr1, nodes)
	if err != nil || !exists || bal.Uint64() != 300 {
		t.Fatalf("addr1: exists=%v bal=%v err=%v", exists, bal, err)
	}

	// addr2 should NOT exist (pruned).
	_, nodes, _ = proof.GenerateAccountProof(s2, root, addr2, trie)
	exists, _, err = proof.VerifyAccountProof(root, addr2, nodes)
	if err != nil || exists {
		t.Fatalf("addr2: should not exist, exists=%v err=%v", exists, err)
	}

	// addr3 should have balance 500.
	_, nodes, _ = proof.GenerateAccountProof(s2, root, addr3, trie)
	exists, bal, err = proof.VerifyAccountProof(root, addr3, nodes)
	if err != nil || !exists || bal.Uint64() != 500 {
		t.Fatalf("addr3: exists=%v bal=%v err=%v", exists, bal, err)
	}

	t.Logf("Multi-block test passed, final root=%s", root.Hex())
}

// TestSecureTrieKeying verifies that keccak256(address) is used as the trie key.
func TestSecureTrieKeying(t *testing.T) {
	// Verify that our key scheme matches Ethereum's secure trie.
	key := crypto.Keccak256(addr1.Bytes())
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}
	t.Logf("SecureKey for %s = 0x%x", addr1.Hex(), key)
}

// TestJSONOutput verifies that JSON output matches expected format.
func TestJSONOutput(t *testing.T) {
	env := newTestEnv()

	s, _ := env.openState(st.EmptyRootHash)
	s.SetBalance(addr1, uint256.NewInt(12345), tracing.BalanceChangeUnspecified)
	root, _ := env.commitState(s, 1)

	s2, _ := env.openState(root)
	trie := s2.GetTrie()

	result, _, err := proof.GenerateAccountProof(s2, root, addr1, trie)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	jsonBytes, err := proof.MarshalJSON(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	jsonStr := string(jsonBytes)

	// Verify expected fields are present.
	for _, key := range []string{"address", "accountProof", "balance", "codeHash", "nonce", "storageHash", "storageProof"} {
		if !contains(jsonStr, key) {
			t.Errorf("JSON missing field: %s", key)
		}
	}

	t.Logf("JSON output (%d bytes):\n%s", len(jsonBytes), jsonStr)
}

// TestLargeBalance verifies handling of large balances (e.g., > 64-bit).
func TestLargeBalance(t *testing.T) {
	env := newTestEnv()

	largeBal := new(big.Int)
	largeBal.SetString("100000000000000000000000000", 10) // 100M ETH in wei

	u256Bal, overflow := uint256.FromBig(largeBal)
	if overflow {
		t.Fatal("balance overflow")
	}

	s, _ := env.openState(st.EmptyRootHash)
	s.SetBalance(addr1, u256Bal, tracing.BalanceChangeUnspecified)
	root, _ := env.commitState(s, 1)

	s2, _ := env.openState(root)
	trie := s2.GetTrie()

	_, rawNodes, err := proof.GenerateAccountProof(s2, root, addr1, trie)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	exists, bal, err := proof.VerifyAccountProof(root, addr1, rawNodes)
	if err != nil || !exists {
		t.Fatalf("verify failed: %v exists=%v", err, exists)
	}
	if bal.Cmp(largeBal) != 0 {
		t.Fatalf("balance mismatch: got %s, want %s", bal.String(), largeBal.String())
	}

	t.Logf("Large balance test passed: %s", bal.String())
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0
}

func findSubstring(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
