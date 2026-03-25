// Package server implements a gRPC proof service for the Verkle-KZG trie.
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
	"github.com/nepal80m/samurai/internal/verkle/keys"
	"github.com/nepal80m/samurai/internal/verklekzg/proof"
	"github.com/nepal80m/samurai/internal/verklekzg/store"
	"github.com/nepal80m/samurai/internal/verklekzg/tree"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ProofServer struct {
	proofpb.UnimplementedVerkleProofServiceServer
	ns      *store.NodeStore
	treeCfg *tree.TreeConfig
}

func NewProofServer(ns *store.NodeStore, treeCfg *tree.TreeConfig) *ProofServer {
	return &ProofServer{ns: ns, treeCfg: treeCfg}
}

func (s *ProofServer) GetRangeProof(req *proofpb.GetRangeProofRequest, stream proofpb.VerkleProofService_GetRangeProofServer) error {
	if req.Account == "" {
		return status.Error(codes.InvalidArgument, "account address is required")
	}
	if req.EndBlock < req.StartBlock {
		return status.Error(codes.InvalidArgument, "end_block must be >= start_block")
	}

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
	stream.SetTrailer(metadata.Pairs("proofgen_duration_ns", strconv.FormatInt(genTimeNs, 10)))
	return nil
}

func (s *ProofServer) generateBlockProof(addr [20]byte, blockNum uint64) (*proofpb.BlockProof, error) {
	root, err := s.ns.LoadTree(blockNum)
	if err != nil {
		return nil, fmt.Errorf("load tree: %w", err)
	}
	resolver := s.ns.VersionedNodeResolverFn(blockNum)
	rootCommitmentBytes := proof.SerializeCommitment(root)

	treeKey := keys.GetTreeKeyForBasicData(addr)
	keySlice := treeKey[:]
	val, getErr := root.Get(keySlice, resolver)
	exists := getErr == nil && val != nil

	var balance *big.Int
	if exists {
		var val32 [32]byte
		copy(val32[:], val)
		balance = keys.UnpackBalance(val32)
	} else {
		balance = new(big.Int)
	}

	result, _, err := proof.GenerateProof(root, addr, rootCommitmentBytes, resolver, s.treeCfg)
	if err != nil {
		return nil, fmt.Errorf("generate proof: %w", err)
	}

	return &proofpb.BlockProof{
		BlockNumber:    blockNum,
		RootCommitment: rootCommitmentBytes,
		VerkleProof:    result.Proof,
		Balance:        balance.Bytes(),
		Exists:         exists,
	}, nil
}

func (s *ProofServer) GenerateProofResult(addr [20]byte, blockNum uint64) (*proof.ProofResult, *proof.Metrics, error) {
	root, err := s.ns.LoadTree(blockNum)
	if err != nil {
		return nil, nil, fmt.Errorf("load tree: %w", err)
	}
	rootBytes := proof.SerializeCommitment(root)
	resolver := s.ns.VersionedNodeResolverFn(blockNum)
	return proof.GenerateProof(root, addr, rootBytes, resolver, s.treeCfg)
}

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

func ListenAndServe(addr string, server *ProofServer) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	proofpb.RegisterVerkleProofServiceServer(grpcServer, server)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received %v, shutting down gracefully...", sig)
		grpcServer.GracefulStop()
	}()

	log.Printf("gRPC Verkle-KZG server listening on %s", addr)
	return grpcServer.Serve(lis)
}

func VerifyBlockProofJSON(rootCommitment, proofJSON []byte, cfg *tree.TreeConfig) error {
	var vkProof proof.VerkleKZGProof
	if err := json.Unmarshal(proofJSON, &vkProof); err != nil {
		return fmt.Errorf("unmarshal proof: %w", err)
	}
	return proof.VerifyProof(rootCommitment, &vkProof, cfg)
}
