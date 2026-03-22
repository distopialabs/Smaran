package benchutil

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

// ProofBenchConfig holds common benchmark parameters across all three proof clients.
type ProofBenchConfig struct {
	ServerAddr string
	RangeSize  int
	NumClients int
	Duration   time.Duration
	FirstBlock uint64 // dataset.FIRST_BLOCK
}

// ClientStats tracks per-goroutine metrics, aggregated at the end.
type ClientStats struct {
	TotalRequests       int
	TotalClientErrors   int
	TotalServerErrors   int
	TotalVerifyFailures int
	TotalProofgenNs     int64
	TotalE2ENs          int64
	TotalVerifyNs       int64
	TotalPayloadBytes   int64
}

// Add merges another ClientStats into this one.
func (s *ClientStats) Add(other ClientStats) {
	s.TotalRequests += other.TotalRequests
	s.TotalClientErrors += other.TotalClientErrors
	s.TotalServerErrors += other.TotalServerErrors
	s.TotalVerifyFailures += other.TotalVerifyFailures
	s.TotalProofgenNs += other.TotalProofgenNs
	s.TotalE2ENs += other.TotalE2ENs
	s.TotalVerifyNs += other.TotalVerifyNs
	s.TotalPayloadBytes += other.TotalPayloadBytes
}

// AggregateStats merges stats from all goroutines into a single summary.
func AggregateStats(stats []ClientStats) ClientStats {
	var agg ClientStats
	for _, s := range stats {
		agg.Add(s)
	}
	return agg
}

// WeightedAccountSelector provides weighted random account selection based on update counts.
type WeightedAccountSelector struct {
	accounts   []string // 0x-prefixed addresses
	cumWeights []int64  // cumulative weights for binary search
	totalW     int64
	mu         sync.Mutex
	rng        *rand.Rand
}

// NewWeightedAccountSelector loads accounts from CSV (Address,UpdateCount header)
// and builds a cumulative weight distribution.
func NewWeightedAccountSelector(csvPath string) (*WeightedAccountSelector, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("open accounts CSV: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	// Skip header.
	if _, err := r.Read(); err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}

	var accounts []string
	var cumWeights []int64
	var cumW int64

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read CSV row: %w", err)
		}
		if len(row) < 2 {
			continue
		}

		addr := row[0]
		if len(addr) > 0 && addr[0] != '0' {
			addr = "0x" + addr
		}

		weight, err := strconv.ParseInt(row[1], 10, 64)
		if err != nil || weight <= 0 {
			continue
		}

		cumW += weight
		accounts = append(accounts, addr)
		cumWeights = append(cumWeights, cumW)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no valid accounts in %s", csvPath)
	}

	return &WeightedAccountSelector{
		accounts:   accounts,
		cumWeights: cumWeights,
		totalW:     cumW,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// Pick returns a randomly selected account address, weighted by update count.
// Safe for concurrent use.
func (s *WeightedAccountSelector) Pick() string {
	s.mu.Lock()
	target := s.rng.Int63n(s.totalW)
	s.mu.Unlock()

	// Binary search for the first cumWeight > target.
	lo, hi := 0, len(s.cumWeights)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if s.cumWeights[mid] <= target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return s.accounts[lo]
}

// Size returns the number of loaded accounts.
func (s *WeightedAccountSelector) Size() int {
	return len(s.accounts)
}

// RandomStartBlock picks a valid random start block for a proof query.
func RandomStartBlock(rng *rand.Rand, firstBlock, latestBlock uint64, rangeSize int) uint64 {
	maxStart := latestBlock - uint64(rangeSize) + 1
	if maxStart <= firstBlock {
		return firstBlock
	}
	return firstBlock + uint64(rng.Int63n(int64(maxStart-firstBlock+1)))
}

// PrintSummary writes the benchmark summary to the given writer.
func PrintSummary(w io.Writer, cfg ProofBenchConfig, stats ClientStats, wallDuration time.Duration) {
	fmt.Fprintf(w, "=== Proof Benchmark Results ===\n")
	fmt.Fprintf(w, "Duration:            %s\n", wallDuration.Round(time.Second))
	fmt.Fprintf(w, "Clients:             %d\n", cfg.NumClients)
	fmt.Fprintf(w, "Range Size:          %d\n", cfg.RangeSize)
	fmt.Fprintf(w, "Total Requests:      %d\n", stats.TotalRequests)
	fmt.Fprintf(w, "Client Errors:       %d\n", stats.TotalClientErrors)
	fmt.Fprintf(w, "Server Errors:       %d\n", stats.TotalServerErrors)
	fmt.Fprintf(w, "Verify Failures:     %d\n", stats.TotalVerifyFailures)

	if stats.TotalRequests > 0 {
		throughput := float64(stats.TotalRequests) / wallDuration.Seconds()
		avgProofgen := time.Duration(stats.TotalProofgenNs / int64(stats.TotalRequests))
		avgE2E := time.Duration(stats.TotalE2ENs / int64(stats.TotalRequests))
		avgVerify := time.Duration(stats.TotalVerifyNs / int64(stats.TotalRequests))
		avgPayload := float64(stats.TotalPayloadBytes) / float64(stats.TotalRequests)

		fmt.Fprintf(w, "Throughput:          %.1f req/s\n", throughput)
		fmt.Fprintf(w, "Avg Proofgen:        %s\n", avgProofgen.Round(100*time.Microsecond))
		fmt.Fprintf(w, "Avg E2E Latency:     %s\n", avgE2E.Round(100*time.Microsecond))
		fmt.Fprintf(w, "Avg Verify:          %s\n", avgVerify.Round(100*time.Microsecond))
		fmt.Fprintf(w, "Avg Payload Size:    %.1fKB\n", avgPayload/1024)
	}
}

// WriteSummaryFile writes the summary to the standard output path for proof benchmarks.
func WriteSummaryFile(protocol string, cfg ProofBenchConfig, stats ClientStats, wallDuration time.Duration) error {
	path, err := ProofOutputPath(protocol, cfg.RangeSize)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create summary file: %w", err)
	}
	defer f.Close()
	PrintSummary(f, cfg, stats, wallDuration)
	return nil
}
