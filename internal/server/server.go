// Package server provides the gRPC server implementation for the Samurai proof service.
package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	proofpb "github.com/nepal80m/samurai/api/proto/samurai/v1"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/db"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/tree"
	"github.com/nepal80m/samurai/internal/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProofServer implements the ProofServiceServer gRPC interface.
type ProofServer struct {
	proofpb.UnimplementedProofServiceServer

	dbs             []*db.SamuraiStore
	precomputedData *config.PrecomputedData
	mptStore        *st.MPTStateStore
	benchLog        *benchutil.BenchLogger // nil when --bench is off
}

// NewProofServer creates a new ProofServer instance.
// benchLog may be nil to disable server-side bench logging.
func NewProofServer(dbs []*db.SamuraiStore, precomputedData *config.PrecomputedData, mptStore *st.MPTStateStore, benchLog *benchutil.BenchLogger) *ProofServer {
	return &ProofServer{
		dbs:             dbs,
		precomputedData: precomputedData,
		mptStore:        mptStore,
		benchLog:        benchLog,
	}
}

// proofResult holds all data needed to build a proof response.
type proofResult struct {
	rangeProofs        []*proof.RangeProof
	balanceInfos       []*tree.HistoricalBalance
	cbInfo             *tree.CurrentBalance
	mptProofNodes      [][]byte
	mptBlockNumber     uint64
	proofgenDurationNs int64
}

// generateProof handles all proof request cases for a given account and block range.
// It routes to the correct case based on the account state:
//   - cbInfo nil:                               NotFound error
//   - version==0 && startBlock <= endBlock:      cbInfo + MPT only
//   - version==0 && endBlock < startBlock:       OutOfRange error
//   - version>0 && cbInfo.StartBlock <= startBlock: top-layer commitment + cbInfo + MPT
//   - version>0 && endBlock < firstHB.StartBlock:   OutOfRange error
//   - version>0 && normal range:                    full range proofs + cbInfo + MPT
func (s *ProofServer) generateProof(ctx context.Context, addr common.Address, startBlock, endBlock uint64) (*proofResult, error) {
	shardIdx := utils.AddressToShardIndex(addr, len(s.dbs))
	sdb := s.dbs[shardIdx]

	log.Printf("generateProof: account=%s, startBlock=%d, endBlock=%d, shard=%d",
		addr.Hex(), startBlock, endBlock, shardIdx)

	// Fetch current balance info.
	cbInfo, err := tree.GetCurrentBalanceInfo(addr, &sdb.StateDB)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "account not found: %v", err)
	}

	// Generate MPT proof (all non-error paths need it).
	var mptProofNodes [][]byte
	var mptBlockNumber uint64
	if s.mptStore != nil {
		mptBlockNumber, mptProofNodes, err = proof.GenerateMPTProof(s.mptStore, addr)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to generate MPT proof: %v", err)
		}
	}

	// Case: version == 0 (account exists but has no historical balances).
	if cbInfo.Version == 0 {
		if cbInfo.StartBlock <= endBlock {
			// log.Printf("Case: version==0, returning cbInfo + MPT only")
			return &proofResult{
				cbInfo:         cbInfo,
				mptProofNodes:  mptProofNodes,
				mptBlockNumber: mptBlockNumber,
			}, nil
		}
		return nil, status.Errorf(codes.OutOfRange,
			"block range outside account's recorded history: account starts at block %d, query ends at block %d",
			cbInfo.StartBlock, endBlock)
	}

	// Case: current balance covers the entire query range.
	if cbInfo.StartBlock <= startBlock {
		// log.Printf("Case: cbInfo.StartBlock(%d) <= startBlock(%d), returning top-layer commitment only",
		// cbInfo.StartBlock, startBlock)
		rangeProofs := proof.GetLatestTopLayerCommitmentAsRangeProof(addr, cbInfo, sdb)
		return &proofResult{
			rangeProofs:    rangeProofs,
			cbInfo:         cbInfo,
			mptProofNodes:  mptProofNodes,
			mptBlockNumber: mptBlockNumber,
		}, nil
	}

	// Case: need historical data — check if any exists in range.
	firstHB := tree.GetHistoricalBalance(addr, 0, &sdb.HistoryDB)
	if endBlock < firstHB.StartBlock {
		return nil, status.Errorf(codes.OutOfRange,
			"block range outside account's recorded history: query ends at block %d, first recorded block is %d",
			endBlock, firstHB.StartBlock)
	}

	// Compute version range.
	var endVersion uint64
	if cbInfo.StartBlock <= endBlock {
		endVersion = cbInfo.Version - 1
	} else {
		endVersion, err = proof.BinarySearchVersionByBlockNumber(endBlock, 0, cbInfo.Version-1, addr, sdb)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to find version for ending block %d: %v", endBlock, err)
		}
	}

	var startVersion uint64
	if startBlock <= firstHB.StartBlock {
		startVersion = 0
	} else {
		startVersion, err = proof.BinarySearchVersionByBlockNumber(startBlock, 0, endVersion, addr, sdb)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to find version for starting block %d: %v", startBlock, err)
		}
	}

	// log.Printf("Case: full range, versions [%d, %d]", startVersion, endVersion)

	// Bail out if the client already disconnected before the expensive proof generation.
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	// Generate full range proofs.
	start := time.Now()
	rangeProofs, balanceInfos := proof.GetNewProofRange(addr, startVersion, endVersion, s.precomputedData, sdb)
	proofgenDurationNs := time.Since(start).Nanoseconds()

	// log.Printf("Generated %d range proofs and %d balance infos in %dns",
	// len(rangeProofs), len(balanceInfos), proofgenDurationNs)

	return &proofResult{
		rangeProofs:        rangeProofs,
		balanceInfos:       balanceInfos,
		cbInfo:             cbInfo,
		mptProofNodes:      mptProofNodes,
		mptBlockNumber:     mptBlockNumber,
		proofgenDurationNs: proofgenDurationNs,
	}, nil
}

// resultToProtoResponse converts a proofResult to a GetProofResponse.
func resultToProtoResponse(res *proofResult) *proofpb.GetProofResponse {
	resp := &proofpb.GetProofResponse{
		RangeProofs:        make([]*proofpb.RangeProof, len(res.rangeProofs)),
		BalanceInfos:       make([]*proofpb.BalanceInfo, len(res.balanceInfos)),
		ProofgenDurationNs: res.proofgenDurationNs,
		MptProofNodes:      res.mptProofNodes,
		CurrentBalance:     res.cbInfo.Bytes(),
		MptBlockNumber:     res.mptBlockNumber,
	}
	for i, rp := range res.rangeProofs {
		resp.RangeProofs[i] = rangeProofToProto(rp)
	}
	for i, bi := range res.balanceInfos {
		resp.BalanceInfos[i] = balanceInfoToProto(bi)
	}
	return resp
}

// GetProof generates range proofs for a given account and block range.
func (s *ProofServer) GetProof(ctx context.Context, req *proofpb.GetProofRequest) (*proofpb.GetProofResponse, error) {
	if req.Account == "" {
		return nil, status.Error(codes.InvalidArgument, "account address is required")
	}
	if req.EndBlock < req.StartBlock {
		return nil, status.Error(codes.InvalidArgument, "end_block must be >= start_block")
	}

	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	res, err := s.generateProof(ctx, common.HexToAddress(req.Account), req.StartBlock, req.EndBlock)
	if err != nil {
		return nil, err
	}

	return resultToProtoResponse(res), nil
}

// streamProofResult sends a proofResult over a gRPC stream, chunking balance infos.
func streamProofResult(res *proofResult, stream proofpb.ProofService_GetProofStreamServer) error {
	protoRangeProofs := make([]*proofpb.RangeProof, len(res.rangeProofs))
	for i, rp := range res.rangeProofs {
		protoRangeProofs[i] = rangeProofToProto(rp)
	}

	const chunkSize = 5000

	if len(res.balanceInfos) == 0 {
		return stream.Send(&proofpb.GetProofResponse{
			RangeProofs:        protoRangeProofs,
			ProofgenDurationNs: res.proofgenDurationNs,
			MptProofNodes:      res.mptProofNodes,
			CurrentBalance:     res.cbInfo.Bytes(),
			MptBlockNumber:     res.mptBlockNumber,
		})
	}

	for i := 0; i < len(res.balanceInfos); i += chunkSize {
		balanceEnd := i + chunkSize
		if balanceEnd > len(res.balanceInfos) {
			balanceEnd = len(res.balanceInfos)
		}

		resp := &proofpb.GetProofResponse{
			BalanceInfos: make([]*proofpb.BalanceInfo, len(res.balanceInfos[i:balanceEnd])),
		}

		if i == 0 {
			resp.RangeProofs = protoRangeProofs
			resp.ProofgenDurationNs = res.proofgenDurationNs
			resp.MptProofNodes = res.mptProofNodes
			resp.CurrentBalance = res.cbInfo.Bytes()
			resp.MptBlockNumber = res.mptBlockNumber
		}

		for j, bi := range res.balanceInfos[i:balanceEnd] {
			resp.BalanceInfos[j] = balanceInfoToProto(bi)
		}

		if err := stream.Send(resp); err != nil {
			return status.Errorf(codes.Internal, "failed to stream proof batch: %v", err)
		}
	}

	return nil
}

// GetProofStream streams range proofs for a given account and block range.
func (s *ProofServer) GetProofStream(req *proofpb.GetProofRequest, stream proofpb.ProofService_GetProofStreamServer) error {
	var benchStartNs int64
	if s.benchLog != nil {
		benchStartNs = time.Now().UnixNano()
		defer func() {
			s.benchLog.Log(benchutil.BenchRecord{
				StartNs:     benchStartNs,
				CompletedNs: time.Now().UnixNano(),
			})
		}()
	}

	if req.Account == "" {
		return status.Error(codes.InvalidArgument, "account address is required")
	}
	if req.EndBlock < req.StartBlock {
		return status.Error(codes.InvalidArgument, "end_block must be >= start_block")
	}

	if err := stream.Context().Err(); err != nil {
		return status.FromContextError(err).Err()
	}

	res, err := s.generateProof(stream.Context(), common.HexToAddress(req.Account), req.StartBlock, req.EndBlock)
	if err != nil {
		return err
	}

	return streamProofResult(res, stream)
}

// GetOldProofStream is a legacy alias for GetProofStream.
func (s *ProofServer) GetOldProofStream(req *proofpb.GetProofRequest, stream proofpb.ProofService_GetProofStreamServer) error {
	var benchStartNs int64
	if s.benchLog != nil {
		benchStartNs = time.Now().UnixNano()
		defer func() {
			s.benchLog.Log(benchutil.BenchRecord{
				StartNs:     benchStartNs,
				CompletedNs: time.Now().UnixNano(),
			})
		}()
	}

	if req.Account == "" {
		return status.Error(codes.InvalidArgument, "account address is required")
	}
	if req.EndBlock < req.StartBlock {
		return status.Error(codes.InvalidArgument, "end_block must be >= start_block")
	}

	if err := stream.Context().Err(); err != nil {
		return status.FromContextError(err).Err()
	}

	res, err := s.generateProof(stream.Context(), common.HexToAddress(req.Account), req.StartBlock, req.EndBlock)
	if err != nil {
		return err
	}

	return streamProofResult(res, stream)
}

// GetInfo returns the latest processed block and its state root.
func (s *ProofServer) GetInfo(ctx context.Context, req *proofpb.GetInfoRequest) (*proofpb.GetInfoResponse, error) {
	if s.mptStore == nil {
		return nil, status.Error(codes.FailedPrecondition, "MPT store not configured")
	}
	lastBlock, err := meta.GetLast(s.mptStore.DiskDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get last block: %v", err)
	}
	root, err := meta.GetRoot(s.mptStore.DiskDB, lastBlock)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get root for block %d: %v", lastBlock, err)
	}
	return &proofpb.GetInfoResponse{
		LatestBlock: lastBlock,
		StateRoot:   root.Hex(),
	}, nil
}

// ListenAndServe starts the gRPC server on the specified address with graceful
// shutdown on SIGINT/SIGTERM. Optional grpc.ServerOption values (e.g.
// grpc.MaxConcurrentStreams) are forwarded to grpc.NewServer.
func ListenAndServe(addr string, server *ProofServer, opts ...grpc.ServerOption) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer(opts...)
	proofpb.RegisterProofServiceServer(grpcServer, server)

	// Graceful shutdown on SIGINT/SIGTERM with timeout fallback to force stop.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down gracefully...", sig)
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
			log.Println("graceful shutdown complete")
		case <-time.After(5 * time.Second):
			log.Println("graceful shutdown timed out, forcing exit")
			// Don't call grpcServer.Stop() — it contends on an internal mutex
			// with the concurrent GracefulStop and can block indefinitely.
			// Just exit: the server is read-only, the bench CSV is periodically
			// flushed (every 1s), and the OS cleans up sockets/FDs on exit.
			os.Exit(1)
		}
	}()

	log.Printf("Starting gRPC server on %s", addr)
	return grpcServer.Serve(lis)
}
