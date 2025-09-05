package main

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/nepal80m/samurai/internal/math/segmenttree"
)

type CurrentBalance struct {
	Version    uint64
	Balance    *big.Int
	StartBlock uint64
}

// type HistoricalBalance struct {
// 	Version    uint64
// 	Balance    *big.Int
// 	StartBlock uint64
// 	EndBlock   uint64
// }

type HistoricalBalance struct {
	CurrentBalance
	EndBlock uint64
}

func (cb *CurrentBalance) DeepCopy() *CurrentBalance {
	return &CurrentBalance{
		Version:    cb.Version,
		Balance:    new(big.Int).Set(cb.Balance),
		StartBlock: cb.StartBlock,
	}
}
func (cb *CurrentBalance) Hash() common.Hash {
	b, err := rlp.EncodeToBytes(cb)
	if err != nil {
		panic(err)
	}
	return segmenttree.BytesToPoseidonHash(b)
}
func (cb *CurrentBalance) Archive(endBlock uint64) *HistoricalBalance {
	return &HistoricalBalance{
		CurrentBalance: *cb.DeepCopy(),
		EndBlock:       endBlock,
	}
}

func (hb *HistoricalBalance) Hash() common.Hash {
	b, err := rlp.EncodeToBytes(hb)
	if err != nil {
		panic(err)
	}
	return segmenttree.BytesToPoseidonHash(b)
}

func main() {
	cb := &CurrentBalance{
		Version:    1,
		Balance:    big.NewInt(1000000000000000000),
		StartBlock: 1000000000000000000,
	}
	hb := &HistoricalBalance{
		CurrentBalance: *cb.DeepCopy(),
		EndBlock:       1000000000000000001,
	}
	fmt.Println(cb)
	fmt.Println(hb)
	fmt.Println(hb.Hash())
	cb.Balance.SetUint64(2000000000000000000)
	cb.Version++
	fmt.Println(cb)
	fmt.Println(hb)
	fmt.Println(hb.Hash())

}
