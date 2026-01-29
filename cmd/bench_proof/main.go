package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	proofpb "github.com/nepal80m/samurai/api/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// StartBlock defines the base start block for queries
	StartBlock = uint64(18908895)
)

type BenchmarkResult struct {
	Label        string
	RequestCount int
	TotalTime    time.Duration
	MinLatency   time.Duration
	MaxLatency   time.Duration
	AvgLatency   time.Duration
	Throughput   float64 // req/sec
	SuccessCount int
	FailureCount int
}

func main() {
	serverAddr := flag.String("server", "localhost:50051", "gRPC server address")
	mode := flag.String("mode", "range", "Benchmark mode: 'range' or 'accounts'")
	account := flag.String("account", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account address to query (for range mode)")
	concurrency := flag.Int("concurrency", 1, "Number of concurrent requests (for accounts mode)")
	flag.Parse()

	conn, err := grpc.NewClient(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := proofpb.NewProofServiceClient(conn)

	fmt.Printf("Starting Proof Benchmark\n")
	fmt.Printf("Server: %s\n", *serverAddr)
	fmt.Printf("Mode: %s\n", *mode)

	switch *mode {
	case "range":
		runRangeBenchmark(client, *account)
	case "accounts":
		runAccountBenchmark(client, *concurrency)
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

func runRangeBenchmark(client proofpb.ProofServiceClient, account string) {
	ranges := []struct {
		Label  string
		Blocks uint64
	}{
		{"1 Week", 50000},     // ~1 week @ 12s/block
		{"1 Month", 200000},   // ~1 month
		{"3 Months", 600000},  // ~3 months
		{"6 Months", 1200000}, // ~6 months
	}

	fmt.Printf("\n=== Range Variance Benchmark ===\n")
	fmt.Printf("%-15s %-15s %-15s\n", "Range", "Blocks", "Latency")

	for _, r := range ranges {
		req := &proofpb.GetProofRequest{
			Account:    account,
			StartBlock: StartBlock,
			EndBlock:   StartBlock + r.Blocks,
		}

		start := time.Now()
		_, err := client.GetProof(context.Background(), req)
		latency := time.Since(start)

		if err != nil {
			fmt.Printf("%-15s %-15d ERROR: %v\n", r.Label, r.Blocks, err)
		} else {
			fmt.Printf("%-15s %-15d %v\n", r.Label, r.Blocks, latency)
		}
	}
}

func runAccountBenchmark(client proofpb.ProofServiceClient, concurrency int) {
	counts := []int{1, 5, 10, 50, 100, 1000}
	rangeSize := uint64(200000) // 1 Month fixed range

	fmt.Printf("\n=== Account Variance Benchmark ===\n")
	fmt.Printf("Fixed Range: %d blocks\n", rangeSize)
	fmt.Printf("%-10s %-15s %-15s %-15s\n", "Requests", "Total Time", "Avg Latency", "Throughput (req/s)")

	for _, count := range counts {
		accounts := generateRandomAccounts(count)

		var wg sync.WaitGroup
		sem := make(chan struct{}, concurrency)

		results := make([]time.Duration, count)
		var mu sync.Mutex

		start := time.Now()

		for i, acc := range accounts {
			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore

			go func(idx int, acct string) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				req := &proofpb.GetProofRequest{
					Account:    acct,
					StartBlock: StartBlock,
					EndBlock:   StartBlock + rangeSize,
				}

				reqStart := time.Now()
				_, err := client.GetProof(context.Background(), req)
				latency := time.Since(reqStart)

				if err != nil {
					// Log error but continue
					// fmt.Printf("Error req %d: %v\n", idx, err)
				}

				mu.Lock()
				results[idx] = latency
				mu.Unlock()
			}(i, acc)
		}

		wg.Wait()
		totalTime := time.Since(start)

		var totalLatency time.Duration
		for _, d := range results {
			totalLatency += d
		}
		avgLatency := totalLatency / time.Duration(count)
		throughput := float64(count) / totalTime.Seconds()

		fmt.Printf("%-10d %-15v %-15v %-15.2f\n", count, totalTime, avgLatency, throughput)
	}
}

func generateRandomAccounts(count int) []string {
	accounts := make([]string, count)
	for i := 0; i < count; i++ {
		bytes := make([]byte, 20)
		rand.Read(bytes)
		accounts[i] = "0x" + hex.EncodeToString(bytes)
	}
	// Make sure at least one valid account is usually included for sanity if needed,
	// but for benchmarking random is fine (server handles non-existent accounts by returning empty proof usually,
	// or we might want to use the valid one repeated if we want to hit data)
	// For stress testing the proof generation of "existing" data, we might need real accounts.
	// But the user asked for "1, 5, 10... accounts", possibly implying different accounts.
	// If the account doesn't exist, the proof is likely minimal (non-inclusion).
	// To stress test "proof generation", we might want valid accounts.
	// However, without a list of valid accounts, random is the best valid approach for "stress testing" the system's handling of keys.
	// But for "proof generation" of actual history, maybe we should reuse the known good account?
	// The prompt says "query for multiple accounts...".
	// I'll stick to random accounts as "multiple accounts" usually implies distinct ones.
	return accounts
}
