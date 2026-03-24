package keys

import (
	"math/big"
	"testing"
)

func TestAddress20To32(t *testing.T) {
	var addr [20]byte
	for i := range addr {
		addr[i] = byte(i + 1)
	}
	addr32 := Address20To32(addr)
	// First 12 bytes should be zero
	for i := 0; i < 12; i++ {
		if addr32[i] != 0 {
			t.Fatalf("expected zero at position %d, got %d", i, addr32[i])
		}
	}
	for i := 0; i < 20; i++ {
		if addr32[12+i] != addr[i] {
			t.Fatalf("mismatch at position %d: expected %d, got %d", 12+i, addr[i], addr32[12+i])
		}
	}
}

func TestGetTreeKeyForBasicData(t *testing.T) {
	var addr [20]byte
	addr[19] = 1 // simple address

	key := GetTreeKeyForBasicData(addr)

	// Key should be 32 bytes with sub-index 0 at position 31
	if key[31] != 0 {
		t.Fatalf("sub-index should be 0, got %d", key[31])
	}

	// Same address should produce same key (deterministic)
	key2 := GetTreeKeyForBasicData(addr)
	if key != key2 {
		t.Fatal("key derivation is not deterministic")
	}

	// Different address should produce different key
	var addr2 [20]byte
	addr2[19] = 2
	key3 := GetTreeKeyForBasicData(addr2)
	if key == key3 {
		t.Fatal("different addresses produced same key")
	}
}

func TestPackBasicDataRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		balance *big.Int
	}{
		{"zero", big.NewInt(0)},
		{"one", big.NewInt(1)},
		{"one_ether", new(big.Int).Mul(big.NewInt(1e18), big.NewInt(1))},
		{"max_128bit", new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packed, err := PackBasicData(tc.balance)
			if err != nil {
				t.Fatalf("PackBasicData: %v", err)
			}
			got := UnpackBalance(packed)
			if tc.balance.Cmp(got) != 0 {
				t.Fatalf("round-trip failed: want %s, got %s", tc.balance, got)
			}
		})
	}
}

func TestPackBasicDataOverflow(t *testing.T) {
	// 129-bit value should fail
	overflow := new(big.Int).Lsh(big.NewInt(1), 128) // exactly 2^128
	_, err := PackBasicData(overflow)
	if err == nil {
		t.Fatal("expected overflow error for 129-bit balance")
	}
}

func TestPackBasicDataNegative(t *testing.T) {
	_, err := PackBasicData(big.NewInt(-1))
	if err == nil {
		t.Fatal("expected error for negative balance")
	}
}

func TestPackBasicDataLayout(t *testing.T) {
	// Verify the layout: bytes 0-15 should be zero (version, code_size, nonce)
	bal := big.NewInt(0x1234)
	packed, err := PackBasicData(bal)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		if packed[i] != 0 {
			t.Fatalf("byte %d should be 0, got %d", i, packed[i])
		}
	}
	// bytes 30..31 should contain 0x12 0x34
	if packed[30] != 0x12 || packed[31] != 0x34 {
		t.Fatalf("expected [0x12 0x34] at end, got [0x%02x 0x%02x]", packed[30], packed[31])
	}
}

func TestUnpackBalance(t *testing.T) {
	var val [32]byte
	// Set balance to 1 ETH = 1e18 in big-endian at offset 16
	bal := new(big.Int).Mul(big.NewInt(1e18), big.NewInt(1))
	b := bal.Bytes()
	copy(val[32-len(b):], b)

	got := UnpackBalance(val)
	if bal.Cmp(got) != 0 {
		t.Fatalf("UnpackBalance: want %s, got %s", bal, got)
	}
}
