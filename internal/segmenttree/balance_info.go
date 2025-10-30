package segmenttree

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	segmenttreepb "github.com/nepal80m/samurai/internal/segmenttree/pb"
	"google.golang.org/protobuf/proto"
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
	b, err := proto.Marshal(&segmenttreepb.CurrentBalance{
		Version:    cb.Version,
		Balance:    cb.Balance.Bytes(),
		StartBlock: cb.StartBlock,
	})
	if err != nil {
		panic(err)
	}
	return b
}

func (hb *HistoricalBalance) Bytes() []byte {
	b, err := proto.Marshal(&segmenttreepb.HistoricalBalance{
		Current: &segmenttreepb.CurrentBalance{
			Version:    hb.Version,
			Balance:    hb.Balance.Bytes(),
			StartBlock: hb.StartBlock,
		},
		EndBlock: hb.EndBlock,
	})
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
