package server

import (
	"math/big"

	bls "github.com/consensys/gnark-crypto/ecc/bls12-381"
	gnark_kzg "github.com/consensys/gnark-crypto/ecc/bls12-381/kzg"
	proofpb "github.com/nepal80m/samurai/api/proto/v1"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/tree"
)

// rangeProofToProto converts a RangeProof to its protobuf representation.
func rangeProofToProto(rp *proof.RangeProof) *proofpb.RangeProof {
	pbProof := &proofpb.RangeProof{
		Layer:                int32(rp.Layer),
		Idx:                  int32(rp.Idx),
		Commitment:           rp.Commitment.Marshal(),
		Proof:                rp.Proof.Marshal(),
		DependentCommitments: make([]int32, len(rp.DependentCommitments)),
	}

	if rp.BlockRange != nil {
		pbProof.BlockRange = &proofpb.BlockRange{
			Start: int32(rp.BlockRange.Start),
			End:   int32(rp.BlockRange.End),
		}
	}

	for i, dc := range rp.DependentCommitments {
		pbProof.DependentCommitments[i] = int32(dc)
	}

	return pbProof
}

// balanceInfoToProto converts a HistoricalBalance to its protobuf representation.
func balanceInfoToProto(bi *tree.HistoricalBalance) *proofpb.BalanceInfo {
	return &proofpb.BalanceInfo{
		Version:    bi.Version,
		StartBlock: bi.StartBlock,
		EndBlock:   bi.EndBlock,
		Balance:    bi.Balance.Bytes(),
	}
}

// RangeProofFromProto converts a protobuf RangeProof to its internal representation.
func RangeProofFromProto(pb *proofpb.RangeProof) *proof.RangeProof {
	rp := &proof.RangeProof{
		Layer:                int(pb.Layer),
		Idx:                  int(pb.Idx),
		DependentCommitments: convertInt32SliceToIntSlice(pb.DependentCommitments),
		Commitment:           bytesToDigest(pb.Commitment),
		Proof:                bytesToG1Affine(pb.Proof),
	}

	if pb.BlockRange != nil {
		rp.BlockRange = &proof.BlockRange{
			Start: int(pb.BlockRange.Start),
			End:   int(pb.BlockRange.End),
		}
	}

	return rp
}

// BalanceInfoFromProto converts a protobuf BalanceInfo to its internal representation.
func BalanceInfoFromProto(pb *proofpb.BalanceInfo) *tree.HistoricalBalance {
	return &tree.HistoricalBalance{
		CurrentBalance: tree.CurrentBalance{
			Version:    pb.Version,
			StartBlock: pb.StartBlock,
			Balance:    new(big.Int).SetBytes(pb.Balance),
		},
		EndBlock: pb.EndBlock,
	}
}

func convertInt32SliceToIntSlice(src []int32) []int {
	dst := make([]int, len(src))
	for i, v := range src {
		dst[i] = int(v)
	}
	return dst
}

func bytesToDigest(b []byte) gnark_kzg.Digest {
	var digest gnark_kzg.Digest
	digest.Unmarshal(b)
	return digest
}

func bytesToG1Affine(b []byte) bls.G1Affine {
	var p bls.G1Affine
	p.Unmarshal(b)
	return p
}
