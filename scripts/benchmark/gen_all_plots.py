#!/usr/bin/env python3
"""
Auto-discover benchmark data and generate all plots via plot_bench.py.

Usage:
    python3 gen_all_plots.py [--data-dir .] [--output-dir plots] [--format pdf] \
                             [--warmup 15] [--cooldown 15] [--dry-run]

Expects data layout:
    <data-dir>/<protocol>/ingestion_<k>.csv
    <data-dir>/<protocol>/proof_range<N>.csv
    (k=0 means "full run" and is excluded from summary plots)
"""

import argparse
import os
import re
import shlex
import subprocess
import sys


def discover_ingestion_data(data_dir: str) -> dict[str, dict[int, str]]:
    """Scan data_dir for protocol subdirectories containing ingestion CSVs.

    Returns {"samurai": {1: "samurai/ingestion_1.csv", ...}, ...}
    """
    result = {}
    for entry in sorted(os.listdir(data_dir)):
        subdir = os.path.join(data_dir, entry)
        if not os.path.isdir(subdir) or entry.startswith("."):
            continue
        files = {}
        for fname in os.listdir(subdir):
            m = re.match(r"^ingestion_(\d+)\.csv$", fname)
            if m:
                k = int(m.group(1))
                files[k] = os.path.join(entry, fname)
        if files:
            result[entry] = files
    return result


def discover_proof_data(data_dir: str) -> dict[str, dict[int, str]]:
    """Scan data_dir for protocol subdirectories containing proof_range CSV files.

    Returns {"samurai": {1: "samurai/proof_range1.csv", ...}, ...}
    """
    result = {}
    for entry in sorted(os.listdir(data_dir)):
        subdir = os.path.join(data_dir, entry)
        if not os.path.isdir(subdir) or entry.startswith("."):
            continue
        files = {}
        for fname in os.listdir(subdir):
            m = re.match(r"^proof_range(\d+)\.csv$", fname)
            if m:
                r = int(m.group(1))
                files[r] = os.path.join(entry, fname)
        if files:
            result[entry] = files
    return result


def discover_openloop_data(data_dir: str) -> dict[str, list[str]]:
    """Scan data_dir for protocol subdirectories containing openloop CSV files.

    Returns {"samuraimpt": ["samuraimpt/openloop_range1000_clients4_20260327.csv", ...], ...}
    """
    result = {}
    for entry in sorted(os.listdir(data_dir)):
        subdir = os.path.join(data_dir, entry)
        if not os.path.isdir(subdir) or entry.startswith("."):
            continue
        files = []
        for fname in sorted(os.listdir(subdir)):
            if re.match(r"^openloop_range\d+_clients\d+.*\.csv$", fname):
                files.append(os.path.join(entry, fname))
        if files:
            result[entry] = files
    return result


def gen_openloop_timeseries_commands(data: dict, args) -> list[list[str]]:
    """Generate one proof-throughput-timeseries command per openloop CSV."""
    commands = []
    for proto, files in sorted(data.items()):
        for fpath in files:
            fname_base = os.path.splitext(os.path.basename(fpath))[0]
            output_dir = os.path.join(args.output_dir, f"openloop_timeseries/{fname_base}")
            cmd = [
                sys.executable, "plot_bench.py", "proof-throughput-timeseries",
                "--output-dir", output_dir,
                "--warmup", str(args.warmup),
                "--cooldown", str(args.cooldown),
                "--format", args.format,
                "--window", str(args.window),
                "--input", f"{proto}:{fpath}",
            ]
            commands.append(cmd)
    return commands


def gen_proof_summary_command(data: dict, args) -> list[str]:
    """Generate a single proof-summary command."""
    cmd = [
        sys.executable, "plot_bench.py", "proof-summary",
        "--output-dir", os.path.join(args.output_dir, "proof_summary"),
        "--format", args.format,
    ]
    for proto in sorted(data):
        entries = sorted(data[proto].items())
        if not entries:
            continue
        cmd.extend(["--protocol", proto])
        inputs_str = ",".join(f"{r}:{path}" for r, path in entries)
        cmd.extend(["--inputs", inputs_str])
    return cmd


def _k_label(k: int) -> str:
    """Human-readable label for output directory names."""
    if k == 0:
        return "all"
    if k >= 1_000_000 and k % 1_000_000 == 0:
        return f"{k // 1_000_000}m"
    if k >= 1_000 and k % 1_000 == 0:
        return f"{k // 1_000}k"
    return str(k)


def gen_timeseries_commands(data: dict, args) -> list[list[str]]:
    """Generate one ingestion-timeseries command per k-value."""
    all_k = sorted(set(k for proto in data.values() for k in proto))
    commands = []
    for k in all_k:
        inputs = []
        for proto in sorted(data):
            if k in data[proto]:
                inputs.extend(["--input", f"{proto}:{data[proto][k]}"])
        if not inputs:
            continue
        label = _k_label(k)
        output_dir = os.path.join(args.output_dir, f"ingestion_timeseries/{label}")
        cmd = [
            sys.executable, "plot_bench.py", "ingestion-timeseries",
            "--output-dir", output_dir,
            "--warmup", str(args.warmup),
            "--cooldown", str(args.cooldown),
            "--format", args.format,
            "--window", str(args.window),
            *inputs,
        ]
        commands.append(cmd)
    return commands


def gen_summary_command(data: dict, args) -> list[str]:
    """Generate a single ingestion-summary command (excludes k=0)."""
    cmd = [
        sys.executable, "plot_bench.py", "ingestion-summary",
        "--output-dir", os.path.join(args.output_dir, "ingestion_summary"),
        "--warmup", str(args.warmup),
        "--cooldown", str(args.cooldown),
        "--format", args.format,
    ]
    for proto in sorted(data):
        entries = sorted((k, path) for k, path in data[proto].items() if k != 0)
        if not entries:
            continue
        cmd.extend(["--protocol", proto])
        inputs_str = ",".join(f"{k}:{path}" for k, path in entries)
        cmd.extend(["--inputs", inputs_str])
    return cmd


def _fmt_cmd(cmd: list[str]) -> str:
    """Format a command list as a copy-pasteable shell string."""
    return " \\\n  ".join(shlex.quote(c) for c in cmd)


def main():
    parser = argparse.ArgumentParser(
        description="Auto-discover benchmark data and generate all plots.",
    )
    parser.add_argument("--data-dir", default=".",
                        help="Root directory with protocol subdirs (default: .)")
    parser.add_argument("--output-dir", default="plots",
                        help="Base output directory (default: plots)")
    parser.add_argument("--format", choices=["pdf", "png"], default="pdf",
                        help="Output format (default: pdf)")
    parser.add_argument("--warmup", type=float, default=15.0,
                        help="Seconds to trim from start (default: 15)")
    parser.add_argument("--cooldown", type=float, default=15.0,
                        help="Seconds to trim from end (default: 15)")
    parser.add_argument("--window", type=float, default=5.0,
                        help="Rolling window for timeseries (default: 5.0)")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print commands without running them")
    parser.add_argument("--skip-timeseries", action="store_true",
                        help="Skip G1-G6 timeseries plots")
    parser.add_argument("--skip-summary", action="store_true",
                        help="Skip G7-G9 summary plots")
    parser.add_argument("--skip-proof-summary", action="store_true",
                        help="Skip G10-G13 proof summary plots")
    parser.add_argument("--skip-openloop", action="store_true",
                        help="Skip G16-G17 open-loop throughput plots")
    parser.add_argument("--protocols", type=str, default=None,
                        help="Comma-separated list of protocols to include (default: all discovered)")
    args = parser.parse_args()

    selected = None
    if args.protocols:
        selected = set(p.strip() for p in args.protocols.split(","))

    ingestion_data = discover_ingestion_data(args.data_dir)
    proof_data = discover_proof_data(args.data_dir)
    openloop_data = discover_openloop_data(args.data_dir)
    if selected:
        ingestion_data = {k: v for k, v in ingestion_data.items() if k in selected}
        proof_data = {k: v for k, v in proof_data.items() if k in selected}
        openloop_data = {k: v for k, v in openloop_data.items() if k in selected}
    if not ingestion_data and not proof_data and not openloop_data:
        sys.exit("No protocol directories with benchmark data found.")

    if ingestion_data:
        print(f"Ingestion data — {len(ingestion_data)} protocols:")
        for proto in sorted(ingestion_data):
            ks = sorted(ingestion_data[proto].keys())
            print(f"  {proto}: {len(ingestion_data[proto])} files, k = {ks}")
    if proof_data:
        print(f"Proof data — {len(proof_data)} protocols:")
        for proto in sorted(proof_data):
            rs = sorted(proof_data[proto].keys())
            print(f"  {proto}: {len(proof_data[proto])} files, range = {rs}")
    if openloop_data:
        print(f"Open-loop data — {len(openloop_data)} protocols:")
        for proto in sorted(openloop_data):
            print(f"  {proto}: {len(openloop_data[proto])} files")

    commands = []
    if not args.skip_timeseries and ingestion_data:
        commands.extend(gen_timeseries_commands(ingestion_data, args))
    if not args.skip_summary and ingestion_data:
        commands.append(gen_summary_command(ingestion_data, args))
    if not args.skip_proof_summary and proof_data:
        commands.append(gen_proof_summary_command(proof_data, args))
    if not args.skip_openloop and openloop_data:
        commands.extend(gen_openloop_timeseries_commands(openloop_data, args))

    print(f"\n{len(commands)} commands to run.\n")

    for i, cmd in enumerate(commands, 1):
        if args.dry_run:
            print(f"# [{i}/{len(commands)}]")
            print(_fmt_cmd(cmd))
            print()
        else:
            print(f"[{i}/{len(commands)}] {' '.join(cmd[2:5])}...")
            subprocess.run(cmd, check=True)

    if not args.dry_run:
        print(f"\nDone. Ran {len(commands)} commands.")


if __name__ == "__main__":
    main()
