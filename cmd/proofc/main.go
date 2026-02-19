// Package main provides a CLI client for the Samurai gRPC proof service.
// Supports single proof verification and benchmark modes (range, concurrency, stress).
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	proofpb "github.com/nepal80m/samurai/api/proto/v1"
	"github.com/nepal80m/samurai/internal/config"
	"github.com/nepal80m/samurai/internal/crypto/kzg"
	"github.com/nepal80m/samurai/internal/proof"
	"github.com/nepal80m/samurai/internal/server"
	"github.com/nepal80m/samurai/internal/tree"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const StartBlock = uint64(18908895)

// Block range definitions (~12s/block)
var defaultRanges = []struct {
	Label  string
	Blocks uint64
}{
	{"1_week", 50000},
	// {"1_month", 200000},
	// {"3_months", 600000},
	// {"6_months", 1200000},
	// {"1_year", 2600000},
}

func main() {
	// Common flags
	serverAddr := flag.String("server", "10.10.1.1:50051", "gRPC server address")
	account := flag.String("account", "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2", "Account address to query")
	startBlock := flag.Uint64("start-block", 20, "Starting block number (relative to data start)")
	endBlock := flag.Uint64("end-block", 119, "Ending block number (relative to data start)")
	paramsDir := flag.String("params-dir", "./data/params", "Path to crypto params")

	// Benchmark mode flags
	benchmark := flag.Bool("benchmark", false, "Enable benchmark mode")
	mode := flag.String("mode", "range", "Benchmark mode: 'range', 'concurrency', or 'stress'")

	// Range mode flags
	verify := flag.Bool("verify", false, "Include verification time (for range mode)")

	// Concurrency mode flags
	levels := flag.String("levels", "1,5,10,20,50,100", "Comma-separated concurrency levels")
	rangeSize := flag.Uint64("range-size", 50000, "Block range size")

	// Stress mode flags
	stressDuration := flag.Duration("stress-duration", 5*time.Minute, "Stress test duration")
	stressClients := flag.Int("stress-clients", 10, "Concurrent clients for stress test")

	// Shared flags
	accountsFile := flag.String("accounts-file", "cmd/proofc/top_1k_accounts_all_blocks.csv", "Path to accounts CSV file")
	outputDir := flag.String("output-dir", "./benchmark_output", "Output directory for benchmark results")

	// Legacy dump flags
	dumpJson := flag.String("dump-json", "", "Path to dump response as JSON (optional)")
	dumpBin := flag.String("dump-bin", "", "Path to dump response as Binary Protobuf (optional)")

	flag.Parse()

	// Setup gRPC connection
	maxMsgSize := 100 * 1024 * 1024 // 100MB for large payloads
	conn, err := grpc.NewClient(*serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize)),
	)
	if err != nil {
		log.Fatalf("failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := proofpb.NewProofServiceClient(conn)

	if *benchmark {
		fmt.Printf("=== Proof Benchmark ===\n")
		fmt.Printf("Server: %s\n", *serverAddr)
		fmt.Printf("Mode: %s\n", *mode)

		// Ensure output directory exists
		if err := os.MkdirAll(*outputDir, 0755); err != nil {
			log.Fatalf("failed to create output directory: %v", err)
		}

		switch *mode {
		case "range":
			opts := RangeOpts{
				Account:    *account,
				StartBlock: *startBlock + StartBlock,
				RangeSize:  *rangeSize,
				Verify:     *verify,
				ParamsDir:  *paramsDir,
				OutputDir:  *outputDir,
				DumpJson:   *dumpJson,
				DumpBin:    *dumpBin,
			}
			runRangeBenchmark(client, opts)

		case "concurrency":
			accounts, err := loadAccounts(*accountsFile)
			if err != nil {
				log.Fatalf("failed to load accounts: %v", err)
			}
			opts := ConcurrencyOpts{
				Levels:     parseLevels(*levels),
				StartBlock: *startBlock + StartBlock,
				RangeSize:  *rangeSize,
				Accounts:   accounts,
				OutputDir:  *outputDir,
			}
			runConcurrencyBenchmark(client, opts)

		case "stress":
			accounts, cumWeights, err := loadWeightedAccounts(*accountsFile)
			if err != nil {
				log.Fatalf("failed to load accounts: %v", err)
			}
			opts := StressOpts{
				Duration:   *stressDuration,
				NumClients: *stressClients,
				StartBlock: *startBlock + StartBlock,
				Accounts:   accounts,
				CumWeights: cumWeights,
				OutputDir:  *outputDir,
			}
			runStressBenchmark(client, opts)

		default:
			log.Fatalf("Unknown benchmark mode: %s", *mode)
		}
	} else {
		// Normal mode (single proof with verification)
		runSingleProof(client, *account, *startBlock, *endBlock, *paramsDir)
	}
}

// =============================================================================
// Options structs
// =============================================================================

type RangeOpts struct {
	Account    string
	StartBlock uint64
	RangeSize  uint64
	Verify     bool
	ParamsDir  string
	OutputDir  string
	DumpJson   string
	DumpBin    string
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
// Range Benchmark (All-in-one: latency, size, network, verification)
// =============================================================================

func runRangeBenchmark(client proofpb.ProofServiceClient, opts RangeOpts) {
	endBlock := opts.StartBlock + opts.RangeSize
	fmt.Printf("\n=== Range Benchmark ===\n")
	fmt.Printf("Account: %s\n", opts.Account)
	fmt.Printf("Blocks: %d - %d (%d blocks)\n", opts.StartBlock, endBlock, opts.RangeSize)
	fmt.Printf("Verify: %v\n", opts.Verify)

	// Setup verification if requested
	var precomputed *config.PrecomputedData
	if opts.Verify {
		var err error
		precomputed, err = SetupPrecomputedData(opts.ParamsDir)
		if err != nil {
			log.Fatalf("failed to setup precomputed data: %v", err)
		}
	}

	// Create CSV file
	timestamp := time.Now().Format("20060102_150405")
	csvPath := filepath.Join(opts.OutputDir, fmt.Sprintf("range_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	// Write header
	header := []string{
		"timestamp_ns", "blocks", "latency_ms", "generation_time_ms",
		"network_overhead_ms", "payload_bytes", "range_proofs_bytes", "balance_infos_bytes",
		"range_proofs_count", "balance_infos_count", "verification_time_ms",
	}
	writer.Write(header)

	// Make request
	req := &proofpb.GetProofRequest{
		Account:    opts.Account,
		StartBlock: opts.StartBlock,
		EndBlock:   endBlock,
	}

	timestampNs := time.Now().UnixNano()
	start := time.Now()
	resp, err := client.GetProof(context.Background(), req)
	latency := time.Since(start)

	if err != nil {
		log.Fatalf("GetProof failed: %v", err)
	}

	// Calculate metrics
	genTime := resp.GenerationTimeMs
	networkOverhead := latency.Milliseconds() - genTime

	payloadBytes := proto.Size(resp)
	rpBytes := 0
	for _, rp := range resp.RangeProofs {
		rpBytes += proto.Size(rp)
	}
	biBytes := 0
	for _, bi := range resp.BalanceInfos {
		biBytes += proto.Size(bi)
	}

	// Verification (optional)
	verifyTimeMs := int64(0)
	if opts.Verify && precomputed != nil {
		verifyTimeMs = runVerification(resp, opts.Account, precomputed)
	}

	// Print results
	fmt.Printf("\n%-10s %-10s %-12s %-12s %-12s %-12s %-10s\n",
		"Blocks", "Versions", "Latency", "GenTime", "Network", "Payload", "Verify")
	fmt.Printf("%-10d %-10d %-12dms %-12dms %-12dms %-12s %-10dms\n",
		opts.RangeSize, len(resp.BalanceInfos), latency.Milliseconds(),
		genTime, networkOverhead, formatBytes(payloadBytes), verifyTimeMs)

	// Write CSV row
	row := []string{
		strconv.FormatInt(timestampNs, 10),
		strconv.FormatUint(opts.RangeSize, 10),
		strconv.FormatInt(latency.Milliseconds(), 10),
		strconv.FormatInt(genTime, 10),
		strconv.FormatInt(networkOverhead, 10),
		strconv.Itoa(payloadBytes),
		strconv.Itoa(rpBytes),
		strconv.Itoa(biBytes),
		strconv.Itoa(len(resp.RangeProofs)),
		strconv.Itoa(len(resp.BalanceInfos)),
		strconv.FormatInt(verifyTimeMs, 10),
	}
	writer.Write(row)

	fmt.Printf("\nResults written to: %s\n", csvPath)

	// Handle dump flags (legacy)
	if opts.DumpJson != "" || opts.DumpBin != "" {
		dumpPayload(client, opts)
	}
}

func runVerification(resp *proofpb.GetProofResponse, account string, precomputed *config.PrecomputedData) int64 {
	addr := common.HexToAddress(account)

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

	start := time.Now()
	proof.VerifyNewRangeProofs(addr, startingVersion, endingVersion, rangeProofs, balanceInfos, precomputed)
	return time.Since(start).Milliseconds()
}

func dumpPayload(client proofpb.ProofServiceClient, opts RangeOpts) {
	fmt.Printf("\nDumping payload for %d blocks...\n", opts.RangeSize)

	req := &proofpb.GetProofRequest{
		Account:    opts.Account,
		StartBlock: opts.StartBlock,
		EndBlock:   opts.StartBlock + opts.RangeSize,
	}

	resp, err := client.GetProof(context.Background(), req)
	if err != nil {
		log.Fatalf("GetProof failed: %v", err)
	}

	if opts.DumpJson != "" {
		file, err := os.Create(opts.DumpJson)
		if err != nil {
			log.Fatalf("Failed to create file: %v", err)
		}
		defer file.Close()

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(resp); err != nil {
			log.Fatalf("Failed to encode response: %v", err)
		}
		fmt.Printf("JSON dumped to %s\n", opts.DumpJson)
	}

	if opts.DumpBin != "" {
		data, err := proto.Marshal(resp)
		if err != nil {
			log.Fatalf("Failed to marshal response: %v", err)
		}
		if err := os.WriteFile(opts.DumpBin, data, 0644); err != nil {
			log.Fatalf("Failed to write file: %v", err)
		}
		fmt.Printf("Binary dumped to %s\n", opts.DumpBin)
	}
}

// =============================================================================
// Concurrency Benchmark
// =============================================================================

func isClientError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		// Not a gRPC status error, treat as server error (legacy check) or client error depending on context?
		// Let's assume network/other errors are server errors for now unless we know otherwise.
		return false
	}
	switch st.Code() {
	case codes.OutOfRange, codes.NotFound, codes.InvalidArgument:
		return true
	default:
		return false
	}
}

func runConcurrencyBenchmark(client proofpb.ProofServiceClient, opts ConcurrencyOpts) {
	fmt.Printf("\n=== Concurrency Benchmark ===\n")
	fmt.Printf("Levels: %v\n", opts.Levels)
	fmt.Printf("Range size: %d blocks\n", opts.RangeSize)
	fmt.Printf("Accounts: %d loaded\n", len(opts.Accounts))
	fmt.Printf("Mode: Fire N requests simultaneously (N = concurrency level)\n")

	// Create CSV file
	timestamp := time.Now().Format("20060102_150405")
	csvPath := filepath.Join(opts.OutputDir, fmt.Sprintf("concurrency_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	// Write header
	header := []string{"timestamp_ns", "concurrency_level", "request_id", "latency_ms", "success", "is_client_error"}
	writer.Write(header)

	// Print table header
	fmt.Printf("\n%-12s %-15s %-15s %-15s %-15s %-15s %-15s\n",
		"Concurrency", "Total Time", "Avg Latency", "Min Latency", "Max Latency", "Server Errors", "Client Errors")

	for _, level := range opts.Levels {
		// Fire exactly 'level' requests simultaneously
		var wg sync.WaitGroup
		results := make([]struct {
			timestamp     int64
			latency       time.Duration
			success       bool
			isClientError bool
		}, level)

		timestampNs := time.Now().UnixNano()
		start := time.Now()

		for i := 0; i < level; i++ {
			wg.Add(1)
			go func(reqID int) {
				defer wg.Done()

				account := opts.Accounts[reqID%len(opts.Accounts)]
				req := &proofpb.GetProofRequest{
					Account:    account,
					StartBlock: opts.StartBlock,
					EndBlock:   opts.StartBlock + opts.RangeSize,
				}

				reqStart := time.Now()
				_, err := client.GetProof(context.Background(), req)
				latency := time.Since(reqStart)

				isClErr := isClientError(err)

				if err != nil && !isClErr {
					fmt.Printf("Server Error [Req %d]: %v\n", reqID, err)
				}

				results[reqID] = struct {
					timestamp     int64
					latency       time.Duration
					success       bool
					isClientError bool
				}{timestampNs, latency, err == nil, isClErr}
			}(i)
		}

		wg.Wait()
		totalTime := time.Since(start)

		// Calculate stats
		var totalLatency time.Duration
		var minLatency, maxLatency time.Duration
		successCount := 0
		clientErrorCount := 0
		serverErrorCount := 0

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

func runStressBenchmark(client proofpb.ProofServiceClient, opts StressOpts) {
	fmt.Printf("\n=== Stress Benchmark ===\n")
	fmt.Printf("Duration: %v\n", opts.Duration)
	fmt.Printf("Clients: %d\n", opts.NumClients)
	rangeLabels := make([]string, len(defaultRanges))
	for i, r := range defaultRanges {
		rangeLabels[i] = fmt.Sprintf("%s(%d)", r.Label, r.Blocks)
	}
	fmt.Printf("Range sizes: %s (random)\n", strings.Join(rangeLabels, ", "))
	fmt.Printf("Accounts: %d loaded (weighted)\n", len(opts.Accounts))

	// Create CSV file
	timestamp := time.Now().Format("20060102_150405")
	csvPath := filepath.Join(opts.OutputDir, fmt.Sprintf("stress_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		log.Fatalf("failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)

	// Write header
	header := []string{"timestamp_ns", "client_id", "latency_ms", "success", "is_client_error", "account", "range_label"}
	writer.Write(header)

	resultsCh := make(chan StressResult, 100000)
	var wg sync.WaitGroup
	deadline := time.Now().Add(opts.Duration)

	// Stats tracking
	var totalRequests, successCount, serverErrorCount, clientErrorCount int64
	var totalLatency int64
	var mu sync.Mutex

	// Writer goroutine
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

	// Launch client goroutines
	fmt.Printf("\nStarting stress test...\n")
	for i := 0; i < opts.NumClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			totalWeight := opts.CumWeights[len(opts.CumWeights)-1]
			for time.Now().Before(deadline) {
				// Weighted account selection
				account := opts.Accounts[sort.SearchInts(opts.CumWeights, rand.Intn(totalWeight)+1)]
				// Random range selection
				r := defaultRanges[rand.Intn(len(defaultRanges))]
				timestampNs := time.Now().UnixNano()

				req := &proofpb.GetProofRequest{
					Account:    account,
					StartBlock: opts.StartBlock,
					EndBlock:   opts.StartBlock + r.Blocks,
				}

				start := time.Now()
				_, err := client.GetProof(context.Background(), req)
				latency := time.Since(start)

				isClErr := isClientError(err)

				resultsCh <- StressResult{
					Timestamp:     timestampNs,
					ClientID:      clientID,
					LatencyMs:     latency.Milliseconds(),
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

	// Final summary
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
// Single Proof (Normal Mode)
// =============================================================================

func runSingleProof(client proofpb.ProofServiceClient, account string, startBlock, endBlock uint64, paramsDir string) {
	precomputedData, err := SetupPrecomputedData(paramsDir)
	if err != nil {
		log.Fatalf("failed to setup precomputed data: %v", err)
	}

	req := &proofpb.GetProofRequest{
		Account:    account,
		StartBlock: startBlock + StartBlock,
		EndBlock:   endBlock + StartBlock,
	}

	fmt.Printf("Requesting proof for account %s, blocks %d-%d\n", account, startBlock, endBlock)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := client.GetProof(ctx, req)
	if err != nil {
		log.Fatalf("GetProof failed: %v", err)
	}

	addr := common.HexToAddress(account)

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
}

// =============================================================================
// Utility Functions
// =============================================================================

func loadAccounts(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var accounts []string
	scanner := bufio.NewScanner(f)

	// Skip header
	if scanner.Scan() {
		// Skip header line
	}

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) >= 1 {
			accounts = append(accounts, parts[0])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return accounts, nil
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

	// Skip header
	if scanner.Scan() {
		// Skip header line
	}

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			accounts = append(accounts, parts[0])
			weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				weight = 1 // fallback for unparseable weights
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
