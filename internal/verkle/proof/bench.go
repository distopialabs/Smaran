package proof

import (
	"fmt"
	"os"
	"time"
)

// BenchResult holds benchmarking metrics for a proof operation.
type BenchResult struct {
	ProofGenTime     time.Duration
	ProofPayloadSize int
	ProofJSONSize    int
	VerifyTime       time.Duration
}

// PrintBenchResult writes benchmark metrics to stderr.
func PrintBenchResult(b *BenchResult) {
	fmt.Fprintf(os.Stderr,
		"\n--- Benchmark Results ---\n"+
			"Proof generation time : %s\n"+
			"Proof payload size    : %d bytes\n"+
			"JSON response size    : %d bytes\n"+
			"Verification time     : %s\n"+
			"-------------------------\n",
		b.ProofGenTime.Round(time.Microsecond),
		b.ProofPayloadSize,
		b.ProofJSONSize,
		b.VerifyTime.Round(time.Microsecond),
	)
}
