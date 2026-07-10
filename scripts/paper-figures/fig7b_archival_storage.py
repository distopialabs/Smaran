#!/usr/bin/env python3
"""
fig7b_archival_storage.py — Paper Figure 7b: impact of archival storage on query latency.

Output: fig7b_archival_storage.pdf — avg response + verify latency vs query range size,
comparing Smaran (stored roots, "optimus") against Smaran without archival storage
("non_optimus", tree rebuilt per query).

Input layout (root passed as positional arg):
    optimus/proof_range{R}[_<timestamp>].csv
    non_optimus/proof_range{R}[_<timestamp>].csv

Timestamped filenames are accepted; when a range size appears in several files the
newest file (by modification time) wins. The set of range sizes plotted is exactly
the set present in the input directories — the experiment scripts decide the points
(paper used range sizes 500 and above).

Trimmed from experiments/optimus.py (branch kt-put-throughput), keeping only the
chart used in the paper. Requires matplotlib + a LaTeX installation.
"""
from __future__ import annotations

import argparse
import csv
from dataclasses import dataclass
from pathlib import Path
import re
from typing import Dict, List

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker


PROTOCOL_LABELS = {
    "optimus":     "Smaran",
    "non_optimus": "Smaran (w/o archival storage)",
}

PROTOCOL_STYLES = {
    "optimus":     {"color": "#0072B2", "marker": "o", "label": "Smaran"},
    "non_optimus": {"color": "#D55E00", "marker": "s", "label": "Smaran (w/o archival storage)"},
}

LATENCY_YTICKS = [100, 200, 500, 1_000, 2_000, 5_000]

_RANGE_RE = re.compile(r"^proof_range(\d+)")


@dataclass(frozen=True)
class ProofBenchmarkPoint:
    protocol: str
    range_size: int
    avg_response_ms: float
    avg_verify_ms: float
    csv_path: Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Plot paper Figure 7b (archival storage impact) from proof benchmark logs."
    )
    parser.add_argument(
        "input",
        help="Root directory containing optimus/ and non_optimus/ subdirectories.",
    )
    parser.add_argument(
        "--output",
        help="Output directory for the plot (default: <input>/output)",
        default=None,
    )
    return parser.parse_args()


def parse_csv(csv_path: Path, range_size: int) -> ProofBenchmarkPoint:
    with csv_path.open(encoding="utf-8") as f:
        reader = csv.DictReader(f)
        row = next(reader)
    return ProofBenchmarkPoint(
        protocol=csv_path.parent.name,
        range_size=range_size,
        avg_response_ms=float(row["avg_response_ms"]),
        avg_verify_ms=float(row["avg_verify_ms"]),
        csv_path=csv_path,
    )


def load_all_points(root: Path) -> List[ProofBenchmarkPoint]:
    # Newest file per (protocol, range_size) wins, so timestamped reruns are handled.
    chosen: Dict[tuple, Path] = {}
    for csv_path in sorted(root.glob("*/proof_range*.csv")):
        if csv_path.parent.name not in PROTOCOL_LABELS:
            continue
        m = _RANGE_RE.match(csv_path.stem)
        if not m:
            print(f"Skipping unrecognized filename: {csv_path.name}")
            continue
        key = (csv_path.parent.name, int(m.group(1)))
        if key not in chosen or csv_path.stat().st_mtime > chosen[key].stat().st_mtime:
            chosen[key] = csv_path

    points: List[ProofBenchmarkPoint] = []
    for (protocol, range_size), csv_path in sorted(chosen.items()):
        try:
            point = parse_csv(csv_path, range_size)
            points.append(point)
            print(
                f"  Loaded: protocol={point.protocol:12s} range={point.range_size:>9,d} "
                f"response={point.avg_response_ms:9.1f} ms verify={point.avg_verify_ms:9.1f} ms"
            )
        except (StopIteration, KeyError, ValueError) as exc:
            print(f"Warning: skipping {csv_path}: {exc}")
    return points


def configure_plot_style() -> None:
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


def _range_formatter(x, pos):
    x = int(x)
    if x >= 1_000_000:
        m = x / 1_000_000
        return f"{m:g}M"
    if x >= 1_000:
        return f"{x // 1_000}k"
    return str(x)


def _ms_unit_formatter(x, pos):
    """Always show values in ms with unit suffix."""
    if x >= 60_000:
        return f"{int(x / 60_000):,}min"
    if x >= 1_000:
        return f"{int(x / 1_000):,}s"
    return f"{x:.10f}".rstrip("0").rstrip(".") + "ms"


def plot_fig7b(points: List[ProofBenchmarkPoint], output_path: Path) -> None:
    configure_plot_style()
    fig, ax = plt.subplots(figsize=(30, 12))

    for protocol in PROTOCOL_LABELS:
        proto_points = sorted(
            (p for p in points if p.protocol == protocol),
            key=lambda p: p.range_size,
        )
        if not proto_points:
            continue
        style = PROTOCOL_STYLES[protocol]
        xs = [p.range_size for p in proto_points]
        ys = [p.avg_response_ms + p.avg_verify_ms for p in proto_points]
        ax.plot(
            xs, ys,
            marker=style["marker"],
            markersize=25,
            linewidth=10,
            markeredgewidth=2,
            color=style["color"],
            label=style["label"],
        )

    ax.set_xscale("log")
    ax.set_yscale("log")
    ax.set_ylim(top=5_000)
    ax.set_yticks(LATENCY_YTICKS)
    ax.set_yticklabels([str(int(t)) for t in LATENCY_YTICKS])
    ax.yaxis.set_minor_locator(matplotlib.ticker.NullLocator())
    ax.xaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_range_formatter))
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_ms_unit_formatter))
    ax.grid(True, which="both", linestyle="--", linewidth=3, alpha=0.7)
    ax.spines["left"].set_linewidth(5)
    ax.spines["bottom"].set_linewidth(5)
    ax.set_xlabel("Query Range Size")
    ax.set_ylabel("Latency")

    handles, labels = ax.get_legend_handles_labels()
    fig.legend(
        handles, labels,
        loc="upper center",
        bbox_to_anchor=(0.5, 1.04),
        ncol=max(1, len(labels)),
        frameon=True,
        edgecolor="black",
        fontsize=plt.rcParams["legend.fontsize"] * 0.75,
        columnspacing=0.3,
    )

    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, format="pdf", bbox_inches="tight")
    plt.close(fig)
    print(f"Saved: {output_path}")


def main() -> int:
    args = parse_args()
    root = Path(args.input).expanduser().resolve()
    if not root.is_dir():
        print(f"Error: {root} is not a directory")
        return 1

    output_dir = (
        Path(args.output).expanduser().resolve() if args.output else root / "output"
    )

    print(f"Loading benchmark data from: {root}")
    points = load_all_points(root)
    if not points:
        print("No benchmark data found.")
        return 1

    print(f"\nLoaded {len(points)} benchmark points.")
    plot_fig7b(points, output_dir / "fig7b_archival_storage.pdf")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
