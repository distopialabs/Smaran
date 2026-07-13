#!/usr/bin/env python3
"""
plot_bench_latest.py — Quick benchmark comparison from the latest ingestion CSVs.

Automatically finds the newest ingestion CSV for each requested protocol
from /data/local/benchmark_output/<protocol>/, filtered by user count.
Produces bar-chart comparisons (one point per protocol).
"""

import argparse
import glob
import os
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

# ---------------------------------------------------------------------------
# Protocol style constants (same as plot_bench.py)
# ---------------------------------------------------------------------------

PROTOCOL_STYLE = {
    "samurai":    {"color": "#2166ac", "label": "Samurai"},
    "samuraimpt": {"color": "#4393c3", "label": "Samurai+MPT"},
    "merkle":     {"color": "#b2182b", "label": "Merkle"},
    "verkle":     {"color": "#1b7837", "label": "Verkle"},
}
_AUTO_COLORS = ["#7b3294", "#e66101", "#4dac26", "#d01c8b", "#f1b6da"]
_auto_color_idx = 0

BENCH_ROOT = "/data/local/benchmark_output"


def _protocol_style(name: str) -> dict:
    global _auto_color_idx
    if name in PROTOCOL_STYLE:
        return PROTOCOL_STYLE[name]
    color = _AUTO_COLORS[_auto_color_idx % len(_AUTO_COLORS)]
    _auto_color_idx += 1
    return {"color": color, "label": name.capitalize()}


# ---------------------------------------------------------------------------
# Matplotlib style
# ---------------------------------------------------------------------------

def apply_paper_style():
    plt.rcParams.update({
        "font.family":        "serif",
        "font.size":          9,
        "axes.titlesize":     9,
        "axes.labelsize":     9,
        "xtick.labelsize":    8,
        "ytick.labelsize":    8,
        "legend.fontsize":    8,
        "axes.spines.top":    False,
        "axes.spines.right":  False,
        "axes.grid":          False,
        "figure.dpi":         150,
    })


# ---------------------------------------------------------------------------
# Figure helpers
# ---------------------------------------------------------------------------

def save_figure(fig, output_dir: str, name: str, fmt: str, dpi: int):
    os.makedirs(output_dir, exist_ok=True)
    path = os.path.join(output_dir, f"{name}.{fmt}")
    fig.savefig(path, format=fmt, dpi=dpi, bbox_inches="tight")
    print(f"  saved {path}")
    plt.close(fig)


# ---------------------------------------------------------------------------
# CSV discovery
# ---------------------------------------------------------------------------

def find_newest_ingestion_csv(protocol: str, user_count: int) -> str:
    """Find the newest ingestion CSV for a protocol/user-count pair."""
    proto_dir = os.path.join(BENCH_ROOT, protocol)
    if not os.path.isdir(proto_dir):
        sys.exit(f"ERROR: protocol directory not found: {proto_dir}")

    pattern = os.path.join(proto_dir, f"ingestion_{user_count}_*.csv")
    matches = sorted(glob.glob(pattern), key=os.path.getmtime, reverse=True)

    if not matches:
        sys.exit(
            f"ERROR: no ingestion CSV found for protocol={protocol}, "
            f"users={user_count} (pattern: {pattern})"
        )

    chosen = matches[0]
    print(f"  {protocol}: using {chosen}")
    return chosen


# ---------------------------------------------------------------------------
# Ingestion data utilities (same as plot_bench.py)
# ---------------------------------------------------------------------------

INGESTION_REQUIRED_COLS = {
    "block_num", "num_selected_updates",
    "queued_at_ns", "start_at_ns", "completed_at_ns",
}


def load_ingestion_csv(path: str) -> pd.DataFrame:
    df = pd.read_csv(path)
    df.columns = [c.strip() for c in df.columns]
    missing = INGESTION_REQUIRED_COLS - set(df.columns)
    if missing:
        sys.exit(f"ERROR: {path} is missing columns: {missing}")
    return df


def trim_df(df: pd.DataFrame, warmup: float, cooldown: float) -> pd.DataFrame:
    rel = (df["queued_at_ns"] - df["queued_at_ns"].min()) / 1e9
    df = df.copy()
    df["rel_time"] = rel
    max_t = rel.max()
    mask = (rel >= warmup) & (rel <= (max_t - cooldown))
    return df[mask].reset_index(drop=True)


def add_per_update_latency_cols(df: pd.DataFrame) -> pd.DataFrame:
    df = df.copy()
    df["block_lat_ms"]      = (df["completed_at_ns"] - df["start_at_ns"])  / 1e6
    df["e2e_block_lat_ms"]  = (df["completed_at_ns"] - df["queued_at_ns"]) / 1e6
    nu = df["num_selected_updates"].replace(0, np.nan)
    df["update_lat_ms"]     = df["block_lat_ms"]     / nu
    df["e2e_update_lat_ms"] = df["e2e_block_lat_ms"] / nu
    return df


def compute_scalars(path: str, warmup: float, cooldown: float) -> dict:
    df = load_ingestion_csv(path)
    df = trim_df(df, warmup, cooldown)
    df = add_per_update_latency_cols(df)
    valid = df[df["num_selected_updates"] > 0]

    avg_update_lat     = valid["update_lat_ms"].mean()
    avg_e2e_update_lat = valid["e2e_update_lat_ms"].mean()

    total_updates = df["num_selected_updates"].sum()
    wall_time_s   = (df["completed_at_ns"].max() - df["queued_at_ns"].min()) / 1e9
    avg_throughput = total_updates / wall_time_s if wall_time_s > 0 else 0.0

    return {
        "avg_update_lat":     avg_update_lat,
        "avg_e2e_update_lat": avg_e2e_update_lat,
        "avg_throughput":     avg_throughput,
    }


# ---------------------------------------------------------------------------
# Plotting
# ---------------------------------------------------------------------------

GRAPHS = [
    ("avg_update_lat",     "Avg Update Latency (ms)",     "latest_update_latency",     "Avg Update Latency"),
    ("avg_e2e_update_lat", "Avg E2E Update Latency (ms)", "latest_e2e_update_latency", "Avg E2E Update Latency"),
    ("avg_throughput",     "Throughput (updates/s)",       "latest_throughput",         "Avg Update Throughput"),
]


def plot_bar_charts(proto_scalars: list, user_count: int, args):
    """proto_scalars: [(protocol_name, scalars_dict), ...]"""
    for col, ylabel, fname, title in GRAPHS:
        fig, ax = plt.subplots(1, 1, figsize=(3.5, 2.6))

        labels = []
        values = []
        colors = []
        for proto, scalars in proto_scalars:
            sty = _protocol_style(proto)
            labels.append(sty["label"])
            values.append(scalars[col])
            colors.append(sty["color"])

        x = np.arange(len(labels))
        bars = ax.bar(x, values, color=colors, width=0.5, edgecolor="white",
                       linewidth=0.6)

        for bar, val in zip(bars, values):
            ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height(),
                    f"{val:.2f}", ha="center", va="bottom", fontsize=7)

        ax.set_xticks(x)
        ax.set_xticklabels(labels)
        ax.set_ylabel(ylabel)
        ax.set_title(f"{title} ({user_count} users)", pad=4)

        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    apply_paper_style()

    parser = argparse.ArgumentParser(
        description="Compare latest ingestion benchmarks across protocols.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--protocol", action="append", required=True,
                        metavar="NAME",
                        help="Protocol name (repeatable). Must match a "
                             "subdirectory under /data/local/benchmark_output/.")
    parser.add_argument("--user", type=int, required=True,
                        help="Number of users (filters CSVs by filename).")
    parser.add_argument("--output-dir", default="benchmark_output/plots",
                        help="Directory for output files (default: benchmark_output/plots)")
    parser.add_argument("--format", choices=["pdf", "png"], default="pdf",
                        help="Output format (default: pdf)")
    parser.add_argument("--dpi", type=int, default=300,
                        help="DPI for PNG output (default: 300)")
    parser.add_argument("--warmup", type=float, default=0.0,
                        help="Seconds to trim from start of data (default: 0)")
    parser.add_argument("--cooldown", type=float, default=0.0,
                        help="Seconds to trim from end of data (default: 0)")

    args = parser.parse_args()

    print(f"Finding newest ingestion CSVs for {args.user} users:")
    proto_scalars = []
    for proto in args.protocol:
        csv_path = find_newest_ingestion_csv(proto, args.user)
        scalars = compute_scalars(csv_path, args.warmup, args.cooldown)
        proto_scalars.append((proto, scalars))

    # Print summary table
    print(f"\n{'Protocol':<15} {'Avg Update Lat (ms)':>20} {'Avg E2E Lat (ms)':>20} {'Throughput (upd/s)':>20}")
    print("-" * 77)
    for proto, scalars in proto_scalars:
        sty = _protocol_style(proto)
        print(f"{sty['label']:<15} {scalars['avg_update_lat']:>20.4f} {scalars['avg_e2e_update_lat']:>20.4f} {scalars['avg_throughput']:>20.2f}")
    print()

    print(f"Generating comparison charts:")
    plot_bar_charts(proto_scalars, args.user, args)


if __name__ == "__main__":
    main()
