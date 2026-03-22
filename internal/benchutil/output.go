package benchutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// IngestionOutputPath returns the standardized output path for ingestion benchmarks.
// Format: benchmark_output/{protocol}/ingestion_{kUsers}_{timestamp}.csv
func IngestionOutputPath(protocol string, kUsers int) (string, error) {
	dir, err := EnsureOutputDir(protocol)
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("ingestion_%d_%s.csv", kUsers, ts)), nil
}

// ProofOutputPath returns the standardized output path for proof benchmarks.
// Format: benchmark_output/{protocol}/proof_range{rangeSize}_{timestamp}.txt
func ProofOutputPath(protocol string, rangeSize int) (string, error) {
	dir, err := EnsureOutputDir(protocol)
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("proof_range%d_%s.txt", rangeSize, ts)), nil
}

// EnsureOutputDir creates benchmark_output/{protocol}/ if it doesn't exist
// and returns the directory path.
func EnsureOutputDir(protocol string) (string, error) {
	dir := filepath.Join("benchmark_output", protocol)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", dir, err)
	}
	return dir, nil
}
