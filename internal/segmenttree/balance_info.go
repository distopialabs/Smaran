package segmenttree

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type CurrentBalance struct {
	Version    uint64
	Balance    *big.Int
	StartBlock uint64
}

type HistoricalBalance struct {
	CurrentBalance
	EndBlock uint64
}

// 48-byte encoding: [Version(8)|Balance(32)|StartBlock(8)]
func (cb *CurrentBalance) MarshalBinary() [48]byte {
	var out [48]byte
	binary.BigEndian.PutUint64(out[0:8], cb.Version)

	// Balance -> 32-byte big-endian (zero-padded)
	b := cb.Balance.Bytes()        // big-endian magnitude
	copy(out[8+32-len(b):8+32], b) // left-pad with zeros

	binary.BigEndian.PutUint64(out[40:48], cb.StartBlock)
	return out
}

func (cb *CurrentBalance) UnmarshalBinary(in []byte) error {
	if len(in) != 48 {
		return fmt.Errorf("want 48 bytes, got %d", len(in))
	}
	v := binary.BigEndian.Uint64(in[0:8])
	bal := new(big.Int).SetBytes(in[8:40])
	sb := binary.BigEndian.Uint64(in[40:48])
	cb.Version = v
	cb.Balance = bal
	cb.StartBlock = sb
	return nil
}

func (hb *HistoricalBalance) MarshalBinary() [56]byte {
	var out [56]byte
	b := hb.CurrentBalance.MarshalBinary()
	copy(out[0:48], b[:])
	binary.BigEndian.PutUint64(out[48:56], hb.EndBlock)
	return out
}

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

func (cb *CurrentBalance) Bytes() []byte {
	b := cb.MarshalBinary()
	return b[:]
	// b, err := proto.Marshal(&segmenttreepb.CurrentBalance{
	// 	Version:    cb.Version,
	// 	Balance:    cb.Balance.Bytes(),
	// 	StartBlock: cb.StartBlock,
	// })
	// if err != nil {
	// 	panic(err)
	// }
	// return b
}

func (hb *HistoricalBalance) Bytes() []byte {
	b := hb.MarshalBinary()
	return b[:]
	// b, err := proto.Marshal(&segmenttreepb.HistoricalBalance{
	// 	Current: &segmenttreepb.CurrentBalance{
	// 		Version:    hb.Version,
	// 		Balance:    hb.Balance.Bytes(),
	// 		StartBlock: hb.StartBlock,
	// 	},
	// 	EndBlock: hb.EndBlock,
	// })
	// if err != nil {
	// 	panic(err)
	// }
	// return b
}
func (cb *CurrentBalance) Hash() common.Hash {
	return BytesToPoseidonHash(cb.Bytes())
}

func (hb *HistoricalBalance) Hash() common.Hash {
	return BytesToPoseidonHash(hb.Bytes())
}

func (cb *CurrentBalance) DeepCopy() *CurrentBalance {
	return &CurrentBalance{
		Version:    cb.Version,
		Balance:    new(big.Int).Set(cb.Balance),
		StartBlock: cb.StartBlock,
	}
}
func (hb *HistoricalBalance) DeepCopy() *HistoricalBalance {
	return &HistoricalBalance{
		CurrentBalance: *hb.CurrentBalance.DeepCopy(),
		EndBlock:       hb.EndBlock,
	}
}

func (cb *CurrentBalance) ToHistoricalBalance(endBlock uint64) *HistoricalBalance {
	return &HistoricalBalance{
		// TODO: recheck if we need to deep copy the CurrentBalance
		// CurrentBalance: *cb.DeepCopy(),
		CurrentBalance: *cb,
		EndBlock:       endBlock,
	}
}
