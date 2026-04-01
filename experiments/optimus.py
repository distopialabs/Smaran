#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
from dataclasses import dataclass
from pathlib import Path
import re
from typing import Dict, List, Tuple

import matplotlib
matplotlib.use("Agg")
import matplotlib.patches as mpatches
import matplotlib.pyplot as plt
import matplotlib.ticker


PROTOCOL_LABELS = {
    "optimus": "Smaran",
    "non_optimus":     "Smaran (w/o archival storage)",
    "verkle":     "Verkle",
}

PROTOCOL_STYLES = {
    "optimus": {"color": "#0072B2", "marker": "o", "label": "Smaran"},
    "non_optimus":     {"color": "#D55E00", "marker": "s", "label": "Smaran (w/o commitment storage)"},
    "verkle":     {"color": "#009E73", "marker": "^", "label": "Verkle"},
}


@dataclass(frozen=True)
class ProofBenchmarkPoint:
    protocol: str
    num_clients: int
    range_size: int
    duration_s: float
    total_requests: int
    client_errors: int
    server_errors: int
    verify_failures: int
    throughput_rps: float
    avg_proofgen_ms: float
    avg_response_ms: float
    avg_verify_ms: float
    avg_payload_kb: float
    csv_path: Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Plot DL query proof latency/throughput benchmarks."
    )
    parser.add_argument(
        "input",
        help="Path to the proof_latency_logs root directory.",
    )
    parser.add_argument(
        "--output",
        help="Output directory for plots (default: <input>/output)",
        default=None,
    )
    return parser.parse_args()


def parse_csv(csv_path: Path) -> ProofBenchmarkPoint:
    """Parse a single proof_range*.csv file.

    Path structure: numclients{N}/{protocol}/proof_range{R}.csv
    """
    protocol = csv_path.parent.name
    num_clients = 32

    with csv_path.open(encoding="utf-8") as f:
        reader = csv.DictReader(f)
        row = next(reader)

    return ProofBenchmarkPoint(
        protocol=protocol,
        num_clients=num_clients,
        range_size=int(row["range_size"]),
        duration_s=float(row["duration_s"]),
        total_requests=int(row["total_requests"]),
        client_errors=int(row["client_errors"]),
        server_errors=int(row["server_errors"]),
        verify_failures=int(row["verify_failures"]),
        throughput_rps=float(row["throughput_rps"]),
        avg_proofgen_ms=float(row["avg_proofgen_ms"]),
        avg_response_ms=float(row["avg_response_ms"]),
        avg_verify_ms=float(row["avg_verify_ms"]),
        avg_payload_kb=float(row["avg_payload_kb"]),
        csv_path=csv_path,
    )


def load_all_points(root: Path) -> List[ProofBenchmarkPoint]:
    points: List[ProofBenchmarkPoint] = []
    for csv_path in sorted(root.glob("*/proof_range*.csv")):
        # Skip timestamped duplicates like proof_range7000_20260328_221347.csv
        # print(csv_path.stem, csv_path.parent.name, re.findall(r'\d+', csv_path.stem))
        if len(re.findall(r'\d+', csv_path.stem)) != 1:
            print(f"Skipping timestamped duplicate: {csv_path.name}")
            continue
        if csv_path.parent.name not in PROTOCOL_LABELS:
            continue
        # try:
        point = parse_csv(csv_path)
        points.append(point)
        print(
            f"  Loaded: clients={point.num_clients:2d} protocol={point.protocol:12s} "
            f"range={point.range_size:>9,d} throughput={point.throughput_rps:8.1f} rps "
            f"response={point.avg_response_ms:9.1f} ms"
        )
        # except (StopIteration, KeyError, ValueError) as exc:
        #     print(f"Warning: skipping {csv_path}: {exc}")
    return sorted(points, key=lambda p: (p.num_clients, p.protocol, p.range_size))


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


def _ms_formatter(x, pos):
    if x >= 60_000:
        return f"{x / 60_000:.0f}min"
    if x >= 1_000:
        return f"{x / 1_000:.0f}s"
    return f"{x:.10f}".rstrip("0").rstrip(".")


def _ms_unit_formatter(x, pos):
    """Always show values in ms with unit suffix."""
    if x >= 60_000:
        return f"{int(x / 60_000):,}min"
    if x >= 1_000:
        return f"{int(x / 1_000):,}s"
    return f"{x:.10f}".rstrip("0").rstrip(".") + "ms"


def _kb_formatter(x, pos):
    if x >= 1_024:
        return f"{x / 1_024:.1f}MB"
    return f"{x:.10f}".rstrip("0").rstrip(".")


def _kib_formatter(x, pos):
    """Format KiB values as plain numbers."""
    return str(int(x)) if x == int(x) else str(x)


PAYLOAD_YTICKS = [1, 5, 10, 50, 100, 500, 1000, 1500]
THROUGHPUT_YTICKS = [1, 10, 100, 500]
LATENCY_YTICKS = [100, 200, 500, 1_000, 2_000, 5_000]


def _rps_formatter(x, pos):
    if x >= 1_000:
        return f"{int(x / 1_000)}k"
    if x == 0:
        return "0"
    return f"{x:.10f}".rstrip("0").rstrip(".")


def _group_by_protocol(
    points: List[ProofBenchmarkPoint],
    num_clients: int,
) -> Dict[str, List[ProofBenchmarkPoint]]:
    grouped: Dict[str, List[ProofBenchmarkPoint]] = {p: [] for p in PROTOCOL_LABELS}
    for point in points:
        if point.num_clients == num_clients:
            grouped[point.protocol].append(point)
    for proto in grouped:
        grouped[proto].sort(key=lambda p: p.range_size)
    return grouped


def _plot_metric(
    points_by_protocol: Dict[str, List[ProofBenchmarkPoint]],
    y_attr: str,
    ylabel: str,
    y_formatter,
    output_path: Path,
    ylim_top: float | None = None,
    yticks: List[float] | None = None,
    annotate: bool = False,
) -> None:
    configure_plot_style()
    fig, ax = plt.subplots(figsize=(30, 12))

    for protocol, proto_points in points_by_protocol.items():
        if not proto_points:
            continue
        style = PROTOCOL_STYLES[protocol]
        xs = [p.range_size for p in proto_points]
        ys = [y_attr(p) if callable(y_attr) else getattr(p, y_attr) for p in proto_points]
        ax.plot(
            xs, ys,
            marker=style["marker"],
            markersize=25,
            linewidth=10,
            markeredgewidth=2,
            color=style["color"],
            label=style["label"],
        )
        if annotate:
            for x, y in zip(xs, ys):
                label = str(int(y)) if y == int(y) else f"{y:.1f}"
                ax.annotate(
                    label,
                    xy=(x, y),
                    xytext=(0, 18),
                    textcoords="offset points",
                    ha="center",
                    va="bottom",
                    fontsize=plt.rcParams["font.size"] * 0.45,
                    bbox=dict(
                        boxstyle="round,pad=0.2",
                        facecolor="white",
                        edgecolor=style["color"],
                        linewidth=2,
                        alpha=0.85,
                    ),
                )

    ax.set_xscale("log")
    ax.set_yscale("log")
    if ylim_top is not None:
        ax.set_ylim(top=ylim_top)
    if yticks is not None:
        ax.set_yticks(yticks)
        ax.set_yticklabels([str(int(t)) if t == int(t) else str(t) for t in yticks])
        ax.yaxis.set_minor_locator(matplotlib.ticker.NullLocator())
    ax.xaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_range_formatter))
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(y_formatter))
    ax.grid(True, which="both", linestyle="--", linewidth=3, alpha=0.7)
    ax.spines["left"].set_linewidth(5)
    ax.spines["bottom"].set_linewidth(5)
    ax.set_xlabel("Query Range Size")
    ax.set_ylabel(ylabel)

    handles, labels = ax.get_legend_handles_labels()
    fig.legend(
        handles,
        labels,
        loc="upper center",
        bbox_to_anchor=(0.5, 1.02),
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


def _categorical_x(
    points_by_protocol: Dict[str, List[ProofBenchmarkPoint]],
) -> Tuple[List[int], List[str]]:
    """Return sorted unique range sizes and their formatted labels."""
    all_ranges = sorted({
        p.range_size
        for pts in points_by_protocol.values()
        for p in pts
    })
    labels = [_range_formatter(r, None) for r in all_ranges]
    return all_ranges, labels


def _plot_stacked_bar(
    points_by_protocol: Dict[str, List[ProofBenchmarkPoint]],
    output_path: Path,
) -> None:
    """Grouped + stacked bar chart: bottom = proofgen, top = verify."""
    configure_plot_style()

    all_ranges, x_labels = _categorical_x(points_by_protocol)
    x_pos = list(range(len(all_ranges)))
    protocols = [p for p in PROTOCOL_LABELS if points_by_protocol.get(p)]
    n_proto = len(protocols)
    bar_width = 0.8 / n_proto

    fig, ax = plt.subplots(figsize=(40, 12))

    for i, protocol in enumerate(protocols):
        pts = points_by_protocol[protocol]
        range_to_point = {p.range_size: p for p in pts}
        style = PROTOCOL_STYLES[protocol]
        offsets = [xi + (i - n_proto / 2 + 0.5) * bar_width for xi in x_pos]

        valid_offsets, proofgen_vals, verify_vals = [], [], []
        for j, r in enumerate(all_ranges):
            pt = range_to_point.get(r)
            if pt is None:
                continue
            valid_offsets.append(offsets[j])
            proofgen_vals.append(pt.avg_proofgen_ms)
            verify_vals.append(pt.avg_verify_ms)

        ax.bar(valid_offsets, proofgen_vals, width=bar_width,
               color=style["color"], label=style["label"])
        ax.bar(valid_offsets, verify_vals, width=bar_width,
               bottom=proofgen_vals, color=style["color"], alpha=0.45, hatch="///")

    ax.set_xticks(x_pos)
    ax.set_xticklabels(x_labels, rotation=45, ha="right")
    ax.set_yscale("log")
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_ms_formatter))
    ax.grid(True, axis="y", which="both", linestyle="--", linewidth=3, alpha=0.7)
    ax.spines["left"].set_linewidth(5)
    ax.spines["bottom"].set_linewidth(5)
    ax.set_xlabel("Query Range Size")
    ax.set_ylabel("Time (ms)")

    # Protocol legend (line handles from bar)
    proto_handles, proto_labels = ax.get_legend_handles_labels()
    # Texture legend
    texture_handles = [
        mpatches.Patch(facecolor="grey", label="Proof Gen"),
        mpatches.Patch(facecolor="grey", alpha=0.45, hatch="///", label="Verify"),
    ]
    fig.legend(
        proto_handles + texture_handles,
        proto_labels + ["Proof Gen", "Verify"],
        loc="upper center",
        bbox_to_anchor=(0.5, 1.06),
        ncol=n_proto + 2,
        frameon=True,
        edgecolor="black",
        fontsize=plt.rcParams["legend.fontsize"] * 0.75,
        columnspacing=0.3,
    )

    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, format="pdf", bbox_inches="tight")
    plt.close(fig)
    print(f"Saved: {output_path}")


def _plot_bar(
    points_by_protocol: Dict[str, List[ProofBenchmarkPoint]],
    y_attr: str,
    ylabel: str,
    y_formatter,
    output_path: Path,
) -> None:
    """Grouped bar chart for a single metric."""
    configure_plot_style()

    all_ranges, x_labels = _categorical_x(points_by_protocol)
    x_pos = list(range(len(all_ranges)))
    protocols = [p for p in PROTOCOL_LABELS if points_by_protocol.get(p)]
    n_proto = len(protocols)
    bar_width = 0.8 / n_proto

    fig, ax = plt.subplots(figsize=(40, 12))

    for i, protocol in enumerate(protocols):
        pts = points_by_protocol[protocol]
        range_to_point = {p.range_size: p for p in pts}
        style = PROTOCOL_STYLES[protocol]
        offsets = [xi + (i - n_proto / 2 + 0.5) * bar_width for xi in x_pos]

        valid_offsets, ys = [], []
        for j, r in enumerate(all_ranges):
            pt = range_to_point.get(r)
            if pt is None:
                continue
            valid_offsets.append(offsets[j])
            ys.append(getattr(pt, y_attr))

        ax.bar(valid_offsets, ys, width=bar_width,
               color=style["color"], label=style["label"])

    ax.set_xticks(x_pos)
    ax.set_xticklabels(x_labels, rotation=45, ha="right")
    ax.set_yscale("log")
    ax.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(y_formatter))
    ax.grid(True, axis="y", which="both", linestyle="--", linewidth=3, alpha=0.7)
    ax.spines["left"].set_linewidth(5)
    ax.spines["bottom"].set_linewidth(5)
    ax.set_xlabel("Query Range Size")
    ax.set_ylabel(ylabel)

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


# (attr, ylabel, formatter_fn, filename_suffix)
LINE_METRICS: List[Tuple[str, str, object, str]] = [
    ("throughput_rps",  "Throughput (ops/s)",            _rps_formatter, "throughput"),
    ("avg_response_ms", "Avg Response Latency (ms)",     _ms_formatter,  "latency_response"),
    ("avg_proofgen_ms", "Avg Proof Generation Time (ms)",_ms_formatter,  "latency_proofgen"),
    ("avg_verify_ms",   "Avg Verify Time (ms)",          _ms_formatter,  "latency_verify"),
]


def create_all_plots(points: List[ProofBenchmarkPoint], output_dir: Path) -> None:
    all_clients = sorted({p.num_clients for p in points})

    for num_clients in all_clients:
        grouped = _group_by_protocol(points, num_clients)
        clients_dir = output_dir / f"numclients{num_clients}"

        for attr, ylabel, formatter, suffix in LINE_METRICS:
            out = clients_dir / f"dl_query_{suffix}.pdf"
            yticks = THROUGHPUT_YTICKS if attr == "throughput_rps" else None
            _plot_metric(grouped, attr, ylabel, formatter, out, yticks=yticks)

        _plot_stacked_bar(grouped, clients_dir / "dl_query_latency_stacked.pdf")

        _plot_metric(
            grouped,
            lambda p: p.avg_response_ms + p.avg_verify_ms,
            "Latency",
            _ms_unit_formatter,
            clients_dir / "dl_query_latency_response_verify.pdf",
            ylim_top=5_000,
            yticks=LATENCY_YTICKS,
        )

        verkle_pts = sorted(grouped.get("verkle", []), key=lambda p: p.range_size)
        payload_ylim_top = verkle_pts[-1].avg_payload_kb if verkle_pts else None
        _plot_metric(grouped, "avg_payload_kb", "Avg Payload Size (KiB)", _kib_formatter,
                     clients_dir / "dl_query_payload.pdf",
                     ylim_top=payload_ylim_top, yticks=PAYLOAD_YTICKS)


def print_summary(points: List[ProofBenchmarkPoint]) -> None:
    print("\nParsed benchmark points:")
    for p in points:
        print(
            f"  clients={p.num_clients:2d} | {PROTOCOL_LABELS[p.protocol]:6s} | "
            f"range={p.range_size:>9,d} | throughput={p.throughput_rps:8.1f} rps | "
            f"response={p.avg_response_ms:9.1f} ms | proofgen={p.avg_proofgen_ms:9.1f} ms | "
            f"verify={p.avg_verify_ms:9.1f} ms | payload={p.avg_payload_kb:8.1f} KB"
        )


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
    print_summary(points)
    print(f"\nGenerating plots to: {output_dir}")
    create_all_plots(points, output_dir)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
