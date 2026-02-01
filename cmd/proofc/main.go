// Package main provides a simple CLI client for testing the Samurai gRPC proof service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/ethereum/go-ethereum/common"
	proofpb "github.com/nepal80m/samurai/api/proto/v1"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/server"
	"github.com/nepal80m/samurai/internal/tree"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const StartBlock = uint64(18908895)

func main() {
	// Parse command-line flags
	serverAddr := flag.String("server", "localhost:50051", "gRPC server address")
	account := flag.String("account", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account address to query")
	startBlock := flag.Uint64("startBlock", 20, "Starting block number (relative to data start)")
	endBlock := flag.Uint64("endBlock", 119, "Ending block number (relative to data start)")
	paramsDir := flag.String("paramsDir", "/data/local/dataset/polynomial", "Path to crypto params")
	flag.Parse()

	// Initialize precomputed data
	precomputedData, err := SetupPrecomputedData(*paramsDir)
	if err != nil {
		log.Fatalf("failed to setup precomputed data: %v", err)
	}

	// Connect to the gRPC server
	conn, err := grpc.NewClient(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Create the client
	client := proofpb.NewProofServiceClient(conn)

	// Create the request
	req := &proofpb.GetProofRequest{
		Account:    *account,
		StartBlock: *startBlock + StartBlock,
		EndBlock:   *endBlock + StartBlock,
	}

	fmt.Printf("Requesting proof for account %s, blocks %d-%d\n", *account, *startBlock, *endBlock)

	// Call the server
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// start := time.Now()
	resp, err := client.GetProof(ctx, req)
	if err != nil {
		log.Fatalf("GetProof failed: %v", err)
	}

	addr := common.HexToAddress(*account)
	// rangeProofs := resp.RangeProofs

	rangeProofs := make([]*proof.RangeProof, len(resp.RangeProofs))
	for i, rp := range resp.RangeProofs {
		rangeProofs[i] = server.RangeProofFromProto(rp)
	}
	balanceInfos := make([]*tree.HistoricalBalance, len(resp.BalanceInfos))
	for i, bi := range resp.BalanceInfos {
		balanceInfos[i] = server.BalanceInfoFromProto(bi)
	}

	var startingVersion uint64 = math.MaxUint64
	var endingVersion uint64 = 0
	for _, balance := range balanceInfos {
		if balance.Version < startingVersion {
			startingVersion = balance.Version
		}
		if balance.Version > endingVersion {
			endingVersion = balance.Version
		}
	}

	proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, precomputedData)

	// elapsed := time.Since(start)

	// Print the response
	// fmt.Printf("\n=== Proof Response ===\n")
	// fmt.Printf("Total time (including network): %v\n", elapsed)
	// fmt.Printf("Server generation time: %d ms\n", resp.GenerationTimeMs)
	// fmt.Printf("Number of range proofs: %d\n", len(resp.RangeProofs))
	// fmt.Printf("Number of balance infos: %d\n", len(resp.BalanceInfos))

	// fmt.Printf("\n--- Range Proofs ---\n")
	// for i, rp := range resp.RangeProofs {
	// 	fmt.Printf("[%d] Layer=%d, Idx=%d", i, rp.Layer, rp.Idx)
	// 	if rp.BlockRange != nil {
	// 		fmt.Printf(", BlockRange=[%d, %d]", rp.BlockRange.Start, rp.BlockRange.End)
	// 	}
	// 	fmt.Printf(", DependentCommitments=%v\n", rp.DependentCommitments)
	// }

	// fmt.Printf("\n--- Balance Infos ---\n")
	// for i, bi := range resp.BalanceInfos {
	// 	balance := new(big.Int).SetBytes(bi.Balance)
	// 	fmt.Printf("[%d] Version=%d, StartBlock=%d, EndBlock=%d, Balance=%s\n",
	// 		i, bi.Version, bi.StartBlock, bi.EndBlock, balance.String())
	// }
}

// SetupPrecomputedData initializes SRS and precomputed polynomial data.
func SetupPrecomputedData(paramsDir string) (*config.PrecomputedData, error) {
	fmt.Println("Setting up precomputed data...")
	start := time.Now()
	srs, err := kzg.SetupSRS(tree.SegmentTreeSize)
	if err != nil {
		return nil, fmt.Errorf("failed to setup SRS: %w", err)
	}

	V, weights, weightCommits := kzg.LoadBarycentricData(tree.SegmentTreeSize, srs, paramsDir)
	fmt.Printf("Precomputed data setup took %v\n", time.Since(start))
	return &config.PrecomputedData{
		V:             V,
		Weights:       weights,
		WeightCommits: weightCommits,
		SRS:           srs,
	}, nil
}
