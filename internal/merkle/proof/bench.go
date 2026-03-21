package proof

import (
	"fmt"
	"os"
	"time"
)

// BenchResult holds benchmarking metrics for a proof operation.
type BenchResult struct {
	ProofGenTime  time.Duration
	ProofByteSize int
	ProofNodes    int
	JSONSize      int
	VerifyTime    time.Duration
}

// ComputeProofByteSize sums the raw RLP-encoded node sizes.
func ComputeProofByteSize(nodes [][]byte) int {
	total := 0
	for _, n := range nodes {
		total += len(n)
	}
	return total
}

// PrintBenchResult writes benchmark metrics to stderr.
func PrintBenchResult(b *BenchResult) {
	fmt.Fprintf(os.Stderr,
		"\n--- Benchmark Results ---\n"+
			"Proof generation time : %s\n"+
			"Proof byte size       : %d bytes (%d nodes)\n"+
			"JSON response size    : %d bytes\n"+
			"Verification time     : %s\n"+
			"-------------------------\n",
		b.ProofGenTime.Round(time.Microsecond),
		b.ProofByteSize, b.ProofNodes,
		b.JSONSize,
		b.VerifyTime.Round(time.Microsecond),
	)
}
