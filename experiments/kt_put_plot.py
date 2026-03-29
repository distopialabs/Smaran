#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple

try:
    import matplotlib

    matplotlib.use("Agg")

    import matplotlib.pyplot as plt
    import seaborn as sns
except ImportError as exc:  # pragma: no cover
    raise SystemExit(
        "This script requires matplotlib and seaborn. "
        "Install them before running `experiments/kt_put_plot.py`."
    ) from exc


PROTOCOL_LABELS = {
    "samurai": "Smaran",
    "optiks": "Optiks",
    "coniks": "Coniks",
}

PROTOCOL_STYLES = {
    "samurai": {"color": "#e66101", "marker": "o", "label": "Smaran"},
    "optiks": {"color": "#5e3c99", "marker": "s", "label": "Optiks"},
    "coniks": {"color": "#4daf4a", "marker": "^", "label": "Coniks"},
}

RUN_PHASE_START_PATTERN = re.compile(r"RUN_PHASE_START_UNIX_NANO=(\d+)")
RUN_PHASE_END_PATTERN = re.compile(r"RUN_PHASE_END_UNIX_NANO=(\d+)")
TOTAL_REQUESTS_PATTERN = re.compile(r"Total requests completed:\s+(\d+)")

CONIKS_PATTERNS = {
    "run_duration": re.compile(r"Benchmark config: .* d=(\d+) seconds"),
    "interval_requests": re.compile(r"\[t=\d+s\] Requests:\s+(\d+)\s+\|"),
    "throughput": re.compile(r"Throughput:\s+([0-9.]+)\s+requests/second"),
}


@dataclass(frozen=True)
class BenchmarkPoint:
    protocol: str
    num_versions: int
    total_requests: int
    run_duration_seconds: float
    throughput_qps: float
    run_dir: Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Plot KT benchmark PUT throughput versus number of versions."
        )
    )
    parser.add_argument(
        "input",
        help=(
            "Path to either the sweep directory containing benchmark run subdirectories "
            "or a previously generated kt_put_summary.csv file."
        ),
    )
    return parser.parse_args()


def parse_node2_log(log_path: Path) -> Tuple[int, float]:
    """Return (total_requests, run_duration_seconds) from a put-mode node2.log."""
    print(f"Processing file: {log_path}")
    log_text = log_path.read_text(encoding="utf-8", errors="replace")

    if "Benchmark config:" in log_text and "Throughput:" in log_text:
        return parse_coniks_put_log(log_path, log_text)

    start_match = RUN_PHASE_START_PATTERN.search(log_text)
    end_match = RUN_PHASE_END_PATTERN.search(log_text)
    requests_match = TOTAL_REQUESTS_PATTERN.search(log_text)

    if start_match is None:
        raise ValueError(f"Could not find RUN_PHASE_START_UNIX_NANO in {log_path}")
    if end_match is None:
        raise ValueError(f"Could not find RUN_PHASE_END_UNIX_NANO in {log_path}")
    if requests_match is None:
        raise ValueError(f"Could not find 'Total requests completed' in {log_path}")

    start_ns = int(start_match.group(1))
    end_ns = int(end_match.group(1))
    total_requests = int(requests_match.group(1))
    run_duration_seconds = (end_ns - start_ns) / 1e9

    print(
        f"  Extracted values: "
        f"total_requests={total_requests}, "
        f"run_duration_seconds={run_duration_seconds:.6f}"
    )
    return total_requests, run_duration_seconds


def parse_coniks_put_log(log_path: Path, log_text: str) -> Tuple[int, float]:
    """Return (total_requests, run_duration_seconds) from a coniks put-mode log."""
    run_duration_match = CONIKS_PATTERNS["run_duration"].search(log_text)
    if run_duration_match is None:
        raise ValueError(f"Could not find coniks run duration in {log_path}")
    run_duration_seconds = float(run_duration_match.group(1))

    total_requests = sum(
        int(match.group(1))
        for match in CONIKS_PATTERNS["interval_requests"].finditer(log_text)
    )
    if total_requests == 0:
        throughput_match = CONIKS_PATTERNS["throughput"].search(log_text)
        if throughput_match is None:
            raise ValueError(f"Could not find coniks throughput in {log_path}")
        total_requests = int(round(float(throughput_match.group(1)) * run_duration_seconds))

    print(
        f"  Extracted values: "
        f"total_requests={total_requests}, "
        f"run_duration_seconds={run_duration_seconds:.6f}"
    )
    return total_requests, run_duration_seconds


def is_run_dir(path: Path) -> bool:
    return path.is_dir() and (path / "config_used.json").exists() and any(
        child.is_file() and child.name.startswith("node") and child.suffix == ".log"
        for child in path.iterdir()
    )


def iter_run_dirs(sweep_root: Path) -> Iterable[Path]:
    return sorted(child for child in sweep_root.iterdir() if is_run_dir(child))


def load_benchmark_point(run_dir: Path) -> BenchmarkPoint:
    config = json.loads((run_dir / "config_used.json").read_text(encoding="utf-8"))
    experiment = config.get("experiment", {})
    applied = config.get("applied_sweeping_parameters", {})

    protocol = str(applied.get("bench_protocol", experiment.get("bench_protocol", ""))).lower()
    num_versions = int(
        applied.get("bench_num_versions", experiment.get("bench_num_versions", 0))
    )
    if protocol not in PROTOCOL_LABELS:
        raise ValueError(f"Unsupported protocol `{protocol}` in {run_dir}")

    node2_log = run_dir / "node2.log"
    if not node2_log.exists():
        raise FileNotFoundError(f"node2.log not found in {run_dir}")

    total_requests, run_duration_seconds = parse_node2_log(node2_log)
    throughput_qps = total_requests / run_duration_seconds if run_duration_seconds > 0 else 0.0

    return BenchmarkPoint(
        protocol=protocol,
        num_versions=num_versions,
        total_requests=total_requests,
        run_duration_seconds=run_duration_seconds,
        throughput_qps=throughput_qps,
        run_dir=run_dir,
    )


def resolve_sweep_root(input_path: Path) -> Path:
    if not input_path.exists():
        raise FileNotFoundError(f"Input path does not exist: {input_path}")
    if not input_path.is_dir():
        raise NotADirectoryError(f"Input path is not a directory: {input_path}")
    if is_run_dir(input_path):
        raise ValueError(
            "Expected a sweep directory containing multiple run directories, "
            "not a single run directory."
        )
    if not any(is_run_dir(child) for child in input_path.iterdir() if child.is_dir()):
        raise FileNotFoundError(
            f"No benchmark run directories found directly under {input_path}"
        )
    return input_path


def load_points(sweep_root: Path) -> List[BenchmarkPoint]:
    points = [load_benchmark_point(run_dir) for run_dir in iter_run_dirs(sweep_root)]
    if not points:
        raise ValueError(f"No benchmark runs found in {sweep_root}")
    return sorted(points, key=lambda point: (point.protocol, point.num_versions))


def load_points_from_summary_csv(summary_csv: Path) -> List[BenchmarkPoint]:
    import pandas as pd

    df = pd.read_csv(summary_csv)
    required_columns = {
        "run_dir",
        "protocol",
        "num_versions",
        "total_requests",
        "run_duration_seconds",
        "throughput_qps",
    }
    missing = required_columns - set(df.columns)
    if missing:
        raise ValueError(f"Summary CSV is missing required columns: {sorted(missing)}")

    points: List[BenchmarkPoint] = []
    for row in df.to_dict(orient="records"):
        protocol = str(row["protocol"]).lower()
        if protocol not in PROTOCOL_LABELS:
            raise ValueError(f"Unsupported protocol `{protocol}` in {summary_csv}")
        points.append(
            BenchmarkPoint(
                protocol=protocol,
                num_versions=int(row["num_versions"]),
                total_requests=int(row["total_requests"]),
                run_duration_seconds=float(row["run_duration_seconds"]),
                throughput_qps=float(row["throughput_qps"]),
                run_dir=Path(str(row["run_dir"])),
            )
        )
    if not points:
        raise ValueError(f"No benchmark rows found in {summary_csv}")
    return sorted(points, key=lambda point: (point.protocol, point.num_versions))


def configure_plot_style() -> None:
    sns.set_theme(style="whitegrid", context="paper")
    plt.rcParams.update(
        {
            "font.family": "serif",
            "font.size": 10,
            "axes.titlesize": 11,
            "axes.labelsize": 12,
            "xtick.labelsize": 10,
            "ytick.labelsize": 10,
            "legend.fontsize": 9,
            "pdf.fonttype": 42,
            "ps.fonttype": 42,
            "lines.linewidth": 1.6,
        }
    )


def create_throughput_plot(
    points: Sequence[BenchmarkPoint],
    output_path: Path,
) -> None:
    configure_plot_style()

    grouped: Dict[str, List[BenchmarkPoint]] = {protocol: [] for protocol in PROTOCOL_LABELS}
    for point in points:
        grouped[point.protocol].append(point)

    all_versions = sorted({point.num_versions for point in points})
    version_to_index = {version: index for index, version in enumerate(all_versions)}

    fig, ax = plt.subplots(figsize=(3.6, 2.2))

    for protocol, protocol_points in grouped.items():
        if not protocol_points:
            continue

        protocol_points = sorted(protocol_points, key=lambda p: p.num_versions)
        style = PROTOCOL_STYLES[protocol]
        xs = [version_to_index[point.num_versions] for point in protocol_points]
        ys = [point.throughput_qps for point in protocol_points]

        ax.plot(
            xs,
            ys,
            marker=style["marker"],
            markersize=4,
            color=style["color"],
            label=style["label"],
        )

    ax.set_ylabel("Throughput (req/s)")
    ax.set_xlabel("Number of versions")

    x_positions = list(range(len(all_versions)))
    ax.set_xticks(x_positions)
    ax.set_xticklabels([str(v) for v in all_versions], rotation=30)
    ax.grid(True, which="major", axis="y", linestyle="--", linewidth=0.5, alpha=0.65)
    ax.grid(True, which="major", axis="x", linestyle="--", linewidth=0.35, alpha=0.25)

    handles, labels = ax.get_legend_handles_labels()
    fig.legend(
        handles,
        labels,
        loc="upper center",
        ncol=max(1, len(labels)),
        frameon=False,
        bbox_to_anchor=(0.5, 1.01),
        columnspacing=1.1,
        handletextpad=0.4,
    )

    sns.despine(fig=fig)
    fig.subplots_adjust(top=0.84, left=0.17, right=0.99, bottom=0.30)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, format="pdf", bbox_inches="tight")
    plt.close(fig)
    print(f"Saved throughput plot to: {output_path}")


def write_summary_csv(points: Sequence[BenchmarkPoint], output_dir: Path) -> Path:
    summary_path = output_dir / "kt_put_summary.csv"
    fieldnames = [
        "run_dir",
        "protocol",
        "protocol_label",
        "num_versions",
        "total_requests",
        "run_duration_seconds",
        "throughput_qps",
    ]

    with summary_path.open("w", encoding="utf-8", newline="") as csv_file:
        writer = csv.DictWriter(csv_file, fieldnames=fieldnames)
        writer.writeheader()
        for point in points:
            writer.writerow(
                {
                    "run_dir": str(point.run_dir),
                    "protocol": point.protocol,
                    "protocol_label": PROTOCOL_LABELS[point.protocol],
                    "num_versions": point.num_versions,
                    "total_requests": point.total_requests,
                    "run_duration_seconds": f"{point.run_duration_seconds:.6f}",
                    "throughput_qps": f"{point.throughput_qps:.6f}",
                }
            )

    return summary_path


def print_summary(points: Sequence[BenchmarkPoint], summary_path: Path) -> None:
    print(f"Saved CSV summary to: {summary_path}")
    print("")
    print("Parsed runs:")
    for point in points:
        print(
            f"  {PROTOCOL_LABELS[point.protocol]:7s} | versions={point.num_versions:4d} "
            f"| throughput={point.throughput_qps:12.2f} req/s "
            f"| total_requests={point.total_requests:10d} "
            f"| duration={point.run_duration_seconds:.3f}s"
        )


def main() -> int:
    args = parse_args()
    requested_input = Path(args.input).expanduser().resolve()

    if requested_input.is_file() and requested_input.suffix.lower() == ".csv":
        points = load_points_from_summary_csv(requested_input)
        output_dir = requested_input.parent / "output_put"
    else:
        sweep_root = resolve_sweep_root(requested_input)
        points = load_points(sweep_root)
        output_dir = sweep_root / "output"

    output_dir.mkdir(parents=True, exist_ok=True)
    throughput_path = output_dir / "kt_put_throughput.pdf"
    create_throughput_plot(points, throughput_path)
    summary_path = write_summary_csv(points, output_dir)
    print_summary(points, summary_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
