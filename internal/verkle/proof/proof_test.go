package proof

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/nepal80m/samurai/internal/verkle/keys"
	verkle "github.com/ethereum/go-verkle"
)

// buildTestTree creates a small Verkle tree with a few addresses for testing.
func buildTestTree(t *testing.T) (verkle.VerkleNode, [][20]byte) {
	t.Helper()
	root := verkle.New()

	addresses := [][20]byte{}
	for i := byte(1); i <= 5; i++ {
		var addr [20]byte
		addr[19] = i
		addresses = append(addresses, addr)

		bal := new(big.Int).Mul(big.NewInt(int64(i)), big.NewInt(1e18))
		key := keys.GetTreeKeyForBasicData(addr)
		val, err := keys.PackBasicData(bal)
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Insert(key[:], val[:], nil); err != nil {
			t.Fatal(err)
		}
	}
	root.Commit()
	return root, addresses
}

func TestGenerateAndVerifyProof(t *testing.T) {
	root, addresses := buildTestTree(t)
	// Use compressed point serialization for the root commitment
	rootBytes := SerializeCommitment(root)

	// Test existing address
	addr := addresses[0]
	result, metrics, err := GenerateProof(root, addr, rootBytes, nil)
	if err != nil {
		t.Fatalf("GenerateProof: %v", err)
	}

	if !result.Exists {
		t.Fatal("expected exists=true")
	}
	if result.Balance != "0x"+new(big.Int).Mul(big.NewInt(1), big.NewInt(1e18)).Text(16) {
		t.Fatalf("unexpected balance: %s", result.Balance)
	}
	if metrics.ProofGenTimeNs <= 0 {
		t.Fatal("proof gen time should be > 0")
	}
	if metrics.ProofJSONBytesLen <= 0 {
		t.Fatal("proof JSON size should be > 0")
	}

	// Verify the proof
	var vp verkle.VerkleProof
	if err := json.Unmarshal(result.VerkleProof, &vp); err != nil {
		t.Fatalf("unmarshal VerkleProof: %v", err)
	}
	var sd verkle.StateDiff
	if err := json.Unmarshal(result.StateDiff, &sd); err != nil {
		t.Fatalf("unmarshal StateDiff: %v", err)
	}

	if err := VerifyProof(rootBytes, &vp, sd); err != nil {
		t.Fatalf("VerifyProof: %v", err)
	}
}

func TestVerifyAndExtract(t *testing.T) {
	root, addresses := buildTestTree(t)
	rootBytes := SerializeCommitment(root)

	addr := addresses[0]
	result, _, err := GenerateProof(root, addr, rootBytes, nil)
	if err != nil {
		t.Fatalf("GenerateProof: %v", err)
	}

	var vp verkle.VerkleProof
	json.Unmarshal(result.VerkleProof, &vp)
	var sd verkle.StateDiff
	json.Unmarshal(result.StateDiff, &sd)

	exists, balance, err := VerifyAndExtract(rootBytes, &vp, sd, addr)
	if err != nil {
		t.Fatalf("VerifyAndExtract: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}
	expected := new(big.Int).Mul(big.NewInt(1), big.NewInt(1e18))
	if balance.Cmp(expected) != 0 {
		t.Fatalf("expected balance %s, got %s", expected, balance)
	}
}

func TestProofNonMembership(t *testing.T) {
	root, _ := buildTestTree(t)
	rootBytes := SerializeCommitment(root)

	// Non-existing address
	var addr [20]byte
	addr[19] = 99

	result, _, err := GenerateProof(root, addr, rootBytes, nil)
	if err != nil {
		t.Fatalf("GenerateProof: %v", err)
	}

	if result.Exists {
		t.Fatal("expected exists=false for non-member")
	}
	if result.Balance != "0x0" {
		t.Fatalf("expected balance 0x0, got %s", result.Balance)
	}
}

func BenchmarkVerkleProofGen(b *testing.B) {
	root := verkle.New()
	for i := 0; i < 100; i++ {
		var addr [20]byte
		addr[18] = byte(i >> 8)
		addr[19] = byte(i)
		bal := new(big.Int).Mul(big.NewInt(int64(i+1)), big.NewInt(1e18))
		key := keys.GetTreeKeyForBasicData(addr)
		val, _ := keys.PackBasicData(bal)
		root.Insert(key[:], val[:], nil)
	}
	root.Commit()
	rootBytes := SerializeCommitment(root)

	var addr [20]byte
	addr[19] = 1

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result, metrics, err := GenerateProof(root, addr, rootBytes, nil)
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(metrics.ProofPayloadBytesLen), "payload_bytes")
		b.ReportMetric(float64(metrics.ProofJSONBytesLen), "json_bytes")
		_ = result
	}
}

func BenchmarkVerkleProofVerify(b *testing.B) {
	root := verkle.New()
	for i := 0; i < 100; i++ {
		var addr [20]byte
		addr[18] = byte(i >> 8)
		addr[19] = byte(i)
		bal := new(big.Int).Mul(big.NewInt(int64(i+1)), big.NewInt(1e18))
		key := keys.GetTreeKeyForBasicData(addr)
		val, _ := keys.PackBasicData(bal)
		root.Insert(key[:], val[:], nil)
	}
	root.Commit()
	rootBytes := SerializeCommitment(root)

	var addr [20]byte
	addr[19] = 1

	result, _, err := GenerateProof(root, addr, rootBytes, nil)
	if err != nil {
		b.Fatal(err)
	}

	var vp verkle.VerkleProof
	json.Unmarshal(result.VerkleProof, &vp)
	var sd verkle.StateDiff
	json.Unmarshal(result.StateDiff, &sd)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := VerifyProof(rootBytes, &vp, sd); err != nil {
			b.Fatal(err)
		}
	}
}
