# Smaran — Artifact Evaluation

This document is the entry point for SOSP artifact evaluators. It reproduces
**Figures 4a, 4b, 4c, and 5** of the Smaran paper.

## What's here

| Path                                             | Purpose                                       |
|--------------------------------------------------|-----------------------------------------------|
| [`Coniks/`](Coniks/)                             | CONIKS (git submodule)                        |
| [`Optiks/`](Optiks/)                             | OPTIKS pointer (implementation in `internal/kt/optiks.go`) |
| [`KeyTransparencyScripts/`](KeyTransparencyScripts/) | Install + full-sweep experiment scripts   |
| [`QuickTesting-KeyTransparency/`](QuickTesting-KeyTransparency/) | Reduced-sweep experiment scripts |
| [`cloudlab-profile/`](cloudlab-profile/)          | CloudLab profile + pre-built image URN       |

## Two evaluation paths

### Path A — pre-built CloudLab image (recommended)

1. Open [`cloudlab-profile/README.md`](cloudlab-profile/README.md) and follow
   the "Instantiate the pre-built image" instructions.
2. SSH into `node0`, `cd ~/Smaran/KeyTransparencyScripts`, and run the
   experiments (see below). All three protocols are pre-installed.

### Path B — install from source on your own two nodes

1. Provision two Ubuntu 22.04 machines (recommended: CloudLab r6615 + c6420).
2. On each node:
   ```
   git clone --recurse-submodules https://github.com/distopialabs/Smaran.git
   cd Smaran/KeyTransparencyScripts
   ./install_coniks.sh
   ./install_optiks.sh
   ./install_smaran.sh
   ```
3. Copy `KeyTransparencyScripts/nodes.env.template` to
   `KeyTransparencyScripts/nodes.env`, edit the values.
4. Run the experiments.

## Experiment scripts

Each script prints a banner, per-datapoint progress, then `Plotting`, then
saves a PDF to `<repo>/output/`.

### Full sweeps (paper figures)

| Script | Figure | Sweep axis | Human time | Compute time |
|---|---|---|---|---|
| `KeyTransparencyScripts/run_fig4a.sh` | 4a — latency vs versions   | versions ∈ {2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2047} | ~10 min | ~60–75 min |
| `KeyTransparencyScripts/run_fig4b.sh` | 4b — throughput vs versions | same sweep as 4a *(reuses cache if 4a ran first)* | ~10 min | ~60–75 min (or ~30 s if cached) |
| `KeyTransparencyScripts/run_fig4c.sh` | 4c — payload vs versions   | same sweep as 4a *(reuses cache if 4a ran first)* | ~10 min | ~60–75 min (or ~30 s if cached) |
| `KeyTransparencyScripts/run_fig5.sh`  | 5  — put throughput vs users | users ∈ {10k, 30k, 100k, 200k, 500k, 1M} | ~15 min | ~90–120 min |

If you only care about Figure 4 as a whole, run **`run_fig4a.sh`** first,
then `run_fig4b.sh` and `run_fig4c.sh` finish in seconds using the cached
sweep. Total compute for Figure 4 in that mode is ~60–75 min, not 3×.

Total end-to-end for all four figures: **~2.5–3.5 hours of compute + ~45 min of human time**.

### Quick-turnaround (reduced sweeps)

| Script | Figure | Reduced sweep | Human time | Compute time |
|---|---|---|---|---|
| `QuickTesting-KeyTransparency/run_fig4a_quick.sh` | 4a | versions ∈ {2, 16, 128, 256, 2047}         | ~5 min  | ~25–35 min |
| `QuickTesting-KeyTransparency/run_fig4b_quick.sh` | 4b | same (cached from 4a_quick)                | ~5 min  | ~25–35 min (or ~30 s if cached) |
| `QuickTesting-KeyTransparency/run_fig4c_quick.sh` | 4c | same (cached from 4a_quick)                | ~5 min  | ~25–35 min (or ~30 s if cached) |
| `QuickTesting-KeyTransparency/run_fig5_quick.sh`  | 5  | users ∈ {10k, 200k, 1M}                    | ~10 min | ~40–60 min |

Total end-to-end for all four quick figures: **~65–95 min of compute + ~25 min of human time**.

*Note*: the compute-time numbers above are estimates measured on
CloudLab r6615+c6420 nodes with `KT_RUN_DURATION=90`. They will be
refreshed after the first full run.

## Expected output

Each script writes a single PDF to `output/`:

- `fig4a_latency.pdf`
- `fig4b_throughput.pdf`
- `fig4c_payload.pdf`
- `fig5_put_throughput.pdf`

Compare each against Figure 4/5 in the paper. Absolute numbers depend on the
hardware but the shape (Smaran outperforming Coniks and matching/slightly
trailing Optiks on Figure 4/5) should be reproducible.

## Troubleshooting

- **SSH permission errors** — verify `KT_SSH_KEY` in `nodes.env` can log into
  both `KT_SERVER_HOST` and `KT_CLIENT_HOST` without a password prompt.
- **Port 3191 already in use** — a previous ktserver didn't shut down.
  `pkill ktserver` on the server node.
- **CONIKS build fails** — run `git submodule update --init Coniks` at the
  repo root and re-run `./install_coniks.sh`.
