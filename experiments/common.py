from __future__ import annotations

import getpass
import importlib
import shlex
import subprocess
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Mapping, Optional, Sequence, Tuple, Union

try:
    import tomli
except ImportError:  # pragma: no cover
    import tomllib as tomli


CommandList = Union[str, Sequence[str]]
CopyPair = Union[Tuple[str, str], Mapping[str, str]]
REMOTE_SHELL = "/bin/zsh"


@dataclass(frozen=True)
class NodeConfig:
    name: str
    ip: str
    port: int
    username: str
    key_path: str


class RemoteClusterError(RuntimeError):
    """Raised when one or more remote operations fail."""

    def __init__(
        self,
        message: str,
        *,
        partial_results: Optional[Dict[str, List[str]]] = None,
        errors: Optional[Dict[str, str]] = None,
    ) -> None:
        super().__init__(message)
        self.partial_results = partial_results or {}
        self.errors = errors or {}


class RemoteCluster:
    def __init__(self, toml_path: Optional[Union[str, Path]] = None) -> None:
        self._nodes: Dict[str, NodeConfig] = {}
        self._lock = threading.RLock()
        if toml_path is not None:
            self.load_toml(toml_path)

    def load_toml(self, toml_path: Union[str, Path]) -> None:
        """
        Load node definitions from a TOML file.

        Supported shapes:
        - [nodes.node1]
        - [vms.node1]
        - [node1]
        """
        path = Path(toml_path).expanduser()
        with path.open("rb") as toml_file:
            data = tomli.load(toml_file)

        raw_nodes = self._extract_nodes_from_toml(data)
        for name, config in raw_nodes.items():
            if not isinstance(config, Mapping):
                raise ValueError(f"TOML entry for `{name}` must be a table.")

            self.tag(
                name=name,
                ip=str(config["ip"]),
                port=int(config.get("port", 22)),
                username=config.get("username"),
                key_path=str(config["key_path"]),
            )

    def tag(
        self,
        *,
        name: str,
        ip: str,
        port: int = 22,
        username: Optional[str] = None,
        key_path: str,
    ) -> NodeConfig:
        """Register or update a VM under a human-readable name."""
        node = NodeConfig(
            name=name,
            ip=ip,
            port=int(port),
            username=username or getpass.getuser(),
            key_path=str(Path(key_path).expanduser()),
        )
        with self._lock:
            self._nodes[name] = node
        return node

    def untag(self, name: str) -> Optional[NodeConfig]:
        """Remove a VM tag if it exists."""
        with self._lock:
            return self._nodes.pop(name, None)

    def clear(self) -> None:
        """Remove all tagged VMs."""
        with self._lock:
            self._nodes.clear()

    def get(self, name: str) -> NodeConfig:
        """Return the config stored for a tagged VM."""
        with self._lock:
            if name not in self._nodes:
                raise KeyError(f"Unknown VM name: {name}")
            return self._nodes[name]

    def list_nodes(self) -> Dict[str, NodeConfig]:
        """Return a snapshot of all tagged VMs."""
        with self._lock:
            return dict(self._nodes)

    def run(
        self,
        name: str,
        commands: CommandList,
        *,
        hide: bool = True,
        connect_timeout: Optional[int] = None,
    ) -> List[str]:
        """Run commands sequentially on a single node."""
        node = self.get(name)
        normalized_commands = self._normalize_commands(commands)
        return self._run_commands_on_node(
            node,
            normalized_commands,
            hide=hide,
            connect_timeout=connect_timeout,
        )

    def run_all(
        self,
        commands_by_node: Mapping[str, CommandList],
        *,
        hide: bool = True,
        max_workers: Optional[int] = None,
        connect_timeout: Optional[int] = None,
    ) -> Dict[str, List[str]]:
        """
        Run each node's command list in parallel.

        Commands for a single node run in the order provided. Nodes run in parallel.
        """
        if not commands_by_node:
            return {}

        tasks = {
            name: self._normalize_commands(commands)
            for name, commands in commands_by_node.items()
        }
        return self._execute_parallel(
            {
                name: (
                    self._run_commands_on_node,
                    self.get(name),
                    node_commands,
                    hide,
                    connect_timeout,
                )
                for name, node_commands in tasks.items()
            },
            max_workers=max_workers,
            error_prefix="Remote command execution failed",
        )

    def rsync_to(
        self,
        name: str,
        local_path: str,
        remote_path: str,
        *,
        recursive: bool = True,
        delete: bool = False,
        extra_args: Optional[Sequence[str]] = None,
    ) -> str:
        """Copy a file or directory from the local machine to a VM."""
        node = self.get(name)
        return self._run_rsync(
            node=node,
            source=local_path,
            destination=remote_path,
            upload=True,
            recursive=recursive,
            delete=delete,
            extra_args=extra_args,
        )

    def rsync_from(
        self,
        name: str,
        remote_path: str,
        local_path: str,
        *,
        recursive: bool = True,
        delete: bool = False,
        extra_args: Optional[Sequence[str]] = None,
    ) -> str:
        """Copy a file or directory from a VM to the local machine."""
        node = self.get(name)
        return self._run_rsync(
            node=node,
            source=remote_path,
            destination=local_path,
            upload=False,
            recursive=recursive,
            delete=delete,
            extra_args=extra_args,
        )

    def rsync_to_all(
        self,
        transfers_by_node: Mapping[str, Sequence[CopyPair]],
        *,
        recursive: bool = True,
        delete: bool = False,
        max_workers: Optional[int] = None,
        extra_args: Optional[Sequence[str]] = None,
    ) -> Dict[str, List[str]]:
        """
        Copy files from local machine to multiple VMs in parallel.

        Each value should be a list of pairs like:
        - [("local/file", "/remote/file")]
        - [{"source": "local/file", "destination": "/remote/file"}]
        """
        if not transfers_by_node:
            return {}

        return self._execute_parallel(
            {
                name: (
                    self._run_rsync_batch,
                    self.get(name),
                    transfers,
                    True,
                    recursive,
                    delete,
                    extra_args,
                )
                for name, transfers in transfers_by_node.items()
            },
            max_workers=max_workers,
            error_prefix="Rsync upload failed",
        )

    def rsync_from_all(
        self,
        transfers_by_node: Mapping[str, Sequence[CopyPair]],
        *,
        recursive: bool = True,
        delete: bool = False,
        max_workers: Optional[int] = None,
        extra_args: Optional[Sequence[str]] = None,
    ) -> Dict[str, List[str]]:
        """
        Copy files from multiple VMs to the local machine in parallel.

        Each value should be a list of pairs like:
        - [("/remote/file", "local/file")]
        - [{"source": "/remote/file", "destination": "local/file"}]
        """
        if not transfers_by_node:
            return {}

        return self._execute_parallel(
            {
                name: (
                    self._run_rsync_batch,
                    self.get(name),
                    transfers,
                    False,
                    recursive,
                    delete,
                    extra_args,
                )
                for name, transfers in transfers_by_node.items()
            },
            max_workers=max_workers,
            error_prefix="Rsync download failed",
        )

    @staticmethod
    def _connection(node: NodeConfig, connect_timeout: Optional[int] = None):
        try:
            fabric_module = importlib.import_module("fabric")
        except ImportError as exc:
            raise ImportError(
                "fabric is required to use RemoteCluster. Install it with `pip install fabric`."
            ) from exc

        return fabric_module.Connection(
            host=node.ip,
            user=node.username,
            port=node.port,
            connect_timeout=connect_timeout,
            connect_kwargs={"key_filename": [node.key_path]},
        )

    @staticmethod
    def _normalize_commands(commands: CommandList) -> List[str]:
        if isinstance(commands, str):
            return [commands]

        normalized = [command for command in commands]
        if not normalized:
            return []

        if not all(isinstance(command, str) for command in normalized):
            raise TypeError("Each command must be a string.")
        return normalized

    @staticmethod
    def _extract_nodes_from_toml(data: Mapping[str, object]) -> Mapping[str, Mapping[str, object]]:
        if "nodes" in data:
            nodes = data["nodes"]
        elif "vms" in data:
            nodes = data["vms"]
        else:
            nodes = data

        if not isinstance(nodes, Mapping):
            raise ValueError("TOML must define node tables under `nodes`, `vms`, or at top level.")
        return nodes

    def _run_commands_on_node(
        self,
        node: NodeConfig,
        commands: Sequence[str],
        hide: bool,
        connect_timeout: Optional[int],
    ) -> List[str]:
        outputs: List[str] = []
        with self._connection(node, connect_timeout=connect_timeout) as connection:
            for command in commands:
                result = connection.run(
                    self._wrap_remote_command(command),
                    hide=hide,
                    warn=True,
                    shell=REMOTE_SHELL,
                )
                if not result.ok:
                    stderr = result.stderr.strip() or result.stdout.strip()
                    raise RuntimeError(
                        f"{node.name}: command `{command}` failed with exit code "
                        f"{result.exited}. {stderr}"
                    )
                outputs.append(result.stdout.strip())
        return outputs

    def _run_rsync_batch(
        self,
        node: NodeConfig,
        transfers: Sequence[CopyPair],
        upload: bool,
        recursive: bool,
        delete: bool,
        extra_args: Optional[Sequence[str]],
    ) -> List[str]:
        outputs: List[str] = []
        for transfer in transfers:
            source, destination = self._normalize_transfer(transfer, upload=upload)
            outputs.append(
                self._run_rsync(
                    node=node,
                    source=source,
                    destination=destination,
                    upload=upload,
                    recursive=recursive,
                    delete=delete,
                    extra_args=extra_args,
                )
            )
        return outputs

    @staticmethod
    def _wrap_remote_command(command: str) -> str:
        return f"zsh -lic {shlex.quote(command)}"

    def _run_rsync(
        self,
        *,
        node: NodeConfig,
        source: str,
        destination: str,
        upload: bool,
        recursive: bool,
        delete: bool,
        extra_args: Optional[Sequence[str]],
    ) -> str:
        cmd = self._build_rsync_command(
            node=node,
            source=source,
            destination=destination,
            upload=upload,
            recursive=recursive,
            delete=delete,
            extra_args=extra_args,
        )
        result = subprocess.run(cmd, capture_output=True, text=True, check=False)
        if result.returncode != 0:
            stderr = result.stderr.strip() or result.stdout.strip()
            direction = "upload" if upload else "download"
            raise RuntimeError(
                f"{node.name}: rsync {direction} failed with exit code "
                f"{result.returncode}. {stderr}"
            )
        return result.stdout.strip()

    def _build_rsync_command(
        self,
        *,
        node: NodeConfig,
        source: str,
        destination: str,
        upload: bool,
        recursive: bool,
        delete: bool,
        extra_args: Optional[Sequence[str]],
    ) -> List[str]:
        ssh_parts = [
            "ssh",
            "-i",
            node.key_path,
            "-p",
            str(node.port),
            "-o",
            "IdentitiesOnly=yes",
        ]
        ssh_command = " ".join(shlex.quote(part) for part in ssh_parts)
        remote_target = f"{node.username}@{node.ip}:{shlex.quote(destination if upload else source)}"

        cmd = ["rsync", "-avz" if recursive else "-vz", "-e", ssh_command]
        if delete:
            cmd.append("--delete")
        if extra_args:
            cmd.extend(extra_args)

        if upload:
            cmd.extend([source, remote_target])
        else:
            cmd.extend([remote_target, destination])
        return cmd

    @staticmethod
    def _normalize_transfer(transfer: CopyPair, *, upload: bool) -> Tuple[str, str]:
        if isinstance(transfer, tuple):
            if len(transfer) != 2:
                raise ValueError("Transfer tuples must contain exactly two paths.")
            return transfer[0], transfer[1]

        source = transfer.get("source")
        destination = transfer.get("destination")
        if source and destination:
            return source, destination

        if upload:
            source = transfer.get("local_path")
            destination = transfer.get("remote_path")
        else:
            source = transfer.get("remote_path")
            destination = transfer.get("local_path")

        if not source or not destination:
            raise ValueError(
                "Transfer mappings must include `source`/`destination` or "
                "`local_path`/`remote_path`."
            )
        return source, destination

    @staticmethod
    def _execute_parallel(
        tasks: Mapping[str, Tuple],
        *,
        max_workers: Optional[int],
        error_prefix: str,
    ) -> Dict[str, List[str]]:
        results: Dict[str, List[str]] = {}
        errors: Dict[str, str] = {}
        worker_count = max_workers or len(tasks)

        with ThreadPoolExecutor(max_workers=worker_count) as executor:
            future_to_name = {
                executor.submit(task[0], *task[1:]): name
                for name, task in tasks.items()
            }
            for future in as_completed(future_to_name):
                name = future_to_name[future]
                try:
                    results[name] = future.result()
                except Exception as exc:  # noqa: BLE001
                    errors[name] = str(exc)

        if errors:
            raise RemoteClusterError(
                f"{error_prefix}: {', '.join(sorted(errors))}",
                partial_results=results,
                errors=errors,
            )
        return {name: results[name] for name in tasks if name in results}
