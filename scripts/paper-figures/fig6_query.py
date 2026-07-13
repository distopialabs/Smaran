#!/usr/bin/env python3
"""
fig6_query.py — Paper Figures 6a, 6b, 6c: closed-loop query benchmark comparison.

  6a: fig6a_query_latency.pdf   — avg response + verify latency vs query range size
  6b: fig6b_query_throughput.pdf — query throughput vs query range size
  6c: fig6c_payload_size.pdf    — avg response payload size vs query range size

Input layout (root passed as positional arg):
    numclients{N}/{merkle,samuraimpt,verkle}/proof_range{R}.csv
    numclients{N}/cauchy/{Query.csv, proof_range{R}.txt}   (optional)

Cauchy is optional: if absent, its line is simply omitted from all three figures.
One output subdirectory is produced per numclients{N} found (paper used 32).

Trimmed from experiments/dl_query_plot.py (branch kt-put-throughput), keeping only
the three charts used in the paper. Requires matplotlib + a LaTeX installation.
"""
from __future__ import annotations

import argparse
import csv
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Tuple

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker


PROTOCOL_LABELS = {
    "samuraimpt": "Smaran",
    "merkle":     "MPT",
    "verkle":     "Verkle",
    "cauchy":     "Cauchy",
}

PROTOCOL_STYLES = {
    "samuraimpt": {"color": "#0072B2", "marker": "o", "label": "Smaran"},
    "merkle":     {"color": "#D55E00", "marker": "s", "label": "MPT"},
    "verkle":     {"color": "#009E73", "marker": "^", "label": "Verkle"},
    "cauchy":     {"color": "#E69F00", "marker": "P", "label": "Cauchy"},
}


@dataclass(frozen=True)
class ProofBenchmarkPoint:
    protocol: str
    num_clients: int
    range_size: int
    throughput_rps: float
    avg_response_ms: float
    avg_verify_ms: float
    avg_payload_kb: float
    csv_path: Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Plot paper Figures 6a/6b/6c from proof benchmark logs."
    )
    parser.add_argument(
        "input",
        help="Path to the proof logs root directory (contains numclients* dirs).",
    )
    parser.add_argument(
        "--output",
        help="Output directory for plots (default: <input>/output)",
        default=None,
    )
    return parser.parse_args()


def _parse_numeric(s: str) -> float:
    """Strip thousands-separator commas from formatted numbers like '1,390'."""
    return float(s.replace(",", "").strip())


def _parse_payload_kb(s: str) -> float:
    """Parse payload strings like '0.37 KB' or '0.18 MB' into KiB."""
    s = s.strip()
    if s.endswith(" MB"):
        return float(s[:-3]) * 1024
    if s.endswith(" KB"):
        return float(s[:-3])
    return float(s)


def _parse_duration_ms(s: str) -> float:
    """Parse duration strings like '9.23ms', '1.39s', '48m28s' into milliseconds."""
    s = s.strip()
    if s.endswith("ms"):
        return float(s[:-2])
    if "m" in s and s.endswith("s"):
        mins, secs = s[:-1].split("m")
        return float(mins) * 60_000 + float(secs) * 1_000
    if s.endswith("s"):
        return float(s[:-1]) * 1_000
    return float(s)


def _parse_cauchy_txt(txt_path: Path) -> Dict[str, float]:
    """Parse a Cauchy proof_range*.txt file into a metrics dict."""
    data: Dict[str, float] = {}
    with txt_path.open(encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line.startswith("Throughput:"):
                data["throughput_rps"] = float(line.split(":")[1].strip().split()[0])
            elif line.startswith("Avg E2E Latency:"):
                data["avg_response_ms"] = _parse_duration_ms(line.split(":", 1)[1].strip())
            elif line.startswith("Avg Verify:"):
                data["avg_verify_ms"] = _parse_duration_ms(line.split(":", 1)[1].strip())
    return data


def parse_cauchy_csv(csv_path: Path) -> List[ProofBenchmarkPoint]:
    """Parse a Cauchy Query.csv file, augmenting throughput from proof_range*.txt files.

    Path structure: numclients{N}/cauchy/Query.csv
    Columns: Range, Latency (ms), Payload, Verification (ms)
    """
    num_clients = int(csv_path.parent.parent.name.replace("numclients", ""))
    cauchy_dir = csv_path.parent

    metrics_map: Dict[int, Dict[str, float]] = {}
    for txt_path in cauchy_dir.glob("proof_range*.txt"):
        try:
            range_size = int(txt_path.stem.replace("proof_range", ""))
            metrics_map[range_size] = _parse_cauchy_txt(txt_path)
        except (ValueError, OSError):
            pass

    points: List[ProofBenchmarkPoint] = []
    with csv_path.open(encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            if not row.get("Range", "").strip():
                continue
            range_size = int(row["Range"])
            m = metrics_map.get(range_size, {})
            points.append(ProofBenchmarkPoint(
                protocol="cauchy",
                num_clients=num_clients,
                range_size=range_size,
                throughput_rps=m.get("throughput_rps", 0.0),
                avg_response_ms=m.get("avg_response_ms", _parse_numeric(row["Latency (ms)"])),
                avg_verify_ms=m.get("avg_verify_ms", _parse_numeric(row["Verification (ms)"])),
                avg_payload_kb=_parse_payload_kb(row["Payload"]),
                csv_path=csv_path,
            ))
    return points


def parse_csv(csv_path: Path) -> ProofBenchmarkPoint:
    """Parse a single proof_range*.csv file.

    Path structure: numclients{N}/{protocol}/proof_range{R}.csv
    """
    protocol = csv_path.parent.name
    num_clients = int(csv_path.parent.parent.name.replace("numclients", ""))

    with csv_path.open(encoding="utf-8") as f:
        reader = csv.DictReader(f)
        row = next(reader)

    return ProofBenchmarkPoint(
        protocol=protocol,
        num_clients=num_clients,
        range_size=int(row["range_size"]),
        throughput_rps=float(row["throughput_rps"]),
        avg_response_ms=float(row["avg_response_ms"]),
        avg_verify_ms=float(row["avg_verify_ms"]),
        avg_payload_kb=float(row["avg_payload_kb"]),
        csv_path=csv_path,
    )


def load_all_points(root: Path) -> List[ProofBenchmarkPoint]:
    points: List[ProofBenchmarkPoint] = []

    for csv_path in sorted(root.glob("numclients*/*/proof_range*.csv")):
        # Skip timestamped duplicates like proof_range7000_20260328_221347.csv
        if len(csv_path.stem.split("_")) > 2:
            print(f"Skipping timestamped duplicate: {csv_path.name}")
            continue
        if csv_path.parent.name not in PROTOCOL_LABELS:
            continue
        try:
            point = parse_csv(csv_path)
            points.append(point)
            print(
                f"  Loaded: clients={point.num_clients:2d} protocol={point.protocol:12s} "
                f"range={point.range_size:>9,d} throughput={point.throughput_rps:8.1f} rps "
                f"response={point.avg_response_ms:9.1f} ms"
            )
        except (StopIteration, KeyError, ValueError) as exc:
            print(f"Warning: skipping {csv_path}: {exc}")

    for csv_path in sorted(root.glob("numclients*/cauchy/Query.csv")):
        try:
            cauchy_points = parse_cauchy_csv(csv_path)
            for point in cauchy_points:
                points.append(point)
                print(
                    f"  Loaded: clients={point.num_clients:2d} protocol={point.protocol:12s} "
                    f"range={point.range_size:>9,d} response={point.avg_response_ms:9.1f} ms"
                )
        except (StopIteration, KeyError, ValueError) as exc:
            print(f"Warning: skipping {csv_path}: {exc}")

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


def _ms_unit_formatter(x, pos):
    """Always show values in ms with unit suffix."""
    if x >= 3600_000:
        return f"{int(x / 3600_000):,}h"
    if x >= 60_000:
        return f"{int(x / 60_000):,}min"
    if x >= 1_000:
        return f"{int(x / 1_000):,}s"
    return f"{x:.10f}".rstrip("0").rstrip(".") + "ms"


def _kib_formatter(x, pos):
    """Format KiB values as plain numbers."""
    return str(int(x)) if x == int(x) else str(x)


def _rps_formatter(x, pos):
    if x >= 1_000:
        return f"{int(x / 1_000)}k"
    if x == 0:
        return "0"
    return f"{x:.10f}".rstrip("0").rstrip(".")


PAYLOAD_YTICKS = [1, 10, 100, 1000, 10_000]
THROUGHPUT_YTICKS = [0.0001, 0.001, 0.01, 0.1, 1, 10, 100, 1000, 10_000]
LATENCY_YTICKS = [10, 100, 1_000, 10_000, 60_000, 600_000, 3600_000, 100_000_000]


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
    y_attr,
    ylabel: str,
    y_formatter,
    output_path: Path,
    ylim_top: float | None = None,
    yticks: List[float] | None = None,
) -> None:
    configure_plot_style()
    fig, ax = plt.subplots(figsize=(30, 12))

    y_seen: List[float] = []
    for protocol, proto_points in points_by_protocol.items():
        if not proto_points:
            continue
        style = PROTOCOL_STYLES[protocol]
        xs = [p.range_size for p in proto_points]
        ys = [y_attr(p) if callable(y_attr) else getattr(p, y_attr) for p in proto_points]
        # Drop zero/non-positive values (e.g. throughput not measured for Cauchy)
        valid = [(x, y) for x, y in zip(xs, ys) if y > 0]
        if not valid:
            continue
        xs, ys = zip(*valid)
        y_seen.extend(ys)
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
    if yticks is not None:
        # The fixed tick lists span the paper's full-scale data (Cauchy
        # reaches 27 h latencies and 1e-4 ops/s). Quick-tier data covers a
        # fraction of that, and keeping every tick would stretch the axis
        # over the unused decades — trim to the decades the plotted data
        # occupies, then size the limits from data and surviving ticks.
        # Full-scale data spans the whole list, so nothing changes there.
        if y_seen:
            lo, hi = min(y_seen) / 3, max(y_seen) * 3
            trimmed = [t for t in yticks if lo <= t <= hi]
            if len(trimmed) >= 2:
                yticks = trimmed
        ax.set_yticks(yticks)
        ax.set_yticklabels([str(int(t)) if t == int(t) else str(t) for t in yticks])
        ax.yaxis.set_minor_locator(matplotlib.ticker.NullLocator())
        if y_seen:
            top = max(y_seen) * 1.3
            if ylim_top is not None:
                top = min(top, ylim_top)   # deliberate clip (payload panel)
            top = max(top, max(yticks))
            ax.set_ylim(bottom=min(min(y_seen) / 1.3, min(yticks)), top=top)
    elif ylim_top is not None:
        ax.set_ylim(top=ylim_top)
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


def create_all_plots(points: List[ProofBenchmarkPoint], output_dir: Path) -> None:
    all_clients = sorted({p.num_clients for p in points})

    for num_clients in all_clients:
        grouped = _group_by_protocol(points, num_clients)
        clients_dir = output_dir / f"numclients{num_clients}"

        # Figure 6a: avg response + verify latency
        _plot_metric(
            grouped,
            lambda p: p.avg_response_ms + p.avg_verify_ms,
            "Latency",
            _ms_unit_formatter,
            clients_dir / "fig6a_query_latency.pdf",
            ylim_top=100_000_000,
            yticks=LATENCY_YTICKS,
        )

        # Figure 6b: query throughput
        _plot_metric(
            grouped,
            "throughput_rps",
            "Throughput (ops/s)",
            _rps_formatter,
            clients_dir / "fig6b_query_throughput.pdf",
            yticks=THROUGHPUT_YTICKS,
        )

        # Figure 6c: response payload size (y-limit anchored to Verkle's largest payload)
        verkle_pts = sorted(grouped.get("verkle", []), key=lambda p: p.range_size)
        payload_ylim_top = verkle_pts[-1].avg_payload_kb if verkle_pts else None
        _plot_metric(
            grouped,
            "avg_payload_kb",
            "Avg Payload Size (KiB)",
            _kib_formatter,
            clients_dir / "fig6c_payload_size.pdf",
            ylim_top=payload_ylim_top,
            yticks=PAYLOAD_YTICKS,
        )


def print_summary(points: List[ProofBenchmarkPoint]) -> None:
    print("\nParsed benchmark points:")
    for p in points:
        print(
            f"  clients={p.num_clients:2d} | {PROTOCOL_LABELS[p.protocol]:6s} | "
            f"range={p.range_size:>9,d} | throughput={p.throughput_rps:8.1f} rps | "
            f"response={p.avg_response_ms:9.1f} ms | "
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
