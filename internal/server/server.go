// Package server provides the gRPC server implementation for the Samurai proof service.
package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/ethereum/go-ethereum/common"
	proofpb "github.com/nepal80m/samurai/api/proto/v1"
	"github.com/nepal80m/samurai/internal/benchmark"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProofServer implements the ProofServiceServer gRPC interface.
type ProofServer struct {
	proofpb.UnimplementedProofServiceServer
	dbs              []*db.SamuraiDB
	precomputedData  *config.PrecomputedData
	cfg              *config.Config
	metricsCollector *benchmark.MetricsCollector
}

// NewProofServer creates a new ProofServer instance.
func NewProofServer(dbs []*db.SamuraiDB, precomputedData *config.PrecomputedData, cfg *config.Config, metricsCollector *benchmark.MetricsCollector) *ProofServer {
	return &ProofServer{
		dbs:              dbs,
		precomputedData:  precomputedData,
		cfg:              cfg,
		metricsCollector: metricsCollector,
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
	startingVersion, endingVersion := proof.BlockRangeToVersionRange(addr, startBlock, endBlock, s.cfg, sdb)

	// Generate proofs
	start := time.Now()
	rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startingVersion, endingVersion, s.precomputedData, s.cfg.Blocks.StartingBlockNumber, sdb)
	generationTimeMs := time.Since(start).Milliseconds()

	log.Printf("Generated %d range proofs and %d balance infos in %dms",
		len(rangeProofs), len(balanceInfos), generationTimeMs)

	// Convert to protobuf response
	resp := &proofpb.GetProofResponse{
		RangeProofs:      make([]*proofpb.RangeProof, len(rangeProofs)),
		BalanceInfos:     make([]*proofpb.BalanceInfo, len(balanceInfos)),
		GenerationTimeMs: generationTimeMs,
	}

	for i, rp := range rangeProofs {
		resp.RangeProofs[i] = rangeProofToProto(rp)
	}

	for i, bi := range balanceInfos {
		resp.BalanceInfos[i] = balanceInfoToProto(bi)
	}

	// Record metrics
	if s.metricsCollector != nil {
		s.metricsCollector.RecordProofGeneration(req.Account, startBlock, endBlock, generationTimeMs, len(rangeProofs), len(balanceInfos))
	}

	return resp, nil
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
