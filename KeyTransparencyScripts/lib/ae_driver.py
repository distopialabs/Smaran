#!/usr/bin/env python3
"""Artifact-evaluation driver for Smaran key-transparency experiments.

Wraps experiments/kt.py without modifying it. Responsibilities:
- Emit "Running experiment Figure <yy>" banner on first line.
- Emit "Running <Protocol> with <x> versions/users" before each sweep point.
- After the sweep, emit "Plotting" and invoke the correct plotter.
- Copy the requested subfigure PDF to the output directory.

Cache behavior: if the previous sweep for this figure profile finished
successfully (marker file present) and KT_FORCE_RERUN is unset, the driver
skips the sweep and only re-invokes the plotter. This lets one full sweep
serve figures 4a, 4b, and 4c without redoing the work.
"""
from __future__ import annotations

import argparse
import hashlib
import os
import shutil
import sys
from pathlib import Path
from typing import Dict, List, Mapping, Optional

# The driver lives under KeyTransparencyScripts/lib/; the repo root is two levels up.
ROOT = Path(__file__).resolve().parents[2]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from experiments import kt as kt_module  # noqa: E402
from experiments import kt_plot as kt_plot_module  # noqa: E402
from experiments import kt_put_plot as kt_put_plot_module  # noqa: E402


PROTOCOL_DISPLAY = {"samurai": "Smaran", "optiks": "Optiks", "coniks": "Coniks"}

# (source name inside plotter output dir, destination name we hand to the user)
FIG_TO_PDF: Dict[str, tuple] = {
    "4a": ("kt_query_latency.pdf",    "fig4a_latency.pdf"),
    "4b": ("kt_query_throughput.pdf", "fig4b_throughput.pdf"),
    "4c": ("kt_query_payload.pdf",    "fig4c_payload.pdf"),
    "5":  ("kt_put_throughput.pdf",   "fig5_put_throughput.pdf"),
}

CACHE_ROOT = Path(os.environ.get("KT_CACHE_ROOT", "logs/ae_cache"))
DONE_MARKER = "SWEEP_DONE"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--figure", choices=list(FIG_TO_PDF), required=True)
    parser.add_argument("--config", type=Path, required=True,
                        help="Rendered kt.py TOML config")
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--cache-key", required=True,
                        help="Key that identifies this sweep configuration for caching")
    return parser.parse_args()


def config_fingerprint(config_path: Path) -> str:
    return hashlib.sha256(config_path.read_bytes()).hexdigest()[:12]


def order_combos(sweep_parameters: Mapping[str, List[object]]) -> List[dict]:
    """Order combos by (protocol, sweep_value) for deterministic user-facing output."""
    combos = kt_module.iter_sweep_combinations(sweep_parameters)

    def sort_key(combo: dict):
        proto = combo.get("bench_protocol", "")
        if "bench_num_versions" in combo:
            x = int(combo["bench_num_versions"])
        elif "bench_num_users" in combo:
            x = int(combo["bench_num_users"])
        else:
            x = 0
        return (proto, x)

    return sorted(combos, key=sort_key)


def running_line(protocol: str, sweep_value: int, is_put: bool) -> str:
    unit = "users" if is_put else "versions"
    display = PROTOCOL_DISPLAY.get(protocol, protocol)
    return f"Running {display} with {sweep_value} {unit}"


def run_sweep(config_path: Path, is_put: bool) -> Path:
    settings = kt_module.load_experiment_settings(config_path)
    sweep_parameters = kt_module.load_sweeping_parameters(config_path)
    if not sweep_parameters:
        raise SystemExit("Config has no [sweeping_parameters] table")

    sweep_root = kt_module.create_experiment_dir(settings.local_logs_dir)
    print(f"Sweep results directory: {sweep_root}", flush=True)

    for combo in order_combos(sweep_parameters):
        run_settings = kt_module.build_sweep_settings(settings, combo)
        run_settings = kt_module.apply_num_versions_divider(run_settings)
        combo_for_path = dict(combo)
        if "bench_num_versions" in combo_for_path:
            combo_for_path["bench_num_versions"] = run_settings.bench_num_versions

        protocol = str(combo["bench_protocol"])
        if is_put:
            x = int(combo["bench_num_users"])
        else:
            x = int(combo["bench_num_versions"])
        print(running_line(protocol, x, is_put), flush=True)

        run_dir = sweep_root / kt_module.format_sweep_dir_name(combo_for_path)
        kt_module.run_experiment_with_settings(
            config_path,
            run_settings,
            experiment_dir=run_dir,
            sweep_values=combo_for_path,
        )

    (sweep_root / DONE_MARKER).write_text("ok\n")
    return sweep_root


def find_cached_sweep(cache_key: str) -> Optional[Path]:
    cache_dir = CACHE_ROOT / cache_key
    if not cache_dir.exists():
        return None
    latest = cache_dir / "latest"
    if latest.is_symlink() and (latest / DONE_MARKER).exists():
        return latest.resolve()
    return None


def record_cached_sweep(cache_key: str, sweep_root: Path) -> None:
    cache_dir = CACHE_ROOT / cache_key
    cache_dir.mkdir(parents=True, exist_ok=True)
    latest = cache_dir / "latest"
    if latest.is_symlink() or latest.exists():
        latest.unlink()
    latest.symlink_to(sweep_root.resolve(), target_is_directory=True)


def invoke_plotter(sweep_root: Path, is_put: bool) -> Path:
    original_argv = sys.argv
    try:
        sys.argv = ["plotter", str(sweep_root)]
        if is_put:
            kt_put_plot_module.main()
        else:
            kt_plot_module.main()
    finally:
        sys.argv = original_argv
    return sweep_root / "output"


def main() -> int:
    args = parse_args()
    figure = args.figure
    is_put = figure == "5"

    print(f"Running experiment Figure {figure}", flush=True)

    force = os.environ.get("KT_FORCE_RERUN", "").lower() in {"1", "true", "yes"}
    cached = None if force else find_cached_sweep(args.cache_key)

    if cached is not None:
        print(f"[cache] Reusing prior sweep at {cached}", flush=True)
        sweep_root = cached
    else:
        sweep_root = run_sweep(args.config.resolve(), is_put)
        record_cached_sweep(args.cache_key, sweep_root)

    print("Plotting", flush=True)
    plotter_output = invoke_plotter(sweep_root, is_put)

    src_name, dst_name = FIG_TO_PDF[figure]
    src = plotter_output / src_name
    if not src.exists():
        raise SystemExit(f"Plotter did not produce {src}")

    args.output_dir.mkdir(parents=True, exist_ok=True)
    dst = args.output_dir / dst_name
    shutil.copy2(src, dst)
    print(f"Saved: {dst}", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
