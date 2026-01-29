package tree

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// CurrentBalance represents the current balance state for an account.
type CurrentBalance struct {
	Version    uint64
	Balance    *big.Int
	StartBlock uint64
}

// HistoricalBalance represents a past balance state for an account.
type HistoricalBalance struct {
	CurrentBalance
	EndBlock uint64
}

// 48-byte encoding: [Version(8)|Balance(32)|StartBlock(8)]
func (cb *CurrentBalance) MarshalBinary() [48]byte {
	var out [48]byte
	binary.BigEndian.PutUint64(out[0:8], cb.Version)
	b := cb.Balance.Bytes()
	copy(out[8+32-len(b):8+32], b)
	binary.BigEndian.PutUint64(out[40:48], cb.StartBlock)
	return out
}

// UnmarshalBinary deserializes bytes into a CurrentBalance.
func (cb *CurrentBalance) UnmarshalBinary(in []byte) error {
	if len(in) != 48 {
		return fmt.Errorf("want 48 bytes, got %d", len(in))
	}
	cb.Version = binary.BigEndian.Uint64(in[0:8])
	cb.Balance = new(big.Int).SetBytes(in[8:40])
	cb.StartBlock = binary.BigEndian.Uint64(in[40:48])
	return nil
}

// MarshalBinary serializes a HistoricalBalance to bytes.
func (hb *HistoricalBalance) MarshalBinary() [56]byte {
	var out [56]byte
	b := hb.CurrentBalance.MarshalBinary()
	copy(out[0:48], b[:])
	binary.BigEndian.PutUint64(out[48:56], hb.EndBlock)
	return out
}

// UnmarshalBinary deserializes bytes into a HistoricalBalance.
func (hb *HistoricalBalance) UnmarshalBinary(in []byte) error {
	if len(in) != 56 {
		return fmt.Errorf("want 56 bytes, got %d", len(in))
	}
	var cb CurrentBalance
	cb.UnmarshalBinary(in[0:48])
	hb.CurrentBalance = cb
	hb.EndBlock = binary.BigEndian.Uint64(in[48:56])
	return nil
}

// Bytes returns the binary representation of CurrentBalance.
func (cb *CurrentBalance) Bytes() []byte {
	b := cb.MarshalBinary()
	return b[:]
}

// Bytes returns the binary representation of HistoricalBalance.
func (hb *HistoricalBalance) Bytes() []byte {
	b := hb.MarshalBinary()
	return b[:]
}

// Hash returns the Poseidon hash of the CurrentBalance.
func (cb *CurrentBalance) Hash() common.Hash {
	return BytesToPoseidonHash(cb.Bytes())
}

// Hash returns the Poseidon hash of the HistoricalBalance.
func (hb *HistoricalBalance) Hash() common.Hash {
	return BytesToPoseidonHash(hb.Bytes())
}

// DeepCopy creates a deep copy of CurrentBalance.
func (cb *CurrentBalance) DeepCopy() *CurrentBalance {
	return &CurrentBalance{
		Version:    cb.Version,
		Balance:    new(big.Int).Set(cb.Balance),
		StartBlock: cb.StartBlock,
	}
}

// DeepCopy creates a deep copy of HistoricalBalance.
func (hb *HistoricalBalance) DeepCopy() *HistoricalBalance {
	return &HistoricalBalance{
		CurrentBalance: *hb.CurrentBalance.DeepCopy(),
		EndBlock:       hb.EndBlock,
	}
}

// ToHistoricalBalance converts a CurrentBalance to a HistoricalBalance.
func (cb *CurrentBalance) ToHistoricalBalance(endBlock uint64) *HistoricalBalance {
	return &HistoricalBalance{
		CurrentBalance: *cb,
		EndBlock:       endBlock,
	}
}
