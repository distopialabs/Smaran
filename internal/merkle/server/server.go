package server

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	proofpb "github.com/nepal80m/samurai/api/proto/merkle/v1"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/merkle/meta"
	"github.com/nepal80m/samurai/internal/merkle/proof"
	st "github.com/nepal80m/samurai/internal/merkle/state"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ProofServer implements the gRPC ProofServiceServer interface.
type ProofServer struct {
	proofpb.UnimplementedProofServiceServer
	store    *st.MPTStateStore
	benchLog *benchutil.BenchLogger
}

// NewProofServer creates a new ProofServer.
func NewProofServer(store *st.MPTStateStore, benchLog *benchutil.BenchLogger) *ProofServer {
	return &ProofServer{store: store, benchLog: benchLog}
}

// GetRangeProof streams account proofs for each block in the range.
func (s *ProofServer) GetRangeProof(req *proofpb.GetRangeProofRequest, stream proofpb.ProofService_GetRangeProofServer) error {
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

	addr := common.HexToAddress(req.Account)
	numBlocks := req.EndBlock - req.StartBlock + 1

	log.Printf("GetRangeProof: account=%s blocks=%d..%d (%d blocks)",
		addr.Hex(), req.StartBlock, req.EndBlock, numBlocks)

	start := time.Now()
	sent := uint64(0)

	for blk := req.StartBlock; blk <= req.EndBlock; blk++ {
		if stream.Context().Err() != nil {
			return status.Error(codes.Canceled, "client disconnected")
		}

		bp, err := s.generateBlockProof(addr, blk)
		if err != nil {
			return status.Errorf(codes.Internal, "block %d: %v", blk, err)
		}

		if err := stream.Send(bp); err != nil {
			return err
		}
		sent++
	}

	genTimeNs := time.Since(start).Nanoseconds()
	log.Printf("Streamed %d block proofs in %dns", sent, genTimeNs)

	// Send generation time as trailing metadata so client can compute network overhead
	stream.SetTrailer(metadata.Pairs("proofgen_duration_ns", strconv.FormatInt(genTimeNs, 10)))
	return nil
}

// generateBlockProof creates a single BlockProof for one block.
func (s *ProofServer) generateBlockProof(addr common.Address, blockNum uint64) (*proofpb.BlockProof, error) {
	root, err := meta.GetRoot(s.store.DiskDB, blockNum)
	if err != nil {
		return nil, fmt.Errorf("no root for block %d: %w", blockNum, err)
	}

	stateDB, err := s.store.OpenState(root)
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}

	stateTrie, err := s.store.OpenTrie(root)
	if err != nil {
		return nil, fmt.Errorf("open trie: %w", err)
	}
	result, rawNodes, err := proof.GenerateAccountProof(stateDB, root, addr, stateTrie)
	if err != nil {
		return nil, fmt.Errorf("generate proof: %w", err)
	}

	accountProof := make([][]byte, len(rawNodes))
	copy(accountProof, rawNodes)

	var balBytes []byte
	exists := true
	if result.Balance == nil || result.Balance.ToInt().Sign() == 0 {
		e, _, _ := proof.VerifyAccountProof(root, addr, rawNodes)
		exists = e
		balBytes = big.NewInt(0).Bytes()
	} else {
		balBytes = result.Balance.ToInt().Bytes()
	}

	return &proofpb.BlockProof{
		BlockNumber:  blockNum,
		StateRoot:    root.Bytes(),
		AccountProof: accountProof,
		Balance:      balBytes,
		Nonce:        uint64(result.Nonce),
		Exists:       exists,
	}, nil
}

// GetInfo returns the latest processed block and its state root.
func (s *ProofServer) GetInfo(ctx context.Context, req *proofpb.GetInfoRequest) (*proofpb.GetInfoResponse, error) {
	lastBlock, err := meta.GetLast(s.store.DiskDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get last block: %v", err)
	}
	root, err := meta.GetRoot(s.store.DiskDB, lastBlock)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get root for block %d: %v", lastBlock, err)
	}
	return &proofpb.GetInfoResponse{
		LatestBlock: lastBlock,
		StateRoot:   root.Hex(),
	}, nil
}

// ListenAndServe starts the gRPC server with graceful shutdown on SIGINT/SIGTERM.
func ListenAndServe(addr string, server *ProofServer, opts ...grpc.ServerOption) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer(opts...)
	proofpb.RegisterProofServiceServer(grpcServer, server)

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
			os.Exit(1)
		}
	}()

	log.Printf("gRPC server listening on %s", addr)
	return grpcServer.Serve(lis)
}
