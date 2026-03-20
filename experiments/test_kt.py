import sys
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

ROOT = Path(__file__).resolve().parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from experiments.kt import (
    KtExperimentSettings,
    build_bench_commands,
    build_log_downloads,
    build_repo_prepare_commands,
    build_server_start_command,
    create_experiment_dir,
    load_experiment_settings,
    load_sweeping_parameters,
    run_experiment,
    run_experiment_with_settings,
)


def make_settings() -> KtExperimentSettings:
    return KtExperimentSettings(
        repo_url="https://github.com/distopialabs/Samurai",
        remote_base_dir="/data/shubham_mishra",
        remote_repo_dir="/data/shubham_mishra/Samurai",
        remote_bin_dir="/data/shubham_mishra/Samurai/bin",
        remote_tmp_dir="/tmp/",
        server_node="node1",
        server_addr="0.0.0.0:3191",
        server_port=3191,
        server_log_path="/data/shubham_mishra/server.log",
        repo_branch="main",
        build_command="make",
        server_binary="/tmp/bin/ktserver",
        server_process_name="ktserver",
        server_batch_size=1024,
        bench_binary="/tmp/bin/ktbench",
        bench_num_users=10000,
        bench_num_load_clients=10,
        bench_num_run_clients=10,
        bench_num_versions=2000,
        bench_run_duration_secs=30,
        bench_protocol="samurai",
        local_logs_dir="logs",
        server_startup_wait_seconds=2.0,
    )


class KtExperimentTest(unittest.TestCase):
    def test_create_experiment_dir_uses_safe_iso_timestamp(self) -> None:
        now = datetime(2026, 3, 19, 10, 11, 12, tzinfo=timezone.utc)

        with tempfile.TemporaryDirectory() as temp_dir:
            experiment_dir = create_experiment_dir(temp_dir, now=now)

        self.assertEqual(experiment_dir.name, "2026-03-19T10-11-12+00-00")

    def test_build_helpers_skip_node1(self) -> None:
        settings = make_settings()
        cluster = MagicMock()
        cluster.get.return_value = SimpleNamespace(ip="10.0.0.1")
        cluster.list_nodes.return_value = {
            "node1": SimpleNamespace(ip="10.0.0.1"),
            "node2": SimpleNamespace(ip="10.0.0.2"),
            "node3": SimpleNamespace(ip="10.0.0.3"),
        }

        commands = build_bench_commands(cluster, settings)
        downloads = build_log_downloads(cluster, Path("logs/run"), settings)

        self.assertEqual(set(commands), {"node2", "node3"})
        self.assertIn("--addr 10.0.0.1:3191", commands["node2"])
        self.assertEqual(downloads["node1"], [(settings.server_log_path, "logs/run/server.log")])
        self.assertEqual(
            downloads["node2"],
            [(f"{settings.remote_base_dir}/node2.log", "logs/run/node2.log")],
        )

    def test_build_repo_prepare_commands_checks_out_local_commit(self) -> None:
        settings = make_settings()
        commands = build_repo_prepare_commands("abc123", settings)

        self.assertEqual(commands[2], "cd /data/shubham_mishra/Samurai && git checkout main")
        self.assertEqual(commands[3], "cd /data/shubham_mishra/Samurai && git pull")
        self.assertEqual(commands[4], "cd /data/shubham_mishra/Samurai && git checkout abc123")
        self.assertEqual(commands[5], "cd /data/shubham_mishra/Samurai && make")

    def test_build_server_start_command_passes_protocol(self) -> None:
        settings = make_settings()

        self.assertEqual(
            build_server_start_command(settings),
            (
                "nohup /tmp/bin/ktserver -batch_size 1024 "
                "--addr 0.0.0.0:3191 "
                "--protocol samurai "
                "> /data/shubham_mishra/server.log 2>&1 < /dev/null &"
            ),
        )

    def test_load_experiment_settings_reads_toml_values(self) -> None:
        settings = load_experiment_settings("experiments/cluster.toml")

        self.assertEqual(settings.server_node, "node1")
        self.assertEqual(settings.remote_bin_dir, "/data/shubham_mishra/Samurai/bin")
        self.assertEqual(settings.server_batch_size, 1024)
        self.assertEqual(settings.bench_run_duration_secs, 120)
        self.assertEqual(settings.bench_protocol, "samurai")
        self.assertEqual(settings.server_startup_wait_seconds, 2.0)

    @patch("experiments.kt.load_toml_config")
    def test_load_sweeping_parameters_returns_none_for_empty_table(
        self,
        load_toml_config_mock: MagicMock,
    ) -> None:
        load_toml_config_mock.return_value = {"sweeping_parameters": {}}

        self.assertIsNone(load_sweeping_parameters("experiments/cluster.toml"))

    @patch("experiments.kt.load_toml_config")
    def test_load_sweeping_parameters_reads_multiple_keys(
        self,
        load_toml_config_mock: MagicMock,
    ) -> None:
        load_toml_config_mock.return_value = {
            "sweeping_parameters": {
                "batch_size": [10, 20],
                "num_users": [100, 200],
            }
        }

        sweeping_parameters = load_sweeping_parameters("experiments/cluster.toml")

        self.assertEqual(
            sweeping_parameters,
            {
                "batch_size": [10, 20],
                "num_users": [100, 200],
            },
        )

    @patch("experiments.kt.dump_run_config")
    @patch("experiments.kt.get_local_commit_hash", return_value="abc123")
    @patch("experiments.kt.time.sleep")
    @patch("experiments.kt.create_experiment_dir")
    @patch("experiments.kt.RemoteCluster")
    def test_run_experiment_with_settings_orchestrates_remote_workflow(
        self,
        remote_cluster_cls: MagicMock,
        create_experiment_dir_mock: MagicMock,
        sleep_mock: MagicMock,
        get_local_commit_hash_mock: MagicMock,
        dump_run_config_mock: MagicMock,
    ) -> None:
        settings = make_settings()
        experiment_dir = Path("logs/run-1")
        create_experiment_dir_mock.return_value = experiment_dir

        cluster = remote_cluster_cls.return_value
        cluster.get.return_value = SimpleNamespace(ip="10.0.0.1")
        cluster.list_nodes.return_value = {
            "node1": SimpleNamespace(ip="10.0.0.1"),
            "node2": SimpleNamespace(ip="10.0.0.2"),
        }

        result = run_experiment_with_settings("experiments/cluster.toml", settings)

        self.assertEqual(result, experiment_dir)
        remote_cluster_cls.assert_called_once_with("experiments/cluster.toml")
        get_local_commit_hash_mock.assert_called_once_with()
        create_experiment_dir_mock.assert_called_once_with("logs")
        dump_run_config_mock.assert_called_once_with(
            "experiments/cluster.toml",
            experiment_dir,
            settings,
            sweep_values=None,
        )
        cluster.run.assert_any_call("node1", build_repo_prepare_commands("abc123", settings), hide=False)
        cluster.rsync_from.assert_called_once_with("node1", settings.remote_bin_dir, str(experiment_dir))
        cluster.rsync_to_all.assert_called_once_with(
            {
                "node1": [(str(experiment_dir / "bin"), settings.remote_tmp_dir)],
                "node2": [(str(experiment_dir / "bin"), settings.remote_tmp_dir)],
            }
        )
        cluster.run_all.assert_called_once_with(
            {
                "node2": (
                    "/tmp/bin/ktbench "
                    "-num-users 10000 "
                    "-num-load-clients 10 "
                    "-num-run-clients 10 "
                    "-num-versions 2000 "
                    "-run-duration-secs 30 "
                    "-protocol samurai "
                    "--addr 10.0.0.1:3191 "
                    "> /data/shubham_mishra/node2.log 2>&1"
                )
            },
            hide=False,
        )
        cluster.rsync_from_all.assert_called_once_with(
            {
                "node1": [(settings.server_log_path, "logs/run-1/server.log")],
                "node2": [(f"{settings.remote_base_dir}/node2.log", "logs/run-1/node2.log")],
            }
        )
        sleep_mock.assert_called_once_with(2.0)

    @patch("experiments.kt.run_experiment_with_settings", return_value=Path("logs/run-2"))
    @patch("experiments.kt.load_sweeping_parameters", return_value=None)
    @patch("experiments.kt.load_experiment_settings")
    def test_run_experiment_loads_settings_from_toml(
        self,
        load_experiment_settings_mock: MagicMock,
        load_sweeping_parameters_mock: MagicMock,
        run_experiment_with_settings_mock: MagicMock,
    ) -> None:
        settings = make_settings()
        load_experiment_settings_mock.return_value = settings

        result = run_experiment("experiments/cluster.toml")

        self.assertEqual(result, Path("logs/run-2"))
        load_experiment_settings_mock.assert_called_once_with("experiments/cluster.toml")
        load_sweeping_parameters_mock.assert_called_once_with("experiments/cluster.toml")
        run_experiment_with_settings_mock.assert_called_once_with("experiments/cluster.toml", settings)

    @patch("experiments.kt.run_experiment_with_settings")
    @patch("experiments.kt.create_experiment_dir", return_value=Path("logs/run-3"))
    @patch(
        "experiments.kt.load_sweeping_parameters",
        return_value={
            "batch_size": [10, 20],
            "num_users": [100, 200],
        },
    )
    @patch("experiments.kt.load_experiment_settings")
    def test_run_experiment_repeats_for_sweeping_parameter_cross_product(
        self,
        load_experiment_settings_mock: MagicMock,
        load_sweeping_parameters_mock: MagicMock,
        create_experiment_dir_mock: MagicMock,
        run_experiment_with_settings_mock: MagicMock,
    ) -> None:
        settings = make_settings()
        load_experiment_settings_mock.return_value = settings

        result = run_experiment("experiments/cluster.toml")

        self.assertEqual(result, Path("logs/run-3"))
        load_experiment_settings_mock.assert_called_once_with("experiments/cluster.toml")
        load_sweeping_parameters_mock.assert_called_once_with("experiments/cluster.toml")
        create_experiment_dir_mock.assert_called_once_with("logs")
        self.assertEqual(run_experiment_with_settings_mock.call_count, 4)
        run_experiment_with_settings_mock.assert_any_call(
            "experiments/cluster.toml",
            KtExperimentSettings(
                **{
                    **settings.__dict__,
                    "server_batch_size": 10,
                    "bench_num_users": 100,
                }
            ),
            experiment_dir=Path("logs/run-3/batch_size_10_num_users_100"),
            sweep_values={"batch_size": 10, "num_users": 100},
        )
        run_experiment_with_settings_mock.assert_any_call(
            "experiments/cluster.toml",
            KtExperimentSettings(
                **{
                    **settings.__dict__,
                    "server_batch_size": 10,
                    "bench_num_users": 200,
                }
            ),
            experiment_dir=Path("logs/run-3/batch_size_10_num_users_200"),
            sweep_values={"batch_size": 10, "num_users": 200},
        )
        run_experiment_with_settings_mock.assert_any_call(
            "experiments/cluster.toml",
            KtExperimentSettings(
                **{
                    **settings.__dict__,
                    "server_batch_size": 20,
                    "bench_num_users": 100,
                }
            ),
            experiment_dir=Path("logs/run-3/batch_size_20_num_users_100"),
            sweep_values={"batch_size": 20, "num_users": 100},
        )
        run_experiment_with_settings_mock.assert_any_call(
            "experiments/cluster.toml",
            KtExperimentSettings(
                **{
                    **settings.__dict__,
                    "server_batch_size": 20,
                    "bench_num_users": 200,
                }
            ),
            experiment_dir=Path("logs/run-3/batch_size_20_num_users_200"),
            sweep_values={"batch_size": 20, "num_users": 200},
        )

    def test_dump_run_config_writes_resolved_settings_json(self) -> None:
        from experiments.kt import dump_run_config

        settings = make_settings()
        with tempfile.TemporaryDirectory() as temp_dir:
            experiment_dir = Path(temp_dir)
            dump_run_config(
                "experiments/cluster.toml",
                experiment_dir,
                settings,
                sweep_values={"batch_size": 2048},
            )

            config_dump = (experiment_dir / "config_used.json").read_text(encoding="utf-8")

        self.assertIn('"source_config_path": "experiments/cluster.toml"', config_dump)
        self.assertIn('"server_batch_size": 1024', config_dump)
        self.assertIn('"applied_sweeping_parameters"', config_dump)
        self.assertIn('"batch_size": 2048', config_dump)


if __name__ == "__main__":
    unittest.main()
