from __future__ import annotations

import argparse
from itertools import product
import json
import os
import subprocess
import sys
import time
from dataclasses import asdict, dataclass, fields, replace
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Mapping, Optional

ROOT = Path(__file__).resolve().parent.parent
if __package__ in {None, ""}:
    if str(ROOT) not in sys.path:
        sys.path.insert(0, str(ROOT))

from experiments.common import RemoteCluster

try:
    import tomli
except ImportError:  # pragma: no cover
    import tomllib as tomli  # pyright: ignore[reportMissingImports]


@dataclass(frozen=False)
class KtExperimentSettings:
    repo_url: str
    remote_base_dir: str
    remote_repo_dir: str
    remote_bin_dir: str
    remote_tmp_dir: str
    server_node: str
    server_addr: str
    server_port: int
    server_log_path: str
    repo_branch: str
    build_command: str
    server_binary: str
    server_process_name: str
    server_batch_size: int
    bench_binary: str
    bench_num_users: int
    bench_num_load_clients: int
    bench_num_run_clients: int
    bench_num_versions: int
    bench_run_duration_secs: int
    bench_protocol: str
    local_logs_dir: str
    server_startup_wait_seconds: float


SWEEP_KEY_ALIASES = {
    "batch_size": "server_batch_size",
    "num_users": "bench_num_users",
    "num_load_clients": "bench_num_load_clients",
    "num_run_clients": "bench_num_run_clients",
    "num_versions": "bench_num_versions",
}


def iso_timestamp(now: Optional[datetime] = None) -> str:
    current_time = now or datetime.now().astimezone()
    return current_time.isoformat(timespec="seconds").replace(":", "-")


def create_experiment_dir(base_dir: Path | str = "logs", *, now: Optional[datetime] = None) -> Path:
    experiment_dir = Path(base_dir) / iso_timestamp(now)
    experiment_dir.mkdir(parents=True, exist_ok=False)
    return experiment_dir


def load_toml_config(config_path: Path | str) -> Mapping[str, object]:
    path = Path(config_path)
    with path.open("rb") as config_file:
        return tomli.load(config_file)


def load_experiment_settings(config_path: Path | str) -> KtExperimentSettings:
    data = load_toml_config(config_path)
    experiment = data.get("experiment")
    if not isinstance(experiment, Mapping):
        raise ValueError("TOML must define an `experiment` table.")

    return KtExperimentSettings(
        repo_url=str(experiment["repo_url"]),
        remote_base_dir=str(experiment["remote_base_dir"]),
        remote_repo_dir=str(experiment["remote_repo_dir"]),
        remote_bin_dir=str(experiment["remote_bin_dir"]),
        remote_tmp_dir=str(experiment["remote_tmp_dir"]),
        server_node=str(experiment["server_node"]),
        server_addr=str(experiment["server_addr"]),
        server_port=int(experiment["server_port"]),
        server_log_path=str(experiment["server_log_path"]),
        repo_branch=str(experiment["repo_branch"]),
        build_command=str(experiment["build_command"]),
        server_binary=str(experiment["server_binary"]),
        server_process_name=str(experiment["server_process_name"]),
        server_batch_size=int(experiment["server_batch_size"]),
        bench_binary=str(experiment["bench_binary"]),
        bench_num_users=int(experiment["bench_num_users"]),
        bench_num_load_clients=int(experiment["bench_num_load_clients"]),
        bench_num_run_clients=int(experiment["bench_num_run_clients"]),
        bench_num_versions=int(experiment["bench_num_versions"]),
        bench_run_duration_secs=int(experiment["bench_run_duration_secs"]),
        bench_protocol=str(experiment["bench_protocol"]),
        local_logs_dir=str(experiment["local_logs_dir"]),
        server_startup_wait_seconds=float(experiment["server_startup_wait_seconds"]),
    )


def load_sweeping_parameters(config_path: Path | str) -> Optional[Dict[str, List[object]]]:
    data = load_toml_config(config_path)
    raw_sweep = data.get("sweeping_parameters")
    if raw_sweep is None:
        return None
    if not isinstance(raw_sweep, Mapping):
        raise ValueError("TOML `sweeping_parameters` must be a table.")
    if not raw_sweep:
        return None

    sweep_parameters: Dict[str, List[object]] = {}
    for sweep_key, sweep_values in raw_sweep.items():
        if not isinstance(sweep_values, list):
            raise ValueError("Each `sweeping_parameters` entry must be a list.")
        if not sweep_values:
            raise ValueError("`sweeping_parameters` lists must not be empty.")
        sweep_parameters[str(sweep_key)] = list(sweep_values)
    return sweep_parameters


def resolve_sweep_setting_name(sweep_key: str) -> str:
    valid_field_names = {field.name for field in fields(KtExperimentSettings)}
    if sweep_key in valid_field_names:
        return sweep_key

    alias = SWEEP_KEY_ALIASES.get(sweep_key)
    if alias is not None:
        return alias

    raise ValueError(f"Unknown sweeping parameter: {sweep_key}")


def coerce_sweep_value(current_value: object, sweep_value: object) -> object:
    if isinstance(current_value, bool):
        return bool(sweep_value)
    if isinstance(current_value, int):
        return int(sweep_value)
    if isinstance(current_value, float):
        return float(sweep_value)
    if isinstance(current_value, str):
        return str(sweep_value)
    return sweep_value


def build_sweep_settings(
    settings: KtExperimentSettings,
    sweep_values: Mapping[str, object],
) -> KtExperimentSettings:
    updates: Dict[str, object] = {}
    for sweep_key, sweep_value in sweep_values.items():
        setting_name = resolve_sweep_setting_name(sweep_key)
        current_value = getattr(settings, setting_name)
        updates[setting_name] = coerce_sweep_value(current_value, sweep_value)
    return replace(settings, **updates)


def format_sweep_value_for_path(sweep_value: object) -> str:
    raw_value = str(sweep_value)
    safe_value = "".join(
        character if character.isalnum() or character in {"-", "_", "."} else "_"
        for character in raw_value
    )
    return safe_value.strip("_") or "value"


def iter_sweep_combinations(
    sweep_parameters: Mapping[str, List[object]],
) -> List[Dict[str, object]]:
    sweep_keys = list(sweep_parameters)
    sweep_value_sets = [sweep_parameters[sweep_key] for sweep_key in sweep_keys]
    return [
        dict(zip(sweep_keys, sweep_values))
        for sweep_values in product(*sweep_value_sets)
    ]


def format_sweep_dir_name(sweep_values: Mapping[str, object]) -> str:
    parts: List[str] = []
    for sweep_key, sweep_value in sweep_values.items():
        parts.append(sweep_key)
        parts.append(format_sweep_value_for_path(sweep_value))
    return "_".join(parts)


def dump_run_config(
    config_path: Path | str,
    experiment_dir: Path,
    settings: KtExperimentSettings,
    *,
    sweep_values: Optional[Mapping[str, object]] = None,
) -> None:
    config_dump = {
        "source_config_path": str(Path(config_path)),
        "experiment": asdict(settings),
    }
    if sweep_values:
        config_dump["applied_sweeping_parameters"] = dict(sweep_values)

    (experiment_dir / "config_used.json").write_text(
        json.dumps(config_dump, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )


def rewrite_coniks_client_config(
    local_bin_dir: Path,
    settings: KtExperimentSettings,
    server_ip: str,
) -> None:
    config_path = local_bin_dir / "coniks-client-config" / "config.toml"
    if not config_path.exists():
        return

    remote_tmp_config_dir = os.path.join(settings.remote_tmp_dir, "bin", "coniks-server-config")
    updated_lines: List[str] = []
    for line in config_path.read_text(encoding="utf-8").splitlines():
        stripped_line = line.strip()
        if stripped_line.startswith("sign_pubkey_path ="):
            updated_lines.append(f'sign_pubkey_path = "{remote_tmp_config_dir}/sign.pub"')
        elif stripped_line.startswith("init_str_path ="):
            updated_lines.append(f'init_str_path = "{remote_tmp_config_dir}/init.str"')
        elif stripped_line.startswith("address ="):
            updated_lines.append(f'address = "tcp://{server_ip}:3000"')
        else:
            updated_lines.append(line)

    config_path.write_text("\n".join(updated_lines) + "\n", encoding="utf-8")


def sync_coniks_init_str(
    cluster: RemoteCluster,
    local_bin_dir: Path,
    settings: KtExperimentSettings,
) -> None:
    remote_server_init_path = os.path.join(
        settings.remote_tmp_dir, "bin", "coniks-server-config", "init.str"
    )
    local_server_config_dir = local_bin_dir / "coniks-server-config"
    local_server_config_dir.mkdir(parents=True, exist_ok=True)
    local_init_path = local_server_config_dir / "init.str"

    cluster.rsync_from(
        settings.server_node,
        remote_server_init_path,
        str(local_init_path),
        recursive=False,
    )

    client_nodes = [
        node_name for node_name in cluster.list_nodes() if node_name != settings.server_node
    ]
    if not client_nodes:
        return

    remote_client_init_path = os.path.join(
        settings.remote_tmp_dir, "bin", "coniks-client-config", "init.str"
    )
    cluster.rsync_to_all(
        {
            node_name: [(str(local_init_path), remote_client_init_path)]
            for node_name in client_nodes
        },
        recursive=False,
    )


def get_local_commit_hash(repo_root: Path | str = ROOT) -> str:
    result = subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=repo_root,
        capture_output=True,
        text=True,
        check=True,
    )
    return result.stdout.strip()


def build_repo_prepare_commands(commit_hash: str, settings: KtExperimentSettings) -> List[str]:
    return [
        f"mkdir -p {settings.remote_base_dir}",
        (
            f"if [ ! -d {settings.remote_repo_dir}/.git ]; then "
            f"git clone --recurse-submodules {settings.repo_url} {settings.remote_repo_dir}; "
            "fi"
        ),
        f"cd {settings.remote_repo_dir} && git checkout {settings.repo_branch}",
        f"cd {settings.remote_repo_dir} && git pull --recurse-submodules",
        f"cd {settings.remote_repo_dir} && git checkout {commit_hash}",
        f"cd {settings.remote_repo_dir} && git submodule sync --recursive",
        f"cd {settings.remote_repo_dir} && git submodule update --init --recursive",
        f"cd {settings.remote_repo_dir} && {settings.build_command}",
    ]


def is_coniks_protocol(settings: KtExperimentSettings) -> bool:
    return settings.bench_protocol == "coniks"


def build_server_start_command(settings: KtExperimentSettings) -> str:
    if is_coniks_protocol(settings):
        server_config_dir = os.path.join(settings.remote_tmp_dir, "bin", "coniks-server-config")
        return (
            f"cd {server_config_dir} && "
            f"nohup ../coniksserver run -p > {settings.server_log_path} 2>&1 < /dev/null &"
        )
    return (
        f"nohup {settings.server_binary} -batch_size {settings.server_batch_size} "
        f"--addr {settings.server_addr} "
        f"--protocol {settings.bench_protocol} "
        f"> {settings.server_log_path} 2>&1 < /dev/null &"
    )


def build_bench_command(
    node_name: str,
    server_ip: str,
    settings: KtExperimentSettings,
) -> str:
    if is_coniks_protocol(settings):
        client_config_dir = os.path.join(settings.remote_tmp_dir, "bin", "coniks-client-config")
        return (
            f"cd {client_config_dir} && "
            f"../coniksbench run -c config.toml "
            f"-n {settings.bench_num_run_clients} "
            f"-k {settings.bench_num_users} "
            f"-d {settings.bench_run_duration_secs} "
            f"-e {settings.bench_num_versions} "
            f"-a {settings.bench_num_versions} "
            f"-v"
            f"> {settings.remote_base_dir}/{node_name}.log 2>&1"
        )
    return (
        f"{settings.bench_binary} "
        f"-num-users {settings.bench_num_users} "
        f"-num-load-clients {settings.bench_num_load_clients} "
        f"-num-run-clients {settings.bench_num_run_clients} "
        f"-num-versions {settings.bench_num_versions} "
        f"-run-duration-secs {settings.bench_run_duration_secs} "
        f"-protocol {settings.bench_protocol} "
        f"--addr {server_ip}:{settings.server_port} "
        f"> {settings.remote_base_dir}/{node_name}.log 2>&1"
    )


def build_bench_commands(cluster: RemoteCluster, settings: KtExperimentSettings) -> Dict[str, str]:
    server_ip = cluster.get(settings.server_node).ip
    commands: Dict[str, str] = {}
    for node_name in sorted(cluster.list_nodes()):
        if node_name == settings.server_node:
            continue
        commands[node_name] = build_bench_command(node_name, server_ip, settings)
    return commands


def build_log_downloads(
    cluster: RemoteCluster,
    experiment_dir: Path,
    settings: KtExperimentSettings,
) -> Dict[str, List[tuple[str, str]]]:
    downloads: Dict[str, List[tuple[str, str]]] = {
        settings.server_node: [(settings.server_log_path, str(experiment_dir / "server.log"))]
    }
    for node_name in sorted(cluster.list_nodes()):
        if node_name == settings.server_node:
            continue
        downloads[node_name] = [
            (f"{settings.remote_base_dir}/{node_name}.log", str(experiment_dir / f"{node_name}.log"))
        ]
    return downloads


def run_experiment_with_settings(
    config_path: Path | str,
    settings: KtExperimentSettings,
    *,
    experiment_dir: Optional[Path] = None,
    sweep_values: Optional[Mapping[str, object]] = None,
) -> Path:
    cluster = RemoteCluster(config_path)
    server_node = cluster.get(settings.server_node)
    commit_hash = get_local_commit_hash()

    if experiment_dir is None:
        experiment_dir = create_experiment_dir(settings.local_logs_dir)
    else:
        experiment_dir.mkdir(parents=True, exist_ok=False)

    dump_run_config(
        config_path,
        experiment_dir,
        settings,
        sweep_values=sweep_values,
    )
    print(f"Created experiment directory: {experiment_dir}")
    print(
        f"Preparing repository and building binaries on {settings.server_node} "
        f"at commit {commit_hash}"
    )
    cluster.run(
        settings.server_node,
        build_repo_prepare_commands(commit_hash, settings),
        hide=False,
    )

    print(f"Downloading built bin directory from {settings.server_node}")
    cluster.rsync_from(settings.server_node, settings.remote_bin_dir, str(experiment_dir))
    local_bin_dir = experiment_dir / "bin"
    rewrite_coniks_client_config(local_bin_dir, settings, server_node.ip)

    print("Distributing bin directory to all nodes")
    cluster.rsync_to_all(
        {
            node_name: [(str(local_bin_dir), settings.remote_tmp_dir)]
            for node_name in cluster.list_nodes()
        }
    )

    bench_commands = build_bench_commands(cluster, settings)
    run_error: Optional[BaseException] = None
    cleanup_errors: List[str] = []

    try:
        print(f"Starting ktserver on {settings.server_node}")
        cluster.run(settings.server_node, build_server_start_command(settings), hide=False)
        if is_coniks_protocol(settings):
            time.sleep(1.0)
            print("Syncing coniks init.str to client nodes")
            sync_coniks_init_str(cluster, local_bin_dir, settings)
            settings.server_process_name = "coniksserver"

        time.sleep(settings.server_startup_wait_seconds)

        if bench_commands:
            print(f"Running ktbench on {len(bench_commands)} nodes")
            cluster.run_all(bench_commands, hide=False)
        else:
            print("No benchmark client nodes configured; skipping ktbench run")
    except BaseException as exc:  # noqa: BLE001
        run_error = exc
    finally:
        print(f"Stopping ktserver on {settings.server_node}")
        try:
            cluster.run(
                settings.server_node,
                [
                    f"pkill -c {settings.server_process_name} || true",
                    "rm -rf /tmp/coniks.sock || true",
                ],
                hide=False,
            )
        except Exception as exc:  # noqa: BLE001
            cleanup_errors.append(f"failed to stop ktserver: {exc}")

        print("Downloading remote logs")
        try:
            cluster.rsync_from_all(build_log_downloads(cluster, experiment_dir, settings))
        except Exception as exc:  # noqa: BLE001
            cleanup_errors.append(f"failed to download logs: {exc}")

    if run_error and cleanup_errors:
        raise RuntimeError("; ".join(cleanup_errors)) from run_error
    if cleanup_errors:
        raise RuntimeError("; ".join(cleanup_errors))
    if run_error:
        raise run_error

    print(f"Experiment completed successfully: {experiment_dir}")
    return experiment_dir


def run_experiment(config_path: Path | str) -> Path:
    settings = load_experiment_settings(config_path)
    sweeping_parameters = load_sweeping_parameters(config_path)
    if not sweeping_parameters:
        return run_experiment_with_settings(config_path, settings)

    sweep_root_dir = create_experiment_dir(settings.local_logs_dir)

    for sweep_values in iter_sweep_combinations(sweeping_parameters):
        run_settings = build_sweep_settings(settings, sweep_values)
        run_dir = sweep_root_dir / format_sweep_dir_name(sweep_values)
        sweep_description = ", ".join(f"{key}={value}" for key, value in sweep_values.items())
        print(f"Running sweep with {sweep_description}")
        run_experiment_with_settings(
            config_path,
            run_settings,
            experiment_dir=run_dir,
            sweep_values=sweep_values,
        )

    return sweep_root_dir


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run the distributed KT experiment.")
    parser.add_argument("config", help="Path to the TOML file describing the remote cluster.")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    run_experiment(args.config)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
