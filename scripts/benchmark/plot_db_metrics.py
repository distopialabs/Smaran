#!/usr/bin/env python3
"""
DB Metrics Visualization Script for Samurai Commit Generation

This script plots Pebble database metrics (compaction, L0 files, etc.) and
overlays them with latency/throughput data to identify correlations.

Usage:
    python plot_db_metrics.py --db bench_db_metrics_*.csv --updates bench_updates_*.csv \
        --window 5 --warmup 30 --cooldown 30 --output ./plots/

Dependencies:
    pip install pandas matplotlib
"""

import argparse
from pathlib import Path

import matplotlib.pyplot as plt
import pandas as pd


def load_db_metrics_csv(filepath: str) -> pd.DataFrame:
    """Load DB metrics CSV file."""
    df = pd.read_csv(filepath)
    # Convert nanoseconds to seconds
    df["timestamp_s"] = df["timestamp_ns"] / 1e9
    # Convert estimated debt to MB
    df["compact_estimated_debt_mb"] = df["compact_estimated_debt"] / (1024 * 1024)
    # Convert sizes to MB
    df["l0_size_mb"] = df["l0_size"] / (1024 * 1024)
    df["total_size_mb"] = df["total_size"] / (1024 * 1024)
    df["memtable_size_mb"] = df["memtable_size"] / (1024 * 1024)
    # Convert write stall duration to ms
    df["write_stall_duration_ms"] = df["write_stall_duration_ns"] / 1e6
    return df


def aggregate_db_metrics_all_shards(df: pd.DataFrame) -> pd.DataFrame:
    """Aggregate DB metrics across all shards per timestamp."""
    if df.empty or "shard_id" not in df.columns:
        return df
    
    agg_df = df.groupby("timestamp_ns").agg({
        "timestamp_s": "first",
        "compact_count": "sum",
        "compact_estimated_debt": "sum",
        "compact_estimated_debt_mb": "sum",
        "compact_in_progress_bytes": "sum",
        "compact_num_in_progress": "sum",
        "flush_count": "sum",
        "memtable_size": "sum",
        "memtable_size_mb": "sum",
        "memtable_count": "sum",
        "l0_num_files": "sum",
        "l0_size": "sum",
        "l0_size_mb": "sum",
        "total_num_files": "sum",
        "total_size": "sum",
        "total_size_mb": "sum",
        "write_stall_count": "sum",
        "write_stall_duration_ns": "sum",
        "write_stall_duration_ms": "sum",
        "block_cache_size": "first",  # Cache is shared
        "block_cache_hits": "first",
        "block_cache_misses": "first",
    }).reset_index(drop=True)
    
    return agg_df


def load_updates_csv(filepath: str) -> pd.DataFrame:
    """Load updates CSV file."""
    df = pd.read_csv(filepath)
    df["completed_at_s"] = df["completed_at_ns"] / 1e9
    df["latency_ms"] = df["latency_ns"] / 1e6
    return df


def trim_and_normalize(
    df: pd.DataFrame, time_col: str, warmup_s: float, cooldown_s: float
) -> pd.DataFrame:
    """Trim warmup/cooldown and normalize time to start from 0."""
    if df.empty:
        return df

    min_time = df[time_col].min()
    max_time = df[time_col].max()

    start_time = min_time + warmup_s
    end_time = max_time - cooldown_s

    if start_time >= end_time:
        print(f"Warning: warmup + cooldown >= total duration. No data left.")
        return df.iloc[0:0]

    trimmed = df[(df[time_col] >= start_time) & (df[time_col] <= end_time)].copy()
    trimmed["time_normalized"] = trimmed[time_col] - start_time
    return trimmed


def aggregate_updates_by_window(df: pd.DataFrame, window_s: float) -> pd.DataFrame:
    """Aggregate update latency/throughput by time window."""
    if df.empty:
        return pd.DataFrame(columns=["window_start", "latency_mean", "throughput"])

    df = df.copy()
    df["window"] = (df["time_normalized"] // window_s).astype(int)

    latency_agg = df.groupby("window")["latency_ms"].mean().reset_index()
    latency_agg = latency_agg.rename(columns={"latency_ms": "latency_mean"})

    count_agg = df.groupby("window").size().reset_index(name="count")
    count_agg["throughput"] = count_agg["count"] / window_s

    result = latency_agg.merge(count_agg[["window", "throughput"]], on="window")
    result["window_start"] = result["window"] * window_s

    return result


def main():
    parser = argparse.ArgumentParser(
        description="Plot DB metrics vs latency/throughput from Samurai benchmark"
    )
    parser.add_argument(
        "--db", required=True, help="Path to DB metrics CSV file (bench_db_metrics_*.csv)"
    )
    parser.add_argument(
        "--updates", required=True, help="Path to updates CSV file (bench_updates_*.csv)"
    )
    parser.add_argument(
        "--window",
        type=float,
        default=5.0,
        help="Time window in seconds for aggregation (default: 5)",
    )
    parser.add_argument(
        "--warmup",
        type=float,
        default=30.0,
        help="Warmup period to trim in seconds (default: 30)",
    )
    parser.add_argument(
        "--cooldown",
        type=float,
        default=30.0,
        help="Cooldown period to trim in seconds (default: 30)",
    )
    parser.add_argument(
        "--output",
        default=".",
        help="Output directory for plots (default: current directory)",
    )
    parser.add_argument(
        "--format",
        choices=["png", "pdf", "svg"],
        default="png",
        help="Output format (default: png)",
    )

    args = parser.parse_args()

    output_dir = Path(args.output)
    output_dir.mkdir(parents=True, exist_ok=True)

    print(f"Loading DB metrics from: {args.db}")
    db_df = load_db_metrics_csv(args.db)
    print(f"Loaded {len(db_df)} DB metric samples")

    # Check for shard_id column
    has_shards = "shard_id" in db_df.columns
    if has_shards:
        shard_ids = sorted(db_df["shard_id"].unique())
        num_shards = len(shard_ids)
        print(f"Found {num_shards} shards: {shard_ids[:5]}{'...' if num_shards > 5 else ''}")

    print(f"Loading updates from: {args.updates}")
    updates_df = load_updates_csv(args.updates)
    print(f"Loaded {len(updates_df)} update records")

    # Trim and normalize
    print(f"Trimming warmup ({args.warmup}s) and cooldown ({args.cooldown}s)...")
    db_trimmed = trim_and_normalize(db_df, "timestamp_s", args.warmup, args.cooldown)
    updates_trimmed = trim_and_normalize(updates_df, "completed_at_s", args.warmup, args.cooldown)

    print(f"After trimming: {len(db_trimmed)} DB samples, {len(updates_trimmed)} updates")

    # Aggregate across shards for main overview plots
    if has_shards:
        db_agg_all = aggregate_db_metrics_all_shards(db_trimmed)
        # Need to add time_normalized to aggregated data
        if len(db_agg_all) > 0:
            min_ts = db_agg_all["timestamp_s"].min()
            db_agg_all["time_normalized"] = db_agg_all["timestamp_s"] - min_ts
    else:
        db_agg_all = db_trimmed

    # Aggregate updates
    updates_agg = aggregate_updates_by_window(updates_trimmed, args.window)

    timestamp = Path(args.db).stem.replace("bench_db_metrics_", "")

    # Create main figure: DB metrics with latency overlay
    fig, axes = plt.subplots(3, 2, figsize=(14, 12))
    fig.suptitle(
        f"Database Metrics vs Performance\n(Window: {args.window}s, Warmup: {args.warmup}s, Cooldown: {args.cooldown}s)",
        fontsize=14,
        fontweight="bold",
    )

    # Plot 1: Compaction Estimated Debt vs Latency (aggregated across shards)
    ax1 = axes[0, 0]
    ax1_twin = ax1.twinx()
    ax1.plot(db_agg_all["time_normalized"], db_agg_all["compact_estimated_debt_mb"],
             color="#e74c3c", linewidth=1.5, label="Compaction Debt (MB)")
    ax1.fill_between(db_agg_all["time_normalized"], db_agg_all["compact_estimated_debt_mb"],
                     alpha=0.3, color="#e74c3c")
    ax1_twin.plot(updates_agg["window_start"], updates_agg["latency_mean"],
                  color="#3498db", linewidth=1.5, linestyle="--", label="Latency (ms)")
    ax1.set_xlabel("Time (seconds)")
    ax1.set_ylabel("Compaction Debt (MB)", color="#e74c3c")
    ax1_twin.set_ylabel("Avg Latency (ms)", color="#3498db")
    ax1.set_title("Compaction Debt vs Latency (All Shards)")
    ax1.grid(True, alpha=0.3)
    ax1.legend(loc="upper left")
    ax1_twin.legend(loc="upper right")

    # Plot 2: L0 Files vs Throughput (aggregated across shards)
    ax2 = axes[0, 1]
    ax2_twin = ax2.twinx()
    ax2.plot(db_agg_all["time_normalized"], db_agg_all["l0_num_files"],
             color="#9b59b6", linewidth=1.5, label="L0 Files")
    ax2.fill_between(db_agg_all["time_normalized"], db_agg_all["l0_num_files"],
                     alpha=0.3, color="#9b59b6")
    ax2_twin.plot(updates_agg["window_start"], updates_agg["throughput"],
                  color="#2ecc71", linewidth=1.5, linestyle="--", label="Throughput")
    ax2.set_xlabel("Time (seconds)")
    ax2.set_ylabel("L0 Files Count", color="#9b59b6")
    ax2_twin.set_ylabel("Throughput (updates/s)", color="#2ecc71")
    ax2.set_title("L0 Files vs Throughput (All Shards)")
    ax2.grid(True, alpha=0.3)
    ax2.legend(loc="upper left")
    ax2_twin.legend(loc="upper right")

    # Plot 3: Compactions In Progress (aggregated)
    ax3 = axes[1, 0]
    ax3.plot(db_agg_all["time_normalized"], db_agg_all["compact_num_in_progress"],
             color="#f39c12", linewidth=1.5, label="Compactions In Progress")
    ax3.fill_between(db_agg_all["time_normalized"], db_agg_all["compact_num_in_progress"],
                     alpha=0.3, color="#f39c12")
    ax3.set_xlabel("Time (seconds)")
    ax3.set_ylabel("Count")
    ax3.set_title("Concurrent Compactions (All Shards)")
    ax3.grid(True, alpha=0.3)
    ax3.legend()

    # Plot 4: Total DB Size (aggregated)
    ax4 = axes[1, 1]
    ax4.plot(db_agg_all["time_normalized"], db_agg_all["total_size_mb"],
             color="#1abc9c", linewidth=1.5, label="Total Size (MB)")
    ax4.fill_between(db_agg_all["time_normalized"], db_agg_all["total_size_mb"],
                     alpha=0.3, color="#1abc9c")
    ax4.set_xlabel("Time (seconds)")
    ax4.set_ylabel("Size (MB)")
    ax4.set_title("Total Database Size (All Shards)")
    ax4.grid(True, alpha=0.3)
    ax4.legend()

    # Plot 5: MemTable Size (aggregated)
    ax5 = axes[2, 0]
    ax5.plot(db_agg_all["time_normalized"], db_agg_all["memtable_size_mb"],
             color="#e67e22", linewidth=1.5, label="MemTable Size (MB)")
    ax5.fill_between(db_agg_all["time_normalized"], db_agg_all["memtable_size_mb"],
                     alpha=0.3, color="#e67e22")
    ax5.set_xlabel("Time (seconds)")
    ax5.set_ylabel("Size (MB)")
    ax5.set_title("MemTable Size (All Shards)")
    ax5.grid(True, alpha=0.3)
    ax5.legend()

    # Plot 6: Flush & Compaction Counts (aggregated, cumulative)
    ax6 = axes[2, 1]
    ax6.plot(db_agg_all["time_normalized"], db_agg_all["flush_count"],
             color="#3498db", linewidth=1.5, label="Flush Count")
    ax6.plot(db_agg_all["time_normalized"], db_agg_all["compact_count"],
             color="#e74c3c", linewidth=1.5, label="Compaction Count")
    ax6.set_xlabel("Time (seconds)")
    ax6.set_ylabel("Count (cumulative)")
    ax6.set_title("Flush & Compaction Counts (All Shards)")
    ax6.grid(True, alpha=0.3)
    ax6.legend()

    plt.tight_layout()

    output_path = output_dir / f"db_metrics_{timestamp}.{args.format}"
    plt.savefig(output_path, dpi=150, bbox_inches="tight")
    print(f"\nDB metrics plots saved to: {output_path}")

    # Create correlation plot: Compaction Debt vs Latency scatter
    fig_corr, ax_corr = plt.subplots(figsize=(10, 6))

    # Merge DB and updates data by time (use aggregated data)
    db_agg_windowed = db_agg_all.copy()
    db_agg_windowed["window"] = (db_agg_windowed["time_normalized"] // args.window).astype(int)
    db_agg_grouped = db_agg_windowed.groupby("window").agg({
        "compact_estimated_debt_mb": "mean",
        "l0_num_files": "mean",
        "compact_num_in_progress": "mean",
    }).reset_index()

    merged = updates_agg.merge(db_agg_grouped, on="window", how="inner")
    # Ensure window_start exists (use window * window_size)
    if "window_start" not in merged.columns:
        merged["window_start"] = merged["window"] * args.window

    if len(merged) > 0:
        scatter = ax_corr.scatter(
            merged["compact_estimated_debt_mb"],
            merged["latency_mean"],
            c=merged["window_start"],
            cmap="viridis",
            alpha=0.7,
            s=50,
        )
        cbar = plt.colorbar(scatter, ax=ax_corr)
        cbar.set_label("Time (seconds)")
        ax_corr.set_xlabel("Compaction Debt (MB)")
        ax_corr.set_ylabel("Avg Latency (ms)")
        ax_corr.set_title("Correlation: Compaction Debt vs Latency")
        ax_corr.grid(True, alpha=0.3)

        # Calculate and display correlation coefficient
        if len(merged) > 2:
            corr = merged["compact_estimated_debt_mb"].corr(merged["latency_mean"])
            ax_corr.text(
                0.02, 0.98, f"Correlation: {corr:.3f}",
                transform=ax_corr.transAxes,
                fontsize=12,
                verticalalignment="top",
                bbox=dict(boxstyle="round", facecolor="wheat", alpha=0.8),
            )
    else:
        ax_corr.text(0.5, 0.5, "No merged data available", ha="center", va="center",
                     transform=ax_corr.transAxes)

    corr_path = output_dir / f"db_correlation_{timestamp}.{args.format}"
    fig_corr.savefig(corr_path, dpi=150, bbox_inches="tight")
    print(f"Correlation plot saved to: {corr_path}")

    # Print summary statistics (aggregated)
    print("\n" + "=" * 70)
    print("DB Metrics Summary (All Shards Aggregated)")
    print("=" * 70)
    if len(db_agg_all) > 0:
        print(f"{'Metric':<30} {'Min':>12} {'Max':>12} {'Mean':>12}")
        print("-" * 70)
        for col, label in [
            ("compact_estimated_debt_mb", "Compaction Debt (MB)"),
            ("l0_num_files", "L0 Files"),
            ("compact_num_in_progress", "Compactions In Progress"),
            ("total_size_mb", "Total Size (MB)"),
            ("memtable_size_mb", "MemTable Size (MB)"),
            ("flush_count", "Flush Count"),
            ("compact_count", "Compaction Count"),
        ]:
            min_val = db_agg_all[col].min()
            max_val = db_agg_all[col].max()
            mean_val = db_agg_all[col].mean()
            print(f"{label:<30} {min_val:>12.2f} {max_val:>12.2f} {mean_val:>12.2f}")
    print("=" * 70)

    # Per-shard visualization
    if has_shards and len(db_trimmed) > 0:
        print("\nGenerating per-shard plots...")
        
        # Color palette for shards
        colors = [
            "#e74c3c", "#3498db", "#2ecc71", "#9b59b6", "#f39c12",
            "#1abc9c", "#e67e22", "#34495e", "#16a085", "#c0392b",
            "#2980b9", "#27ae60", "#8e44ad", "#d35400", "#7f8c8d",
            "#f1c40f", "#00bcd4", "#ff5722", "#795548", "#607d8b",
            "#e91e63", "#673ab7", "#03a9f4", "#4caf50", "#ff9800",
            "#9c27b0", "#00bcd4", "#cddc39", "#ffc107", "#8bc34a",
            "#3f51b5", "#009688",
        ]

        # Per-shard compaction debt overlay
        fig_shard_debt, ax_shard_debt = plt.subplots(figsize=(14, 6))
        for i, shard_id in enumerate(shard_ids):
            shard_df = db_trimmed[db_trimmed["shard_id"] == shard_id]
            if len(shard_df) > 0:
                ax_shard_debt.plot(
                    shard_df["time_normalized"],
                    shard_df["compact_estimated_debt_mb"],
                    color=colors[i % len(colors)],
                    linewidth=1,
                    alpha=0.7,
                    label=f"Shard {shard_id}",
                )
        ax_shard_debt.set_xlabel("Time (seconds)")
        ax_shard_debt.set_ylabel("Compaction Debt (MB)")
        ax_shard_debt.set_title("Per-Shard Compaction Debt Over Time")
        ax_shard_debt.grid(True, alpha=0.3)
        if num_shards <= 16:
            ax_shard_debt.legend(loc="upper right", fontsize=8, ncol=4)
        
        shard_debt_path = output_dir / f"db_shard_compaction_debt_{timestamp}.{args.format}"
        fig_shard_debt.savefig(shard_debt_path, dpi=150, bbox_inches="tight")
        print(f"Per-shard compaction debt plot saved to: {shard_debt_path}")

        # Per-shard L0 files overlay
        fig_shard_l0, ax_shard_l0 = plt.subplots(figsize=(14, 6))
        for i, shard_id in enumerate(shard_ids):
            shard_df = db_trimmed[db_trimmed["shard_id"] == shard_id]
            if len(shard_df) > 0:
                ax_shard_l0.plot(
                    shard_df["time_normalized"],
                    shard_df["l0_num_files"],
                    color=colors[i % len(colors)],
                    linewidth=1,
                    alpha=0.7,
                    label=f"Shard {shard_id}",
                )
        ax_shard_l0.set_xlabel("Time (seconds)")
        ax_shard_l0.set_ylabel("L0 Files Count")
        ax_shard_l0.set_title("Per-Shard L0 Files Over Time")
        ax_shard_l0.grid(True, alpha=0.3)
        if num_shards <= 16:
            ax_shard_l0.legend(loc="upper right", fontsize=8, ncol=4)

        shard_l0_path = output_dir / f"db_shard_l0_files_{timestamp}.{args.format}"
        fig_shard_l0.savefig(shard_l0_path, dpi=150, bbox_inches="tight")
        print(f"Per-shard L0 files plot saved to: {shard_l0_path}")

        # Per-shard summary table
        print("\n" + "=" * 100)
        print("Per-Shard DB Metrics Summary (Final Values)")
        print("=" * 100)
        print(f"{'Shard':<8} {'Debt(MB)':>12} {'L0 Files':>10} {'Compactions':>12} {'Size(MB)':>12} {'Flushes':>10} {'Compacts':>10}")
        print("-" * 100)
        
        for shard_id in shard_ids:
            shard_df = db_trimmed[db_trimmed["shard_id"] == shard_id]
            if len(shard_df) > 0:
                last_row = shard_df.iloc[-1]
                print(f"{shard_id:<8} {last_row['compact_estimated_debt_mb']:>12.2f} {last_row['l0_num_files']:>10.0f} "
                      f"{last_row['compact_num_in_progress']:>12.0f} {last_row['total_size_mb']:>12.2f} "
                      f"{last_row['flush_count']:>10.0f} {last_row['compact_count']:>10.0f}")
        print("=" * 100)

    plt.show()


if __name__ == "__main__":
    main()

