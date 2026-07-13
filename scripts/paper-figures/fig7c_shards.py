#!/usr/bin/env python3
"""
fig7c_shards.py — Paper Figure 7c: impact of sharding on ingestion throughput.

Output: fig7c_shards_throughput.pdf — grouped bar chart, one group per user
count, one bar per shard count, for a single protocol (paper used samurai).

Input layout (--shards-root, default logs/shards/microbench_shards_output):
    shardN/<protocol>/ingestion_<users>_<timestamp>.csv

Copy of scripts/benchmark/plot_shards_throughput.py with paper-figure output
naming. Requires matplotlib/pandas/numpy + a LaTeX installation.
"""

import argparse
import glob
import os
import re
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker
import numpy as np
import pandas as pd

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

SHARDS_ROOT = "logs/shards/microbench_shards_output"

# Okabe-Ito colorblind-safe palette
_SHARD_COLORS = [
    "#0072B2",  # blue
    "#D55E00",  # vermillion
    "#009E73",  # teal
    "#CC79A7",  # pink
    "#E69F00",  # orange
    "#56B4E9",  # sky blue
    "#F0E442",  # yellow
    "#000000",  # black
]

_SHARD_HATCHES = ["/", "\\", "x", "+", "-", "|", ".", "o"]

# ---------------------------------------------------------------------------
# Matplotlib style (mirrors plot_bench_all_users.py)
# ---------------------------------------------------------------------------

def apply_paper_style():
    plt.rcParams.update({
        "text.usetex":         True,
        "text.latex.preamble": r"\usepackage{amsmath}\usepackage{times}",
        "font.family":         "serif",
        "font.size":           70,
        "axes.titlesize":      70,
        "axes.labelsize":      70,
        "xtick.labelsize":     65,
        "ytick.labelsize":     70,
        "legend.fontsize":     70,
        "axes.spines.top":     False,
        "axes.spines.right":   False,
        "axes.grid":           False,
        "figure.dpi":          150,
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
# Shard discovery
# ---------------------------------------------------------------------------

def discover_shards(root: str) -> list[tuple[int, str]]:
    """Return sorted list of (shard_number, shard_dir_path) for all shardN dirs."""
    shards = []
    if not os.path.isdir(root):
        sys.exit(f"ERROR: shards root not found: {root}")
    for entry in os.listdir(root):
        m = re.fullmatch(r"shard(\d+)", entry)
        if m:
            n = int(m.group(1))
            shards.append((n, os.path.join(root, entry)))
    shards.sort(key=lambda x: x[0])
    return shards


def discover_user_counts(shards: list[tuple[int, str]], protocol: str, skip_users: set) -> list[int]:
    """Find all user counts available across any shard for the given protocol."""
    user_counts: set[int] = set()
    for _, shard_dir in shards:
        proto_dir = os.path.join(shard_dir, protocol)
        if not os.path.isdir(proto_dir):
            continue
        for path in glob.glob(os.path.join(proto_dir, "ingestion_*.csv")):
            basename = os.path.basename(path)
            rest = basename[len("ingestion_"):]
            digits = ""
            for ch in rest:
                if ch.isdigit():
                    digits += ch
                else:
                    break
            if digits:
                uc = int(digits)
                if uc not in skip_users:
                    user_counts.add(uc)
    return sorted(user_counts)


def find_newest_ingestion_csv(shard_dir: str, protocol: str, user_count: int) -> str | None:
    """Find the newest ingestion CSV for a shard/protocol/user-count triple."""
    proto_dir = os.path.join(shard_dir, protocol)
    if not os.path.isdir(proto_dir):
        return None
    pattern = os.path.join(proto_dir, f"ingestion_{user_count}_*.csv")
    matches = sorted(glob.glob(pattern), key=os.path.getmtime, reverse=True)
    if not matches:
        return None
    return matches[0]


# ---------------------------------------------------------------------------
# CSV parsing (mirrors plot_bench_all_users.py exactly)
# ---------------------------------------------------------------------------

INGESTION_REQUIRED_COLS = {
    "block_num", "num_selected_updates",
    "queued_at_ns", "start_at_ns", "completed_at_ns",
}

CAUCHY_EXTRA_COLS = {"Tracked_Accounts"}


def load_ingestion_csv(path: str, protocol: str = "") -> pd.DataFrame:
    df = pd.read_csv(path)
    df.columns = [c.strip() for c in df.columns]
    if protocol == "cauchy":
        df = df.drop(columns=[c for c in CAUCHY_EXTRA_COLS if c in df.columns])
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
    df["block_lat_ms"]     = (df["completed_at_ns"] - df["start_at_ns"])  / 1e6
    df["e2e_block_lat_ms"] = (df["completed_at_ns"] - df["queued_at_ns"]) / 1e6
    nu = df["num_selected_updates"].replace(0, np.nan)
    df["update_lat_ms"]     = df["block_lat_ms"]     / nu
    df["e2e_update_lat_ms"] = df["e2e_block_lat_ms"] / nu
    return df


def compute_scalars(path: str, warmup: float, cooldown: float, protocol: str = "") -> dict:
    df = load_ingestion_csv(path, protocol)
    df = trim_df(df, warmup, cooldown)
    df = add_per_update_latency_cols(df)

    total_updates = df["num_selected_updates"].sum()
    wall_time_s   = (df["completed_at_ns"].max() - df["queued_at_ns"].min()) / 1e9
    avg_throughput = total_updates / wall_time_s if wall_time_s > 0 else 0.0

    return {"avg_throughput": avg_throughput}


# ---------------------------------------------------------------------------
# X-axis formatter (mirrors plot_bench_all_users.py)
# ---------------------------------------------------------------------------

def _user_count_formatter(x, pos):
    x = int(x)
    if x >= 1_000_000:
        m = x / 1_000_000
        return f"{m:g}M"
    if x >= 100_000:
        m = x / 1_000_000
        return f"0.{int(m * 10)}M"
    return str(x)


def _y_formatter_kilo(x, pos):
    if x >= 1_000:
        return f"{int(x / 1000)}k"
    if x == 0:
        return "0"
    return f"{x:.10f}".rstrip("0").rstrip(".")


# ---------------------------------------------------------------------------
# Plotting
# ---------------------------------------------------------------------------

def plot_bar_chart(
    data: dict,           # data[user_count][shard_n] = avg_throughput (or per-user)
    shards: list[tuple[int, str]],
    user_counts: list[int],
    protocol: str,
    args,
):
    """Grouped bar chart: x = user count groups, bars = shards."""

    shard_nums = [n for n, _ in shards]
    n_shards   = len(shard_nums)
    n_groups   = len(user_counts)

    bar_width  = 0.8 / n_shards
    x_positions = np.arange(n_groups)

    fig, ax = plt.subplots(figsize=(max(30, 5 * n_groups), 12))

    for i, (shard_n, _) in enumerate(shards):
        color   = _SHARD_COLORS[i % len(_SHARD_COLORS)]
        hatch   = _SHARD_HATCHES[i % len(_SHARD_HATCHES)]
        offset  = (i - (n_shards - 1) / 2) * bar_width
        values  = [data[uc].get(shard_n, None) for uc in user_counts]

        xs = [x_positions[j] + offset for j, v in enumerate(values) if v is not None]
        ys = [v for v in values if v is not None]

        if xs:
            ax.bar(
                xs, ys,
                width=bar_width * 0.9,
                color=color,
                hatch=hatch,
                label=str(shard_n),
                edgecolor="black",
                linewidth=2,
            )

    ax.set_xticks(x_positions)
    ax.set_yticks([0, 20_000, 40_000, 60_000, 80_000])
    ax.set_xticklabels([_user_count_formatter(uc, None) for uc in user_counts])
    ax.set_xlabel("Number of users")
    ax.set_ylabel("Throughput (ops/s)", labelpad=20, loc="center")
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter_kilo))
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.spines["left"].set_linewidth(5)
    ax.spines["bottom"].set_linewidth(5)
    ax.tick_params(axis="both", which="major", width=3, length=8)
    ax.grid(linestyle="--", linewidth=3, alpha=0.7)
    ax.set_axisbelow(True)

    legend = ax.legend(
        loc="lower center",
        bbox_to_anchor=(0.5, 0.96),
        frameon=True,
        edgecolor="black",
        fontsize=plt.rcParams["legend.fontsize"] * 0.75,
        ncol=n_shards,
        columnspacing=0.5,
        title="Number of shards",
        title_fontsize=plt.rcParams["legend.fontsize"] * 0.75,
    )
    legend.get_title().set_fontweight("bold")

    fname = "fig7c_shards_throughput"
    if args.per_user:
        fname += "_per_user"
    save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    apply_paper_style()

    parser = argparse.ArgumentParser(
        description="Bar chart of throughput per shard across user counts for one protocol.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--protocol", default="samurai",
                        help="Protocol name (must match a subdirectory inside each shardN; "
                             "default: samurai, as used in the paper).")
    parser.add_argument("--per-user", action="store_true",
                        help="Divide throughput by number of users (throughput/user).")
    parser.add_argument("--skip-users", action="append", type=int, default=None,
                        metavar="N",
                        help="User count to exclude (repeatable; default: 0).")
    parser.add_argument("--shards-root", default=SHARDS_ROOT,
                        help=f"Root directory containing shardN dirs (default: {SHARDS_ROOT})")
    parser.add_argument("--output-dir", default="benchmark_output/plots",
                        help="Directory for output files (default: benchmark_output/plots)")
    parser.add_argument("--format", choices=["pdf", "png"], default="pdf",
                        help="Output format (default: pdf)")
    parser.add_argument("--dpi", type=int, default=300,
                        help="DPI for PNG output (default: 300)")
    parser.add_argument("--warmup", type=float, default=0.0,
                        help="Seconds to trim from start (default: 0)")
    parser.add_argument("--cooldown", type=float, default=0.0,
                        help="Seconds to trim from end (default: 0)")

    args = parser.parse_args()

    protocol   = args.protocol
    skip_users = set(args.skip_users) if args.skip_users is not None else {0}

    shards = discover_shards(args.shards_root)
    if not shards:
        sys.exit(f"ERROR: no shardN directories found under {args.shards_root}")
    print(f"Found shards: {[f'shard{n}' for n, _ in shards]}")

    user_counts = discover_user_counts(shards, protocol, skip_users)
    if not user_counts:
        sys.exit(f"ERROR: no user counts discovered for protocol={protocol}")
    # Drop user_count=0 when --per-user is requested (avoid division by zero)
    if args.per_user:
        user_counts = [uc for uc in user_counts if uc > 0]
    print(f"User counts: {user_counts}")

    # data[user_count][shard_n] = throughput value
    data: dict[int, dict[int, float]] = {uc: {} for uc in user_counts}

    print(f"\nCollecting data for protocol={protocol}:")
    for shard_n, shard_dir in shards:
        for uc in user_counts:
            csv_path = find_newest_ingestion_csv(shard_dir, protocol, uc)
            if csv_path is None:
                print(f"  WARNING: shard{shard_n}, users={uc} — no CSV found, skipping")
                continue
            scalars = compute_scalars(csv_path, args.warmup, args.cooldown, protocol)
            throughput = scalars["avg_throughput"]
            if args.per_user:
                throughput = throughput / uc
            data[uc][shard_n] = throughput
            print(f"  shard{shard_n:>5}, users={uc:>8}: throughput={scalars['avg_throughput']:.2f} upd/s"
                  + (f"  ({throughput:.4f} upd/s/user)" if args.per_user else ""))

    # Print summary table
    col_w = max(len(f"shard{n}") for n, _ in shards) + 2
    header = f"{'Users':>10}  " + "  ".join(f"shard{n:>{col_w-5}}" for n, _ in shards)
    print(f"\n{header}")
    print("-" * len(header))
    for uc in user_counts:
        row = f"{uc:>10}  "
        for shard_n, _ in shards:
            val = data[uc].get(shard_n)
            cell = f"{val:.4f}" if val is not None else "N/A"
            row += f"{cell:>{col_w}}  "
        print(row)

    print("\nGenerating bar chart:")
    plot_bar_chart(data, shards, user_counts, protocol, args)


if __name__ == "__main__":
    main()
