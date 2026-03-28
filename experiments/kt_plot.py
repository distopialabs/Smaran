#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Iterable, List, Sequence, Tuple

try:
    import matplotlib

    matplotlib.use("Agg")

    import matplotlib.pyplot as plt
    from matplotlib.ticker import LogLocator, NullFormatter
    import seaborn as sns
except ImportError as exc:  # pragma: no cover
    raise SystemExit(
        "This script requires matplotlib and seaborn. "
        "Install them before running `experiments/kt_plot.py`."
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

PROTOCOL_HATCHES = {
    "samurai": "///",
    "optiks": "\\\\\\",
    "coniks": "...",
}

SUMMARY_PATTERNS = {
    "run_duration": re.compile(r"Run phase complete in ([0-9a-zA-Z.µ]+)"),
    "total_requests": re.compile(r"Total requests completed:\s+(\d+)"),
    "avg_latency": re.compile(r"Avg latency:\s+([0-9.]+)\s*(ms|us|µs)"),
    "avg_generation": re.compile(r"Avg proof-gen latency:\s+([0-9.]+)\s*(ms|us|µs)"),
    "avg_verification": re.compile(r"Avg verify latency:\s+([0-9.]+)\s*(ms|us|µs)"),
    "avg_payload": re.compile(r"Avg payload:\s+([0-9.]+)\s*(bytes|byte|B)"),
}

CONIKS_PATTERNS = {
    "run_duration": re.compile(r"Benchmark config: .* d=(\d+) seconds"),
    "interval_requests": re.compile(r"\[t=\d+s\] Requests:\s+(\d+)\s+\|"),
    "throughput": re.compile(r"Throughput:\s+([0-9.]+)\s+requests/second"),
    "avg_latency": re.compile(r"Mean total latency:\s+([0-9.]+|NaN)\s+ms"),
    "avg_generation": re.compile(r"Mean generation time:\s+([0-9.]+|NaN)\s+ms"),
    "avg_verification": re.compile(r"Mean verification time:\s+([0-9.]+|NaN)\s+ms"),
    "avg_response_payload": re.compile(r"Mean response payload:\s+([0-9.]+|NaN)\s+B"),
}


@dataclass(frozen=True)
class ClientSummary:
    total_requests: int
    run_duration_seconds: float
    avg_latency_ms: float
    avg_generation_ms: float
    avg_verification_ms: float
    avg_payload_kib: float


@dataclass(frozen=True)
class BenchmarkPoint:
    protocol: str
    num_versions: int
    total_requests: int
    run_duration_seconds: float
    throughput_qps: float
    avg_latency_ms: float
    avg_generation_ms: float
    avg_verification_ms: float
    avg_payload_kib: float
    run_dir: Path
    sweep_parameters: Dict[str, Any]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Plot KT benchmark query throughput and latency versus number of versions."
        )
    )
    parser.add_argument(
        "input",
        help=(
            "Path to either the exact sweep directory containing benchmark run subdirectories "
            "or a previously generated kt_query_summary.csv file."
        ),
    )
    return parser.parse_args()


def parse_duration_seconds(duration_text: str) -> float:
    duration_text = duration_text.strip()
    if duration_text.endswith("ms"):
        return float(duration_text[:-2]) / 1000.0

    match = re.fullmatch(
        r"(?:(?P<hours>\d+)h)?(?:(?P<minutes>\d+)m)?(?P<seconds>\d+(?:\.\d+)?)s",
        duration_text,
    )
    if match is None:
        raise ValueError(f"Unsupported duration format: {duration_text}")

    hours = int(match.group("hours") or 0)
    minutes = int(match.group("minutes") or 0)
    seconds = float(match.group("seconds"))
    return hours * 3600 + minutes * 60 + seconds


def extract_required_match(pattern_name: str, text: str) -> str:
    match = SUMMARY_PATTERNS[pattern_name].search(text)
    if match is None:
        raise ValueError(f"Could not find `{pattern_name}` in benchmark log.")
    return match.group(1)


def parse_latency_milliseconds(log_text: str) -> float:
    match = SUMMARY_PATTERNS["avg_latency"].search(log_text)
    if match is None:
        raise ValueError("Could not find `avg_latency` in benchmark log.")

    latency_value = float(match.group(1))
    latency_unit = match.group(2)
    if latency_unit == "ms":
        return latency_value
    if latency_unit in {"us", "µs"}:
        return latency_value / 1000.0
    raise ValueError(f"Unsupported latency unit: {latency_unit}")


def parse_named_latency_milliseconds(log_text: str, pattern_name: str) -> float:
    match = SUMMARY_PATTERNS[pattern_name].search(log_text)
    if match is None:
        raise ValueError(f"Could not find `{pattern_name}` in benchmark log.")

    latency_value = float(match.group(1))
    latency_unit = match.group(2)
    if latency_unit == "ms":
        return latency_value
    if latency_unit in {"us", "µs"}:
        return latency_value / 1000.0
    raise ValueError(f"Unsupported latency unit: {latency_unit}")


def parse_payload_kib(log_text: str) -> float:
    match = SUMMARY_PATTERNS["avg_payload"].search(log_text)
    if match is None:
        raise ValueError("Could not find `avg_payload` in benchmark log.")

    payload_value = float(match.group(1))
    payload_unit = match.group(2)
    if payload_unit in {"bytes", "byte", "B"}:
        return payload_value / 1024.0
    raise ValueError(f"Unsupported payload unit: {payload_unit}")


def parse_client_summary(log_path: Path) -> ClientSummary:
    print(f"Processing file: {log_path}")
    log_text = log_path.read_text(encoding="utf-8", errors="replace")
    if "Mean total latency:" in log_text and "Throughput:" in log_text:
        return parse_coniks_client_summary(log_path, log_text)

    run_duration_seconds = parse_duration_seconds(
        extract_required_match("run_duration", log_text)
    )
    total_requests = int(extract_required_match("total_requests", log_text))
    avg_latency_ms = parse_latency_milliseconds(log_text)
    avg_generation_ms = parse_named_latency_milliseconds(log_text, "avg_generation")
    avg_verification_ms = parse_named_latency_milliseconds(log_text, "avg_verification")
    avg_payload_kib = parse_payload_kib(log_text)
    summary = ClientSummary(
        total_requests=total_requests,
        run_duration_seconds=run_duration_seconds,
        avg_latency_ms=avg_latency_ms,
        avg_generation_ms=avg_generation_ms,
        avg_verification_ms=avg_verification_ms,
        avg_payload_kib=avg_payload_kib,
    )
    print(
        "  Extracted values: "
        f"total_requests={summary.total_requests}, "
        f"run_duration_seconds={summary.run_duration_seconds:.6f}, "
        f"avg_latency_ms={summary.avg_latency_ms:.6f}, "
        f"avg_generation_ms={summary.avg_generation_ms:.6f}, "
        f"avg_verification_ms={summary.avg_verification_ms:.6f}, "
        f"avg_payload_kib={summary.avg_payload_kib:.6f}"
    )
    return summary


def parse_coniks_client_summary(log_path: Path, log_text: str) -> ClientSummary:
    run_duration_match = CONIKS_PATTERNS["run_duration"].search(log_text)
    if run_duration_match is None:
        raise ValueError("Could not find coniks run duration in benchmark log.")
    run_duration_seconds = float(run_duration_match.group(1))

    total_requests = sum(
        int(match.group(1))
        for match in CONIKS_PATTERNS["interval_requests"].finditer(log_text)
    )
    if total_requests == 0:
        throughput_match = CONIKS_PATTERNS["throughput"].search(log_text)
        if throughput_match is None:
            raise ValueError("Could not find coniks throughput in benchmark log.")
        total_requests = int(round(float(throughput_match.group(1)) * run_duration_seconds))

    avg_latency_ms = parse_coniks_metric(log_text, "avg_latency")
    avg_generation_ms = parse_coniks_metric(log_text, "avg_generation")
    avg_verification_ms = parse_coniks_metric(log_text, "avg_verification")
    avg_payload_kib = parse_coniks_metric(log_text, "avg_response_payload") / 1024.0

    summary = ClientSummary(
        total_requests=total_requests,
        run_duration_seconds=run_duration_seconds,
        avg_latency_ms=avg_latency_ms,
        avg_generation_ms=avg_generation_ms,
        avg_verification_ms=avg_verification_ms,
        avg_payload_kib=avg_payload_kib,
    )
    print(
        "  Extracted values: "
        f"total_requests={summary.total_requests}, "
        f"run_duration_seconds={summary.run_duration_seconds:.6f}, "
        f"avg_latency_ms={summary.avg_latency_ms:.6f}, "
        f"avg_generation_ms={summary.avg_generation_ms:.6f}, "
        f"avg_verification_ms={summary.avg_verification_ms:.6f}, "
        f"avg_payload_kib={summary.avg_payload_kib:.6f}"
    )
    return summary


def parse_coniks_metric(log_text: str, pattern_name: str) -> float:
    match = CONIKS_PATTERNS[pattern_name].search(log_text)
    if match is None:
        raise ValueError(f"Could not find coniks `{pattern_name}` in benchmark log.")
    return float(match.group(1))


def is_run_dir(path: Path) -> bool:
    return path.is_dir() and (path / "config_used.json").exists() and any(
        child.is_file() and child.name.startswith("node") and child.suffix == ".log"
        for child in path.iterdir()
    )


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
        "avg_latency_ms",
        "avg_generation_ms",
        "avg_verification_ms",
        "avg_payload_kib",
    }
    missing = required_columns - set(df.columns)
    if missing:
        raise ValueError(f"Summary CSV is missing required columns: {sorted(missing)}")

    points: List[BenchmarkPoint] = []
    for row in df.to_dict(orient="records"):
        protocol = str(row["protocol"]).lower()
        if protocol not in PROTOCOL_LABELS:
            raise ValueError(f"Unsupported protocol `{protocol}` in {summary_csv}")
        sweep_parameters = {
            key: row[key]
            for key in row.keys()
            if key not in required_columns and key != "protocol_label"
        }
        points.append(
            BenchmarkPoint(
                protocol=protocol,
                num_versions=int(row["num_versions"]),
                total_requests=int(row["total_requests"]),
                run_duration_seconds=float(row["run_duration_seconds"]),
                throughput_qps=float(row["throughput_qps"]),
                avg_latency_ms=float(row["avg_latency_ms"]),
                avg_generation_ms=float(row["avg_generation_ms"]),
                avg_verification_ms=float(row["avg_verification_ms"]),
                avg_payload_kib=float(row["avg_payload_kib"]),
                run_dir=Path(str(row["run_dir"])),
                sweep_parameters=sweep_parameters,
            )
        )
    if not points:
        raise ValueError(f"No benchmark rows found in {summary_csv}")
    return sorted(points, key=lambda point: (point.protocol, point.num_versions))


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

    client_logs = sorted(
        child
        for child in run_dir.iterdir()
        if child.is_file() and child.name.startswith("node") and child.suffix == ".log"
    )
    if not client_logs:
        raise ValueError(f"No client logs found in {run_dir}")

    client_summaries = [parse_client_summary(log_path) for log_path in client_logs]
    total_requests = sum(summary.total_requests for summary in client_summaries)
    run_duration_seconds = max(summary.run_duration_seconds for summary in client_summaries)

    weighted_latency_sum = sum(
        summary.avg_latency_ms * summary.total_requests for summary in client_summaries
    )
    weighted_generation_sum = sum(
        summary.avg_generation_ms * summary.total_requests
        for summary in client_summaries
    )
    weighted_verification_sum = sum(
        summary.avg_verification_ms * summary.total_requests
        for summary in client_summaries
    )
    weighted_payload_sum = sum(
        summary.avg_payload_kib * summary.total_requests for summary in client_summaries
    )

    if total_requests > 0:
        avg_latency_ms = weighted_latency_sum / total_requests
        avg_generation_ms = weighted_generation_sum / total_requests
        avg_verification_ms = weighted_verification_sum / total_requests
        avg_payload_kib = weighted_payload_sum / total_requests
    else:
        avg_latency_ms = 0.0
        avg_generation_ms = 0.0
        avg_verification_ms = 0.0
        avg_payload_kib = 0.0

    throughput_qps = (
        total_requests / run_duration_seconds if run_duration_seconds > 0 else 0.0
    )

    return BenchmarkPoint(
        protocol=protocol,
        num_versions=num_versions,
        total_requests=total_requests,
        run_duration_seconds=run_duration_seconds,
        throughput_qps=throughput_qps,
        avg_latency_ms=avg_latency_ms,
        avg_generation_ms=avg_generation_ms,
        avg_verification_ms=avg_verification_ms,
        avg_payload_kib=avg_payload_kib,
        run_dir=run_dir,
        sweep_parameters=dict(applied),
    )


def load_points(sweep_root: Path) -> List[BenchmarkPoint]:
    points = [load_benchmark_point(run_dir) for run_dir in iter_run_dirs(sweep_root)]
    if not points:
        raise ValueError(f"No benchmark runs found in {sweep_root}")
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
            "hatch.linewidth": 0.7,
        }
    )

def apply_y_scale(ax: plt.Axes, scale: str) -> None:
    if scale == "symlog":
        ax.set_yscale("symlog", linthresh=1.0, linscale=1.0, base=10)
        ax.set_ylim(bottom=0)
        ax.grid(True, which="major", axis="y", linestyle="--", linewidth=0.5, alpha=0.65)
        ax.grid(True, which="minor", axis="y", linestyle=":", linewidth=0.35, alpha=0.35)
    elif scale == "log":
        ax.set_yscale("log")
        ax.yaxis.set_major_locator(LogLocator(base=10.0))
        ax.yaxis.set_minor_locator(LogLocator(base=10.0, subs=tuple(range(2, 10))))
        ax.yaxis.set_minor_formatter(NullFormatter())
        ax.grid(True, which="major", axis="y", linestyle="--", linewidth=0.5, alpha=0.65)
        ax.grid(True, which="minor", axis="y", linestyle=":", linewidth=0.35, alpha=0.35)
    else:
        ax.grid(True, which="major", axis="y", linestyle="--", linewidth=0.5, alpha=0.65)


def style_axis(ax: plt.Axes, x_values: Sequence[int], *, y_scale: str) -> None:
    x_positions = list(range(len(x_values)))
    ax.set_xticks(x_positions)
    ax.set_xticklabels([str(v) for v in x_values], rotation=30)
    apply_y_scale(ax, y_scale)
    ax.grid(True, which="major", axis="x", linestyle="--", linewidth=0.35, alpha=0.25)


def create_single_plot(
    points: Sequence[BenchmarkPoint],
    output_path: Path,
    *,
    ylabel: str,
    value_attr: str,
    y_scale: str,
) -> None:
    configure_plot_style()

    protocol_styles = PROTOCOL_STYLES

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
        style = protocol_styles[protocol]
        xs = [version_to_index[point.num_versions] for point in protocol_points]
        values = [getattr(point, value_attr) for point in protocol_points]
        label = protocol_styles[protocol]["label"]

        ax.plot(
            xs,
            values,
            marker=style["marker"],
            markersize=4,
            color=style["color"],
            label=label,
        )

    ax.set_ylabel(ylabel)
    ax.set_xlabel("Number of versions")
    style_axis(ax, all_versions, y_scale=y_scale)

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


def create_latency_breakdown_plot(
    points: Sequence[BenchmarkPoint],
    output_path: Path,
) -> None:
    configure_plot_style()

    protocol_styles = {
        protocol: {
            "generation": PROTOCOL_STYLES[protocol]["color"],
            "verification": "#bdbdbd",
        }
        for protocol in PROTOCOL_LABELS
    }

    grouped: Dict[str, List[BenchmarkPoint]] = {protocol: [] for protocol in PROTOCOL_LABELS}
    for point in points:
        grouped[point.protocol].append(point)

    all_versions = sorted({point.num_versions for point in points})
    x_positions = list(range(len(all_versions)))
    version_to_index = {version: index for index, version in enumerate(all_versions)}

    fig, ax = plt.subplots(figsize=(3.8, 2.4))
    protocol_order = list(PROTOCOL_LABELS)
    bar_width = 0.8 / max(1, len(protocol_order))
    offsets = {
        protocol: (index - (len(protocol_order) - 1) / 2) * bar_width
        for index, protocol in enumerate(protocol_order)
    }

    for protocol, protocol_points in grouped.items():
        if not protocol_points:
            continue

        protocol_points = sorted(protocol_points, key=lambda p: p.num_versions)
        style = protocol_styles[protocol]
        xs = [version_to_index[point.num_versions] + offsets[protocol] for point in protocol_points]
        generation_values = [point.avg_generation_ms for point in protocol_points]
        verification_values = [point.avg_verification_ms for point in protocol_points]

        ax.bar(
            xs,
            generation_values,
            width=bar_width,
            color=style["generation"],
            hatch=PROTOCOL_HATCHES[protocol],
            edgecolor="black",
            linewidth=0.5,
            label=f"{PROTOCOL_STYLES[protocol]['label']} generation",
        )
        ax.bar(
            xs,
            verification_values,
            width=bar_width,
            bottom=generation_values,
            color=style["verification"],
            hatch=PROTOCOL_HATCHES[protocol],
            edgecolor="black",
            linewidth=0.5,
            label=f"{PROTOCOL_STYLES[protocol]['label']} verification",
        )

    ax.set_ylabel("Latency (ms)")
    ax.set_xlabel("Number of versions")
    ax.set_xticks(x_positions)
    ax.set_xticklabels([str(version) for version in all_versions], rotation=30)
    apply_y_scale(ax, "symlog")

    handles, labels = ax.get_legend_handles_labels()
    fig.legend(
        handles,
        labels,
        loc="upper center",
        ncol=max(1, min(3, len(labels))),
        frameon=False,
        bbox_to_anchor=(0.5, 1.02),
        columnspacing=0.9,
        handletextpad=0.4,
    )

    sns.despine(fig=fig)
    fig.subplots_adjust(top=0.78, left=0.17, right=0.99, bottom=0.30)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, format="pdf", bbox_inches="tight")
    plt.close(fig)


def create_payload_plot(
    points: Sequence[BenchmarkPoint],
    output_path: Path,
) -> None:
    configure_plot_style()

    protocol_colors = {
        protocol: PROTOCOL_STYLES[protocol]["color"] for protocol in PROTOCOL_LABELS
    }

    grouped: Dict[str, List[BenchmarkPoint]] = {protocol: [] for protocol in PROTOCOL_LABELS}
    for point in points:
        grouped[point.protocol].append(point)

    all_versions = sorted({point.num_versions for point in points})
    x_positions = list(range(len(all_versions)))
    version_to_index = {version: index for index, version in enumerate(all_versions)}

    fig, ax = plt.subplots(figsize=(3.8, 2.3))
    protocol_order = list(PROTOCOL_LABELS)
    bar_width = 0.8 / max(1, len(protocol_order))
    offsets = {
        protocol: (index - (len(protocol_order) - 1) / 2) * bar_width
        for index, protocol in enumerate(protocol_order)
    }

    for protocol, protocol_points in grouped.items():
        if not protocol_points:
            continue

        protocol_points = sorted(protocol_points, key=lambda p: p.num_versions)
        xs = [version_to_index[point.num_versions] + offsets[protocol] for point in protocol_points]
        payload_values = [point.avg_payload_kib for point in protocol_points]

        ax.bar(
            xs,
            payload_values,
            width=bar_width,
            color=protocol_colors[protocol],
            hatch=PROTOCOL_HATCHES[protocol],
            edgecolor="black",
            linewidth=0.5,
            label=PROTOCOL_STYLES[protocol]["label"],
        )

    ax.set_ylabel("Payload (KiB)")
    ax.set_xlabel("Number of versions")
    ax.set_xticks(x_positions)
    ax.set_xticklabels([str(version) for version in all_versions], rotation=30)
    apply_y_scale(ax, "log")

    handles, labels = ax.get_legend_handles_labels()
    fig.legend(
        handles,
        labels,
        loc="upper center",
        ncol=max(1, min(3, len(labels))),
        frameon=False,
        bbox_to_anchor=(0.5, 1.02),
        columnspacing=0.9,
        handletextpad=0.4,
    )

    sns.despine(fig=fig)
    fig.subplots_adjust(top=0.78, left=0.17, right=0.99, bottom=0.30)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output_path, format="pdf", bbox_inches="tight")
    plt.close(fig)


def create_plots(points: Sequence[BenchmarkPoint], output_dir: Path) -> Tuple[Path, Path, Path, Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    throughput_path = output_dir / "kt_query_throughput.pdf"
    latency_path = output_dir / "kt_query_latency.pdf"
    latency_breakdown_path = output_dir / "kt_query_latency_breakdown.pdf"
    payload_path = output_dir / "kt_query_payload.pdf"

    create_single_plot(
        points,
        throughput_path,
        ylabel="Throughput",
        value_attr="throughput_qps",
        y_scale="symlog",
    )
    create_single_plot(
        points,
        latency_path,
        ylabel="Latency (ms)",
        value_attr="avg_latency_ms",
        y_scale="symlog",
    )
    create_latency_breakdown_plot(points, latency_breakdown_path)
    create_payload_plot(points, payload_path)
    return throughput_path, latency_path, latency_breakdown_path, payload_path


def write_summary_csv(points: Sequence[BenchmarkPoint], output_dir: Path) -> Path:
    summary_path = output_dir / "kt_query_summary.csv"
    sweep_keys = sorted({str(key) for point in points for key in point.sweep_parameters})
    fieldnames = [
        "run_dir",
        "protocol",
        "protocol_label",
        "num_versions",
        "total_requests",
        "run_duration_seconds",
        "throughput_qps",
        "avg_latency_ms",
        "avg_generation_ms",
        "avg_verification_ms",
        "avg_payload_kib",
        *sweep_keys,
    ]

    with summary_path.open("w", encoding="utf-8", newline="") as csv_file:
        writer = csv.DictWriter(csv_file, fieldnames=fieldnames)
        writer.writeheader()
        for point in points:
            row = {
                "run_dir": str(point.run_dir),
                "protocol": point.protocol,
                "protocol_label": PROTOCOL_LABELS[point.protocol],
                "num_versions": point.num_versions,
                "total_requests": point.total_requests,
                "run_duration_seconds": f"{point.run_duration_seconds:.6f}",
                "throughput_qps": f"{point.throughput_qps:.6f}",
                "avg_latency_ms": f"{point.avg_latency_ms:.6f}",
                "avg_generation_ms": f"{point.avg_generation_ms:.6f}",
                "avg_verification_ms": f"{point.avg_verification_ms:.6f}",
                "avg_payload_kib": f"{point.avg_payload_kib:.6f}",
            }
            for key in sweep_keys:
                row[key] = point.sweep_parameters.get(key, "")
            writer.writerow(row)

    return summary_path


def print_summary(
    points: Sequence[BenchmarkPoint],
    throughput_path: Path,
    latency_path: Path,
    latency_breakdown_path: Path,
    payload_path: Path,
    summary_path: Path,
) -> None:
    print(f"Saved throughput plot to: {throughput_path}")
    print(f"Saved latency plot to: {latency_path}")
    print(f"Saved latency breakdown plot to: {latency_breakdown_path}")
    print(f"Saved payload plot to: {payload_path}")
    print(f"Saved CSV summary to: {summary_path}")
    print("")
    print("Parsed runs:")
    for point in points:
        print(
            f"  {PROTOCOL_LABELS[point.protocol]:7s} | versions={point.num_versions:4d} "
            f"| throughput={point.throughput_qps:8.2f} qps "
            f"| latency={point.avg_latency_ms:7.2f} ms "
            f"| generation={point.avg_generation_ms:7.2f} ms "
            f"| verification={point.avg_verification_ms:7.2f} ms "
            f"| payload={point.avg_payload_kib:8.2f} KiB"
        )


def main() -> int:
    args = parse_args()
    requested_input = Path(args.input).expanduser().resolve()

    if requested_input.is_file() and requested_input.suffix.lower() == ".csv":
        points = load_points_from_summary_csv(requested_input)
        output_dir = requested_input.parent / "output_log"
    else:
        sweep_root = resolve_sweep_root(requested_input)
        points = load_points(sweep_root)
        output_dir = sweep_root / "output"

    throughput_path, latency_path, latency_breakdown_path, payload_path = create_plots(
        points, output_dir
    )
    summary_path = write_summary_csv(points, output_dir)
    print_summary(
        points,
        throughput_path,
        latency_path,
        latency_breakdown_path,
        payload_path,
        summary_path,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())