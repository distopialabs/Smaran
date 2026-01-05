#!/usr/bin/env python3
"""
Benchmark Visualization Script for Samurai Commit Generation

This script plots latency and throughput graphs from benchmark CSV files.
It supports configurable time windows and warmup/cooldown trimming.

Usage:
    python plot_bench.py --updates bench_updates_*.csv --blocks bench_blocks_*.csv \
        --window 5 --warmup 30 --cooldown 30 --output ./plots/

Dependencies:
    pip install pandas matplotlib
"""

import argparse
import os
from pathlib import Path

import matplotlib.pyplot as plt
import pandas as pd


def load_updates_csv(filepath: str) -> pd.DataFrame:
    """Load updates CSV file."""
    df = pd.read_csv(filepath)
    # Convert nanoseconds to seconds for time-based grouping
    df["completed_at_s"] = df["completed_at_ns"] / 1e9
    # Convert latency to milliseconds for readability
    df["latency_ms"] = df["latency_ns"] / 1e6
    return df


def load_blocks_csv(filepath: str) -> pd.DataFrame:
    """Load blocks CSV file."""
    df = pd.read_csv(filepath)
    # Convert nanoseconds to seconds
    df["submitted_at_s"] = df["submitted_at_ns"] / 1e9
    df["completed_at_s"] = df["completed_at_ns"] / 1e9
    # Calculate latency in milliseconds
    df["latency_ms"] = (df["completed_at_ns"] - df["submitted_at_ns"]) / 1e6
    return df


def trim_warmup_cooldown(
    df: pd.DataFrame, time_col: str, warmup_s: float, cooldown_s: float
) -> pd.DataFrame:
    """Trim warmup and cooldown periods from the data."""
    if df.empty:
        return df

    min_time = df[time_col].min()
    max_time = df[time_col].max()

    start_time = min_time + warmup_s
    end_time = max_time - cooldown_s

    if start_time >= end_time:
        print(
            f"Warning: warmup ({warmup_s}s) + cooldown ({cooldown_s}s) >= total duration. No data left."
        )
        return df.iloc[0:0]  # Return empty dataframe with same columns

    trimmed = df[(df[time_col] >= start_time) & (df[time_col] <= end_time)].copy()
    # Normalize time to start from 0
    trimmed["time_normalized"] = trimmed[time_col] - start_time
    return trimmed


def aggregate_by_window(
    df: pd.DataFrame, time_col: str, window_s: float, value_col: str, agg_func: str
) -> pd.DataFrame:
    """Aggregate data by time window."""
    if df.empty:
        return pd.DataFrame(columns=["window_start", "value"])

    # Create window bins
    df = df.copy()
    df["window"] = (df[time_col] // window_s).astype(int)

    if agg_func == "mean":
        result = df.groupby("window")[value_col].mean().reset_index()
    elif agg_func == "count":
        result = df.groupby("window").size().reset_index(name="value")
        result["value"] = result["value"] / window_s  # Convert to per-second rate
    else:
        raise ValueError(f"Unknown aggregation function: {agg_func}")

    result["window_start"] = result["window"] * window_s
    if agg_func == "mean":
        result = result.rename(columns={value_col: "value"})

    return result[["window_start", "value"]]


def plot_latency(
    ax: plt.Axes,
    df: pd.DataFrame,
    title: str,
    ylabel: str,
    color: str,
    window_s: float,
):
    """Plot latency over time."""
    if df.empty:
        ax.text(0.5, 0.5, "No data", ha="center", va="center", transform=ax.transAxes)
        ax.set_title(title)
        return

    agg = aggregate_by_window(df, "time_normalized", window_s, "latency_ms", "mean")

    ax.plot(agg["window_start"], agg["value"], color=color, linewidth=1.5)
    ax.fill_between(agg["window_start"], agg["value"], alpha=0.3, color=color)
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel(ylabel)
    ax.set_title(title)
    ax.grid(True, alpha=0.3)

    # Add statistics annotation
    if not df.empty:
        mean_lat = df["latency_ms"].mean()
        p50 = df["latency_ms"].median()
        p95 = df["latency_ms"].quantile(0.95)
        p99 = df["latency_ms"].quantile(0.99)
        stats_text = f"Mean: {mean_lat:.2f}ms\nP50: {p50:.2f}ms\nP95: {p95:.2f}ms\nP99: {p99:.2f}ms"
        ax.text(
            0.98,
            0.98,
            stats_text,
            transform=ax.transAxes,
            fontsize=9,
            verticalalignment="top",
            horizontalalignment="right",
            bbox=dict(boxstyle="round", facecolor="wheat", alpha=0.8),
        )


def plot_throughput(
    ax: plt.Axes,
    df: pd.DataFrame,
    time_col: str,
    title: str,
    ylabel: str,
    color: str,
    window_s: float,
):
    """Plot throughput over time using line chart with fill (matching latency style)."""
    if df.empty:
        ax.text(0.5, 0.5, "No data", ha="center", va="center", transform=ax.transAxes)
        ax.set_title(title)
        return

    agg = aggregate_by_window(df, "time_normalized", window_s, None, "count")

    ax.plot(agg["window_start"], agg["value"], color=color, linewidth=1.5)
    ax.fill_between(agg["window_start"], agg["value"], alpha=0.3, color=color)
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel(ylabel)
    ax.set_title(title)
    ax.grid(True, alpha=0.3)

    # Add statistics annotation
    if not agg.empty:
        avg_throughput = agg["value"].mean()
        max_throughput = agg["value"].max()
        min_throughput = agg["value"].min()
        stats_text = (
            f"Avg: {avg_throughput:.1f}/s\nMax: {max_throughput:.1f}/s\nMin: {min_throughput:.1f}/s"
        )
        ax.text(
            0.98,
            0.98,
            stats_text,
            transform=ax.transAxes,
            fontsize=9,
            verticalalignment="top",
            horizontalalignment="right",
            bbox=dict(boxstyle="round", facecolor="wheat", alpha=0.8),
        )


def main():
    parser = argparse.ArgumentParser(
        description="Plot benchmark results from Samurai commit generation"
    )
    parser.add_argument(
        "--updates", required=True, help="Path to updates CSV file (bench_updates_*.csv)"
    )
    parser.add_argument(
        "--blocks", required=True, help="Path to blocks CSV file (bench_blocks_*.csv)"
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

    # Create output directory if needed
    output_dir = Path(args.output)
    output_dir.mkdir(parents=True, exist_ok=True)

    print(f"Loading data...")
    print(f"  Updates: {args.updates}")
    print(f"  Blocks:  {args.blocks}")

    # Load data
    updates_df = load_updates_csv(args.updates)
    blocks_df = load_blocks_csv(args.blocks)

    print(f"Loaded {len(updates_df)} update records and {len(blocks_df)} block records")

    # Trim warmup and cooldown
    print(f"Trimming warmup ({args.warmup}s) and cooldown ({args.cooldown}s)...")
    updates_trimmed = trim_warmup_cooldown(
        updates_df, "completed_at_s", args.warmup, args.cooldown
    )
    blocks_trimmed = trim_warmup_cooldown(
        blocks_df, "completed_at_s", args.warmup, args.cooldown
    )

    print(
        f"After trimming: {len(updates_trimmed)} updates and {len(blocks_trimmed)} blocks"
    )

    # Create figure with 2x2 subplots
    fig, axes = plt.subplots(2, 2, figsize=(14, 10))
    fig.suptitle(
        f"Samurai Commit Generation Benchmark\n(Window: {args.window}s, Warmup: {args.warmup}s, Cooldown: {args.cooldown}s)",
        fontsize=14,
        fontweight="bold",
    )

    # Plot 1: Update Latency
    plot_latency(
        axes[0, 0],
        updates_trimmed,
        "Update Latency Over Time",
        "Avg Latency (ms)",
        "#2ecc71",
        args.window,
    )

    # Plot 2: Block Latency
    plot_latency(
        axes[0, 1],
        blocks_trimmed,
        "Block Latency Over Time",
        "Avg Latency (ms)",
        "#3498db",
        args.window,
    )

    # Plot 3: Update Throughput
    plot_throughput(
        axes[1, 0],
        updates_trimmed,
        "time_normalized",
        "Update Throughput Over Time",
        "Updates/second",
        "#e74c3c",
        args.window,
    )

    # Plot 4: Block Throughput
    plot_throughput(
        axes[1, 1],
        blocks_trimmed,
        "time_normalized",
        "Block Throughput Over Time",
        "Blocks/second",
        "#9b59b6",
        args.window,
    )

    plt.tight_layout()

    # Save figure
    timestamp = Path(args.updates).stem.replace("bench_updates_", "")
    output_path = output_dir / f"benchmark_plots_{timestamp}.{args.format}"
    plt.savefig(output_path, dpi=150, bbox_inches="tight")
    print(f"\nPlots saved to: {output_path}")

    # Also save individual plots for higher resolution
    for idx, (name, title) in enumerate(
        [
            ("update_latency", "Update Latency"),
            ("block_latency", "Block Latency"),
            ("update_throughput", "Update Throughput"),
            ("block_throughput", "Block Throughput"),
        ]
    ):
        fig_single, ax_single = plt.subplots(figsize=(10, 6))
        row, col = idx // 2, idx % 2

        if idx == 0:
            plot_latency(
                ax_single,
                updates_trimmed,
                f"{title} Over Time",
                "Avg Latency (ms)",
                "#2ecc71",
                args.window,
            )
        elif idx == 1:
            plot_latency(
                ax_single,
                blocks_trimmed,
                f"{title} Over Time",
                "Avg Latency (ms)",
                "#3498db",
                args.window,
            )
        elif idx == 2:
            plot_throughput(
                ax_single,
                updates_trimmed,
                "time_normalized",
                f"{title} Over Time",
                "Updates/second",
                "#e74c3c",
                args.window,
            )
        else:
            plot_throughput(
                ax_single,
                blocks_trimmed,
                "time_normalized",
                f"{title} Over Time",
                "Blocks/second",
                "#9b59b6",
                args.window,
            )

        single_path = output_dir / f"{name}_{timestamp}.{args.format}"
        fig_single.savefig(single_path, dpi=150, bbox_inches="tight")
        plt.close(fig_single)
        print(f"  {name}: {single_path}")

    plt.show()


if __name__ == "__main__":
    main()

