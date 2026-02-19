#!/usr/bin/env python3
"""
Proof Server Benchmark Analysis Script

Unified analysis and visualization for all benchmark modes:
- range: Latency, payload size, network overhead vs block range
- concurrency: Latency distribution vs concurrency level
- stress: Throughput and latency over time

Usage:
    python analyze_benchmarks.py --mode range --input benchmark_output/range_*.csv --warmup 30
    python analyze_benchmarks.py --mode concurrency --input benchmark_output/concurrency_*.csv
    python analyze_benchmarks.py --mode stress --input benchmark_output/stress_*.csv --warmup 60 --output plots/
"""

import argparse
import glob
import os
from datetime import datetime

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd


def parse_args():
    parser = argparse.ArgumentParser(description="Analyze proof server benchmark results")
    parser.add_argument("--mode", required=True, choices=["range", "concurrency", "stress"],
                        help="Benchmark mode to analyze")
    parser.add_argument("--input", required=True, help="Input CSV file pattern (glob)")
    parser.add_argument("--warmup", type=int, default=0, help="Warmup seconds to exclude")
    parser.add_argument("--output", default="./benchmark_output/plots", help="Output directory for plots")
    return parser.parse_args()


def load_data(pattern: str) -> pd.DataFrame:
    """Load and concatenate all matching CSV files."""
    files = glob.glob(pattern)
    if not files:
        raise FileNotFoundError(f"No files matching: {pattern}")
    
    dfs = []
    for f in files:
        df = pd.read_csv(f)
        df['source_file'] = os.path.basename(f)
        dfs.append(df)
    
    return pd.concat(dfs, ignore_index=True)


def filter_warmup(df: pd.DataFrame, warmup_seconds: int, timestamp_col: str = "timestamp_ns") -> pd.DataFrame:
    """Filter out warmup period based on timestamps."""
    if warmup_seconds <= 0 or timestamp_col not in df.columns:
        return df
    
    start_time = df[timestamp_col].min()
    warmup_ns = warmup_seconds * 1_000_000_000
    return df[df[timestamp_col] >= start_time + warmup_ns]


def analyze_range(df: pd.DataFrame, output_dir: str):
    """Analyze range benchmark results."""
    print("\n=== Range Benchmark Analysis ===")
    
    # Group by range label
    grouped = df.groupby("range_label").agg({
        "latency_ms": ["mean", "std", "min", "max", lambda x: np.percentile(x, 50), 
                       lambda x: np.percentile(x, 95), lambda x: np.percentile(x, 99)],
        "generation_time_ms": "mean",
        "network_overhead_ms": "mean",
        "payload_bytes": "mean",
        "range_proofs_bytes": "mean",
        "balance_infos_bytes": "mean",
        "range_proofs_count": "mean",
        "balance_infos_count": "mean",
        "verification_time_ms": "mean",
        "blocks": "first",
    })
    
    # Flatten column names
    grouped.columns = ['_'.join(col).strip('_') for col in grouped.columns]
    grouped = grouped.reset_index()
    grouped = grouped.sort_values("blocks_first")
    
    print("\nLatency Summary:")
    print(grouped[["range_label", "blocks_first", "latency_ms_mean", "latency_ms_<lambda_0>", 
                   "latency_ms_<lambda_1>", "latency_ms_<lambda_2>"]].to_string(index=False))
    
    # Create plots
    fig, axes = plt.subplots(2, 2, figsize=(14, 10))
    
    # Plot 1: Latency vs Block Range
    ax1 = axes[0, 0]
    x = range(len(grouped))
    ax1.bar(x, grouped["latency_ms_mean"], yerr=grouped["latency_ms_std"], capsize=5, alpha=0.7)
    ax1.set_xticks(x)
    ax1.set_xticklabels(grouped["range_label"], rotation=45, ha="right")
    ax1.set_ylabel("Latency (ms)")
    ax1.set_title("Total Latency vs Block Range")
    ax1.grid(axis="y", alpha=0.3)
    
    # Plot 2: Latency Breakdown (Generation vs Network)
    ax2 = axes[0, 1]
    width = 0.35
    ax2.bar([i - width/2 for i in x], grouped["generation_time_ms_mean"], width, label="Generation", alpha=0.7)
    ax2.bar([i + width/2 for i in x], grouped["network_overhead_ms_mean"], width, label="Network+Overhead", alpha=0.7)
    ax2.set_xticks(x)
    ax2.set_xticklabels(grouped["range_label"], rotation=45, ha="right")
    ax2.set_ylabel("Time (ms)")
    ax2.set_title("Latency Breakdown")
    ax2.legend()
    ax2.grid(axis="y", alpha=0.3)
    
    # Plot 3: Payload Size Breakdown
    ax3 = axes[1, 0]
    rp_mb = grouped["range_proofs_bytes_mean"] / (1024 * 1024)
    bi_mb = grouped["balance_infos_bytes_mean"] / (1024 * 1024)
    ax3.bar(x, rp_mb, label="Range Proofs", alpha=0.7)
    ax3.bar(x, bi_mb, bottom=rp_mb, label="Balance Infos", alpha=0.7)
    ax3.set_xticks(x)
    ax3.set_xticklabels(grouped["range_label"], rotation=45, ha="right")
    ax3.set_ylabel("Size (MB)")
    ax3.set_title("Payload Size Breakdown")
    ax3.legend()
    ax3.grid(axis="y", alpha=0.3)
    
    # Plot 4: Verification Time (if available)
    ax4 = axes[1, 1]
    if grouped["verification_time_ms_mean"].sum() > 0:
        ax4.bar(x, grouped["verification_time_ms_mean"], alpha=0.7, color="green")
        ax4.set_ylabel("Verification Time (ms)")
        ax4.set_title("Client Verification Time")
    else:
        ax4.bar(x, grouped["range_proofs_count_mean"], alpha=0.7, label="Range Proofs", color="blue")
        ax4.bar(x, grouped["balance_infos_count_mean"], alpha=0.7, label="Balance Infos", color="orange", bottom=grouped["range_proofs_count_mean"])
        ax4.set_ylabel("Count")
        ax4.set_title("Proofs & Balance Info Count")
        ax4.legend()
    ax4.set_xticks(x)
    ax4.set_xticklabels(grouped["range_label"], rotation=45, ha="right")
    ax4.grid(axis="y", alpha=0.3)
    
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, "range_analysis.png"), dpi=150)
    print(f"\nSaved: {os.path.join(output_dir, 'range_analysis.png')}")
    plt.close()


def analyze_concurrency(df: pd.DataFrame, output_dir: str):
    """Analyze concurrency benchmark results."""
    print("\n=== Concurrency Benchmark Analysis ===")
    
    # Filter successful requests
    df_success = df[df["success"] == True]
    
    # Group by concurrency level
    grouped = df_success.groupby("concurrency_level").agg({
        "latency_ms": ["mean", "std", lambda x: np.percentile(x, 50), 
                       lambda x: np.percentile(x, 95), lambda x: np.percentile(x, 99)],
    })
    grouped.columns = ["mean", "std", "p50", "p95", "p99"]
    grouped = grouped.reset_index()
    
    # Calculate throughput per level
    throughput = []
    for level in grouped["concurrency_level"]:
        level_df = df[df["concurrency_level"] == level]
        duration = (level_df["timestamp_ns"].max() - level_df["timestamp_ns"].min()) / 1e9
        success_count = level_df["success"].sum()
        throughput.append(success_count / duration if duration > 0 else 0)
    grouped["throughput"] = throughput
    
    # Error rate
    error_rate = []
    for level in grouped["concurrency_level"]:
        level_df = df[df["concurrency_level"] == level]
        error_rate.append(100 * (1 - level_df["success"].mean()))
    grouped["error_rate"] = error_rate
    
    print("\nConcurrency Summary:")
    print(grouped.to_string(index=False))
    
    # Create plots
    fig, axes = plt.subplots(2, 2, figsize=(14, 10))
    
    x = grouped["concurrency_level"]
    
    # Plot 1: Latency vs Concurrency
    ax1 = axes[0, 0]
    ax1.errorbar(x, grouped["mean"], yerr=grouped["std"], marker='o', capsize=5)
    ax1.set_xlabel("Concurrency Level")
    ax1.set_ylabel("Latency (ms)")
    ax1.set_title("Average Latency vs Concurrency")
    ax1.grid(alpha=0.3)
    ax1.set_xscale("log")
    
    # Plot 2: Latency Percentiles
    ax2 = axes[0, 1]
    ax2.plot(x, grouped["p50"], marker='o', label="p50")
    ax2.plot(x, grouped["p95"], marker='s', label="p95")
    ax2.plot(x, grouped["p99"], marker='^', label="p99")
    ax2.set_xlabel("Concurrency Level")
    ax2.set_ylabel("Latency (ms)")
    ax2.set_title("Latency Percentiles")
    ax2.legend()
    ax2.grid(alpha=0.3)
    ax2.set_xscale("log")
    
    # Plot 3: Throughput vs Concurrency
    ax3 = axes[1, 0]
    ax3.plot(x, grouped["throughput"], marker='o', color="green")
    ax3.set_xlabel("Concurrency Level")
    ax3.set_ylabel("Throughput (req/s)")
    ax3.set_title("Throughput vs Concurrency")
    ax3.grid(alpha=0.3)
    ax3.set_xscale("log")
    
    # Plot 4: Error Rate
    ax4 = axes[1, 1]
    ax4.bar(range(len(x)), grouped["error_rate"], color="red", alpha=0.7)
    ax4.set_xticks(range(len(x)))
    ax4.set_xticklabels(x)
    ax4.set_xlabel("Concurrency Level")
    ax4.set_ylabel("Error Rate (%)")
    ax4.set_title("Error Rate vs Concurrency")
    ax4.grid(axis="y", alpha=0.3)
    
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, "concurrency_analysis.png"), dpi=150)
    print(f"\nSaved: {os.path.join(output_dir, 'concurrency_analysis.png')}")
    plt.close()


def analyze_stress(df: pd.DataFrame, output_dir: str):
    """Analyze stress benchmark results."""
    print("\n=== Stress Benchmark Analysis ===")
    
    # Convert timestamp to relative seconds
    start_time = df["timestamp_ns"].min()
    df["elapsed_s"] = (df["timestamp_ns"] - start_time) / 1e9
    
    # Create time windows (1 second buckets)
    df["time_bucket"] = df["elapsed_s"].astype(int)
    
    # Aggregate by time bucket
    time_series = df.groupby("time_bucket").agg({
        "success": ["sum", "count"],
        "latency_ms": ["mean", lambda x: np.percentile(x, 50), 
                       lambda x: np.percentile(x, 95), lambda x: np.percentile(x, 99)],
    })
    time_series.columns = ["success_count", "total_count", "latency_mean", "latency_p50", "latency_p95", "latency_p99"]
    time_series["throughput"] = time_series["success_count"]  # requests per second
    time_series["error_rate"] = 100 * (1 - time_series["success_count"] / time_series["total_count"])
    time_series = time_series.reset_index()
    
    # Summary stats
    total_duration = df["elapsed_s"].max()
    total_requests = len(df)
    success_count = df["success"].sum()
    error_count = total_requests - success_count
    avg_throughput = success_count / total_duration if total_duration > 0 else 0
    avg_latency = df[df["success"]]["latency_ms"].mean()
    
    print(f"\nDuration: {total_duration:.1f}s")
    print(f"Total Requests: {total_requests}")
    print(f"Successful: {success_count}")
    print(f"Errors: {error_count} ({100*error_count/total_requests:.2f}%)")
    print(f"Avg Throughput: {avg_throughput:.2f} req/s")
    print(f"Avg Latency: {avg_latency:.1f}ms")
    
    # Create plots
    fig, axes = plt.subplots(2, 2, figsize=(14, 10))
    
    x = time_series["time_bucket"]
    
    # Plot 1: Throughput Over Time
    ax1 = axes[0, 0]
    ax1.plot(x, time_series["throughput"], alpha=0.7)
    ax1.axhline(y=avg_throughput, color='r', linestyle='--', label=f"Avg: {avg_throughput:.1f}")
    ax1.set_xlabel("Time (s)")
    ax1.set_ylabel("Throughput (req/s)")
    ax1.set_title("Throughput Over Time")
    ax1.legend()
    ax1.grid(alpha=0.3)
    
    # Plot 2: Latency Percentiles Over Time
    ax2 = axes[0, 1]
    ax2.plot(x, time_series["latency_p50"], label="p50", alpha=0.7)
    ax2.plot(x, time_series["latency_p95"], label="p95", alpha=0.7)
    ax2.plot(x, time_series["latency_p99"], label="p99", alpha=0.7)
    ax2.set_xlabel("Time (s)")
    ax2.set_ylabel("Latency (ms)")
    ax2.set_title("Latency Percentiles Over Time")
    ax2.legend()
    ax2.grid(alpha=0.3)
    
    # Plot 3: Latency Distribution (Histogram)
    ax3 = axes[1, 0]
    latencies = df[df["success"]]["latency_ms"]
    ax3.hist(latencies, bins=50, alpha=0.7, edgecolor='black')
    ax3.axvline(x=latencies.median(), color='r', linestyle='--', label=f"Median: {latencies.median():.0f}ms")
    ax3.set_xlabel("Latency (ms)")
    ax3.set_ylabel("Count")
    ax3.set_title("Latency Distribution")
    ax3.legend()
    ax3.grid(alpha=0.3)
    
    # Plot 4: Error Rate Over Time
    ax4 = axes[1, 1]
    ax4.plot(x, time_series["error_rate"], color="red", alpha=0.7)
    ax4.set_xlabel("Time (s)")
    ax4.set_ylabel("Error Rate (%)")
    ax4.set_title("Error Rate Over Time")
    ax4.grid(alpha=0.3)
    ax4.set_ylim(bottom=0)
    
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, "stress_analysis.png"), dpi=150)
    print(f"\nSaved: {os.path.join(output_dir, 'stress_analysis.png')}")
    plt.close()


def main():
    args = parse_args()
    
    # Create output directory
    os.makedirs(args.output, exist_ok=True)
    
    # Load data
    print(f"Loading data from: {args.input}")
    df = load_data(args.input)
    print(f"Loaded {len(df)} rows")
    
    # Filter warmup
    if args.warmup > 0:
        original_len = len(df)
        df = filter_warmup(df, args.warmup)
        print(f"Filtered warmup ({args.warmup}s): {original_len} -> {len(df)} rows")
    
    # Analyze based on mode
    if args.mode == "range":
        analyze_range(df, args.output)
    elif args.mode == "concurrency":
        analyze_concurrency(df, args.output)
    elif args.mode == "stress":
        analyze_stress(df, args.output)
    
    print("\nDone!")


if __name__ == "__main__":
    main()
