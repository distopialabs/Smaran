# Smaran

Smaran is an authenticated data structure that serves time-travel queries
(over a single point in the past or an interval) with a constant number of
fixed-size proofs, independent of the query range or history length. It
combines segment trees with cryptographic vector commitments, integrated
into two applications: key transparency and decentralized ledgers.

This repository is the artifact for the SOSP paper on **Smaran**. It
reproduces both evaluation usecases with one standard workflow:

| Usecase | Figures | Scope name | Guide |
|---|---|---|---|
| **Key Transparency** (§7.1) | 4a, 4b, 4c, 5 | `kt` | [below](#key-transparency) |
| **Decentralized Ledger** (§7.2) | 6a-6c, 7a-7c | `dl` | [below](#decentralized-ledger) |

## How to run: the three choices

**1. Where does it run?**

| Setup | What you do |
|---|---|
| **A. CloudLab, our profile** *(recommended: zero install)* | Instantiate [the profile](https://www.cloudlab.us/p/DistopiaLabs/smaran-artifact); both nodes self-configure at boot ([Quick start](#quick-start-cloudlab-profile)) |
| **B. CloudLab, manual** | Instantiate any two-node Ubuntu 22.04 experiment, then follow the same steps as C. Exception: inter-node SSH is automatic, since `./run.sh setup` fetches the experiment key on any CloudLab node (run it once per node) |
| **C. Your own servers** | Two Ubuntu 22.04 machines that can SSH each other; see [Install](#install-paths-bc-one-time). DL needs the dataset ([Zenodo DOI 10.5281/zenodo.21317398](https://doi.org/10.5281/zenodo.21317398)); KT needs no dataset |

**2. Where do you drive it from?** Everything is `./run.sh` on node0. Either
SSH in and use it directly, or stay on your laptop and let `run_ae.sh` call
it over SSH for you (no prior login needed on the profile; CloudLab installs
your key at boot):

| On node0 | From your laptop |
|---|---|
| `./run.sh start quick all` | `bash run_ae.sh <user> <node0-host> start quick all` |
| `./run.sh status` | `bash run_ae.sh <user> <node0-host> status` |
| `./run.sh follow` | `bash run_ae.sh <user> <node0-host> follow` |
| `./run.sh results` | `bash run_ae.sh <user> <node0-host> fetch ~/Desktop/smaran-figs` |

(`run_ae.sh` is in this repo; see [Run from your laptop](#run-from-your-laptop-optional).)

**3. What do you run?** `start <mode> <scope>`:

| Mode | Meaning | KT time | DL time |
|---|---|---|---|
| `smoke` | pipeline check, one point | ~5 min | ~2 min (plots from paper logs) |
| `quick` | reduced sweep, same qualitative trends | ~2 h | ~50 min |
| `full` | paper scale | ~3 h | tens of hours (full ingest) |

Scope is `all`, `kt`, `dl`, or individual figures (`fig4a` ... `fig7c`), e.g.
`./run.sh start quick fig6a fig7b`. Bare `./run.sh` walks you through the
same choices as menus and prints the equivalent command.

**Runs are detached by default**: `start` returns immediately with a time
estimate; closing your terminal never loses a run. `status` shows progress,
`follow` tails the log live (Ctrl+C only stops watching), `stop` aborts
(rerunning is safe, since finished figures are cached), `results` lists or
copies the PDFs.

The rest of this README: [Quick start](#quick-start-cloudlab-profile),
[where output lands](#where-everything-lives), then one guide per usecase
with the same structure (the figures, what to check, usecase notes):

- **[Key Transparency](#key-transparency)**: Smaran as a key-transparency
  log, vs **CONIKS** and **OPTIKS**, four figures.
- **[Decentralized Ledger](#decentralized-ledger)**: Smaran as a sharded
  KZG-based decentralized ledger, vs **Merkle (MPT)** and **Verkle**, six
  figures.

Shared reference material follows the guides: [full-scale runs](#full-scale-runs),
[laptop driving](#run-from-your-laptop-optional), [viewing figures](#viewing-figures),
[install](#install-paths-bc-one-time), [troubleshooting](#troubleshooting),
[code layout](#code-layout).

> **Naming note:** the system was renamed to *Smaran* during submission; the
> codebase still uses its original name *samurai* in binary, package, and log
> names (`samuraimpt` in query-benchmark logs). Reviewer-facing scripts say
> Smaran; anything on disk named samurai is the same system.

## Quick start (CloudLab profile)

**Step 0: register an SSH key on your CloudLab account (before
instantiating).** CloudLab installs your public key on the nodes *at boot
time only*; without one registered first you will get
`Permission denied (publickey)` when you SSH in.

```bash
ls ~/.ssh/id_ed25519.pub || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519
cat ~/.ssh/id_ed25519.pub    # copy the whole line
```

Paste it at <https://www.cloudlab.us/manage_profile.php?nav=ssh> → **Add
Key**. If you already instantiated before adding the key, either terminate
that experiment and instantiate again (keys are only pushed at boot), or use
the browser shell on the experiment page (per-node menu → **Shell**, no key
needed) to append your public key to `~/.ssh/authorized_keys` on node0
yourself.

1. **Instantiate** [the profile](https://www.cloudlab.us/p/distopialabs-PG0/smaran-artifact).
   The defaults are the paper's node pair (r6615 server + c6420 client at
   Clemson); if unavailable, pick a fallback pair from the parameter
   dropdowns (kept in the same cluster, because the dataset is
   cluster-local; priority order in
   [Hardware fallbacks](#hardware-fallbacks-cloudlab-profile)).
2. **Wait until the Startup column shows `Finished` for both nodes** on the
   experiment page (~4 min after boot). The green "ready" banner appears
   earlier, at boot. The SSH login banner is the authoritative signal:
   `setup READY` means go; `IN PROGRESS` means wait (it shows a log to
   watch); `FAILED` means see `/local/setup.log`.
3. **SSH into `node0`** (the client, and the only node you ever touch) and
   go to the repo clone:
   ```bash
   cd /local/repository
   ```
4. **Smoke check** (~8 min): KT runs one full pipeline point; DL regenerates
   all six paper figures from the curated paper logs:
   ```bash
   ./run.sh start smoke all
   ./run.sh status        # until it says done
   ```
5. **First real experiment** (~16 min): one quick-scale DL figure
   end-to-end (ingest on the server node → serve → 32 proof clients → plot):
   ```bash
   ./run.sh start quick fig6a
   ```
6. **Look at the PDFs**: `./run.sh results`, and see
   [Viewing figures](#viewing-figures).

Target: first figure within ~15 minutes of clicking the profile link.

## Where everything lives

Everything you look at is on **node0 (client)** under
`/local/repository/results/` (DL) and `/local/repository/output/` (KT);
everything heavy is on node1 (server) and you never touch it.

| What | Where | Node |
|---|---|---|
| Code (repo clone) | `/local/repository` | both |
| Block dataset, account CSVs, paper logs | `/smaran-dataset` (read-only mount) | both |
| Ingested databases (big, regenerable) | `/data/local/artifact-dbs` | server |
| Benchmark logs | `results/logs/` and per-figure `results/fig*/logs/` (DL); `logs/` (KT) | client |
| DL figures (PDFs) | `results/fig*/` and `results/paper-figures/` | client |
| KT figures (PDFs) | `output/` | client |
| Cluster config (generated by the profile) | `/local/cluster.env` | both |

On manual/own-server setups you write `/local/cluster.env` yourself (one
line, `SERVER_HOST=...`; see [Install](#install-paths-bc-one-time)); the
scripts then behave identically.

## Key Transparency

Smaran vs **CONIKS** and **OPTIKS** as key-transparency logs (§7.1). Each
figure is one sweep across the three protocols: `ktserver` runs on node1
(built there automatically from a GitHub clone pinned to this repo's
commit), `ktbench` drives it from node0, and a PDF lands in `output/`.
Run with `./run.sh start <mode> kt` (or per-figure, e.g. `quick fig4a`).

### The figures

| Figure | Shows | Quick | Full |
|---|---|---|---|
| `fig4a` | monitoring-query latency vs versions | ~50 min (runs the fig4 sweep) | 11 version counts {2 ... 2047} |
| `fig4b` / `fig4c` | throughput / payload vs versions | ~1 min (re-plot of 4a's sweep) | re-plot of the full sweep |
| `fig5` | key-update (put) throughput vs users | ~55 min (3 user counts) | 6 user counts {10k ... 1M} |

Full `kt` scope is ~3 h compute; quick is ~2 h with 5 version counts and 3
user counts, keeping the trends and protocol ordering. Figures 4a/4b/4c
come from one sweep: the first script to run performs it, the other two
re-plot from its logs (`KT_FORCE_RERUN=1` to redo).

Underlying scripts: `KeyTransparencyScripts/run_fig*.sh` (full) and
`QuickTesting-KeyTransparency/run_fig*_quick.sh` (quick), same scripts with
reduced parameters; `./run.sh` invokes these.

### What to check

Absolute numbers depend on hardware; trends and protocol ordering are the
criteria.

- **Automated:** `python3 KeyTransparencyScripts/verify.py` checks the
  shapes against the paper's §7.1 claims; exit code 0 means all of them
  held on your data.
- **Visual:** compare `output/*.pdf` against the paper's Figures 4a/4b/4c/5,
  or against [`reference_pdfs/`](reference_pdfs/), the PDFs from our own
  full sweep on the paper's hardware pair.

| Figure | Expected shape |
|---|---|
| 4a latency | Coniks steepest rise, reaching ~5 s at 2047 versions. Optiks linear rise. Smaran near-flat, then climbs after 512. |
| 4b throughput | Optiks highest at low versions, crosses below Smaran around 128-256. Smaran near-flat then declines. Coniks lowest throughout. |
| 4c payload | Optiks and Coniks track together, growing steeply. Smaran grows much more slowly. |
| 5 put throughput | Broken y-axis. Upper: Optiks and Smaran in tens of thousands of ops/s with mild decline. Lower: Coniks flat at ~640 ops/s. |

### KT notes

- **Single run per point.** The paper averages 3 runs; the artifact runs
  each point once to fit the time budget, so individual points may look
  noisier than the paper (Fig 4a Smaran at 700 and 1500 versions, Fig 5
  Optiks between 50k and 1M users). Shape and ordering are preserved.
- **Coniks fork.** The `Coniks/` submodule is `coniks-history-extension`, a
  fork of official CONIKS with a Merkle Patricia Trie extension. Its
  per-request cost is user-count-independent, which is why our Fig 5 Coniks
  curve is flat while the paper's declines slightly. Fig 4 shape matches
  the paper.
- KT needs no dataset; everything is generated during the sweep.

## Decentralized Ledger

Smaran vs **Merkle (MPT)** and **Verkle** as decentralized-ledger state
(§7.2). Each figure ingests Ethereum blocks on node1, serves proofs to
clients on node0, and lands a PDF under `results/`. Run with
`./run.sh start <mode> dl` (or per-figure, e.g. `quick fig6a`).

### The figures

Each script announces `Running experiment Figure <yy>`, prints one
`Running <protocol> with <x> ...` line per data point, then `Plotting`, and
ends with the figure's path. The table uses the underlying per-figure
scripts; `./run.sh` invokes these same scripts (`start smoke dl` =
`plot_paper_figures.sh`, "Tier 0").

| Script | Shows | Quick | Full scale (extrapolated) |
|---|---|---|---|
| `plot_paper_figures.sh` | all six, from paper logs | ~1 min | n/a |
| `run_fig6a.sh` (first of 6a/6b/6c) | query latency vs range | ~16 min first run; ~9 min on cached DBs | one-time full ingest per protocol: tens of hours; then ~25 min per protocol |
| `run_fig6b.sh` / `run_fig6c.sh` | throughput / payload size | seconds (re-plot of 6a's sweep) | seconds |
| `run_fig7a.sh` | ingestion throughput | ~11 min | ~19 h |
| `run_fig7b.sh` | archival storage impact | ~9 min (reuses fig6's DB) | ~1.5 h after fig6's ingest |
| `run_fig7c.sh` | sharding impact | ~12 min | ~5 h |

Quick-tier times are as measured end-to-end on the paper's two-node
CloudLab pair (r6615 server + c6420 client); slower storage inflates them
considerably, chiefly through Smaran's shard-database setup/teardown.

Underlying scripts: `DecentralizedLedgerScripts/run_fig*.sh` (full) and
`QuickTesting-DecentralizedLedgerScripts/run_fig*.sh` (quick), same scripts
with reduced parameters. All parameters (both tiers) are defined in one
place, **`DecentralizedLedgerScripts/config.sh`**, and any value can be
overridden per-run via environment variables (examples in that file).

### What to check

Absolute numbers at quick scale are far below the paper's (small ingest
window, short durations); the *trends* are what to check. Compare
`results/fig*/` against the paper's Figures 6-7, or against
`results/paper-figures/` (regenerated from the paper's own logs by the
smoke tier).

| Figure | Expected shape |
|---|---|
| 6a latency | Smaran and MPT track at small ranges; MPT keeps climbing past ~100 while Smaran flattens. Verkle far above both. |
| 6b throughput | Mirror of 6a: Smaran near-flat, dominant at large ranges; MPT falls steeply below it; Verkle lowest. |
| 6c payload | Smaran near-constant; MPT and Verkle grow roughly linearly with range, orders of magnitude larger. |
| 7a ingestion | Smaran above MPT above Verkle at every user count, all gently declining. |
| 7b archival | Full scale: the archival leg stays flat while the non-archival leg climbs at large ranges. Quick scale: the two legs overlap (see notes). |
| 7c sharding | Throughput scales monotonically with shard count at every user count. |

### DL notes

- **Figures 6a/6b/6c come from one benchmark sweep.** The first of the three
  scripts runs it; the other two reuse the cached logs and just re-plot
  (`FORCE_RERUN=1` to redo).
- **Ingested databases are cached** under `/data/local/artifact-dbs` and
  reused across runs (Figure 7b reuses Figure 6's Smaran database).
- **Smaran runs have a fixed setup cost:** creating/opening its ~1000 shard
  databases adds a delay before ingestion or serving begins (about a minute
  on NVMe, several minutes on slower disks), and again at teardown. The
  scripts say so while you wait; this is normal.
- **Interrupting is safe.** Ctrl+C stops the run and its servers (on both
  nodes); rerunning redoes any unfinished work from clean state.
- **Quick figures omit the Cauchy baseline.** Cauchy exists only as prebaked
  paper-scale logs (a separate Rust codebase, far too slow to rerun), which
  would sit meaninglessly next to quick-scale numbers; Tier 0 and full-scale
  figures include it.
- **Quick-scale fig7b: the two curves nearly overlap. Expected, not a
  bug.** The gains of Smaran's archival storage are realized when the
  ingested window is large: hundreds of thousands to millions of blocks
  (the paper ingests 2.6M). At the quick tier's 10k-block window Smaran
  performs the same with or without it: we verified the two legs run
  distinct code paths and stay within a few percent even at a 60k-block
  window with query ranges to 50k. Smaran being this fast *without*
  archival storage at small scales is itself a good property, but it means
  the paper's visual separation only appears at full scale (or raise
  `N_BLOCKS` / `RANGES_7B` yourself; see `config.sh`).
- DL needs the block dataset: mounted automatically on the profile, from
  [Zenodo](https://doi.org/10.5281/zenodo.21317398) elsewhere (see
  [Install](#install-paths-bc-one-time)).

## Full-scale runs

`./run.sh start full kt` and `./run.sh start full dl` run each usecase at
paper scale (KT ~3 h; DL tens of hours, dominated by the one-time full
ingests). For per-figure control, the underlying DL scripts offer the same
detached behavior:

```bash
./DecentralizedLedgerScripts/run_fig<yy>.sh --detach
```

Identical scripts at the paper's parameters (DL: query ranges to 2.6M, 32
clients, 2 min per point, 5 user counts to 2M, 6 shard counts to 1000; KT:
11 version counts to 2047, 6 user counts to 1M). **Always use `--detach`**
for full-scale runs: the run survives SSH disconnects, and

```bash
./DecentralizedLedgerScripts/status.sh
```

shows each run's state, elapsed vs estimated time, last progress line, and,
when finished, the figure's path. Console output is captured to
`results/logs/<figure>.console.log`. `--detach` works for quick runs too;
inline (no flag) is the default everywhere.

### Hardware fallbacks (CloudLab profile)

If the paper pair is busy, try in this order (profile parameter dropdowns);
server types are NVMe-only by design:

| Priority | Server | Client | Cluster |
|---|---|---|---|
| 1 (paper) | r6615 | c6420 | Clemson |
| 2 | r650 | c6420 | Clemson |
| 3 | r6525 | c6420 (or second r6525) | Clemson |
| 4 | c6525-100g | xl170 | Utah |

## Run from your laptop (optional)

No experiment ever runs on your laptop. `run_ae.sh` (repo root) drives
`run.sh` on node0 remotely; see [the table at the top](#how-to-run-the-three-choices).
Get it without cloning:

```bash
curl -sLO https://raw.githubusercontent.com/distopialabs/Smaran/main/run_ae.sh
bash run_ae.sh <user> <node0-host> start quick all
```

To copy everything (figures and logs) back by hand:

```bash
rsync -a <user>@<node0>:/local/repository/results/ ./smaran-results/
```

## Viewing figures

PDFs can't render in a terminal; in order of convenience:

1. **VS Code Remote-SSH** *(recommended)*: connect to node0, open
   `/local/repository`, and click any PDF under `results/` or `output/`;
   terminal and file browser in the same window.
2. **Copy to your laptop**: `bash run_ae.sh <user> <node0-host> fetch`, or
   the `rsync` one-liner above.
3. **Throwaway HTTP preview**: `cd results && python3 -m http.server 8000`,
   then browse `http://<node0>:8000` (remember to stop it: open port).

## Install (Paths B/C, one-time)

The artifact runs on **two** Ubuntu 22.04 machines that can SSH each other:
a *server* (NVMe/SSD, ~100 GB free, 64 GB+ RAM) and a *client* you work
from, mirroring the paper's topology and the CloudLab profile. (Manual
CloudLab experiments are exactly this: treat node0 as the client, node1 as
the server.)

On **both** machines:

1. Clone this repo (same path on both keeps things simple).
2. Get the block dataset from
   [Zenodo (DOI 10.5281/zenodo.21317398)](https://doi.org/10.5281/zenodo.21317398)
   and point the scripts at it: `export SMARAN_DATASET_DIR=<dir with blk_*.dat>`
   (a shared/NFS copy is fine). KT does not use the dataset.
3. Run any install script (idempotent; all three leave everything installed:
   Go 1.25, LaTeX + Python plotting stack, all binaries in `bin/`,
   `/data/local` made writable, dataset located):
   ```bash
   ./DecentralizedLedgerScripts/install_merkle.sh
   ./DecentralizedLedgerScripts/install_verkle.sh
   ./DecentralizedLedgerScripts/install_smaran.sh
   ```
   For KT, also run the three KT installers (idempotent as well) on both
   machines:
   ```bash
   ./KeyTransparencyScripts/install_coniks.sh
   ./KeyTransparencyScripts/install_optiks.sh
   ./KeyTransparencyScripts/install_smaran.sh
   ```
4. Tell the scripts where the server is: create `/local/cluster.env`
   (both machines) with:
   ```
   SERVER_HOST=<server hostname or IP>
   ```
   For KT, also set the two host variables at the top of
   `KeyTransparencyScripts/nodes.env` (created from the template on first
   `./run.sh setup`) if your hostnames are not `node0`/`node1`.

On the **client**:

5. Regenerate the account-statistics CSVs (Zenodo carries only the blocks;
   ~5 s + ~4 min, needs ~10 GB RAM):
   ```bash
   ./scripts/artifact/generate_account_stats.sh
   ```
6. Verify: `./run.sh setup`, then
   `./DecentralizedLedgerScripts/check_setup.sh` (every line ✓).

The paper-logs bundle (Tier 0 and the prebaked Cauchy series) ships in the
CloudLab dataset; on your own machine, obtain `smaran-paper-logs.tar.gz` from
the CloudLab dataset or the authors and set
`SMARAN_PAPER_LOGS=<extracted paper-logs dir>`. Fresh benchmark figures plot
without the Cauchy series if the bundle is absent (Cauchy exists only in
prebaked form; it comes from a separate Rust codebase and is far too slow to
rerun).

## Troubleshooting

- **A script says `Setup incomplete: ...`**: it names the missing piece and
  the fix. On CloudLab this usually means node setup is still running (the
  SSH login banner shows its status). Full diagnosis:
  `./DecentralizedLedgerScripts/check_setup.sh`.
- **Login banner says `setup FAILED`**: `cat /local/setup.log` on that node;
  re-run `sudo bash /local/repository/cloudlab/setup-node.sh <role> <server-ip>`
  or re-instantiate.
- **A run was interrupted / a node rebooted**: just rerun the script;
  servers left behind are cleaned up automatically and unfinished benchmark
  points are redone. `status.sh` reports a detached run whose process died.
- **A KT sweep hangs for more than ~5 min on one point**: `./run.sh stop`,
  delete the newest directory under `logs/`, rerun (finished protocols are
  cached).
- **Two-node scripts report `Permission denied` between the nodes**: rerun
  `./run.sh setup` on node0. If it broke *after you changed your SSH keys on
  the CloudLab portal mid-experiment*: CloudLab then rewrites
  `~/.ssh/authorized_keys` on every node, removing the experiment-internal
  key the setup added. SSH into node1 (your portal key still works) and run
  `cat ~/.ssh/id_cloudlab.pub >> ~/.ssh/authorized_keys` to restore it.
- **Dataset mounted somewhere unusual**: `export SMARAN_DATASET_DIR=<dir>`.
- **Anything else**: open an issue at
  <https://github.com/distopialabs/Smaran/issues>.

## Code layout

All three DL protocols share one Go module (`internal/` holds the shared
dataset, benchmark, and proof machinery); each protocol has its own
entry-point directories:

| Protocol | Server / ingestion binary | Query (proof) client |
|---|---|---|
| Merkle (MPT) | `cmd/merkle` | `cmd/merkle-proofc` |
| Verkle | `cmd/verkle` | `cmd/verkle-proofc` |
| Smaran | `cmd/samurai` | `cmd/proofc` |

The Key Transparency usecase has its own binaries, `cmd/ktserver` and
`cmd/ktbench` (Smaran and OPTIKS in one binary; CONIKS builds from the
`Coniks/` submodule), with KT-specific tree/proof variants under
`internal/kt/` and scripts in **`KeyTransparencyScripts/`** +
**`QuickTesting-KeyTransparency/`**.

DL reviewer-facing scripts live in **`DecentralizedLedgerScripts/`**
(install, full-scale experiments, plot-only mode, `check_setup.sh`,
`status.sh`) and **`QuickTesting-DecentralizedLedgerScripts/`** (quick
variants of the same scripts). The CloudLab profile is **`profile.py`** at
the repo root (where CloudLab requires it); node setup and the image recipe
live in **`cloudlab/`**. All experiment output is written under **`results/`**
(gitignored). `docs/DEVELOPMENT.md` documents every binary and subcommand.
