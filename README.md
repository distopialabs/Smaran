# Smaran — Artifact Evaluation

This repository is the artifact for the SOSP paper on **Smaran**, a sharded
KZG-based decentralized ledger, evaluated against **Merkle (MPT)** and
**Verkle** baselines. It contains the full source for all three protocols and
scripts that reproduce the paper's six experiment figures (6a, 6b, 6c, 7a,
7b, 7c).

> **Naming note:** the system was renamed to *Smaran* during submission; the
> codebase still uses its original name *samurai* in binary, package, and log
> names (`samuraimpt` in query-benchmark logs). Reviewer-facing scripts say
> Smaran; anything on disk named samurai is the same system.

## Code layout

All three protocols share one Go module (`internal/` holds the shared
dataset, benchmark, and proof machinery); each protocol has its own
entry-point directories:

| Protocol | Server / ingestion binary | Query (proof) client |
|---|---|---|
| Merkle (MPT) | `cmd/merkle` | `cmd/merkle-proofc` |
| Verkle | `cmd/verkle` | `cmd/verkle-proofc` |
| Smaran | `cmd/samurai` | `cmd/proofc` |

Reviewer-facing scripts live in:

- **`DecentralizedLedgerScripts/`** — install scripts, full-scale per-figure
  experiment scripts, and plot-only mode.
- **`QuickTesting-DecentralizedLedgerScripts/`** — quick-turnaround variants
  of the same per-figure scripts (reduced ranges/users/durations; same code
  paths) that show each figure's trend without paper-scale runtimes.

All experiment output (logs, figures) is written under **`results/`**
(gitignored). `docs/DEVELOPMENT.md` documents every binary and subcommand.

## Requirements

- A CloudLab node from our profile (recommended — everything below is then
  pre-arranged), or any Ubuntu 22.04 x86_64 machine with ~40 cores, 64 GB+
  RAM, and ~100 GB free disk besides the dataset.
- The **SmaranEthereumDataset** CloudLab dataset
  (`urn:publicid:IDN+clemson.cloudlab.us:distopialabs-pg0+ltdataset+SmaranEthereumDataset`).
  It contains the ~23 GB Ethereum block dataset, the account-statistics CSVs
  used by the benchmarks, and the curated benchmark logs behind the paper's
  figures (`smaran-paper-logs.tar.gz`). Outside CloudLab, the block dataset
  is also downloadable from Zenodo (link TBD).

## Install (one-time, ~5 min + dataset acquisition)

```bash
./DecentralizedLedgerScripts/install_merkle.sh
./DecentralizedLedgerScripts/install_verkle.sh
./DecentralizedLedgerScripts/install_smaran.sh
```

Each prints `Installing <protocol>`. The three protocols share one build, so
the scripts are idempotent and any one of them leaves everything installed:
Go 1.25, LaTeX + Python plotting stack, all binaries in `bin/`, `/data/local`
made writable, and the dataset located (set `SMARAN_DATASET_DIR=<dir>` if it
is mounted somewhere unusual).

## Tier 0 — kick the tires (~2 min)

```bash
./DecentralizedLedgerScripts/plot_paper_figures.sh
```

Regenerates **all six paper figures** from the curated benchmark logs that
produced the figures in the paper (no benchmarks run). Output:
`results/paper-figures/`. Compare against the paper. This verifies the whole
plotting toolchain and shows the expected shape of every figure — including
the Cauchy series, which exists only in prebaked form (it comes from a
separate Rust codebase and is far too slow to rerun; fresh runs plot it from
these logs automatically, or omit it if the bundle is absent).

## Tier 1 — quick-turnaround experiments

```bash
./QuickTesting-DecentralizedLedgerScripts/run_fig6a.sh   # also 6b, 6c
./QuickTesting-DecentralizedLedgerScripts/run_fig7a.sh
./QuickTesting-DecentralizedLedgerScripts/run_fig7b.sh
./QuickTesting-DecentralizedLedgerScripts/run_fig7c.sh
```

Same pipeline as the full experiments (ingest → serve → benchmark → plot)
with reduced parameters: Figures 6a/6b/6c use query ranges up to 1000 over a
10k-block ingest; Figure 7b uses the paper's range points below 10k; Figure
7a uses 100k/200k/500k users with a 10k-block limit per point; Figure 7c uses
shard counts 1/10/100. Each script announces `Running experiment Figure
<yy>`, prints one `Running <protocol> with <x> ...` line per data point, then
`Plotting`, and ends with the figure's path under `results/`.

All parameters (both tiers) are defined in one place —
**`DecentralizedLedgerScripts/config.sh`** — and any value can also be
overridden per-run via environment variables (examples in that file).

Notes that apply to both tiers:

- **Figures 6a/6b/6c come from one benchmark sweep.** The first of the three
  scripts runs it; the other two reuse the cached logs and just re-plot
  (`FORCE_RERUN=1` to redo).
- **Ingested databases are cached** under `/data/local/artifact-dbs` and
  reused across runs (Figure 7b reuses Figure 6's Smaran database).
- **Smaran runs have a fixed setup cost:** creating/opening its ~1000 shard
  databases adds a delay before ingestion or serving begins (about a minute
  on NVMe/SSD storage, several minutes on slower disks), and again at
  teardown. The scripts say so while you wait — this is normal.
- Absolute numbers at quick scale are far below the paper's (small ingest
  window, short durations); the *trends* are what to check.

### Estimated times (human ≈ 1 min to launch each; compute below)

| Script | Quick | Full scale (extrapolated) |
|---|---|---|
| `plot_paper_figures.sh` | ~1 min | — |
| `run_fig6a.sh` (first of 6a/6b/6c) | ~15 min (one-time ingest per protocol; ~7 min re-run on cached databases) | one-time full ingest per protocol: tens of hours; then ~25 min benchmarking per protocol |
| `run_fig6b.sh` / `run_fig6c.sh` (cached) | seconds (re-plot from the cached sweep) | seconds |
| `run_fig7a.sh` | ~11 min | ~19 h (15 min × 15 points + Smaran overhead) |
| `run_fig7b.sh` | ~9 min (reuses fig6's database) | ~1.5 h benchmarking after fig6's ingest |
| `run_fig7c.sh` | ~12 min | ~5 h (5 min × 30 points + shard setup) |

Full-scale figures need the complete 2.6M-block window ingested once per
protocol; these estimates are extrapolations from measured small-scale runs
and the paper logs' recorded durations. Quick-tier numbers were measured on
an SSD (NVMe)-backed CloudLab node — the recommended hardware; slower
storage inflates them considerably, chiefly through Smaran's shard-database
setup/teardown.

## Tier 2 — full-scale experiments

```bash
./DecentralizedLedgerScripts/run_fig<yy>.sh
```

Identical scripts at the paper's parameters (query ranges to 2.6M, 32
clients, 2 min per point, 5 user counts to 2M, 6 shard counts to 1000). See
the table above before starting — the one-time full ingests are the dominant
cost.
