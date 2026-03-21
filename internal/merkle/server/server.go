package server

import (
	"fmt"
	"log"
	"math/big"
	"net"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	proofpb "github.com/nepal80m/samurai/api/proto/merkle/v1"
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
	store *st.MPTStateStore
}

// NewProofServer creates a new ProofServer.
func NewProofServer(store *st.MPTStateStore) *ProofServer {
	return &ProofServer{store: store}
}

// GetRangeProof streams account proofs for each block in the range.
func (s *ProofServer) GetRangeProof(req *proofpb.GetRangeProofRequest, stream proofpb.ProofService_GetRangeProofServer) error {
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

	genTimeMs := time.Since(start).Milliseconds()
	log.Printf("Streamed %d block proofs in %dms", sent, genTimeMs)

	// Send generation time as trailing metadata so client can compute network overhead
	stream.SetTrailer(metadata.Pairs("generation_time_ms", strconv.FormatInt(genTimeMs, 10)))
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

	stateTrie := stateDB.GetTrie()
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

// ListenAndServe starts the gRPC server.
func ListenAndServe(addr string, server *ProofServer) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	grpcServer := grpc.NewServer()
	proofpb.RegisterProofServiceServer(grpcServer, server)

	log.Printf("gRPC server listening on %s", addr)
	return grpcServer.Serve(lis)
}
