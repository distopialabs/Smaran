package benchutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultOutputDir is the default root directory for benchmark output.
const DefaultOutputDir = "/data/local/benchmark_output"

// IngestionOutputPath returns the standardized output path for ingestion benchmarks.
// Format: {baseDir}/{protocol}/ingestion_{kUsers}_{timestamp}.csv
func IngestionOutputPath(baseDir, protocol string, kUsers int) (string, error) {
	dir, err := EnsureOutputDir(baseDir, protocol)
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("ingestion_%d_%s.csv", kUsers, ts)), nil
}

// UpdateMetricsOutputPath returns the standardized output path for update-level metrics.
// Format: {baseDir}/{protocol}/update_metrics_{kUsers}_{timestamp}.csv
func UpdateMetricsOutputPath(baseDir, protocol string, kUsers int) (string, error) {
	dir, err := EnsureOutputDir(baseDir, protocol)
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("update_metrics_%d_%s.csv", kUsers, ts)), nil
}

// ProofOutputPath returns the standardized output path for proof benchmarks.
// Format: {baseDir}/{protocol}/proof_range{rangeSize}_{timestamp}.csv
func ProofOutputPath(baseDir, protocol string, rangeSize int) (string, error) {
	dir, err := EnsureOutputDir(baseDir, protocol)
	if err != nil {
		return "", err
	}
	ts := time.Now().Format("20060102_150405")
	return filepath.Join(dir, fmt.Sprintf("proof_range%d_%s.csv", rangeSize, ts)), nil
}

// EnsureOutputDir creates {baseDir}/{protocol}/ if it doesn't exist
// and returns the directory path.
func EnsureOutputDir(baseDir, protocol string) (string, error) {
	dir := filepath.Join(baseDir, protocol)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", dir, err)
	}
	return dir, nil
}
