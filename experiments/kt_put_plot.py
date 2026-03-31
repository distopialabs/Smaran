#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker


PROTOCOL_LABELS = {
    "samurai": "Smaran",
    "optiks": "Optiks",
    "coniks": "Coniks",
}

PROTOCOL_STYLES = {
    "samurai": {"color": "#0072B2", "marker": "o", "label": "Smaran"},
    "optiks":  {"color": "#D55E00", "marker": "s", "label": "Optiks"},
    "coniks":  {"color": "#009E73", "marker": "^", "label": "Coniks"},
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
    num_users: int
    num_versions: int
    total_requests: int
    run_duration_seconds: float
    throughput_qps: float
    run_dir: Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Plot KT benchmark PUT throughput versus number of users."
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
    num_users = int(
        applied.get("bench_num_users", experiment.get("bench_num_users", 0))
    )
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
        num_users=num_users,
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
    return sorted(points, key=lambda point: (point.protocol, point.num_users))


def load_points_from_summary_csv(summary_csv: Path) -> List[BenchmarkPoint]:
    import pandas as pd

    df = pd.read_csv(summary_csv)
    required_columns = {
        "run_dir",
        "protocol",
        "num_users",
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
                num_users=int(row["num_users"]),
                num_versions=int(row["num_versions"]),
                total_requests=int(row["total_requests"]),
                run_duration_seconds=float(row["run_duration_seconds"]),
                throughput_qps=float(row["throughput_qps"]),
                run_dir=Path(str(row["run_dir"])),
            )
        )
    if not points:
        raise ValueError(f"No benchmark rows found in {summary_csv}")
    return sorted(points, key=lambda point: (point.protocol, point.num_users))


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


def create_throughput_plot(
    points: Sequence[BenchmarkPoint],
    output_path: Path,
) -> None:
    configure_plot_style()

    def _x_formatter(x, pos):
        x = int(x)
        if x >= 100_000:
            m = x / 1_000_000
            if m < 1:
                return f"0.{int(m * 10)}M"
            return f"{m:g}M"
        if x >= 1_000:
            return f"{int(x / 1000)}k"
        return str(x)

    def _y_formatter(x, pos):
        if x >= 1000:
            return f"{int(x / 1000)}k"
        if x == 0:
            return "0"
        return f"{x:.10f}".rstrip("0").rstrip(".")

    grouped: Dict[str, List[BenchmarkPoint]] = {protocol: [] for protocol in PROTOCOL_LABELS}
    for point in points:
        grouped[point.protocol].append(point)

    all_users = sorted({point.num_users for point in points})

    _BRK_MARKER = [(-1, -1), (1, 1)]
    _BRK_KW = dict(markersize=20, color="k", mec="k", mew=3, clip_on=False, linestyle="none")

    fig, (ax_top, ax_bot) = plt.subplots(
        2, 1, sharex=True, figsize=(30, 12),
        gridspec_kw={"height_ratios": [1, 1]},
    )
    fig.subplots_adjust(hspace=0.2)

    for ax in (ax_top, ax_bot):
        for protocol, protocol_points in grouped.items():
            if not protocol_points:
                continue
            protocol_points = sorted(protocol_points, key=lambda p: p.num_users)
            style = PROTOCOL_STYLES[protocol]
            xs = [point.num_users for point in protocol_points]
            ys = [point.throughput_qps for point in protocol_points]
            ax.plot(
                xs, ys,
                marker=style["marker"],
                markersize=25,
                linewidth=10,
                markeredgewidth=2,
                color=style["color"],
                label=style["label"],
            )

    ax_top.set_ylim(30_000, 120_000)
    ax_bot.set_ylim(0, 700)

    ax_top.set_yticks([30_000, 60_000, 90_000, 120_000])
    ax_bot.set_yticks([0, 200, 400, 600])

    ax_top.spines["bottom"].set_visible(False)
    ax_bot.spines["top"].set_visible(False)
    ax_top.tick_params(bottom=False)

    # y-break diagonal markers
    ax_top.plot([0], [0], marker=_BRK_MARKER, transform=ax_top.transAxes, **_BRK_KW)
    ax_bot.plot([0], [1], marker=_BRK_MARKER, transform=ax_bot.transAxes, **_BRK_KW)

    ax_top.set_xscale("log")
    ax_top.set_xticks(all_users)
    ax_top.xaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_x_formatter))
    ax_top.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter))
    ax_bot.yaxis.set_major_formatter(matplotlib.ticker.FuncFormatter(_y_formatter))
    ax_top.grid(True, which="both", linestyle="--", linewidth=3, alpha=0.7)
    ax_bot.grid(True, which="both", linestyle="--", linewidth=3, alpha=0.7)

    for ax in (ax_top, ax_bot):
        ax.spines["left"].set_linewidth(5)
        ax.spines["bottom"].set_linewidth(5)

    fig.supylabel("Throughput (ops/s)", x=0.025)
    ax_bot.set_xlabel("Number of Users")

    handles, labels = ax_top.get_legend_handles_labels()
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
    print(f"Saved throughput plot to: {output_path}")


def write_summary_csv(points: Sequence[BenchmarkPoint], output_dir: Path) -> Path:
    summary_path = output_dir / "kt_put_summary.csv"
    fieldnames = [
        "run_dir",
        "protocol",
        "protocol_label",
        "num_users",
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
                    "num_users": point.num_users,
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
            f"  {PROTOCOL_LABELS[point.protocol]:7s} | users={point.num_users:8d} "
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
