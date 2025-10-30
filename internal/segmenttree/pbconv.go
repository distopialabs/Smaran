package segmenttree

import (
	"math/big"

	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	"github.com/ethereum/go-ethereum/common"
	segmenttreepb "github.com/nepal80m/samurai/internal/segmenttree/pb"
)

func protoFromCurrentBalance(cb *CurrentBalance) *segmenttreepb.CurrentBalance {
	if cb == nil {
		return nil
	}
	return &segmenttreepb.CurrentBalance{
		Version:    cb.Version,
		Balance:    cb.Balance.Bytes(),
		StartBlock: cb.StartBlock,
	}
}

func currentBalanceFromProto(m *segmenttreepb.CurrentBalance) *CurrentBalance {
	if m == nil {
		return nil
	}
	return &CurrentBalance{
		Version:    m.GetVersion(),
		Balance:    new(big.Int).SetBytes(m.GetBalance()),
		StartBlock: m.GetStartBlock(),
	}
}

func protoFromHistoricalBalance(hb *HistoricalBalance) *segmenttreepb.HistoricalBalance {
	if hb == nil {
		return nil
	}
	return &segmenttreepb.HistoricalBalance{
		Current:  protoFromCurrentBalance(&hb.CurrentBalance),
		EndBlock: hb.EndBlock,
	}
}

func historicalBalanceFromProto(m *segmenttreepb.HistoricalBalance) *HistoricalBalance {
	if m == nil {
		return nil
	}
	return &HistoricalBalance{
		CurrentBalance: *currentBalanceFromProto(m.GetCurrent()),
		EndBlock:       m.GetEndBlock(),
	}
}

func protoFromBatchTree(t *BatchTree) *segmenttreepb.BatchTree {
	if t == nil {
		return nil
	}
	m := &segmenttreepb.BatchTree{Nodes: make([][]byte, 0, SegmentTreeSize)}
	for i := 0; i < SegmentTreeSize; i++ {
		m.Nodes = append(m.Nodes, (*t)[i].Bytes())
	}
	return m
	// note: if we find this to be heavy, we can consider packing into a single flat []byte
}

func batchTreeFromProto(m *segmenttreepb.BatchTree) BatchTree {
	var out BatchTree
	if m == nil || len(m.Nodes) == 0 {
		return out
	}
	limit := SegmentTreeSize
	if len(m.Nodes) < limit {
		limit = len(m.Nodes)
	}
	for i := 0; i < limit; i++ {
		out[i] = common.BytesToHash(m.Nodes[i])
	}
	return out
	// any remaining entries stay zero-value
}

func protoFromLXBatchTree(t *LXBatchTree) *segmenttreepb.LXBatchTree {
	if t == nil {
		return nil
	}
	m := &segmenttreepb.LXBatchTree{Layers: make([]*segmenttreepb.BatchTree, 0, MaxLayer)}
	for layer := 0; layer < MaxLayer; layer++ {
		bt := (*BatchTree)(&(*t)[layer])
		m.Layers = append(m.Layers, protoFromBatchTree(bt))
	}
	return m
}

func lxBatchTreeFromProto(m *segmenttreepb.LXBatchTree) *LXBatchTree {
	var out LXBatchTree
	if m == nil || len(m.Layers) == 0 {
		return &out
	}
	limit := MaxLayer
	if len(m.Layers) < limit {
		limit = len(m.Layers)
	}
	for layer := 0; layer < limit; layer++ {
		bt := batchTreeFromProto(m.Layers[layer])
		out[layer] = bt
	}
	return &out
}

func protoFromDigest(d gnark_kzg.Digest) *segmenttreepb.KZGDigest {
	b := d.Bytes()
	return &segmenttreepb.KZGDigest{Digest: b[:]}
}

func digestFromProto(m *segmenttreepb.KZGDigest) gnark_kzg.Digest {
	var d gnark_kzg.Digest
	if m != nil {
		d.SetBytes(m.GetDigest())
	}
	return d
}

func protoFromLXBatchCommitment(c *LXBatchCommitment) *segmenttreepb.LXBatchCommitment {
	if c == nil {
		return nil
	}
	m := &segmenttreepb.LXBatchCommitment{Layers: make([]*segmenttreepb.KZGDigest, 0, MaxLayer)}
	for i := 0; i < MaxLayer; i++ {
		m.Layers = append(m.Layers, protoFromDigest((*c)[i]))
	}
	return m
}

func lxBatchCommitmentFromProto(m *segmenttreepb.LXBatchCommitment) *LXBatchCommitment {
	var out LXBatchCommitment
	if m == nil || len(m.Layers) == 0 {
		return &out
	}
	limit := MaxLayer
	if len(m.Layers) < limit {
		limit = len(m.Layers)
	}
	for i := 0; i < limit; i++ {
		out[i] = digestFromProto(m.Layers[i])
	}
	return &out
}

func protoFromAccountInfo(a *AccountInfo) *segmenttreepb.AccountInfo {
	if a == nil {
		return nil
	}
	return &segmenttreepb.AccountInfo{
		Account:                  a.Account.Bytes(),
		CurrentBalance:           protoFromCurrentBalance(a.CurrentBalanceInfo),
		CurrentLxBatchTree:       protoFromLXBatchTree(a.CurrentLXBatchTree),
		CurrentLxBatchCommitment: protoFromLXBatchCommitment(a.CurrentLXBatchCommitment),
	}
}

func accountInfoFromProto(m *segmenttreepb.AccountInfo) *AccountInfo {
	if m == nil {
		return nil
	}
	return &AccountInfo{
		Account:                  common.BytesToAddress(m.GetAccount()),
		CurrentBalanceInfo:       currentBalanceFromProto(m.GetCurrentBalance()),
		CurrentLXBatchTree:       lxBatchTreeFromProto(m.GetCurrentLxBatchTree()),
		CurrentLXBatchCommitment: lxBatchCommitmentFromProto(m.GetCurrentLxBatchCommitment()),
		// PrecomputedData is intentionally not part of persisted data
	}
}
