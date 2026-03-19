// Package server provides the gRPC server implementation for the Samurai proof service.
package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/common"
	proofpb "github.com/nepal80m/samurai/api/proto/v1"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"
	st "github.com/nepal80m/samurai/mpt/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProofServer implements the ProofServiceServer gRPC interface.
type ProofServer struct {
	proofpb.UnimplementedProofServiceServer
	dbs             []*db.SamuraiStore
	precomputedData *config.PrecomputedData
	cfg             *config.Config
	mptStore        *st.MPTStateStore
}

// NewProofServer creates a new ProofServer instance.
func NewProofServer(dbs []*db.SamuraiStore, precomputedData *config.PrecomputedData, cfg *config.Config, mptStore *st.MPTStateStore) *ProofServer {
	return &ProofServer{
		dbs:             dbs,
		precomputedData: precomputedData,
		cfg:             cfg,
		mptStore:        mptStore,
	}
}

// GetProof generates range proofs for a given account and block range.
func (s *ProofServer) GetProof(ctx context.Context, req *proofpb.GetProofRequest) (*proofpb.GetProofResponse, error) {
	// Validate request
	if req.Account == "" {
		return nil, status.Error(codes.InvalidArgument, "account address is required")
	}
	if req.EndBlock < req.StartBlock {
		return nil, status.Error(codes.InvalidArgument, "end_block must be >= start_block")
	}

	// Parse account address
	addr := common.HexToAddress(req.Account)

	// Calculate actual block numbers (add offset)
	// startBlock := req.StartBlock + s.cfg.Blocks.StartingBlockNumber
	// endBlock := req.EndBlock + s.cfg.Blocks.StartingBlockNumber
	startBlock := req.StartBlock
	endBlock := req.EndBlock

	// Get the appropriate shard database
	shardIdx := utils.AddressToShardIndex(addr, s.cfg.Database.Shards)
	sdb := s.dbs[shardIdx]

	log.Printf("GetProof request: account=%s, startBlock=%d, endBlock=%d, shard=%d",
		addr.Hex(), startBlock, endBlock, shardIdx)

	// Convert block range to version range
	startingVersion, endingVersion, err := proof.BlockRangeToVersionRange(addr, startBlock, endBlock, s.cfg, sdb)
	if err != nil {
		log.Printf("Error converting block range [%d, %d] to version range for account %s: %v", startBlock, endBlock, addr.Hex(), err)
		// Check error type using sentinel errors
		if errors.Is(err, proof.ErrBlockRangeOutOfBounds) {
			return nil, status.Errorf(codes.OutOfRange, "block range outside account's recorded history: %v", err)
		}
		if errors.Is(err, proof.ErrAccountNotFound) {
			return nil, status.Errorf(codes.NotFound, "account not found: %v", err)
		}
		if errors.Is(err, proof.ErrVersionNotFound) {
			return nil, status.Errorf(codes.Internal, "failed to find version: %v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to process block range: %v", err)
	}

	log.Printf("Resolved version range: startVersion=%d, endVersion=%d", startingVersion, endingVersion)

	// Generate proofs
	start := time.Now()
	rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startingVersion, endingVersion, s.precomputedData, sdb)
	generationTimeMs := time.Since(start).Milliseconds()

	log.Printf("Generated %d range proofs and %d balance infos in %dms",
		len(rangeProofs), len(balanceInfos), generationTimeMs)

	// Get current balance for the account
	cbInfo, err := tree.GetCurrentBalanceInfo(addr, sdb.StateDB)
	if err != nil {
		log.Printf("Error getting current balance for account %s: %v", addr.Hex(), err)
		return nil, status.Errorf(codes.Internal, "failed to get current balance: %v", err)
	}

	// Generate MPT proof
	var mptProofNodes [][]byte
	var mptBlockNumber uint64
	if s.mptStore != nil {
		mptBlockNumber, mptProofNodes, err = proof.GenerateMPTProof(s.mptStore, addr)
		if err != nil {
			log.Printf("Error generating MPT proof for account %s: %v", addr.Hex(), err)
			return nil, status.Errorf(codes.Internal, "failed to generate MPT proof: %v", err)
		}
		log.Printf("Generated MPT proof with %d nodes for block %d", len(mptProofNodes), mptBlockNumber)
	}

	// Convert to protobuf response
	resp := &proofpb.GetProofResponse{
		RangeProofs:      make([]*proofpb.RangeProof, len(rangeProofs)),
		BalanceInfos:     make([]*proofpb.BalanceInfo, len(balanceInfos)),
		GenerationTimeMs: generationTimeMs,
		MptProofNodes:    mptProofNodes,
		CurrentBalance:   cbInfo.Bytes(),
		MptBlockNumber:   mptBlockNumber,
	}

	for i, rp := range rangeProofs {
		resp.RangeProofs[i] = rangeProofToProto(rp)
	}

	for i, bi := range balanceInfos {
		resp.BalanceInfos[i] = balanceInfoToProto(bi)
	}

	return resp, nil
}

// GetProofStream streams range proofs for a given account and block range.
func (s *ProofServer) GetProofStream(req *proofpb.GetProofRequest, stream proofpb.ProofService_GetProofStreamServer) error {
	// Validate request
	if req.Account == "" {
		return status.Error(codes.InvalidArgument, "account address is required")
	}
	if req.EndBlock < req.StartBlock {
		return status.Error(codes.InvalidArgument, "end_block must be >= start_block")
	}

	// Parse account address
	addr := common.HexToAddress(req.Account)

	startBlock := req.StartBlock
	endBlock := req.EndBlock

	// Get the appropriate shard database
	shardIdx := utils.AddressToShardIndex(addr, s.cfg.Database.Shards)
	sdb := s.dbs[shardIdx]

	log.Printf("GetProofStream request: account=%s, startBlock=%d, endBlock=%d, shard=%d",
		addr.Hex(), startBlock, endBlock, shardIdx)

	// Convert block range to version range
	startingVersion, endingVersion, err := proof.BlockRangeToVersionRange(addr, startBlock, endBlock, s.cfg, sdb)
	if err != nil {
		log.Printf("Error converting block range [%d, %d] to version range for account %s: %v", startBlock, endBlock, addr.Hex(), err)
		if errors.Is(err, proof.ErrBlockRangeOutOfBounds) {
			return status.Errorf(codes.OutOfRange, "block range outside account's recorded history: %v", err)
		}
		if errors.Is(err, proof.ErrAccountNotFound) {
			return status.Errorf(codes.NotFound, "account not found: %v", err)
		}
		if errors.Is(err, proof.ErrVersionNotFound) {
			return status.Errorf(codes.Internal, "failed to find version: %v", err)
		}
		return status.Errorf(codes.Internal, "failed to process block range: %v", err)
	}

	log.Printf("Resolved version range: startVersion=%d, endVersion=%d", startingVersion, endingVersion)

	// Generate proofs
	start := time.Now()
	rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startingVersion, endingVersion, s.precomputedData, sdb)
	generationTimeMs := time.Since(start).Milliseconds()

	log.Printf("Generated %d range proofs and %d balance infos in %dms. Streaming...",
		len(rangeProofs), len(balanceInfos), generationTimeMs)

	// Get current balance for the account
	cbInfo, err := tree.GetCurrentBalanceInfo(addr, sdb.StateDB)
	if err != nil {
		log.Printf("Error getting current balance for account %s: %v", addr.Hex(), err)
		return status.Errorf(codes.Internal, "failed to get current balance: %v", err)
	}

	// Generate MPT proof
	var mptProofNodes [][]byte
	var mptBlockNumber uint64
	if s.mptStore != nil {
		mptBlockNumber, mptProofNodes, err = proof.GenerateMPTProof(s.mptStore, addr)
		if err != nil {
			log.Printf("Error generating MPT proof for account %s: %v", addr.Hex(), err)
			return status.Errorf(codes.Internal, "failed to generate MPT proof: %v", err)
		}
		log.Printf("Generated MPT proof with %d nodes for block %d", len(mptProofNodes), mptBlockNumber)
	}

	// Range proofs are small (max 7), we can send them all in the first message
	protoRangeProofs := make([]*proofpb.RangeProof, len(rangeProofs))
	for i, rp := range rangeProofs {
		protoRangeProofs[i] = rangeProofToProto(rp)
	}

	// Send BalanceInfos in chunks
	const chunkSize = 5000

	// Handle the case where there are no balance infos but we still want to return a response
	if len(balanceInfos) == 0 {
		return stream.Send(&proofpb.GetProofResponse{
			RangeProofs:      protoRangeProofs,
			BalanceInfos:     nil,
			GenerationTimeMs: generationTimeMs,
			MptProofNodes:    mptProofNodes,
			CurrentBalance:   cbInfo.Bytes(),
			MptBlockNumber:   mptBlockNumber,
		})
	}

	for i := 0; i < len(balanceInfos); i += chunkSize {
		balanceEnd := i + chunkSize
		if balanceEnd > len(balanceInfos) {
			balanceEnd = len(balanceInfos)
		}

		resp := &proofpb.GetProofResponse{
			RangeProofs:      nil,
			BalanceInfos:     make([]*proofpb.BalanceInfo, len(balanceInfos[i:balanceEnd])),
			GenerationTimeMs: 0,
		}

		if i == 0 {
			resp.RangeProofs = protoRangeProofs
			resp.GenerationTimeMs = generationTimeMs
			resp.MptProofNodes = mptProofNodes
			resp.CurrentBalance = cbInfo.Bytes()
			resp.MptBlockNumber = mptBlockNumber
		}

		for j, bi := range balanceInfos[i:balanceEnd] {
			resp.BalanceInfos[j] = balanceInfoToProto(bi)
		}

		if err := stream.Send(resp); err != nil {
			return status.Errorf(codes.Internal, "failed to stream proof batch: %v", err)
		}
	}

	return nil
}

// ListenAndServe starts the gRPC server on the specified address.
func ListenAndServe(addr string, server *ProofServer) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	proofpb.RegisterProofServiceServer(grpcServer, server)

	log.Printf("Starting gRPC server on %s", addr)
	return grpcServer.Serve(lis)
}
