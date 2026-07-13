package kt

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"sync"
	"testing"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-381/fr/fft"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"

	"github.com/nepal80m/samurai/internal/crypto/polynomial"
	"github.com/nepal80m/samurai/internal/kt/proof"
	"github.com/nepal80m/samurai/internal/kt/tree"
)

const testParamsDir = "/tmp/testdata/params"

func setupTestParamsDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(testParamsDir, 0o755); err != nil {
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

func TestSamuraiKZGProofGenerateAndVerify(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

	user := []byte("proof-user")
	const numPuts = 10

	for i := 0; i < numPuts; i++ {
		s.Put(user, []byte(fmt.Sprintf("key-v%d", i+1)))
	}
	_ = s.GetCommitment()

	account := userToAddress(user)
	userKey := string(user)
	version := s.currentVersion[userKey]
	if version < 2 {
		t.Fatalf("need at least 2 versions for proof generation, got %d", version)
	}

	// Diagnostic: check that the stored commitment matches a direct KZG commit
	// on the polynomial recovered via FFT from the tree evaluations.
	ai := s.samuraiAccounts[account.Hex()]
	batchTree := &ai.CurrentLXBatchTree[0]
	storedCommit := ai.CurrentLXBatchCommitment[0]

	evals := make([]fr.Element, len(batchTree))
	for i, v := range batchTree {
		evals[i] = polynomial.HashToFieldElement(v)
	}
	domain := fft.NewDomain(uint64(len(evals)))
	fft.BitReverse(evals)
	domain.FFTInverse(evals, fft.DIF)
	fft.BitReverse(evals)

	directCommit, err := gnark_kzg.Commit(evals, s.precomputedData.SRS.Inner.Pk)
	if err != nil {
		t.Fatalf("direct commit: %v", err)
	}
	if !storedCommit.Equal(&directCommit) {
		t.Logf("STORED  commitment: %x", storedCommit.Marshal()[:16])
		t.Logf("DIRECT  commitment: %x", directCommit.Marshal()[:16])
		t.Fatalf("incremental commitment does not match direct KZG commit — WeightCommits may be stale")
	}
	t.Logf("commitment match OK")

	rangeProofs, historicalBalances := s.GetProofRangeInMemory(account, 0, version-1, userKey)
	if len(rangeProofs) == 0 {
		t.Fatal("expected non-empty range proofs")
	}
	if len(historicalBalances) == 0 {
		t.Fatal("expected non-empty historical balances")
	}
	t.Logf("versions=%d  proofs=%d  balances=%d", version, len(rangeProofs), len(historicalBalances))

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("proof verification panicked: %v", r)
			}
		}()
		proof.VerifyNewRangeProofs(account, 0, version-1, rangeProofs, historicalBalances, s.precomputedData)
	}()
}

func TestSamuraiKZGProofViaGetResponse(t *testing.T) {
	s := newTestSamuraiServer(t, 1)

	user := []byte("roundtrip-user")
	const numPuts = 10
	for i := 0; i < numPuts; i++ {
		s.Put(user, []byte(fmt.Sprintf("key-v%d", i+1)))
	}
	_ = s.GetCommitment()

	result, err := s.Get(user)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if result.CurrentVersion < 2 {
		t.Skipf("too few versions (%d) to generate proofs", result.CurrentVersion)
	}
	if len(result.SamuraiProofs) == 0 {
		t.Fatal("expected non-empty samurai proofs in response")
	}
	if len(result.HistoricalBalances) == 0 {
		t.Fatal("expected non-empty historical balances in response")
	}

	account := userToAddress(user)

	historicalBalances := make([]*tree.HistoricalBalance, len(result.HistoricalBalances))
	for i, hbBytes := range result.HistoricalBalances {
		hb := &tree.HistoricalBalance{}
		if err := hb.UnmarshalBinary(hbBytes); err != nil {
			t.Fatalf("unmarshal historical balance %d: %v", i, err)
		}
		historicalBalances[i] = hb
	}

	rangeProofs := make([]*proof.RangeProof, len(result.SamuraiProofs))
	for i, sp := range result.SamuraiProofs {
		var commitment gnark_kzg.Digest
		if _, err := commitment.SetBytes(sp.Commitment); err != nil {
			t.Fatalf("unmarshal commitment %d: %v", i, err)
		}
		var proofG1 bls.G1Affine
		if _, err := proofG1.SetBytes(sp.Proof); err != nil {
			t.Fatalf("unmarshal proof %d: %v", i, err)
		}
		rangeProofs[i] = &proof.RangeProof{
			Idx:                  sp.Idx,
			Layer:                sp.Layer,
			Commitment:           commitment,
			Proof:                proofG1,
			BlockRange:           sp.BlockRange,
			DependentCommitments: sp.DependentCommitments,
		}
	}

	t.Logf("versions=%d  proofs=%d  balances=%d",
		result.CurrentVersion, len(rangeProofs), len(historicalBalances))

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("proof verification panicked: %v", r)
			}
		}()
		proof.VerifyNewRangeProofs(account, 0, result.CurrentVersion-1, rangeProofs, historicalBalances, s.precomputedData)
	}()
}

func TestSamuraiConcurrentPutGetCommitment(t *testing.T) {
	const (
		numWriters       = 4
		putsPerWriter    = 50
		usersPerWriter   = 3
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

func TestFFT(t *testing.T) {
	domain := fft.NewDomain(uint64(4096))

	poly := make(polynomial.Polynomial, 4096)
	for i := 0; i < 4096; i++ {
		var randBytes [32]byte
		rand.Read(randBytes[:])
		rand_hash := common.Hash(randBytes)
		poly[i] = polynomial.HashToFieldElement(rand_hash)
	}

	points := make([]fr.Element, 4096)
	for i := 0; i < 4096; i++ {
		points[i] = poly[i]
	}

	domain.FFT(points, fft.DIF)
	fft.BitReverse(points)

	_poly := polynomial.Polynomial(poly)

	evals := make([]fr.Element, 4096)
	for i := 0; i < 4096; i++ {
		var omegaI fr.Element
		omegaI.Exp(domain.Generator, new(big.Int).SetInt64(int64(i)))
		evals[i] = _poly.Eval(&omegaI)

		if !evals[i].Equal(&points[i]) {
			t.Fatalf("bad at %d", i)
		}
	}

	domain.FFTInverse(evals, fft.DIF)
	fft.BitReverse(evals)

	for i := 0; i < 4096; i++ {
		if !evals[i].Equal(&poly[i]) {
			t.Fatalf("bad at %d", i)
		}
	}
}
