#!/usr/bin/env python3
"""
fig7a_ingestion_throughput.py — Paper Figure 7a: commitment generation (ingestion)
throughput across protocols and user counts.

Output: fig7a_ingestion_throughput.pdf (+ fig7a_summary.csv with the plotted numbers).

Input layout (--input-root, default logs/ingestion_logs):
    <input-root>/<protocol>/ingestion_<users>[_<timestamp>].csv

Protocols are passed with repeated --protocol flags; paper used:
    --protocol samurai --protocol merkle --protocol verkle --protocol cauchy

Cauchy is optional. It only has data for small user counts (16-1024); when both
small (<100k) and large (>=100k) user counts are present the chart automatically
switches to a broken-axis layout (x-break, plus y-break when values drop below
10k ops/s). Without cauchy a regular single-panel chart is produced.

User counts are auto-discovered from the filenames. ingestion_0.csv ("all users"
full runs) and ingestion_1.csv are skipped by default (not in the paper); override
with repeated --skip-users flags.

Trimmed from scripts/benchmark/plot_bench_all_users.py, keeping only the throughput
chart used in the paper. Requires matplotlib/pandas/numpy + a LaTeX installation.
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
# Protocol style constants (Okabe-Ito colorblind-safe palette)
# ---------------------------------------------------------------------------

PROTOCOL_STYLE = {
    "samurai":    {"color": "#0072B2", "label": "Smaran",      "marker": "o"},
    "samuraimpt": {"color": "#CC79A7", "label": "Smaran+MPT",  "marker": "s"},
    "merkle":     {"color": "#D55E00", "label": "MPT",         "marker": "^"},
    "verkle":     {"color": "#009E73", "label": "Verkle",      "marker": "D"},
    "cauchy":     {"color": "#E69F00", "label": "Cauchy",      "marker": "P"},
}
_AUTO_COLORS  = ["#E69F00", "#CC79A7", "#F0E442", "#000000", "#999999"]
_AUTO_MARKERS = ["P", "X", "v", "p", "h"]
_auto_color_idx = 0


def _protocol_style(name: str) -> dict:
    global _auto_color_idx
    if name in PROTOCOL_STYLE:
        return PROTOCOL_STYLE[name]
    idx = _auto_color_idx % len(_AUTO_COLORS)
    _auto_color_idx += 1
    return {"color": _AUTO_COLORS[idx], "label": name.capitalize(), "marker": _AUTO_MARKERS[idx]}


# ---------------------------------------------------------------------------
# Matplotlib style
# ---------------------------------------------------------------------------

def apply_paper_style():
    plt.rcParams.update({
        "text.usetex":        True,
        "text.latex.preamble": r"\usepackage{amsmath}\usepackage{times}",
        "font.family":        "serif",
        "font.size":          70,
        "axes.titlesize":     70,
        "axes.labelsize":     70,
        "xtick.labelsize":    65,
        "ytick.labelsize":    70,
        "legend.fontsize":    70,
        "axes.spines.top":    False,
        "axes.spines.right":  False,
        "axes.grid":          False,
        "figure.dpi":         150,
    })


def save_figure(fig, output_dir: str, name: str, fmt: str, dpi: int):
    os.makedirs(output_dir, exist_ok=True)
    path = os.path.join(output_dir, f"{name}.{fmt}")
    fig.savefig(path, format=fmt, dpi=dpi, bbox_inches="tight")
    print(f"  saved {path}")
    plt.close(fig)


# ---------------------------------------------------------------------------
# Data discovery
# ---------------------------------------------------------------------------

def discover_user_counts(input_root: str, protocols: list, skip_users: set) -> list:
    """Discover all user counts present across all protocol directories."""
    user_counts = set()
    for protocol in protocols:
        proto_dir = os.path.join(input_root, protocol)
        if not os.path.isdir(proto_dir):
            continue
        for path in glob.glob(os.path.join(proto_dir, "ingestion_*.csv")):
            basename = os.path.basename(path)
            # basename looks like ingestion_<count><rest>.csv
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


def find_newest_ingestion_csv(input_root: str, protocol: str, user_count: int) -> str | None:
    """Find the newest ingestion CSV for a protocol/user-count pair, or None if missing.

    Accepts both plain (ingestion_<k>.csv) and timestamped (ingestion_<k>_<ts>.csv) names.
    """
    proto_dir = os.path.join(input_root, protocol)
    if not os.path.isdir(proto_dir):
        # Covered by the per-protocol zero-coverage warning after loading.
        return None

    matches = glob.glob(os.path.join(proto_dir, f"ingestion_{user_count}.csv"))
    matches += glob.glob(os.path.join(proto_dir, f"ingestion_{user_count}_*.csv"))
    matches = sorted(matches, key=os.path.getmtime, reverse=True)

    if not matches:
        # Expected whenever a protocol was never run at this user count (the
        # prebaked Cauchy series covers small counts the live protocols skip,
        # and vice versa) — each protocol is plotted over its own counts.
        return None

    chosen = matches[0]
    print(f"  {protocol} ({user_count} users): using {chosen}")
    return chosen


# ---------------------------------------------------------------------------
# Ingestion data utilities (same as plot_bench.py)
# ---------------------------------------------------------------------------

INGESTION_REQUIRED_COLS = {
    "block_num", "num_selected_updates",
    "queued_at_ns", "start_at_ns", "completed_at_ns",
}

# Cauchy logs have an extra leading column not present in other protocols.
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


def compute_scalars(path: str, warmup: float, cooldown: float, protocol: str = "") -> dict:
    df = load_ingestion_csv(path, protocol)
    df = trim_df(df, warmup, cooldown)

    total_updates = df["num_selected_updates"].sum()
    wall_time_s   = (df["completed_at_ns"].max() - df["queued_at_ns"].min()) / 1e9
    avg_throughput = total_updates / wall_time_s if wall_time_s > 0 else 0.0

    return {"avg_throughput": avg_throughput}


# ---------------------------------------------------------------------------
# Plotting
# ---------------------------------------------------------------------------

# (col, ylabel, fname, title, scale) — only the throughput chart (paper Figure 7a)
GRAPHS = [
    ("avg_throughput", "Throughput (ops/s)", "fig7a_ingestion_throughput", "Avg Update Throughput", 1.0),
]


def _user_count_formatter(x, pos):
    """Format x-axis tick labels: integers < 100000 as plain integers, >= 100000 as e.g. '0.1M', '1M'."""
    x = int(x)
    if x >= 100_000:
        m = x / 1_000_000
        if m < 1:
            return f"0.{int(m * 10)}M"
        return f"{m:g}M"
    return str(x)


def _y_formatter_kilo(x, pos):
    """No scientific notation on either panel."""
    if x >= 1000:
        return f"{int(x / 1000)}k"
    if x == 0:
        return "0"
    return f"{x:.10f}".rstrip("0").rstrip(".")


def _y_formatter_normal(x, pos):
    if x == 0:
        return "0"
    return f"{x:.10f}".rstrip("0").rstrip(".")


def plot_line_charts(
    data: dict,          # data[user_count][protocol] = scalars_dict
    protocols: list,     # ordered list of protocol names
    user_counts: list,   # ordered list of user counts (x-axis)
    proto_styles: dict,  # precomputed styles keyed by protocol name
    args,
):
    """Produces one line chart per metric. X-axis = number of users, one line per protocol."""
    small_ucs   = [uc for uc in user_counts if uc < 100_000]
    large_ucs   = [uc for uc in user_counts if uc >= 100_000]
    use_x_broken = bool(small_ucs and large_ucs)
    Y_BREAK = 10000

    # --- helpers ---

    def _plot_proto_on_ax(ax, ucs, y_normal: bool = False):
        for proto in protocols:
            sty = proto_styles[proto]
            proto_ucs = [uc for uc in ucs if proto in data[uc]]
            if not proto_ucs:
                continue
            values = [data[uc][proto][col] * scale for uc in proto_ucs]
            ax.plot(
                proto_ucs, values,
                color=sty["color"],
                label=sty["label"],
                marker=sty.get("marker", "o"),
                markersize=25,
                linewidth=10,
                markeredgewidth=2,
            )
        ax.set_xscale("log")
        ax.set_xticks(ucs)
        ax.xaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_user_count_formatter))
        if y_normal:
            ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter_normal))
        else:
            ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter_kilo))
        ax.grid(True, which="both", linestyle="--", linewidth=3, alpha=0.7)

    _X_BRK_MARKER = [(-1, -1), (1, 1)]
    _Y_BRK_MARKER = [(-1, -1), (1, 1)]
    _BRK_KW = dict(markersize=20, color="k", mec="k", mew=3, clip_on=False, linestyle="none")

    def _add_x_break(ax_l, ax_r):
        ax_l.plot([1], [0], marker=_X_BRK_MARKER, transform=ax_l.transAxes, **_BRK_KW)
        ax_r.plot([0], [0], marker=_X_BRK_MARKER, transform=ax_r.transAxes, **_BRK_KW)

    def _add_y_break(ax_top, ax_bot):
        ax_top.plot([0], [0], marker=_Y_BRK_MARKER, transform=ax_top.transAxes, **_BRK_KW)
        ax_bot.plot([0], [1], marker=_Y_BRK_MARKER, transform=ax_bot.transAxes, **_BRK_KW)

    def _collect_handles(*axes_list):
        seen, handles, labels = set(), [], []
        for ax in axes_list:
            for h, l in zip(*ax.get_legend_handles_labels()):
                if l not in seen:
                    handles.append(h); labels.append(l); seen.add(l)
        return handles, labels

    def _add_legend(fig, handles, labels):
        fig.legend(
            handles, labels,
            loc="upper center", bbox_to_anchor=(0.5, 1.04),
            ncol=len(handles), frameon=True, edgecolor="black",
            fontsize=plt.rcParams["legend.fontsize"]*.75,
            columnspacing=0.3,
        )

    # --- per-metric loop ---

    for col, ylabel, fname, title, scale in GRAPHS:
        all_vals = [
            data[uc][proto][col] * scale
            for uc in user_counts for proto in protocols
            if proto in data[uc]
        ]
        use_y_broken = bool(all_vals) and min(all_vals) <= Y_BREAK

        if use_y_broken:
            ymin_d, ymax_d = min(all_vals), max(all_vals)
            pad = (ymax_d - ymin_d) * 0.08 or 1e-5
            # Ensure top panel range is never inverted (data may all be below Y_BREAK)
            top_upper = max(ymax_d + pad, Y_BREAK * 1.5)
            y_top_lim = (Y_BREAK, top_upper)
            y_bot_lim = (max(ymin_d - pad, 0), 0.6)
            h_ratio = [1, 1]

        if use_x_broken and use_y_broken:
            n_s, n_l = max(len(small_ucs), 1), max(len(large_ucs), 1)
            fig, axes = plt.subplots(
                2, 2, figsize=(30, 12),
                sharex="col", sharey="row",
                gridspec_kw={"width_ratios": [n_s, n_l], "height_ratios": h_ratio},
            )
            fig.subplots_adjust(wspace=0.1, hspace=0.08)
            ax_tl, ax_tr = axes[0, 0], axes[0, 1]
            ax_bl, ax_br = axes[1, 0], axes[1, 1]

            for ax in [ax_tl, ax_tr, ax_bl, ax_br]:
                ax.spines["left"].set_linewidth(5)
                ax.spines["bottom"].set_linewidth(5)

            for i, ax in enumerate([ax_tl, ax_bl]): _plot_proto_on_ax(ax, small_ucs, y_normal=(i == 1))
            for i, ax in enumerate([ax_tr, ax_br]): _plot_proto_on_ax(ax, large_ucs, y_normal=(i == 1))

            # x-break spines
            for ax in [ax_tl, ax_bl]: ax.spines["right"].set_visible(False)
            for ax in [ax_tr, ax_br]:
                ax.spines["left"].set_visible(False)
                ax.tick_params(left=False)
            # y-break spines
            for ax in [ax_tl, ax_tr]:
                ax.spines["bottom"].set_visible(False)
                ax.tick_params(bottom=False)
            for ax in [ax_bl, ax_br]: ax.spines["top"].set_visible(False)

            _add_x_break(ax_bl, ax_br)   # bottom row only — top row marks land in the middle
            _add_y_break(ax_tl, ax_bl)   # left column only — right column marks land in the middle

            # Set limits last so nothing above overrides them
            for ax in [ax_tl, ax_tr]: ax.set_ylim(*y_top_lim)
            for ax in [ax_bl, ax_br]: ax.set_ylim(*y_bot_lim)

            for ax in [ax_bl, ax_br]: ax.set_yticks([0, 0.2, 0.4, 0.6])

            fig.supylabel(ylabel, position=(0.04, 0.5))
            fig.text(0.5, -0.04, "Number of users", ha="center",
                     fontsize=plt.rcParams["axes.labelsize"])
            handles, labels = _collect_handles(ax_tl, ax_tr, ax_bl, ax_br)
            _add_legend(fig, handles, labels)

        elif use_x_broken:
            n_s, n_l = max(len(small_ucs), 1), max(len(large_ucs), 1)
            fig, (ax_l, ax_r) = plt.subplots(
                1, 2, sharey=True, figsize=(30, 12),
                gridspec_kw={"width_ratios": [n_s, n_l]},
            )
            fig.subplots_adjust(wspace=0.08)

            _plot_proto_on_ax(ax_l, small_ucs)
            _plot_proto_on_ax(ax_r, large_ucs)

            ax_l.spines["right"].set_visible(False)
            ax_r.spines["left"].set_visible(False)
            ax_r.tick_params(left=False)
            _add_x_break(ax_l, ax_r)

            ax_l.set_ylabel(ylabel)
            fig.text(0.5, -0.04, "Number of users", ha="center",
                     fontsize=plt.rcParams["axes.labelsize"])
            handles, labels = _collect_handles(ax_l, ax_r)
            _add_legend(fig, handles, labels)

        elif use_y_broken:
            fig, (ax_top, ax_bot) = plt.subplots(
                2, 1, sharex=True, figsize=(30, 12),
                gridspec_kw={"height_ratios": h_ratio},
            )
            fig.subplots_adjust(hspace=0.08)

            _plot_proto_on_ax(ax_top, user_counts)
            _plot_proto_on_ax(ax_bot, user_counts)

            ax_top.spines["bottom"].set_visible(False)
            ax_bot.spines["top"].set_visible(False)
            ax_top.tick_params(bottom=False)
            _add_y_break(ax_top, ax_bot)

            # Set limits last so nothing above overrides them
            ax_top.set_ylim(*y_top_lim)
            ax_bot.set_ylim(*y_bot_lim)

            ax_top.set_ylabel(ylabel)
            ax_bot.set_xlabel("Number of users")
            handles, labels = _collect_handles(ax_top, ax_bot)
            _add_legend(fig, handles, labels)

        else:
            fig, ax = plt.subplots(1, 1, figsize=(30, 12))
            _plot_proto_on_ax(ax, user_counts)
            ax.set_xlabel("Number of users")
            ax.set_ylabel(ylabel)
            handles, labels = _collect_handles(ax)
            _add_legend(fig, handles, labels)

        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    apply_paper_style()

    parser = argparse.ArgumentParser(
        description="Paper Figure 7a: ingestion throughput across protocols and user counts.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--input-root", default="logs/ingestion_logs",
                        metavar="DIR",
                        help="Directory containing <protocol>/ingestion_<users>.csv files "
                             "(default: logs/ingestion_logs)")
    parser.add_argument("--protocol", action="append", required=True,
                        metavar="NAME",
                        help="Protocol name (repeatable). Must match a subdirectory "
                             "under --input-root.")
    parser.add_argument("--skip-users", action="append", type=int, default=None,
                        metavar="N",
                        help="User count to exclude from auto-discovery (repeatable; "
                             "default: 0 and 1).")
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

    protocols  = args.protocol
    skip_users = set(args.skip_users) if args.skip_users is not None else {0, 1}

    user_counts = discover_user_counts(args.input_root, protocols, skip_users)
    if not user_counts:
        sys.exit(f"ERROR: no user counts discovered under {args.input_root}.")

    print(f"Skipping user counts: {sorted(skip_users)}")
    print(f"Discovered user counts: {user_counts}")
    print(f"Finding newest ingestion CSVs for users={user_counts}, protocols={protocols}:")

    # data[user_count][protocol] = scalars_dict  (only populated when data exists)
    data: dict = {}
    for uc in user_counts:
        data[uc] = {}
        for proto in protocols:
            csv_path = find_newest_ingestion_csv(args.input_root, proto, uc)
            if csv_path is None:
                continue
            data[uc][proto] = compute_scalars(csv_path, args.warmup, args.cooldown, proto)
    for proto in protocols:
        if not any(proto in data[uc] for uc in user_counts):
            print(f"  WARNING: no ingestion CSVs for protocol={proto} under {args.input_root} — it will not appear in the figure")

    # Print + save summary table
    print(f"\n{'Users':<10} {'Protocol':<15} {'Throughput (upd/s)':<20}")
    print("-" * 45)
    rows = []
    for uc in user_counts:
        for proto in protocols:
            if proto not in data[uc]:
                continue
            s = data[uc][proto]
            print(f"{uc:<10} {proto:<15} {s['avg_throughput']:<20.4f}")
            rows.append({
                "Users": uc,
                "Protocol": proto,
                "Throughput (upd/s)": round(s["avg_throughput"], 4),
            })
        print()
    os.makedirs(args.output_dir, exist_ok=True)
    csv_path = os.path.join(args.output_dir, "fig7a_summary.csv")
    pd.DataFrame(rows).to_csv(csv_path, index=False)
    print(f"  saved {csv_path}")

    # Precompute styles once
    global _auto_color_idx
    _auto_color_idx = 0
    proto_styles = {p: _protocol_style(p) for p in protocols}

    print("Generating chart:")
    plot_line_charts(data, protocols, user_counts, proto_styles, args)


if __name__ == "__main__":
    main()
