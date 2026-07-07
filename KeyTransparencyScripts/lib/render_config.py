#!/usr/bin/env python3
"""Render a kt.py-compatible TOML config from a figure profile + env vars.

Reads env vars (see DEFAULTS) and emits a single TOML that
experiments/kt.py consumes. Kept separate from ae_driver.py so callers
can inspect the rendered config for debugging.
"""
from __future__ import annotations

import argparse
import os
from pathlib import Path

DEFAULTS = {
    "KT_SERVER_HOST": "node0",
    "KT_SERVER_PORT": "22",
    "KT_CLIENT_HOST": "node1",
    "KT_CLIENT_PORT": "22",
    "KT_SSH_USER": os.environ.get("USER", ""),
    "KT_SSH_KEY": os.path.expanduser("~/.ssh/id_rsa"),
    "KT_REMOTE_DIR": "/tmp/smaran-ae",
    "KT_REPO_URL": "https://github.com/distopialabs/Smaran.git",
    "KT_REPO_BRANCH": "artifact-eval",
    "KT_RUN_DURATION": "90",
    "KT_LOCAL_LOGS_DIR": "logs",
}

FIGURE_PROFILES = {
    "fig4_full": {
        "axis": "versions",
        "values": [2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2047],
        "mode": "get", "num_users": 10000, "batch": 1024,
        "num_load_clients": 10, "num_run_clients": 10,
    },
    "fig4_quick": {
        "axis": "versions",
        "values": [2, 16, 128, 256, 2047],
        "mode": "get", "num_users": 10000, "batch": 1024,
        "num_load_clients": 10, "num_run_clients": 10,
    },
    "fig5_full": {
        "axis": "users",
        "values": [10000, 30000, 100000, 200000, 500000, 1000000],
        "mode": "put", "num_users": 100000, "batch": 16384,
        "num_load_clients": 500, "num_run_clients": 500,
    },
    "fig5_quick": {
        "axis": "users",
        "values": [10000, 200000, 1000000],
        "mode": "put", "num_users": 100000, "batch": 16384,
        "num_load_clients": 500, "num_run_clients": 500,
    },
}


def _env(name: str) -> str:
    return os.environ.get(name, DEFAULTS[name])


def render(profile_name: str, out_path: Path) -> None:
    prof = FIGURE_PROFILES[profile_name]
    is_put = prof["mode"] == "put"

    remote_dir = _env("KT_REMOTE_DIR")
    bench_binary_base = "/tmp/bin/ktbench"
    bench_binary = bench_binary_base + (" -run-mode put" if is_put else "")

    if prof["axis"] == "versions":
        sweep_line = f"bench_num_versions = {prof['values']}"
    else:
        sweep_line = f"bench_num_users = {prof['values']}"

    content = f'''[nodes.node1]
ip = "{_env("KT_SERVER_HOST")}"
port = {int(_env("KT_SERVER_PORT"))}
username = "{_env("KT_SSH_USER")}"
key_path = "{_env("KT_SSH_KEY")}"

[nodes.node2]
ip = "{_env("KT_CLIENT_HOST")}"
port = {int(_env("KT_CLIENT_PORT"))}
username = "{_env("KT_SSH_USER")}"
key_path = "{_env("KT_SSH_KEY")}"

[experiment]
repo_url = "{_env("KT_REPO_URL")}"
remote_base_dir = "{remote_dir}"
remote_repo_dir = "{remote_dir}/Smaran"
remote_bin_dir = "{remote_dir}/Smaran/bin"
remote_tmp_dir = "/tmp/"
server_node = "node1"
server_addr = "0.0.0.0:3191"
server_port = 3191
server_log_path = "{remote_dir}/server.log"
repo_branch = "{_env("KT_REPO_BRANCH")}"
build_command = "make"
server_binary = "/tmp/bin/ktserver"
server_process_name = "ktserver"
server_batch_size = {int(prof["batch"])}
bench_binary = "{bench_binary}"
bench_num_users = {int(prof["num_users"])}
bench_num_load_clients = {int(prof["num_load_clients"])}
bench_num_run_clients = {int(prof["num_run_clients"])}
bench_num_versions = 20
coniks_num_versions_divider = 1
bench_run_duration_secs = {int(_env("KT_RUN_DURATION"))}
bench_protocol = "samurai"
local_logs_dir = "{_env("KT_LOCAL_LOGS_DIR")}"
server_startup_wait_seconds = 2.0

[sweeping_parameters]
bench_protocol = ["samurai", "optiks", "coniks"]
{sweep_line}
'''
    out_path.write_text(content, encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile", required=True, choices=list(FIGURE_PROFILES))
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    render(args.profile, args.out)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
