// Package main provides a CLI client for the baseline-verkle gRPC proof service.
// Supports single range-proof queries and benchmark modes (range, concurrency, stress).
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	proofpb "github.com/nepal80m/samurai/api/proto/verkle/v1"
	"github.com/nepal80m/samurai/internal/verkle/keys"
	verkle "github.com/ethereum/go-verkle"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const StartBlock = uint64(18908895)

// Block range definitions for stress test
var defaultRanges = []struct {
	Label  string
	Blocks uint64
}{
	{"50k_blocks", 50000},
	{"100k_blocks", 100000},
	{"200k_blocks", 200000},
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "proofc",
		Short: "Verkle proof gRPC client and benchmark tool",
	}

	rootCmd.AddCommand(queryCmd())
	rootCmd.AddCommand(benchCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// =============================================================================
// Query command (single range-proof query)
// =============================================================================

func queryCmd() *cobra.Command {
	var (
		serverAddr string
		account    string
		startBlock uint64
		endBlock   uint64
		verify     bool
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query account proofs for a block range",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := grpc.NewClient(serverAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer conn.Close()

			client := proofpb.NewVerkleProofServiceClient(conn)
			runSingleQuery(client, account, startBlock, endBlock, verify)
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "localhost:50051", "gRPC server address")
	cmd.Flags().StringVar(&account, "account", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account address")
	cmd.Flags().Uint64Var(&startBlock, "start-block", StartBlock, "Start block number")
	cmd.Flags().Uint64Var(&endBlock, "end-block", StartBlock+10, "End block number")
	cmd.Flags().BoolVar(&verify, "verify", false, "Verify proofs locally")

	return cmd
}

// =============================================================================
// Benchmark command
// =============================================================================

func benchCmd() *cobra.Command {
	var (
		serverAddr string
		account    string
		startBlock uint64
		mode       string
		rangeSize  uint64
		verify     bool
		levels     string
		stressDur  time.Duration
		stressN    int
		acctFile   string
		outputDir  string
	)

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Run proof benchmarks (range, concurrency, stress)",
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := grpc.NewClient(serverAddr,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer conn.Close()

			client := proofpb.NewVerkleProofServiceClient(conn)

			fmt.Printf("=== Verkle Proof Benchmark ===\n")
			fmt.Printf("Server: %s\n", serverAddr)
			fmt.Printf("Mode: %s\n", mode)

			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}

			switch mode {
			case "range":
				runRangeBenchmark(client, RangeOpts{
					Account:    account,
					StartBlock: startBlock,
					RangeSize:  rangeSize,
					Verify:     verify,
					OutputDir:  outputDir,
				})
			case "concurrency":
				accounts := mustLoadAccounts(acctFile, account)
				runConcurrencyBenchmark(client, ConcurrencyOpts{
					Levels:     parseLevels(levels),
					StartBlock: startBlock,
					RangeSize:  rangeSize,
					Accounts:   accounts,
					OutputDir:  outputDir,
				})
			case "stress":
				accounts, cumWeights := mustLoadWeightedAccounts(acctFile, account)
				runStressBenchmark(client, StressOpts{
					Duration:   stressDur,
					NumClients: stressN,
					StartBlock: startBlock,
					Accounts:   accounts,
					CumWeights: cumWeights,
					OutputDir:  outputDir,
				})
			default:
				return fmt.Errorf("unknown mode: %s (use range, concurrency, or stress)", mode)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&serverAddr, "server", "localhost:50051", "gRPC server address")
	cmd.Flags().StringVar(&account, "account", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Default account")
	cmd.Flags().Uint64Var(&startBlock, "start-block", StartBlock, "Start block number")
	cmd.Flags().StringVar(&mode, "mode", "range", "Benchmark mode: range, concurrency, or stress")
	cmd.Flags().Uint64Var(&rangeSize, "range-size", 50000, "Block range size")
	cmd.Flags().BoolVar(&verify, "verify", false, "Verify proofs (range mode)")
	cmd.Flags().StringVar(&levels, "levels", "1,5,10,20,50,100", "Concurrency levels")
	cmd.Flags().DurationVar(&stressDur, "stress-duration", 5*time.Minute, "Stress test duration")
	cmd.Flags().IntVar(&stressN, "stress-clients", 10, "Concurrent clients for stress")
	cmd.Flags().StringVar(&acctFile, "accounts-file", "", "Path to accounts CSV (address[,weight])")
	cmd.Flags().StringVar(&outputDir, "output-dir", "./benchmark_output", "Output directory for CSV results")

	return cmd
}

// =============================================================================
// Options structs
// =============================================================================

type RangeOpts struct {
	Account    string
	StartBlock uint64
	RangeSize  uint64
	Verify     bool
	OutputDir  string
}

type ConcurrencyOpts struct {
	Levels     []int
	StartBlock uint64
	RangeSize  uint64
	Accounts   []string
	OutputDir  string
}

type StressOpts struct {
	Duration   time.Duration
	NumClients int
	StartBlock uint64
	Accounts   []string
	CumWeights []int
	OutputDir  string
}

type StressResult struct {
	Timestamp     int64
	ClientID      int
	LatencyMs     int64
	Success       bool
	IsClientError bool
	Account       string
	RangeLabel    string
}

// =============================================================================
// Stream helper
// =============================================================================

func recvAllProofs(stream proofpb.VerkleProofService_GetRangeProofClient) ([]*proofpb.BlockProof, error) {
	var proofs []*proofpb.BlockProof
	for {
		bp, err := stream.Recv()
		if err == io.EOF {
			return proofs, nil
		}
		if err != nil {
			return proofs, err
		}
		proofs = append(proofs, bp)
	}
}

type streamResult struct {
	Proofs    []*proofpb.BlockProof
	Latency   time.Duration
	GenTimeMs int64
}

func callRangeProof(client proofpb.VerkleProofServiceClient, req *proofpb.GetRangeProofRequest) (streamResult, error) {
	start := time.Now()
	stream, err := client.GetRangeProof(context.Background(), req)
	if err != nil {
		return streamResult{Latency: time.Since(start)}, err
	}
	proofs, err := recvAllProofs(stream)
	latency := time.Since(start)

	var genTimeMs int64
	if trailer := stream.Trailer(); trailer != nil {
		if vals := trailer.Get("generation_time_ms"); len(vals) > 0 {
			genTimeMs, _ = strconv.ParseInt(vals[0], 10, 64)
		}
	}

	return streamResult{
		Proofs:    proofs,
		Latency:   latency,
		GenTimeMs: genTimeMs,
	}, err
}

// =============================================================================
// Single Query
// =============================================================================

func runSingleQuery(client proofpb.VerkleProofServiceClient, account string, startBlock, endBlock uint64, doVerify bool) {
	fmt.Printf("Querying account %s, blocks %d - %d\n", account, startBlock, endBlock)

	req := &proofpb.GetRangeProofRequest{
		Account:    account,
		StartBlock: startBlock,
		EndBlock:   endBlock,
	}

	sr, err := callRangeProof(client, req)
	if err != nil {
		log.Fatalf("GetRangeProof failed: %v", err)
	}

	proofs := sr.Proofs
	fmt.Printf("\n=== Range Proof Response ===\n")
	fmt.Printf("Round-trip time:   %v\n", sr.Latency)
	fmt.Printf("Server gen time:   %d ms\n", sr.GenTimeMs)
	fmt.Printf("Network overhead:  %d ms\n", sr.Latency.Milliseconds()-sr.GenTimeMs)
	fmt.Printf("Block proofs:      %d\n", len(proofs))

	var totalProofBytes int
	verified := 0
	for _, bp := range proofs {
		proofBytes := len(bp.VerkleProof) + len(bp.StateDiff)
		totalProofBytes += proofBytes
		balance := new(big.Int).SetBytes(bp.Balance)
		existsStr := "exists"
		if !bp.Exists {
			existsStr = "absent"
		}

		fmt.Printf("  block=%d  balance=%-20s  proof_bytes=%d  %s",
			bp.BlockNumber, balance.String(), proofBytes, existsStr)

		if doVerify {
			ok, verErr := verifyBlockProof(bp, account)
			if verErr != nil {
				fmt.Printf("  VERIFY_FAILED: %v", verErr)
			} else if ok {
				fmt.Printf("  VERIFIED")
				verified++
			}
		}
		fmt.Println()
	}

	fmt.Printf("\nTotal proof bytes: %d\n", totalProofBytes)
	if doVerify {
		fmt.Printf("Verified: %d/%d\n", verified, len(proofs))
	}
}

// =============================================================================
// Range Benchmark
// =============================================================================

func runRangeBenchmark(client proofpb.VerkleProofServiceClient, opts RangeOpts) {
	endBlock := opts.StartBlock + opts.RangeSize - 1
	fmt.Printf("\n=== Range Benchmark ===\n")
	fmt.Printf("Account: %s\n", opts.Account)
	fmt.Printf("Blocks: %d - %d (%d blocks)\n", opts.StartBlock, endBlock, opts.RangeSize)
	fmt.Printf("Verify: %v\n", opts.Verify)

	timestamp := time.Now().Format("20060102_150405")
	csvPath := filepath.Join(opts.OutputDir, fmt.Sprintf("range_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("create CSV: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	header := []string{
		"timestamp_ns", "blocks", "latency_ms", "generation_time_ms",
		"network_overhead_ms", "proof_bytes", "block_proofs_count", "verification_time_ms",
	}
	writer.Write(header)

	req := &proofpb.GetRangeProofRequest{
		Account:    opts.Account,
		StartBlock: opts.StartBlock,
		EndBlock:   endBlock,
	}

	timestampNs := time.Now().UnixNano()
	sr, err := callRangeProof(client, req)
	if err != nil {
		log.Fatalf("GetRangeProof failed: %v", err)
	}
	proofs := sr.Proofs
	latency := sr.Latency
	genTime := sr.GenTimeMs
	networkOverhead := latency.Milliseconds() - genTime

	proofBytes := 0
	for _, bp := range proofs {
		proofBytes += len(bp.VerkleProof) + len(bp.StateDiff)
	}

	verifyTimeMs := int64(0)
	if opts.Verify && len(proofs) > 0 {
		vStart := time.Now()
		for _, bp := range proofs {
			verifyBlockProof(bp, opts.Account)
		}
		verifyTimeMs = time.Since(vStart).Milliseconds()
	}

	fmt.Printf("\n%-10s %-10s %-12s %-12s %-12s %-12s %-10s\n",
		"Blocks", "Proofs", "Latency", "GenTime", "Network", "ProofBytes", "Verify")
	fmt.Printf("%-10d %-10d %-12dms %-12dms %-12dms %-12s %-10dms\n",
		opts.RangeSize, len(proofs), latency.Milliseconds(),
		genTime, networkOverhead, formatBytes(proofBytes), verifyTimeMs)

	row := []string{
		strconv.FormatInt(timestampNs, 10),
		strconv.FormatUint(opts.RangeSize, 10),
		strconv.FormatInt(latency.Milliseconds(), 10),
		strconv.FormatInt(genTime, 10),
		strconv.FormatInt(networkOverhead, 10),
		strconv.Itoa(proofBytes),
		strconv.Itoa(len(proofs)),
		strconv.FormatInt(verifyTimeMs, 10),
	}
	writer.Write(row)

	fmt.Printf("\nResults written to: %s\n", csvPath)
}

// =============================================================================
// Concurrency Benchmark
// =============================================================================

func runConcurrencyBenchmark(client proofpb.VerkleProofServiceClient, opts ConcurrencyOpts) {
	fmt.Printf("\n=== Concurrency Benchmark ===\n")
	fmt.Printf("Levels: %v\n", opts.Levels)
	fmt.Printf("Range size: %d blocks\n", opts.RangeSize)
	fmt.Printf("Accounts: %d loaded\n", len(opts.Accounts))

	timestamp := time.Now().Format("20060102_150405")
	csvPath := filepath.Join(opts.OutputDir, fmt.Sprintf("concurrency_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("create CSV: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	header := []string{"timestamp_ns", "concurrency_level", "request_id", "latency_ms", "success", "is_client_error"}
	writer.Write(header)

	fmt.Printf("\n%-12s %-15s %-15s %-15s %-15s %-15s %-15s\n",
		"Concurrency", "Total Time", "Avg Latency", "Min Latency", "Max Latency", "Server Errors", "Client Errors")

	for _, level := range opts.Levels {
		var wg sync.WaitGroup
		type result struct {
			timestamp     int64
			latency       time.Duration
			success       bool
			isClientError bool
		}
		results := make([]result, level)

		timestampNs := time.Now().UnixNano()
		start := time.Now()

		for i := 0; i < level; i++ {
			wg.Add(1)
			go func(reqID int) {
				defer wg.Done()
				acct := opts.Accounts[reqID%len(opts.Accounts)]
				req := &proofpb.GetRangeProofRequest{
					Account:    acct,
					StartBlock: opts.StartBlock,
					EndBlock:   opts.StartBlock + opts.RangeSize - 1,
				}

				sr, err := callRangeProof(client, req)
				isClErr := isClientError(err)
				if err != nil && !isClErr {
					fmt.Printf("Server Error [Req %d]: %v\n", reqID, err)
				}

				results[reqID] = result{timestampNs, sr.Latency, err == nil, isClErr}
			}(i)
		}

		wg.Wait()
		totalTime := time.Since(start)

		var totalLatency, minLatency, maxLatency time.Duration
		successCount, clientErrorCount, serverErrorCount := 0, 0, 0

		for i, r := range results {
			writer.Write([]string{
				strconv.FormatInt(r.timestamp, 10),
				strconv.Itoa(level),
				strconv.Itoa(i),
				strconv.FormatInt(r.latency.Milliseconds(), 10),
				strconv.FormatBool(r.success),
				strconv.FormatBool(r.isClientError),
			})
			if r.success {
				totalLatency += r.latency
				if minLatency == 0 || r.latency < minLatency {
					minLatency = r.latency
				}
				if r.latency > maxLatency {
					maxLatency = r.latency
				}
				successCount++
			} else if r.isClientError {
				clientErrorCount++
			} else {
				serverErrorCount++
			}
		}

		avgLatency := time.Duration(0)
		if successCount > 0 {
			avgLatency = totalLatency / time.Duration(successCount)
		}

		fmt.Printf("%-12d %-15v %-15v %-15v %-15v %-15d %-15d\n",
			level, totalTime.Round(time.Millisecond), avgLatency.Round(time.Millisecond),
			minLatency.Round(time.Millisecond), maxLatency.Round(time.Millisecond), serverErrorCount, clientErrorCount)
	}

	fmt.Printf("\nResults written to: %s\n", csvPath)
}

// =============================================================================
// Stress Benchmark
// =============================================================================

func runStressBenchmark(client proofpb.VerkleProofServiceClient, opts StressOpts) {
	fmt.Printf("\n=== Stress Benchmark ===\n")
	fmt.Printf("Duration: %v\n", opts.Duration)
	fmt.Printf("Clients: %d\n", opts.NumClients)
	rangeLabels := make([]string, len(defaultRanges))
	for i, r := range defaultRanges {
		rangeLabels[i] = fmt.Sprintf("%s(%d)", r.Label, r.Blocks)
	}
	fmt.Printf("Range sizes: %s (random)\n", strings.Join(rangeLabels, ", "))
	fmt.Printf("Accounts: %d loaded (weighted)\n", len(opts.Accounts))

	timestamp := time.Now().Format("20060102_150405")
	csvPath := filepath.Join(opts.OutputDir, fmt.Sprintf("stress_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("create CSV: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	header := []string{"timestamp_ns", "client_id", "latency_ms", "success", "is_client_error", "account", "range_label"}
	writer.Write(header)

	resultsCh := make(chan StressResult, 100000)
	var wg sync.WaitGroup
	deadline := time.Now().Add(opts.Duration)

	var totalRequests, successCount, serverErrorCount, clientErrorCount int64
	var totalLatency int64
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		for r := range resultsCh {
			writer.Write([]string{
				strconv.FormatInt(r.Timestamp, 10),
				strconv.Itoa(r.ClientID),
				strconv.FormatInt(r.LatencyMs, 10),
				strconv.FormatBool(r.Success),
				strconv.FormatBool(r.IsClientError),
				r.Account,
				r.RangeLabel,
			})
			mu.Lock()
			totalRequests++
			if r.Success {
				successCount++
				totalLatency += r.LatencyMs
			} else if r.IsClientError {
				clientErrorCount++
			} else {
				serverErrorCount++
			}
			mu.Unlock()
		}
		close(done)
	}()

	// Progress reporting
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		startTime := time.Now()
		for {
			select {
			case <-ticker.C:
				mu.Lock()
				elapsed := time.Since(startTime)
				rps := float64(totalRequests) / elapsed.Seconds()
				avgLat := int64(0)
				if successCount > 0 {
					avgLat = totalLatency / successCount
				}
				fmt.Printf("[%v] Requests: %d, RPS: %.2f, Avg Latency: %dms, Server Errors: %d, Client Errors: %d\n",
					elapsed.Round(time.Second), totalRequests, rps, avgLat, serverErrorCount, clientErrorCount)
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()

	fmt.Printf("\nStarting stress test...\n")
	for i := 0; i < opts.NumClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			totalWeight := opts.CumWeights[len(opts.CumWeights)-1]
			for time.Now().Before(deadline) {
				account := opts.Accounts[sort.SearchInts(opts.CumWeights, rand.Intn(totalWeight)+1)]
				r := defaultRanges[rand.Intn(len(defaultRanges))]
				timestampNs := time.Now().UnixNano()

				req := &proofpb.GetRangeProofRequest{
					Account:    account,
					StartBlock: opts.StartBlock,
					EndBlock:   opts.StartBlock + r.Blocks - 1,
				}

				sr, err := callRangeProof(client, req)
				isClErr := isClientError(err)

				resultsCh <- StressResult{
					Timestamp:     timestampNs,
					ClientID:      clientID,
					LatencyMs:     sr.Latency.Milliseconds(),
					Success:       err == nil,
					IsClientError: isClErr,
					Account:       account,
					RangeLabel:    r.Label,
				}
			}
		}(i)
	}

	wg.Wait()
	close(resultsCh)
	<-done
	writer.Flush()

	mu.Lock()
	elapsed := opts.Duration
	rps := float64(totalRequests) / elapsed.Seconds()
	avgLat := int64(0)
	if successCount > 0 {
		avgLat = totalLatency / successCount
	}
	mu.Unlock()

	fmt.Printf("\n=== Stress Test Complete ===\n")
	fmt.Printf("Duration: %v\n", elapsed)
	fmt.Printf("Total Requests: %d\n", totalRequests)
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Server Errors: %d\n", serverErrorCount)
	fmt.Printf("Client Errors: %d\n", clientErrorCount)
	fmt.Printf("Throughput: %.2f req/s\n", rps)
	fmt.Printf("Avg Latency: %dms\n", avgLat)
	fmt.Printf("\nResults written to: %s\n", csvPath)
}

// =============================================================================
// Verify helper
// =============================================================================

func verifyBlockProof(bp *proofpb.BlockProof, account string) (bool, error) {
	var vp verkle.VerkleProof
	if err := json.Unmarshal(bp.VerkleProof, &vp); err != nil {
		return false, fmt.Errorf("unmarshal VerkleProof: %w", err)
	}
	var sd verkle.StateDiff
	if err := json.Unmarshal(bp.StateDiff, &sd); err != nil {
		return false, fmt.Errorf("unmarshal StateDiff: %w", err)
	}

	if err := verkle.Verify(&vp, bp.RootCommitment, bp.RootCommitment, sd); err != nil {
		return false, err
	}

	// Additionally extract balance from state diff for the given address
	addrStr := strings.TrimPrefix(account, "0x")
	addrBytes, _ := parseHexAddr(addrStr)
	treeKey := keys.GetTreeKeyForBasicData(addrBytes)
	stem := treeKey[:31]
	suffix := treeKey[31]

	for _, ssd := range sd {
		if bytes.Equal(ssd.Stem[:], stem) {
			for _, suffDiff := range ssd.SuffixDiffs {
				if suffDiff.Suffix == suffix {
					return true, nil
				}
			}
		}
	}

	return true, nil
}

func parseHexAddr(hexStr string) ([20]byte, error) {
	var addr [20]byte
	b, err := parseHex(hexStr)
	if err != nil || len(b) != 20 {
		return addr, fmt.Errorf("invalid address: %s", hexStr)
	}
	copy(addr[:], b)
	return addr, nil
}

func parseHex(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b := make([]byte, len(s)/2)
	for i := 0; i < len(b); i++ {
		hi := unhex(s[2*i])
		lo := unhex(s[2*i+1])
		if hi == 0xFF || lo == 0xFF {
			return nil, fmt.Errorf("invalid hex character")
		}
		b[i] = hi<<4 | lo
	}
	return b, nil
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	default:
		return 0xFF
	}
}



// =============================================================================
// Utility Functions
// =============================================================================

func isClientError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.OutOfRange, codes.NotFound, codes.InvalidArgument:
		return true
	default:
		return false
	}
}

func mustLoadAccounts(filename string, defaultAccount string) []string {
	if filename == "" {
		fmt.Printf("No accounts file provided, using default + random accounts\n")
		accounts := []string{defaultAccount}
		for i := 0; i < 99; i++ {
			b := make([]byte, 20)
			rand.Read(b)
			accounts = append(accounts, fmt.Sprintf("0x%040x", b))
		}
		return accounts
	}
	accounts, err := loadAccounts(filename)
	if err != nil {
		log.Fatalf("load accounts: %v", err)
	}
	return accounts
}

func mustLoadWeightedAccounts(filename string, defaultAccount string) ([]string, []int) {
	if filename == "" {
		accounts := mustLoadAccounts("", defaultAccount)
		cumWeights := make([]int, len(accounts))
		for i := range accounts {
			cumWeights[i] = i + 1
		}
		return accounts, cumWeights
	}
	accounts, cumWeights, err := loadWeightedAccounts(filename)
	if err != nil {
		log.Fatalf("load weighted accounts: %v", err)
	}
	return accounts, cumWeights
}

func loadAccounts(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var accounts []string
	scanner := bufio.NewScanner(f)
	if scanner.Scan() { /* skip header */ }
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ",")
		if len(parts) >= 1 {
			accounts = append(accounts, strings.TrimSpace(parts[0]))
		}
	}
	return accounts, scanner.Err()
}

func loadWeightedAccounts(filename string) ([]string, []int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	var accounts []string
	var cumWeights []int
	cumSum := 0
	scanner := bufio.NewScanner(f)
	if scanner.Scan() { /* skip header */ }
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ",")
		if len(parts) >= 2 {
			accounts = append(accounts, strings.TrimSpace(parts[0]))
			weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				weight = 1
			}
			cumSum += weight
			cumWeights = append(cumWeights, cumSum)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if len(accounts) == 0 {
		return nil, nil, fmt.Errorf("no accounts loaded from %s", filename)
	}
	return accounts, cumWeights, nil
}

func parseLevels(s string) []int {
	parts := strings.Split(s, ",")
	levels := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if n, err := strconv.Atoi(p); err == nil {
			levels = append(levels, n)
		}
	}
	return levels
}

func formatBytes(bytes int) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
	if bytes >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%dB", bytes)
}
