package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	proofpb "github.com/nepal80m/samurai/api/proto/verkle/v1"
	"github.com/nepal80m/samurai/internal/benchutil"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verkle/proof"
	"github.com/nepal80m/samurai/internal/verkle/store"
	verkle "github.com/ethereum/go-verkle"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ProofServer implements the gRPC VerkleProofServiceServer interface.
type ProofServer struct {
	proofpb.UnimplementedVerkleProofServiceServer
	ns       *store.NodeStore
	benchLog *benchutil.BenchLogger
}

// NewProofServer creates a new ProofServer.
func NewProofServer(ns *store.NodeStore, benchLog *benchutil.BenchLogger) *ProofServer {
	return &ProofServer{ns: ns, benchLog: benchLog}
}

// GetRangeProof streams account proofs for each block in the range.
func (s *ProofServer) GetRangeProof(req *proofpb.GetRangeProofRequest, stream proofpb.VerkleProofService_GetRangeProofServer) error {
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

	// Parse address
	addrStr := strings.TrimPrefix(req.Account, "0x")
	addrBytes, err := hex.DecodeString(addrStr)
	if err != nil || len(addrBytes) != 20 {
		return status.Errorf(codes.InvalidArgument, "invalid address: %s", req.Account)
	}
	var addr [20]byte
	copy(addr[:], addrBytes)

	numBlocks := req.EndBlock - req.StartBlock + 1
	log.Printf("GetRangeProof: account=0x%s blocks=%d..%d (%d blocks)",
		addrStr, req.StartBlock, req.EndBlock, numBlocks)

	start := time.Now()
	sent := uint64(0)

	// Create a cache for resolved nodes during this range request.
	// Keys are string(path), values are the serialized node bytes.
	// Since node versions change across blocks, we technically should cache
	// by (blockNum, path). However, to avoid repetitive DB hits for the
	// *same* block's proof (e.g., Get + MakeVerkleMultiProof both resolving),
	// and to reuse unchanged nodes across sequential blocks, we can just let
	// Pebble's OS page cache handle the sequential blocks, and use a simple
	// per-block cache here to avoid the 3-4x repetitive lookups within a single block.
	// Actually, let's cache by path. Since sequential blocks share 99% of nodes,
	// caching the latest resolved node for a path is highly effective. If it's the
	// wrong version (e.g., block N+1 needs a newer version than block N), our
	// VersionedNodeResolverFn handles finding the *correct* version from DB.
	// Wait, we cannot naively cache across blocks without checking versions.
	// Let's implement a simple per-block cache first to see the impact.
	for blk := req.StartBlock; blk <= req.EndBlock; blk++ {
		if stream.Context().Err() != nil {
			return status.Error(codes.Canceled, "client disconnected")
		}

		// Per-block cache for resolved nodes to avoid repetitive lookups during one proof gen
		blockCache := make(map[string][]byte)
		baseResolver := s.ns.VersionedNodeResolverFn(blk)

		cachingResolver := func(path []byte) ([]byte, error) {
			pathStr := string(path)
			if cached, ok := blockCache[pathStr]; ok {
				return cached, nil
			}
			val, err := baseResolver(path)
			if err == nil {
				blockCache[pathStr] = val
			}
			return val, err
		}

		bp, err := s.generateBlockProof(addr, blk, cachingResolver)
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

	// Send generation time as trailing metadata
	stream.SetTrailer(metadata.Pairs("proofgen_duration_ns", strconv.FormatInt(genTimeNs, 10)))
	return nil
}

// generateBlockProof creates a single BlockProof for one block.
func (s *ProofServer) generateBlockProof(addr [20]byte, blockNum uint64, resolver verkle.NodeResolverFn) (*proofpb.BlockProof, error) {
	// Parse tree at this specific block's state.
	rootBytes, err := resolver(nil)
	if err != nil {
		return nil, fmt.Errorf("load root node: %w", err)
	}
	root, err := verkle.ParseNode(rootBytes, 0)
	if err != nil {
		return nil, fmt.Errorf("parse root node: %w", err)
	}

	// Derive compressed root commitment from the loaded tree
	rootCommitmentBytes := proof.SerializeCommitment(root)

	// Check existence and get balance
	treeKey := keys.GetTreeKeyForBasicData(addr)
	keySlice := treeKey[:]
	val, err := root.Get(keySlice, resolver)
	exists := err == nil && val != nil

	var balance *big.Int
	if exists {
		var val32 [32]byte
		copy(val32[:], val)
		balance = keys.UnpackBalance(val32)
	} else {
		balance = new(big.Int)
	}

	// Generate proof
	verkleProof, _, _, _, err := verkle.MakeVerkleMultiProof(root, root, [][]byte{keySlice}, resolver)
	if err != nil {
		return nil, fmt.Errorf("MakeVerkleMultiProof: %w", err)
	}

	vp, sd, err := verkle.SerializeProof(verkleProof)
	if err != nil {
		return nil, fmt.Errorf("SerializeProof: %w", err)
	}

	vpJSON, err := json.Marshal(vp)
	if err != nil {
		return nil, fmt.Errorf("marshal VerkleProof: %w", err)
	}
	sdJSON, err := json.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("marshal StateDiff: %w", err)
	}

	return &proofpb.BlockProof{
		BlockNumber:    blockNum,
		RootCommitment: rootCommitmentBytes,
		VerkleProof:    vpJSON,
		StateDiff:      sdJSON,
		Balance:        balance.Bytes(),
		Exists:         exists,
	}, nil
}

// GenerateProofResult wraps proof.GenerateProof for server use, loading tree from DB.
func (s *ProofServer) GenerateProofResult(addr [20]byte, blockNum uint64) (*proof.ProofResult, *proof.Metrics, error) {
	root, err := s.ns.LoadTree(blockNum)
	if err != nil {
		return nil, nil, fmt.Errorf("load tree: %w", err)
	}
	rootBytes := proof.SerializeCommitment(root)
	resolver := s.ns.VersionedNodeResolverFn(blockNum)
	return proof.GenerateProof(root, addr, rootBytes, resolver)
}

// GetInfo returns the latest processed block and its root commitment.
func (s *ProofServer) GetInfo(ctx context.Context, req *proofpb.GetInfoRequest) (*proofpb.GetInfoResponse, error) {
	lastBlock, ok := s.ns.GetLastProcessed()
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "no blocks processed yet")
	}
	rootBytes, err := s.ns.GetRootCommitment(lastBlock)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get root commitment for block %d: %v", lastBlock, err)
	}
	return &proofpb.GetInfoResponse{
		LatestBlock: lastBlock,
		StateRoot:   hex.EncodeToString(rootBytes),
	}, nil
}

// ListenAndServe starts the gRPC server with graceful shutdown on SIGINT/SIGTERM.
func ListenAndServe(addr string, server *ProofServer, opts ...grpc.ServerOption) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer(opts...)
	proofpb.RegisterVerkleProofServiceServer(grpcServer, server)

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
