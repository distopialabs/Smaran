package segmenttree

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
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

func (cb *CurrentBalance) Bytes() []byte {
	b, err := rlp.EncodeToBytes(cb)
	if err != nil {
		panic(err)
	}
	return b
}

func (hb *HistoricalBalance) Bytes() []byte {
	b, err := rlp.EncodeToBytes(hb)
	if err != nil {
		panic(err)
	}
	return b
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
