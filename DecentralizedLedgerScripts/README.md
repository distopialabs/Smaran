# DecentralizedLedgerScripts

Reviewer-facing scripts for the Smaran artifact. See the repository README for
the full guide, requirements, and time estimates.

| Script | Purpose |
|---|---|
| `install_merkle.sh` / `install_verkle.sh` / `install_smaran.sh` | One-time setup (toolchain, build, plot deps, dataset). Idempotent; any one leaves all three protocols installed. |
| `plot_paper_figures.sh` | Regenerate all six paper figures from the curated paper logs (no benchmarks; ~5 min). |
| `run_fig6a.sh` / `run_fig6b.sh` / `run_fig6c.sh` | Query latency / throughput / payload figures. The three share one benchmark sweep; the first run executes it, the others re-plot from the cached logs (`FORCE_RERUN=1` to redo). |
| `run_fig7a.sh` | Ingestion (commitment generation) throughput across protocols and user counts. |
| `run_fig7b.sh` | Archival-storage impact: Smaran optimus (stored roots) vs non_optimus (`--old`). |
| `run_fig7c.sh` | Sharding impact on Smaran ingestion throughput. |
| `status.sh` | Progress of detached runs: state, elapsed vs estimate, last progress line, figure path when done. |
| `check_setup.sh` | Verbose setup verifier (troubleshooting / Path-B install check). Experiment scripts run the same checks quietly before starting. |
| `lib/` | Shared machinery (`common.sh`: install, setup checks, server lifecycle, remote mode; `experiments.sh`: parameters and pipelines). Not run directly. |

These scripts run the paper-scale parameters. For trend-scale runs, use the
identically named scripts in `../QuickTesting-DecentralizedLedgerScripts/`,
which set `QUICK=1` and exec these.

Every `run_fig*.sh` accepts `--detach`: the run continues in the background
(surviving SSH disconnects), console output goes to
`results/logs/<figure>.console.log`, and `./status.sh` tracks it. Recommended
for the full-scale runs, which take hours.

On a CloudLab experiment from our profile the scripts drive the server node
over SSH automatically (`/local/cluster.env`, written at instantiation);
without that file everything runs on the local machine.

Every script writes logs and figures under `results/` and prints the output
path when it finishes. Ingested databases are cached under
`/data/local/artifact-dbs`.
