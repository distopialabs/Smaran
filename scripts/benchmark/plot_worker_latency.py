#!/usr/bin/env python3
"""
Per-Worker Latency & Throughput Visualization Script for Samurai Commit Generation

This script plots latency and throughput graphs for individual workers from benchmark CSV files.

Usage:
    python plot_worker_latency.py --updates bench_updates_*.csv \
        --window 5 --warmup 30 --cooldown 30 --output ./plots/

Dependencies:
    pip install pandas matplotlib
"""

import argparse
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
        return df.iloc[0:0]

    trimmed = df[(df[time_col] >= start_time) & (df[time_col] <= end_time)].copy()
    trimmed["time_normalized"] = trimmed[time_col] - start_time
    return trimmed


def aggregate_latency_by_window(
    df: pd.DataFrame, time_col: str, window_s: float
) -> pd.DataFrame:
    """Aggregate latency by time window."""
    if df.empty:
        return pd.DataFrame(columns=["window_start", "mean", "p50", "p95", "p99"])

    df = df.copy()
    df["window"] = (df[time_col] // window_s).astype(int)

    result = df.groupby("window")["latency_ms"].agg(
        mean="mean", p50="median", p95=lambda x: x.quantile(0.95), p99=lambda x: x.quantile(0.99)
    ).reset_index()
    result["window_start"] = result["window"] * window_s

    return result


def aggregate_throughput_by_window(
    df: pd.DataFrame, time_col: str, window_s: float
) -> pd.DataFrame:
    """Aggregate throughput (count per second) by time window."""
    if df.empty:
        return pd.DataFrame(columns=["window_start", "throughput"])

    df = df.copy()
    df["window"] = (df[time_col] // window_s).astype(int)

    result = df.groupby("window").size().reset_index(name="count")
    result["throughput"] = result["count"] / window_s  # Convert to per-second rate
    result["window_start"] = result["window"] * window_s

    return result[["window_start", "throughput"]]


def plot_worker_latency(
    ax: plt.Axes,
    df: pd.DataFrame,
    worker_id: int,
    color: str,
    window_s: float,
):
    """Plot latency over time for a single worker."""
    if df.empty:
        ax.text(0.5, 0.5, "No data", ha="center", va="center", transform=ax.transAxes)
        ax.set_title(f"Worker {worker_id}")
        return

    agg = aggregate_latency_by_window(df, "time_normalized", window_s)

    ax.plot(agg["window_start"], agg["mean"], color=color, linewidth=1.5, label="Mean")
    ax.fill_between(agg["window_start"], agg["mean"], alpha=0.3, color=color)
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Latency (ms)")
    ax.set_title(f"Worker {worker_id}")
    ax.grid(True, alpha=0.3)

    # Add statistics annotation
    mean_lat = df["latency_ms"].mean()
    p50 = df["latency_ms"].median()
    p95 = df["latency_ms"].quantile(0.95)
    p99 = df["latency_ms"].quantile(0.99)
    count = len(df)
    stats_text = f"N: {count:,}\nMean: {mean_lat:.2f}ms\nP50: {p50:.2f}ms\nP95: {p95:.2f}ms\nP99: {p99:.2f}ms"
    ax.text(
        0.98,
        0.98,
        stats_text,
        transform=ax.transAxes,
        fontsize=8,
        verticalalignment="top",
        horizontalalignment="right",
        bbox=dict(boxstyle="round", facecolor="wheat", alpha=0.8),
    )


def plot_all_workers_latency_overlay(
    ax: plt.Axes,
    df: pd.DataFrame,
    colors: list,
    window_s: float,
):
    """Plot all workers' latency on a single chart."""
    if df.empty:
        ax.text(0.5, 0.5, "No data", ha="center", va="center", transform=ax.transAxes)
        ax.set_title("All Workers Latency")
        return

    worker_ids = sorted(df["worker_id"].unique())

    for i, worker_id in enumerate(worker_ids):
        worker_df = df[df["worker_id"] == worker_id]
        agg = aggregate_latency_by_window(worker_df, "time_normalized", window_s)
        color = colors[i % len(colors)]
        ax.plot(
            agg["window_start"],
            agg["mean"],
            color=color,
            linewidth=1.2,
            label=f"Worker {worker_id}",
            alpha=0.8,
        )

    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Avg Latency (ms)")
    ax.set_title("All Workers Latency Comparison")
    ax.grid(True, alpha=0.3)
    ax.legend(loc="upper right", fontsize=8, ncol=2)


def plot_worker_throughput(
    ax: plt.Axes,
    df: pd.DataFrame,
    worker_id: int,
    color: str,
    window_s: float,
):
    """Plot throughput over time for a single worker."""
    if df.empty:
        ax.text(0.5, 0.5, "No data", ha="center", va="center", transform=ax.transAxes)
        ax.set_title(f"Worker {worker_id}")
        return

    agg = aggregate_throughput_by_window(df, "time_normalized", window_s)

    ax.plot(agg["window_start"], agg["throughput"], color=color, linewidth=1.5)
    ax.fill_between(agg["window_start"], agg["throughput"], alpha=0.3, color=color)
    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Updates/sec")
    ax.set_title(f"Worker {worker_id}")
    ax.grid(True, alpha=0.3)

    # Add statistics annotation
    avg_tp = agg["throughput"].mean()
    max_tp = agg["throughput"].max()
    min_tp = agg["throughput"].min()
    count = len(df)
    stats_text = f"N: {count:,}\nAvg: {avg_tp:.1f}/s\nMax: {max_tp:.1f}/s\nMin: {min_tp:.1f}/s"
    ax.text(
        0.98,
        0.98,
        stats_text,
        transform=ax.transAxes,
        fontsize=8,
        verticalalignment="top",
        horizontalalignment="right",
        bbox=dict(boxstyle="round", facecolor="wheat", alpha=0.8),
    )


def plot_all_workers_throughput_overlay(
    ax: plt.Axes,
    df: pd.DataFrame,
    colors: list,
    window_s: float,
):
    """Plot all workers' throughput on a single chart."""
    if df.empty:
        ax.text(0.5, 0.5, "No data", ha="center", va="center", transform=ax.transAxes)
        ax.set_title("All Workers Throughput")
        return

    worker_ids = sorted(df["worker_id"].unique())

    for i, worker_id in enumerate(worker_ids):
        worker_df = df[df["worker_id"] == worker_id]
        agg = aggregate_throughput_by_window(worker_df, "time_normalized", window_s)
        color = colors[i % len(colors)]
        ax.plot(
            agg["window_start"],
            agg["throughput"],
            color=color,
            linewidth=1.2,
            label=f"Worker {worker_id}",
            alpha=0.8,
        )

    ax.set_xlabel("Time (seconds)")
    ax.set_ylabel("Updates/sec")
    ax.set_title("All Workers Throughput Comparison")
    ax.grid(True, alpha=0.3)
    ax.legend(loc="upper right", fontsize=8, ncol=2)


def main():
    parser = argparse.ArgumentParser(
        description="Plot per-worker latency from Samurai benchmark"
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

    print(f"Loading data from: {args.updates}")
    updates_df = load_updates_csv(args.updates)
    print(f"Loaded {len(updates_df)} update records")

    # Check if worker_id column exists
    if "worker_id" not in updates_df.columns:
        print("Error: CSV does not contain 'worker_id' column. Run benchmark with updated code.")
        return

    worker_ids = sorted(updates_df["worker_id"].unique())
    num_workers = len(worker_ids)
    print(f"Found {num_workers} workers: {worker_ids}")

    # Trim warmup and cooldown
    print(f"Trimming warmup ({args.warmup}s) and cooldown ({args.cooldown}s)...")
    updates_trimmed = trim_warmup_cooldown(
        updates_df, "completed_at_s", args.warmup, args.cooldown
    )
    print(f"After trimming: {len(updates_trimmed)} updates")

    # Color palette
    colors = [
        "#e74c3c", "#3498db", "#2ecc71", "#9b59b6", "#f39c12",
        "#1abc9c", "#e67e22", "#34495e", "#16a085", "#c0392b",
        "#2980b9", "#27ae60", "#8e44ad", "#d35400", "#7f8c8d",
        "#f1c40f",
    ]

    # Determine grid layout
    cols = min(4, num_workers)
    rows = (num_workers + cols - 1) // cols

    # Create figure with subplots for each worker
    fig, axes = plt.subplots(rows, cols, figsize=(5 * cols, 4 * rows))
    fig.suptitle(
        f"Per-Worker Latency\n(Window: {args.window}s, Warmup: {args.warmup}s, Cooldown: {args.cooldown}s)",
        fontsize=14,
        fontweight="bold",
    )

    # Flatten axes for easy iteration
    if num_workers == 1:
        axes = [axes]
    else:
        axes = axes.flatten()

    for i, worker_id in enumerate(worker_ids):
        worker_df = updates_trimmed[updates_trimmed["worker_id"] == worker_id]
        plot_worker_latency(
            axes[i],
            worker_df,
            worker_id,
            colors[i % len(colors)],
            args.window,
        )

    # Hide unused subplots
    for i in range(num_workers, len(axes)):
        axes[i].set_visible(False)

    plt.tight_layout()

    timestamp = Path(args.updates).stem.replace("bench_updates_", "")
    output_path = output_dir / f"worker_latency_{timestamp}.{args.format}"
    plt.savefig(output_path, dpi=150, bbox_inches="tight")
    print(f"\nPer-worker latency plots saved to: {output_path}")

    # Create per-worker throughput plots
    fig_tp, axes_tp = plt.subplots(rows, cols, figsize=(5 * cols, 4 * rows))
    fig_tp.suptitle(
        f"Per-Worker Throughput\n(Window: {args.window}s, Warmup: {args.warmup}s, Cooldown: {args.cooldown}s)",
        fontsize=14,
        fontweight="bold",
    )

    if num_workers == 1:
        axes_tp = [axes_tp]
    else:
        axes_tp = axes_tp.flatten()

    for i, worker_id in enumerate(worker_ids):
        worker_df = updates_trimmed[updates_trimmed["worker_id"] == worker_id]
        plot_worker_throughput(
            axes_tp[i],
            worker_df,
            worker_id,
            colors[i % len(colors)],
            args.window,
        )

    for i in range(num_workers, len(axes_tp)):
        axes_tp[i].set_visible(False)

    plt.tight_layout()

    tp_output_path = output_dir / f"worker_throughput_{timestamp}.{args.format}"
    fig_tp.savefig(tp_output_path, dpi=150, bbox_inches="tight")
    print(f"Per-worker throughput plots saved to: {tp_output_path}")

    # Create latency overlay plot
    fig_overlay, ax_overlay = plt.subplots(figsize=(12, 6))
    plot_all_workers_latency_overlay(ax_overlay, updates_trimmed, colors, args.window)
    ax_overlay.set_title(
        f"All Workers Latency Comparison\n(Window: {args.window}s, Warmup: {args.warmup}s, Cooldown: {args.cooldown}s)",
        fontsize=12,
        fontweight="bold",
    )

    overlay_path = output_dir / f"worker_latency_overlay_{timestamp}.{args.format}"
    fig_overlay.savefig(overlay_path, dpi=150, bbox_inches="tight")
    print(f"Latency overlay plot saved to: {overlay_path}")

    # Create throughput overlay plot
    fig_tp_overlay, ax_tp_overlay = plt.subplots(figsize=(12, 6))
    plot_all_workers_throughput_overlay(ax_tp_overlay, updates_trimmed, colors, args.window)
    ax_tp_overlay.set_title(
        f"All Workers Throughput Comparison\n(Window: {args.window}s, Warmup: {args.warmup}s, Cooldown: {args.cooldown}s)",
        fontsize=12,
        fontweight="bold",
    )

    tp_overlay_path = output_dir / f"worker_throughput_overlay_{timestamp}.{args.format}"
    fig_tp_overlay.savefig(tp_overlay_path, dpi=150, bbox_inches="tight")
    print(f"Throughput overlay plot saved to: {tp_overlay_path}")

    # Print summary statistics per worker
    total_duration = updates_trimmed["time_normalized"].max() if len(updates_trimmed) > 0 else 1

    print("\n" + "=" * 90)
    print("Per-Worker Summary Statistics")
    print("=" * 90)
    print(f"{'Worker':<8} {'Count':>10} {'Avg Throughput':>16} {'Mean(ms)':>12} {'P50(ms)':>10} {'P95(ms)':>10} {'P99(ms)':>10}")
    print("-" * 90)
    for worker_id in worker_ids:
        worker_df = updates_trimmed[updates_trimmed["worker_id"] == worker_id]
        if len(worker_df) > 0:
            count = len(worker_df)
            throughput = count / total_duration if total_duration > 0 else 0
            mean = worker_df["latency_ms"].mean()
            p50 = worker_df["latency_ms"].median()
            p95 = worker_df["latency_ms"].quantile(0.95)
            p99 = worker_df["latency_ms"].quantile(0.99)
            print(f"{worker_id:<8} {count:>10,} {throughput:>14.1f}/s {mean:>12.2f} {p50:>10.2f} {p95:>10.2f} {p99:>10.2f}")
    print("=" * 90)

    plt.show()


if __name__ == "__main__":
    main()


