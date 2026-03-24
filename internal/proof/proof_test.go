package proof_test

import (
	"math/big"
	"os"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/tree"
)

// ---------------------------------------------------------------------------
// In-memory DB for tests (implements db.DB)
// ---------------------------------------------------------------------------

type memDB struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMemDB() *memDB {
	return &memDB{data: make(map[string][]byte)}
}

func (m *memDB) Get(key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[string(key)]
	if !ok {
		return nil, db.ErrNotFound
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (m *memDB) Set(key, value []byte, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[string(key)] = cp
	return nil
}

func (m *memDB) Delete(key []byte, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, string(key))
	return nil
}

func (m *memDB) Close() error { return nil }

func (m *memDB) NewBatch() db.Batch { return &memBatch{db: m} }

type memBatch struct {
	db  *memDB
	ops []func()
}

func (b *memBatch) Set(key, value []byte, sync bool) {
	k := make([]byte, len(key))
	copy(k, key)
	v := make([]byte, len(value))
	copy(v, value)
	b.ops = append(b.ops, func() { b.db.Set(k, v, sync) })
}

func (b *memBatch) Delete(key []byte, sync bool) {
	k := make([]byte, len(key))
	copy(k, key)
	b.ops = append(b.ops, func() { b.db.Delete(k, sync) })
}

func (b *memBatch) Commit(_ bool) error {
	for _, op := range b.ops {
		op()
	}
	return nil
}

func (b *memBatch) Close() error { return nil }

// ---------------------------------------------------------------------------
// Package-level precomputed data (initialised once in TestMain)
// ---------------------------------------------------------------------------

var precomputed *config.PrecomputedData

func TestMain(m *testing.M) {
	srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
	if err != nil {
		panic(err)
	}

	tmpDir, err := os.MkdirTemp("", "samurai-test-params-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, tmpDir)
	precomputed = &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}

	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func newTestStore() *db.SamuraiStore {
	return &db.SamuraiStore{
		StateDB:   newMemDB(),
		TreeDB:    newMemDB(),
		HistoryDB: newMemDB(),
	}
}

// ---------------------------------------------------------------------------
// Test: create account → 5 updates → range query → verify proof
// ---------------------------------------------------------------------------

func TestRangeProofEndToEnd(t *testing.T) {
	account := common.HexToAddress("0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	sdb := newTestStore()
	accountInfo := tree.NewAccountInfo(account, precomputed)

	// Six balance updates produce 5 historical versions (v0–v4) plus current v5.
	type update struct {
		block   uint64
		balance int64
	}
	updates := []update{
		{10, 100},
		{20, 200},
		{30, 300},
		{40, 400},
		{50, 500},
		{60, 600},
	}

	for _, u := range updates {
		accountInfo.Update(u.block, big.NewInt(u.balance), sdb)
	}

	// Persist current state, commitments, and batch roots to the store.
	accountInfo.Save(sdb)

	// --- Range query: blocks 10–55 → versions 0–4 ---

	startVersion, endVersion, err := proof.BlockRangeToVersionRange(account, 10, 55, sdb)
	if err != nil {
		t.Fatalf("BlockRangeToVersionRange: %v", err)
	}
	if startVersion != 0 || endVersion != 4 {
		t.Fatalf("expected version range [0,4], got [%d,%d]", startVersion, endVersion)
	}

	// --- Generate KZG range proofs ---

	rangeProofs, balanceInfos := proof.GetNewProofRange(account, startVersion, endVersion, precomputed, sdb)
	if len(rangeProofs) == 0 {
		t.Fatal("no range proofs generated")
	}
	t.Logf("generated %d range proofs covering %d historical balances", len(rangeProofs), len(balanceInfos))

	// Sanity-check returned historical balances.
	expectedBalances := []int64{100, 200, 300, 400, 500}
	if len(balanceInfos) != len(expectedBalances) {
		t.Fatalf("expected %d balance infos, got %d", len(expectedBalances), len(balanceInfos))
	}
	for i, hb := range balanceInfos {
		if hb.Balance.Int64() != expectedBalances[i] {
			t.Fatalf("balance[%d]: got %d, want %d", i, hb.Balance.Int64(), expectedBalances[i])
		}
	}

	// --- Verify proofs (skip MPT trust anchor) ---

	err = proof.VerifyNewRangeProofs(
		account,
		startVersion, endVersion,
		rangeProofs,
		balanceInfos,
		precomputed,
		nil,           // no MPT proof nodes
		common.Hash{}, // no state root
		nil,           // no current balance (MPT path skipped)
	)
	if err != nil {
		t.Fatalf("VerifyNewRangeProofs: %v", err)
	}

	t.Log("end-to-end range proof verification passed")
}
