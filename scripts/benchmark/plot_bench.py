#!/usr/bin/env python3
"""
plot_bench.py — Research-quality benchmark visualisation.

Subcommands:
  ingestion-timeseries   Time-series graphs (G1–G6) from one or more ingestion CSVs.
  ingestion-summary      Summary comparison across k-user experiments (G7–G9).
  proof-summary          Summary comparison across range-size experiments (G10–G13).
"""

import argparse
import os
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np
import pandas as pd

# ---------------------------------------------------------------------------
# Protocol style constants
# ---------------------------------------------------------------------------

PROTOCOL_STYLE = {
    "samurai":    {"color": "#e66101", "marker": "o", "label": "Samurai"},
    "samuraimpt": {"color": "#7b3294", "marker": "D", "label": "Samurai+MPT"},
    "merkle":     {"color": "#2166ac", "marker": "^", "label": "Merkle"},
    "verkle":     {"color": "#1b7837", "marker": "s", "label": "Verkle"},
    "cauchy":     {"color": "#b2182b", "marker": "D", "label": "Cauchy"},
}
_AUTO_COLORS = [
    "#4393c3", "#4dac26", "#d01c8b", "#f1b6da",
]
_auto_color_idx = 0


def _protocol_style(name: str) -> dict:
    """Return style dict for a protocol, auto-assigning unknown protocols."""
    global _auto_color_idx
    if name in PROTOCOL_STYLE:
        return PROTOCOL_STYLE[name]
    color = _AUTO_COLORS[_auto_color_idx % len(_AUTO_COLORS)]
    _auto_color_idx += 1
    return {"color": color, "marker": "D", "label": name.capitalize()}


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


def _make_fig():
    return plt.subplots(1, 1, figsize=(3.5, 2.6))


def _finalize_ax(ax, xlabel, ylabel, title, labels):
    ax.set_xlabel(xlabel)
    ax.set_ylabel(ylabel)
    ax.set_title(title, pad=4)
    if labels:
        if len(labels) > 2:
            ax.legend(bbox_to_anchor=(1.01, 1), loc="upper left",
                      borderaxespad=0, frameon=False)
        else:
            ax.legend(loc="best", frameon=False)


# ---------------------------------------------------------------------------
# Ingestion data utilities
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
    """Compute per-row latency columns BEFORE windowing."""
    df = df.copy()
    df["block_lat_ms"]      = (df["completed_at_ns"] - df["start_at_ns"])   / 1e6
    df["e2e_block_lat_ms"]  = (df["completed_at_ns"] - df["queued_at_ns"])  / 1e6
    nu = df["num_selected_updates"].replace(0, np.nan)
    df["update_lat_ms"]     = df["block_lat_ms"]     / nu
    df["e2e_update_lat_ms"] = df["e2e_block_lat_ms"] / nu
    return df


def rolling_window_stats(df: pd.DataFrame, window_sec: float) -> pd.DataFrame:
    """Bin rows by window_sec buckets and compute per-window statistics."""
    df = df.copy()
    df["_bucket"] = np.floor(df["rel_time"] / window_sec).astype(int)
    grp = df.groupby("_bucket")

    t_mid            = grp["rel_time"].mean()
    mean_block_lat   = grp["block_lat_ms"].mean()
    mean_e2e_block   = grp["e2e_block_lat_ms"].mean()
    mean_update_lat  = grp["update_lat_ms"].mean()     # NaN rows excluded
    mean_e2e_update  = grp["e2e_update_lat_ms"].mean() # NaN rows excluded
    block_throughput = grp["block_lat_ms"].count() / window_sec
    update_throughput= grp["num_selected_updates"].sum() / window_sec

    out = pd.DataFrame({
        "t_mid":               t_mid,
        "mean_block_lat":      mean_block_lat,
        "mean_e2e_block_lat":  mean_e2e_block,
        "mean_update_lat":     mean_update_lat,
        "mean_e2e_update_lat": mean_e2e_update,
        "block_throughput":    block_throughput,
        "update_throughput":   update_throughput,
    }).reset_index(drop=True)
    return out.sort_values("t_mid").reset_index(drop=True)


# ---------------------------------------------------------------------------
# Subcommand 1: ingestion-timeseries  (G1–G6)
# ---------------------------------------------------------------------------

_TIMESERIES_GRAPHS = {
    "G1": ("mean_block_lat",      "Latency (ms)",       "G1_block_latency",       "Block Latency"),
    "G2": ("mean_e2e_block_lat",  "Latency (ms)",       "G2_e2e_block_latency",   "E2E Block Latency"),
    "G3": ("mean_e2e_update_lat", "Latency (ms)",       "G3_e2e_update_latency",  "E2E Update Latency"),
    "G4": ("mean_update_lat",     "Latency (ms)",       "G4_update_latency",      "Update Latency"),
    "G5": ("block_throughput",    "Throughput (blocks/s)", "G5_block_throughput", "Block Throughput"),
    "G6": ("update_throughput",   "Throughput (updates/s)","G6_update_throughput","Update Throughput"),
}


def _parse_input_args(raw: list) -> list:
    """Parse ['label:path', ...] into [(label, path), ...]."""
    result = []
    for item in raw:
        if ":" not in item:
            sys.exit(f"ERROR: --input must be 'label:path', got: {item!r}")
        label, path = item.split(":", 1)
        result.append((label.strip(), path.strip()))
    return result


def cmd_ingestion_timeseries(args):
    inputs = _parse_input_args(args.input)
    graphs_to_plot = (
        set(_TIMESERIES_GRAPHS.keys())
        if args.graphs == "all"
        else {g.strip().upper() for g in args.graphs.split(",")}
    )

    # Load and process all inputs
    windowed = []
    for label, path in inputs:
        df = load_ingestion_csv(path)
        df = trim_df(df, args.warmup, args.cooldown)
        df = add_per_update_latency_cols(df)
        w  = rolling_window_stats(df, args.window)
        windowed.append((label, w))

    title_suffix = " — " + ", ".join(lbl for lbl, _ in inputs) if len(inputs) == 1 else ""

    for gid, (col, ylabel, fname, gtitle) in _TIMESERIES_GRAPHS.items():
        if gid not in graphs_to_plot:
            continue
        fig, ax = _make_fig()
        for label, w in windowed:
            sty = _protocol_style(label)
            x = w["t_mid"].values
            y = w[col].values
            ax.plot(x, y, color=sty["color"], linewidth=1,
                    label=sty["label"])
        _finalize_ax(ax, "Time (s)", ylabel,
                     f"{gtitle}{title_suffix}",
                     [lbl for lbl, _ in inputs])
        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Subcommand 2: ingestion-summary  (G7–G9)
# ---------------------------------------------------------------------------

def _parse_inputs_string(s: str) -> list:
    """Parse 'k:path,k:path,...' into [(int_k, path), ...]."""
    result = []
    for item in s.split(","):
        item = item.strip()
        if not item:
            continue
        if ":" not in item:
            sys.exit(f"ERROR: --inputs entry must be 'k:path', got: {item!r}")
        k_str, path = item.split(":", 1)
        result.append((int(k_str.strip()), path.strip()))
    return sorted(result, key=lambda x: x[0])


def _compute_ingestion_scalars(path: str, warmup: float, cooldown: float) -> dict:
    df = load_ingestion_csv(path)
    df = trim_df(df, warmup, cooldown)
    df = add_per_update_latency_cols(df)
    valid = df[df["num_selected_updates"] > 0]

    avg_update_lat     = valid["update_lat_ms"].mean()
    avg_e2e_update_lat = valid["e2e_update_lat_ms"].mean()

    total_updates  = df["num_selected_updates"].sum()
    wall_time_s    = (df["completed_at_ns"].max() - df["queued_at_ns"].min()) / 1e9
    avg_throughput = total_updates / wall_time_s if wall_time_s > 0 else 0.0

    return {
        "avg_update_lat":     avg_update_lat,
        "avg_e2e_update_lat": avg_e2e_update_lat,
        "avg_throughput":     avg_throughput,
    }


def cmd_ingestion_summary(args):
    if len(args.protocol) != len(args.inputs):
        sys.exit("ERROR: number of --protocol and --inputs args must match")

    proto_data = []
    for proto, inputs_str in zip(args.protocol, args.inputs):
        entries = _parse_inputs_string(inputs_str)
        points = []
        for k, path in entries:
            scalars = _compute_ingestion_scalars(path, args.warmup, args.cooldown)
            points.append((k, scalars))
        proto_data.append((proto, points))

    graphs = [
        ("avg_update_lat",     "Avg Update Latency (ms)",    "G7_update_latency_vs_kusers",     "Avg Update Latency vs K-Users"),
        ("avg_e2e_update_lat", "Avg E2E Update Latency (ms)","G8_e2e_update_latency_vs_kusers",  "Avg E2E Update Latency vs K-Users"),
        ("avg_throughput",     "Throughput (updates/s)",      "G9_throughput_vs_kusers",         "Avg Update Throughput vs K-Users"),
    ]

    for col, ylabel, fname, title in graphs:
        fig, ax = _make_fig()
        for proto, points in proto_data:
            sty = _protocol_style(proto)
            xs = [p[0] for p in points]
            ys = [p[1][col] for p in points]
            ax.plot(xs, ys, color=sty["color"], marker=sty["marker"],
                    markersize=5, linewidth=1.2, label=sty["label"])
        ax.set_xscale("log")
        _finalize_ax(ax, "K-Users (log scale)", ylabel, title,
                     [p for p, _ in proto_data])
        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Proof summary parsing
# ---------------------------------------------------------------------------

def parse_proof_summary(path: str) -> dict:
    """Parse a proof summary CSV file into a dict with numeric values."""
    df = pd.read_csv(path)
    if len(df) == 0:
        return {}
    row = df.iloc[0]
    return {col: float(row[col]) for col in df.columns}


# ---------------------------------------------------------------------------
# Subcommand 3: proof-summary  (G10–G13)
# ---------------------------------------------------------------------------

def cmd_proof_summary(args):
    if len(args.protocol) != len(args.inputs):
        sys.exit("ERROR: number of --protocol and --inputs args must match")

    proto_data = []
    for proto, inputs_str in zip(args.protocol, args.inputs):
        entries = []
        for item in inputs_str.split(","):
            item = item.strip()
            if not item:
                continue
            if ":" not in item:
                sys.exit(f"ERROR: --inputs entry must be 'range:path', got: {item!r}")
            range_str, path = item.split(":", 1)
            entries.append((int(range_str.strip()), path.strip()))
        entries.sort(key=lambda x: x[0])
        points = [(rng, parse_proof_summary(path)) for rng, path in entries]
        proto_data.append((proto, points))

    # Auto-detect log scale for x-axis
    all_ranges = [pt[0] for _, pts in proto_data for pt in pts]
    use_log_x = args.log_x or (
        len(all_ranges) >= 2 and max(all_ranges) / min(all_ranges) > 10
    )

    graphs = [
        ("avg_proofgen_ms", "Latency (ms)",       "G10_proofgen_vs_range",   "Proofgen Latency vs Range"),
        ("avg_response_ms", "Latency (ms)",        "G11_response_vs_range",   "Response Latency vs Range"),
        ("avg_verify_ms",   "Latency (ms)",        "G12_verify_vs_range",     "Verify Latency vs Range"),
        ("throughput_rps",  "Throughput (req/s)",  "G13_throughput_vs_range", "Throughput vs Range"),
    ]

    for col, ylabel, fname, title in graphs:
        fig, ax = _make_fig()
        for proto, points in proto_data:
            sty = _protocol_style(proto)
            xs = [p[0] for p in points]
            ys = [p[1].get(col, float("nan")) for p in points]
            ax.plot(xs, ys, color=sty["color"], marker=sty["marker"],
                    markersize=5, linewidth=1.2, label=sty["label"])
        if use_log_x:
            ax.set_xscale("log")
        _finalize_ax(ax, "Range Size", ylabel, title,
                     [p for p, _ in proto_data])
        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Subcommand 4: update-timeseries  (G14–G15)
# ---------------------------------------------------------------------------

UPDATE_METRICS_REQUIRED_COLS = {"window_end_ns", "updates_completed", "sum_compute_ns"}


def load_update_metrics_csv(path: str) -> pd.DataFrame:
    df = pd.read_csv(path)
    df.columns = [c.strip() for c in df.columns]
    missing = UPDATE_METRICS_REQUIRED_COLS - set(df.columns)
    if missing:
        sys.exit(f"ERROR: {path} is missing columns: {missing}")
    return df


def process_update_metrics(df: pd.DataFrame, warmup: float, cooldown: float) -> pd.DataFrame:
    """Compute per-window throughput and latency from update metrics CSV."""
    df = df.copy()
    # Compute relative time from the first window.
    df["rel_time"] = (df["window_end_ns"] - df["window_end_ns"].min()) / 1e9

    # Trim warmup/cooldown.
    max_t = df["rel_time"].max()
    mask = (df["rel_time"] >= warmup) & (df["rel_time"] <= (max_t - cooldown))
    df = df[mask].reset_index(drop=True)

    # Derive per-window metrics.  Assume 1s windows (delta between consecutive
    # window_end_ns values).  Fall back to 1.0s when only one row.
    dt = df["window_end_ns"].diff() / 1e9
    dt.iloc[0] = dt.iloc[1] if len(dt) > 1 else 1.0
    df["update_throughput"] = df["updates_completed"] / dt
    nu = df["updates_completed"].replace(0, np.nan)
    df["avg_update_latency_ms"] = (df["sum_compute_ns"] / nu) / 1e6
    return df


_UPDATE_TS_GRAPHS = {
    "G14": ("update_throughput",      "Throughput (updates/s)", "G14_update_throughput_measured", "Update Throughput (Measured)"),
    "G15": ("avg_update_latency_ms",  "Latency (ms)",          "G15_update_latency_measured",    "Avg Update Latency (Measured)"),
}


def smooth_update_metrics(df: pd.DataFrame, window_sec: float) -> pd.DataFrame:
    """Apply a rolling mean over 1-second update-metric rows."""
    if window_sec <= 1.0 or len(df) < 2:
        return df
    # Rows are ~1s apart; window_sec rows ≈ window_sec seconds.
    win = max(1, int(round(window_sec)))
    out = df.copy()
    out["update_throughput"] = df["update_throughput"].rolling(win, min_periods=1, center=True).mean()
    out["avg_update_latency_ms"] = df["avg_update_latency_ms"].rolling(win, min_periods=1, center=True).mean()
    return out


def cmd_update_timeseries(args):
    inputs = _parse_input_args(args.input)
    graphs_to_plot = (
        set(_UPDATE_TS_GRAPHS.keys())
        if args.graphs == "all"
        else {g.strip().upper() for g in args.graphs.split(",")}
    )

    processed = []
    for label, path in inputs:
        df = load_update_metrics_csv(path)
        df = process_update_metrics(df, args.warmup, args.cooldown)
        df = smooth_update_metrics(df, args.window)
        processed.append((label, df))

    title_suffix = " — " + ", ".join(lbl for lbl, _ in inputs) if len(inputs) == 1 else ""

    for gid, (col, ylabel, fname, gtitle) in _UPDATE_TS_GRAPHS.items():
        if gid not in graphs_to_plot:
            continue
        fig, ax = _make_fig()
        for label, df in processed:
            sty = _protocol_style(label)
            x = df["rel_time"].values
            y = df[col].values
            ax.plot(x, y, color=sty["color"], linewidth=1, label=sty["label"])
        _finalize_ax(ax, "Time (s)", ylabel,
                     f"{gtitle}{title_suffix}",
                     [lbl for lbl, _ in inputs])
        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Subcommand 5: proof-throughput-timeseries  (G16–G17)
# ---------------------------------------------------------------------------

PROOF_THROUGHPUT_REQUIRED_COLS = {"start_at_ns", "completed_at_ns"}


def load_proof_throughput_csv(path: str) -> pd.DataFrame:
    df = pd.read_csv(path)
    df.columns = [c.strip() for c in df.columns]
    missing = PROOF_THROUGHPUT_REQUIRED_COLS - set(df.columns)
    if missing:
        sys.exit(f"ERROR: {path} is missing columns: {missing}")
    return df


def process_proof_throughput(df: pd.DataFrame, warmup: float, cooldown: float,
                             window_sec: float) -> pd.DataFrame:
    """Compute per-window throughput and latency from server bench log CSV."""
    df = df.copy()
    # Relative time based on completion timestamp.
    t0 = df["completed_at_ns"].min()
    df["rel_time"] = (df["completed_at_ns"] - t0) / 1e9

    # Per-request latency.
    df["latency_ms"] = (df["completed_at_ns"] - df["start_at_ns"]) / 1e6

    # Trim warmup/cooldown.
    max_t = df["rel_time"].max()
    mask = (df["rel_time"] >= warmup) & (df["rel_time"] <= (max_t - cooldown))
    df = df[mask].reset_index(drop=True)

    if len(df) == 0:
        return pd.DataFrame(columns=["t_mid", "request_throughput", "avg_latency_ms"])

    # Bin into time windows.
    df["_bucket"] = np.floor(df["rel_time"] / window_sec).astype(int)
    grp = df.groupby("_bucket")

    out = pd.DataFrame({
        "t_mid":              grp["rel_time"].mean(),
        "request_throughput": grp["latency_ms"].count() / window_sec,
        "avg_latency_ms":     grp["latency_ms"].mean(),
    }).reset_index(drop=True)
    return out.sort_values("t_mid").reset_index(drop=True)


_PROOF_THROUGHPUT_GRAPHS = {
    "G16": ("request_throughput", "Throughput (req/s)", "G16_proof_throughput", "Proof Request Throughput"),
    "G17": ("avg_latency_ms",    "Latency (ms)",       "G17_proof_latency",    "Avg Proof Request Latency"),
}


def cmd_proof_throughput_timeseries(args):
    inputs = _parse_input_args(args.input)
    graphs_to_plot = (
        set(_PROOF_THROUGHPUT_GRAPHS.keys())
        if args.graphs == "all"
        else {g.strip().upper() for g in args.graphs.split(",")}
    )

    processed = []
    for label, path in inputs:
        df = load_proof_throughput_csv(path)
        df = process_proof_throughput(df, args.warmup, args.cooldown, args.window)
        processed.append((label, df))

    title_suffix = " — " + ", ".join(lbl for lbl, _ in inputs) if len(inputs) == 1 else ""

    for gid, (col, ylabel, fname, gtitle) in _PROOF_THROUGHPUT_GRAPHS.items():
        if gid not in graphs_to_plot:
            continue
        fig, ax = _make_fig()
        for label, df in processed:
            sty = _protocol_style(label)
            x = df["t_mid"].values
            y = df[col].values
            ax.plot(x, y, color=sty["color"], linewidth=1, label=sty["label"])
        _finalize_ax(ax, "Time (s)", ylabel,
                     f"{gtitle}{title_suffix}",
                     [lbl for lbl, _ in inputs])
        save_figure(fig, args.output_dir, fname, args.format, args.dpi)


# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------

def _add_global_opts(p: argparse.ArgumentParser):
    p.add_argument("--output-dir", default="benchmark_output/plots",
                   help="Directory for output files (default: benchmark_output/plots)")
    p.add_argument("--format", choices=["pdf", "png"], default="pdf",
                   help="Output format (default: pdf)")
    p.add_argument("--dpi", type=int, default=300,
                   help="DPI for PNG output (default: 300)")
    p.add_argument("--warmup", type=float, default=0.0,
                   help="Seconds to trim from start of data (default: 0)")
    p.add_argument("--cooldown", type=float, default=0.0,
                   help="Seconds to trim from end of data (default: 0)")


def main():
    apply_paper_style()

    parser = argparse.ArgumentParser(
        description="Generate publication-quality benchmark graphs.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    sub = parser.add_subparsers(dest="cmd", required=True)

    # ingestion-timeseries
    p1 = sub.add_parser("ingestion-timeseries",
                         help="Time-series graphs G1–G6 from ingestion CSVs")
    _add_global_opts(p1)
    p1.add_argument("--input", action="append", required=True, metavar="LABEL:PATH",
                    help="'label:csv_path' (repeatable for multi-protocol overlay)")
    p1.add_argument("--window", type=float, default=5.0,
                    help="Rolling window size in seconds (default: 5.0)")
    p1.add_argument("--graphs", default="all",
                    help="Graphs to produce: 'all' or comma-separated G1,G2,... (default: all)")
    p1.set_defaults(func=cmd_ingestion_timeseries)

    # ingestion-summary
    p2 = sub.add_parser("ingestion-summary",
                         help="K-users summary graphs G7–G9")
    _add_global_opts(p2)
    p2.add_argument("--protocol", action="append", required=True, metavar="LABEL",
                    help="Protocol label (repeatable, matches --inputs order)")
    p2.add_argument("--inputs", action="append", required=True, metavar="K:PATH[,...]",
                    help="'k:csv_path,...' for this protocol (repeatable)")
    p2.set_defaults(func=cmd_ingestion_summary)

    # proof-summary
    p3 = sub.add_parser("proof-summary",
                         help="Range-size summary graphs G10–G13 from proof summary files")
    _add_global_opts(p3)
    p3.add_argument("--protocol", action="append", required=True, metavar="LABEL",
                    help="Protocol label (repeatable, matches --inputs order)")
    p3.add_argument("--inputs", action="append", required=True, metavar="RANGE:PATH[,...]",
                    help="'range:summary_path,...' for this protocol (repeatable)")
    p3.add_argument("--log-x", action="store_true",
                    help="Force log scale on x-axis (auto-detected otherwise)")
    p3.set_defaults(func=cmd_proof_summary)

    # update-timeseries
    p4 = sub.add_parser("update-timeseries",
                         help="Update-level time-series graphs G14–G15 from update metrics CSVs")
    _add_global_opts(p4)
    p4.add_argument("--input", action="append", required=True, metavar="LABEL:PATH",
                    help="'label:csv_path' (repeatable for multi-protocol overlay)")
    p4.add_argument("--window", type=float, default=5.0,
                    help="Rolling window size in seconds for smoothing (default: 5.0)")
    p4.add_argument("--graphs", default="all",
                    help="Graphs to produce: 'all' or comma-separated G14,G15 (default: all)")
    p4.set_defaults(func=cmd_update_timeseries)

    # proof-throughput-timeseries
    p5 = sub.add_parser("proof-throughput-timeseries",
                         help="Proof throughput time-series graphs G16–G17 from server bench CSVs")
    _add_global_opts(p5)
    p5.add_argument("--input", action="append", required=True, metavar="LABEL:PATH",
                    help="'label:csv_path' (repeatable for multi-protocol overlay)")
    p5.add_argument("--window", type=float, default=5.0,
                    help="Rolling window size in seconds (default: 5.0)")
    p5.add_argument("--graphs", default="all",
                    help="Graphs to produce: 'all' or comma-separated G16,G17 (default: all)")
    p5.set_defaults(func=cmd_proof_throughput_timeseries)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
