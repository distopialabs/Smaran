# Smaran — Artifact Evaluation

Smaran is an authenticated data structure that serves time-travel queries —
over a single point in the past or an interval — with a constant number of
fixed-size proofs, independent of the query range or history length. It
combines segment trees with cryptographic vector commitments, integrated
into two applications: key transparency and decentralized ledgers.

This repository is the artifact for the SOSP paper on **Smaran**. It
reproduces both evaluation usecases with one standard workflow:

| Usecase | Figures | Scope name |
|---|---|---|
| **Key Transparency** (§7.1) | 4a, 4b, 4c, 5 | `kt` |
| **Decentralized Ledger** (§7.2) | 6a–6c, 7a–7c | `dl` |

## How to run — the three choices

**1. Where does it run?**

| Setup | What you do |
|---|---|
| **A. CloudLab, our profile** *(recommended — zero install)* | Instantiate [the profile](https://www.cloudlab.us/p/distopialabs-PG0/smaran-artifact); both nodes self-configure at boot ([Quick start](#quick-start-cloudlab-profile)) |
| **B. CloudLab, manual** | Instantiate any two-node Ubuntu 22.04 experiment, then follow the same steps as C — except inter-node SSH is automatic: `./run.sh setup` fetches the experiment key on any CloudLab node (run it once per node) |
| **C. Your own servers** | Two Ubuntu 22.04 machines that can SSH each other; see [Install](#install-paths-bc--one-time). DL needs the dataset ([Zenodo DOI 10.5281/zenodo.21317398](https://doi.org/10.5281/zenodo.21317398)); KT needs no dataset |

**2. Where do you drive it from?** Everything is `./run.sh` on node0 — either
SSH in and use it directly, or stay on your laptop and let `run_ae.sh` call
it over SSH for you (no prior login needed on the profile; CloudLab installs
your key at boot):

| On node0 | From your laptop |
|---|---|
| `./run.sh start quick all` | `bash run_ae.sh <user> <node0-host> start quick all` |
| `./run.sh status` | `bash run_ae.sh <user> <node0-host> status` |
| `./run.sh follow` | `bash run_ae.sh <user> <node0-host> follow` |
| `./run.sh results` | `bash run_ae.sh <user> <node0-host> fetch ~/Desktop/smaran-figs` |

(`run_ae.sh` is in this repo — download it, or clone the repo on your laptop.)

**3. What do you run?** `start <mode> <scope>`:

| Mode | Meaning | KT time | DL time |
|---|---|---|---|
| `smoke` | pipeline check, one point | ~4 min | ~1 min (plots from paper logs) |
| `quick` | reduced sweep, same qualitative trends | ~90 min | ~50 min |
| `full` | paper scale | ~3 h | tens of hours (full ingest) |

Scope is `all`, `kt`, `dl`, or individual figures (`fig4a` … `fig7c`), e.g.
`./run.sh start quick fig6a fig7b`. Bare `./run.sh` walks you through the
same choices as menus and prints the equivalent command.

**Runs are detached by default**: `start` returns immediately with a time
estimate; closing your terminal never loses a run. `status` shows progress,
`follow` tails the log live (Ctrl+C only stops watching), `stop` aborts
(rerunning is safe — finished figures are cached), `results` lists or copies
the PDFs.

The rest of this README: [Quick start](#quick-start-cloudlab-profile) for
the profile path, then the **Decentralized Ledger** guide in depth — Smaran
as a sharded KZG-based decentralized ledger, evaluated against **Merkle
(MPT)** and **Verkle** baselines across six figures: **6a** (query latency),
**6b** (query throughput), **6c** (payload size), **7a** (ingestion
throughput), **7b** (impact of archival storage), **7c** (impact of
sharding). The equivalent **Key Transparency** guide (Smaran vs. CONIKS and
OPTIKS) is [docs/kt-artifact-guide.md](docs/kt-artifact-guide.md).

> **Naming note:** the system was renamed to *Smaran* during submission; the
> codebase still uses its original name *samurai* in binary, package, and log
> names (`samuraimpt` in query-benchmark logs). Reviewer-facing scripts say
> Smaran; anything on disk named samurai is the same system.

## Quick start (CloudLab profile)

**Step 0 — register an SSH key on your CloudLab account (before
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
   dropdowns (kept in the same cluster — the dataset is cluster-local).
2. **Wait until the Startup column shows `Finished` for both nodes** on the
   experiment page (~4 min after boot). The green "ready" banner appears
   earlier, at boot. The SSH login banner is the authoritative signal:
   `setup READY` means go; `IN PROGRESS` means wait (it shows a log to
   watch); `FAILED` means see `/local/setup.log`.
3. **SSH into `node0`** (the client — the only node you ever touch) and go to
   the repo clone:
   ```bash
   cd /local/repository
   ```
4. **Smoke check** (~5 min) — KT runs one full pipeline point; DL
   regenerates all six paper figures from the curated paper logs:
   ```bash
   ./run.sh start smoke all
   ./run.sh status        # until it says done
   ```
5. **First real experiment** (~16 min) — one quick-scale DL figure
   end-to-end (ingest on the server node → serve → 32 proof clients → plot):
   ```bash
   ./run.sh start quick fig6a
   ```
6. **Look at the PDFs** — `./run.sh results`, and see
   [Viewing figures](#viewing-figures).

Target: first figure within ~15 minutes of clicking the profile link.

## Where everything lives

Everything you look at is on **node0 (client)** under
`/local/repository/results/`; everything heavy is on node1 (server) and you
never touch it.

| What | Where | Node |
|---|---|---|
| Code (repo clone) | `/local/repository` | both |
| Block dataset, account CSVs, paper logs | `/smaran-dataset` (read-only mount) | both |
| Ingested databases (big, regenerable) | `/data/local/artifact-dbs` | server |
| Benchmark logs | `results/logs/` and per-figure `results/fig*/logs/` | client |
| DL figures (PDFs) | `results/fig*/` and `results/paper-figures/` | client |
| KT figures (PDFs) | `output/` | client |
| Cluster config (generated by the profile) | `/local/cluster.env` | both |

On manual/own-server setups you write `/local/cluster.env` yourself (one
line, `SERVER_HOST=...` — see [Install](#install-paths-bc--one-time)); the
scripts then behave identically.

## The six figures

Each script announces `Running experiment Figure <yy>`, prints one
`Running <protocol> with <x> ...` line per data point, then `Plotting`, and
ends with the figure's path under `results/`. Quick variants live in
`QuickTesting-DecentralizedLedgerScripts/`, full-scale in
`DecentralizedLedgerScripts/` — same scripts, reduced parameters.

The tables below use the underlying per-figure scripts; `./run.sh` invokes
these same scripts (`start smoke dl` = `plot_paper_figures.sh`, "Tier 0").

| Script | Shows | Quick | Full scale (extrapolated) |
|---|---|---|---|
| `plot_paper_figures.sh` | all six, from paper logs | ~1 min | — |
| `run_fig6a.sh` (first of 6a/6b/6c) | query latency vs range | ~16 min first run; ~9 min on cached DBs | one-time full ingest per protocol: tens of hours; then ~25 min per protocol |
| `run_fig6b.sh` / `run_fig6c.sh` | throughput / payload size | seconds (re-plot of 6a's sweep) | seconds |
| `run_fig7a.sh` | ingestion throughput | ~11 min | ~19 h |
| `run_fig7b.sh` | archival storage impact | ~9 min (reuses fig6's DB) | ~1.5 h after fig6's ingest |
| `run_fig7c.sh` | sharding impact | ~12 min | ~5 h |

Quick-tier times are as measured end-to-end on the paper's two-node
CloudLab pair (r6615 server + c6420 client); slower storage inflates them
considerably, chiefly through Smaran's shard-database setup/teardown.

Notes that apply to both tiers:

- **Figures 6a/6b/6c come from one benchmark sweep.** The first of the three
  scripts runs it; the other two reuse the cached logs and just re-plot
  (`FORCE_RERUN=1` to redo).
- **Ingested databases are cached** under `/data/local/artifact-dbs` and
  reused across runs (Figure 7b reuses Figure 6's Smaran database).
- **Smaran runs have a fixed setup cost:** creating/opening its ~1000 shard
  databases adds a delay before ingestion or serving begins (about a minute
  on NVMe, several minutes on slower disks), and again at teardown. The
  scripts say so while you wait — this is normal.
- **Interrupting is safe.** Ctrl+C stops the run and its servers (on both
  nodes); rerunning redoes any unfinished work from clean state.
- Absolute numbers at quick scale are far below the paper's (small ingest
  window, short durations); the *trends* are what to check.
- **Quick figures omit the Cauchy baseline.** Cauchy exists only as prebaked
  paper-scale logs (a separate Rust codebase, far too slow to rerun), which
  would sit meaninglessly next to quick-scale numbers; Tier 0 and full-scale
  figures include it.
- All parameters (both tiers) are defined in one place —
  **`DecentralizedLedgerScripts/config.sh`** — and any value can be
  overridden per-run via environment variables (examples in that file).
- **Quick-scale fig7b: the two curves nearly overlap — expected, not a
  bug.** The gains of Smaran's archival storage are realized when the
  ingested window is large — hundreds of thousands to millions of blocks
  (the paper ingests 2.6M). At the quick tier's 10k-block window Smaran
  performs the same with or without it: we verified the two legs run
  distinct code paths and stay within a few percent even at a 60k-block
  window with query ranges to 50k. Smaran being this fast *without*
  archival storage at small scales is itself a good property, but it means
  the paper's visual separation only appears at full scale (or raise
  `N_BLOCKS` / `RANGES_7B` yourself — see `config.sh`).

## Full-scale runs

`./run.sh start full dl` runs all six figures at paper scale. For per-figure
control, the underlying scripts offer the same detached behavior:

```bash
./DecentralizedLedgerScripts/run_fig<yy>.sh --detach
```

Identical scripts at the paper's parameters (query ranges to 2.6M, 32
clients, 2 min per point, 5 user counts to 2M, 6 shard counts to 1000). The
one-time full ingests are the dominant cost (see the table) — **always use
`--detach`** for full-scale runs: the run survives SSH disconnects, and

```bash
./DecentralizedLedgerScripts/status.sh
```

shows each run's state, elapsed vs estimated time, last progress line, and —
when finished — the figure's path. Console output is captured to
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
`run.sh` on node0 remotely — see [the table at the top](#how-to-run--the-three-choices).
To copy everything (figures and logs) back by hand:

```bash
rsync -a <user>@<node0>:/local/repository/results/ ./smaran-results/
```

## Viewing figures

PDFs can't render in a terminal; in order of convenience:

1. **VS Code Remote-SSH** *(recommended)*: connect to node0, open
   `/local/repository` — click any PDF under `results/`; terminal and file
   browser in the same window.
2. **Copy to your laptop**: the `rsync` one-liner above.
3. **Throwaway HTTP preview**: `cd results && python3 -m http.server 8000`,
   then browse `http://<node0>:8000` — remember to stop it (open port).

## Install (Paths B/C — one-time)

The artifact runs on **two** Ubuntu 22.04 machines that can SSH each other —
a *server* (NVMe/SSD, ~100 GB free, 64 GB+ RAM) and a *client* you work
from — mirroring the paper's topology and the CloudLab profile. (Manual
CloudLab experiments are exactly this: treat node0 as the client, node1 as
the server.)

On **both** machines:

1. Clone this repo (same path on both keeps things simple).
2. Get the block dataset from
   [Zenodo (DOI 10.5281/zenodo.21317398)](https://doi.org/10.5281/zenodo.21317398)
   and point the scripts at it: `export SMARAN_DATASET_DIR=<dir with blk_*.dat>`
   (a shared/NFS copy is fine). KT does not use the dataset.
3. Run any install script (idempotent; all three leave everything installed —
   Go 1.25, LaTeX + Python plotting stack, all binaries in `bin/`,
   `/data/local` made writable, dataset located):
   ```bash
   ./DecentralizedLedgerScripts/install_merkle.sh
   ./DecentralizedLedgerScripts/install_verkle.sh
   ./DecentralizedLedgerScripts/install_smaran.sh
   ```
4. Tell the scripts where the server is — create `/local/cluster.env`
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
prebaked form — it comes from a separate Rust codebase and is far too slow to
rerun).

## Troubleshooting

- **A script says `Setup incomplete: ...`** — it names the missing piece and
  the fix. On CloudLab this usually means node setup is still running (the
  SSH login banner shows its status). Full diagnosis:
  `./DecentralizedLedgerScripts/check_setup.sh`.
- **Login banner says `setup FAILED`** — `cat /local/setup.log` on that node;
  re-run `sudo bash /local/repository/cloudlab/setup-node.sh <role> <server-ip>`
  or re-instantiate.
- **A run was interrupted / a node rebooted** — just rerun the script;
  servers left behind are cleaned up automatically and unfinished benchmark
  points are redone. `status.sh` reports a detached run whose process died.
- **Two-node scripts report the server unreachable (`Permission denied`)
  after you changed your SSH keys on the CloudLab portal mid-experiment** —
  CloudLab then rewrites `~/.ssh/authorized_keys` on every node, removing the
  experiment-internal key the setup added. SSH into node1 (your portal key
  still works) and run
  `cat ~/.ssh/id_cloudlab.pub >> ~/.ssh/authorized_keys` to restore it.
- **Dataset mounted somewhere unusual** — `export SMARAN_DATASET_DIR=<dir>`.

## Code layout

All three protocols share one Go module (`internal/` holds the shared
dataset, benchmark, and proof machinery); each protocol has its own
entry-point directories:

| Protocol | Server / ingestion binary | Query (proof) client |
|---|---|---|
| Merkle (MPT) | `cmd/merkle` | `cmd/merkle-proofc` |
| Verkle | `cmd/verkle` | `cmd/verkle-proofc` |
| Smaran | `cmd/samurai` | `cmd/proofc` |

The Key Transparency usecase has its own binaries — `cmd/ktserver` and
`cmd/ktbench` (Smaran and OPTIKS in one binary; CONIKS builds from the
`Coniks/` submodule) — with KT-specific tree/proof variants under
`internal/kt/` and scripts in **`KeyTransparencyScripts/`** +
**`QuickTesting-KeyTransparency/`**.

DL reviewer-facing scripts live in **`DecentralizedLedgerScripts/`**
(install, full-scale experiments, plot-only mode, `check_setup.sh`,
`status.sh`) and **`QuickTesting-DecentralizedLedgerScripts/`** (quick
variants of the same scripts). The CloudLab profile is **`profile.py`** at
the repo root (where CloudLab requires it); node setup and the image recipe
live in **`cloudlab/`**. All experiment output is written under **`results/`**
(gitignored). `docs/DEVELOPMENT.md` documents every binary and subcommand.
