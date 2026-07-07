# Smaran — Artifact Evaluation

This document is the **entry point for SOSP artifact-evaluation reviewers**.
It reproduces **Figures 4a, 4b, 4c, and 5** of the Smaran paper — the four
Key-Transparency experiments in §7.1.

## Scope

This artifact reproduces the **Key-Transparency (KT) experiments** in the
paper's §7.1 only. The paper's §7.2 Decentralized-Ledger experiments
(Figures 6 and 7) are **out of scope** for this artifact.

## Time budget — read this before starting

Following the hierarchy recommended by
[Padhye's AE tips](https://blog.padhye.org/Artifact-Evaluation-Tips-for-Authors/#tip-2-estimate-human--compute-time-and-declare-it-upfront):

```
Smaran — Key Transparency Artifact
# Overview
* Provision cluster        (10 human-minutes +  10 compute-minutes)
* Install everything       ( 5 human-minutes +   5 compute-minutes)
* Quick-turnaround (all 4) (10 human-minutes +  90 compute-minutes)   RECOMMENDED
* Full sweeps      (all 4) (15 human-minutes + 200 compute-minutes)   OPTIONAL
* Validate output          (10 human-minutes +   0 compute-minutes)
```

**Fastest path to a signed-off review: quick turnaround.** Total:
~35 human-minutes + ~1h 45m compute-time. Reproduces the paper's trend
faithfully with a reduced sweep.

**Full sweeps** match the exact sweep points in the paper. Only run these
if you have ~3.5 hours of compute to spare and want side-by-side match.

## Two ways to run

### Path A — Pre-built CloudLab image (recommended)

Skip installation entirely. Instantiate the profile in
[`cloudlab-profile/`](cloudlab-profile/) with `useImage=checked` and the URN
in [`cloudlab-profile/image_urn.txt`](cloudlab-profile/image_urn.txt).

*Status of the image*: check `cloudlab-profile/image_urn.txt`. If it still
says `PLACEHOLDER`, the image is not yet published — use Path B.

### Path B — Install from source

Provision two Ubuntu 22.04 machines that can SSH each other, then run the
installers. Detailed in the sections below.

---

## 1. Provision cluster  *(10 human-min + 10 compute-min)*

Both nodes must be able to SSH to each other. The paper uses CloudLab
Clemson with two node types:

| Role   | Recommended hardware | Purpose               |
|--------|----------------------|-----------------------|
| server | `r6615` (AMD EPYC 9354P, 32 cores, 192 GiB RAM) | runs `ktserver` |
| client | `c6420` (Intel Xeon Gold 6142m, 16 cores)        | runs `ktbench`  |

**Steps on CloudLab:**

1. Log in to <https://www.cloudlab.us>.
2. **Experiments → Create Experiment Profile** → upload
   [`cloudlab-profile/profile.py`](cloudlab-profile/profile.py). Save as `smaran-ae`.
3. Click **Instantiate**. On the parameter form:
   - `Boot from the pre-built Smaran image` — **unchecked** (for Path B).
   - Keep `serverHW=r6615`, `clientHW=c6420`.
   - Cluster: **Clemson**.
4. Click **Next → Finish**. Wait ~5–10 min for status = **Ready**.
5. On the **List View** tab, note both SSH commands. `node0` is the server
   (r6615); `node1` is the client (c6420).

**Side effects**: creates a fresh CloudLab experiment tied to your account.
No files on your local machine.

---

## 2. Install everything  *(5 human-min + 5 compute-min)*

**Skip this section entirely if using Path A (the pre-built image).**

SSH into **both nodes** and run the same three installers on each. The
installers are idempotent — re-running is safe.

**On `node0` (server):**
```bash
git clone --branch artifact-eval --recurse-submodules \
    https://github.com/distopialabs/Smaran.git ~/Smaran
cd ~/Smaran

./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"

# Enable inter-node SSH so node0 can drive the sweep:
[ -f ~/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519
cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/id_ed25519.pub | ssh node1 "cat >> ~/.ssh/authorized_keys"
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go   # for non-interactive SSH

# Configure the run:
cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env
# Defaults point at node0/node1 with your SSH key — no edits usually needed.
```

**On `node1` (client):** same three installers. Or just wait — the
experiment script auto-distributes binaries built on node0.

**Side effects (per node)**:
- Installs Go 1.24 to `/usr/local/go/`
- Installs apt packages: `build-essential`, `git`, `make`, `python3-pip`,
  `protobuf-compiler`, `rsync`
- Installs Python packages to `~/.local/lib/python3.10/site-packages/`
  (from `experiments/requirements.txt`)
- Produces binaries in `~/Smaran/bin/`:
  - `ktserver`, `ktbench` (Smaran + OPTIKS)
  - `samurai`, `proofc`, `makedataset` (Smaran tooling)
  - `coniksserver`, `coniksclient`, `coniksbench`, `coniksbot` (CONIKS)

---

## 3. Run experiments

Each script prints:
- `Running experiment Figure <yy>` on its first line.
- `Running <Coniks|Optiks|Smaran> with <x> versions` (or `<x> users` for
  Fig 5) before each datapoint.
- `Plotting` at the end, followed by `Saved: <path>`.

### 3a. Quick turnaround  *(10 human-min + 90 compute-min)* — RECOMMENDED

Reduced sweep points, keeps the trends. From `node0`:

```bash
cd ~/Smaran
./QuickTesting-KeyTransparency/run_fig4a_quick.sh   # ~36 min, versions ∈ {2,16,128,256,2047}
./QuickTesting-KeyTransparency/run_fig4b_quick.sh   # ~2 s  (cached from 4a_quick)
./QuickTesting-KeyTransparency/run_fig4c_quick.sh   # ~2 s  (cached from 4a_quick)
./QuickTesting-KeyTransparency/run_fig5_quick.sh    # ~40–60 min, users ∈ {10k, 200k, 1M}
```

Cache mechanism: Figures 4a/4b/4c share a single experimental sweep. The
first one you run does the full sweep; the next two only re-plot.
Set `KT_FORCE_RERUN=1` to force a fresh sweep.

### 3b. Full sweeps  *(15 human-min + 200 compute-min)* — OPTIONAL

Exact paper sweep points. Same caching behaviour.

```bash
./KeyTransparencyScripts/run_fig4a.sh   # ~80 min, versions ∈ {2,4,8,16,32,64,128,256,512,1024,2047}
./KeyTransparencyScripts/run_fig4b.sh   # ~5 s (cached)
./KeyTransparencyScripts/run_fig4c.sh   # ~5 s (cached)
./KeyTransparencyScripts/run_fig5.sh    # ~90–120 min, users ∈ {10k, 30k, 100k, 200k, 500k, 1M}
```

### Side effects (per experiment script)
- Writes a fresh sweep directory at `~/Smaran/logs/<ISO-timestamp>/`.
- Under that directory, one subdir per `(protocol, x)` datapoint containing
  `config_used.json` and `node2.log` (raw ktbench/coniksbench output).
- Populates a plot directory `~/Smaran/logs/<sweep>/output/` with
  intermediate PDFs and `kt_query_summary.csv` / `kt_put_summary.csv`.
- Copies the target subfigure PDF to `~/Smaran/output/<figNN>.pdf`.
- Caches the "sweep-done" marker under `~/Smaran/logs/ae_cache/<profile>/latest`
  as a symlink to the completed sweep dir.

Nothing outside `~/Smaran/` is written or modified.

---

## 4. Validate  *(10 human-min)*

Open the four PDFs under `~/Smaran/output/`:

```
output/
  fig4a_latency.pdf         — reproduces Figure 4a (latency vs versions)
  fig4b_throughput.pdf      — reproduces Figure 4b (throughput vs versions)
  fig4c_payload.pdf         — reproduces Figure 4c (payload vs versions)
  fig5_put_throughput.pdf   — reproduces Figure 5  (put throughput vs users)
```

### Claim ↔ evidence mapping

| Paper claim (§7.1) | Reproducing figure/file | Expected qualitative shape |
|---|---|---|
| "Smaran reduces end-to-end latency by 99.1% relative to CONIKS and by 82.5% relative to OPTIKS" at 2047 versions | Fig 4a → `output/fig4a_latency.pdf` and `kt_query_summary.csv` | Optiks rises linearly; Smaran near-flat; Coniks flat |
| "Smaran delivers 113× higher throughput than CONIKS and 5.7× higher than OPTIKS" at 2047 versions | Fig 4b → `output/fig4b_throughput.pdf` | Optiks drops steeply; Smaran gentle decline; Coniks flat |
| "Smaran's payload at 2047 versions is 97.0% lower than CONIKS and 95.2% lower than OPTIKS" | Fig 4c → `output/fig4c_payload.pdf` | Optiks payload grows ~1000×; Smaran ~50× across the sweep |
| "Smaran yields about 130× higher throughput than CONIKS at 10K users…and about 115.6× at 1M users" | Fig 5 → `output/fig5_put_throughput.pdf` | Smaran between CONIKS (low) and OPTIKS (high) across all user counts |

**Absolute numbers depend on hardware.** The qualitative shape and the
protocol-ordering are what to check.

### Human-readable summary tables

Every sweep also drops a CSV alongside the PDFs:
- `logs/<sweep>/output/kt_query_summary.csv` (Fig 4)
- `logs/<sweep>/output/kt_put_summary.csv`   (Fig 5)

Columns include `protocol`, `num_versions` (or `num_users`),
`throughput_qps`, `avg_latency_ms`, `avg_payload_kib`, etc. Open in a
spreadsheet or plain-text viewer.

---

## Terminology and cross-references

- The paper calls the new system **Smaran**; historic module names in the
  Go code use `samurai` (an earlier working name). The plotting layer
  maps `samurai → Smaran` in all output labels. If you read the source,
  wherever you see `samurai`, it is Smaran.
- `bench_protocol` values in configs and logs:
  `samurai == Smaran`, `optiks == OPTIKS`, `coniks == CONIKS`.

## Reusing / extending the artifact

- Change sweep points by editing
  [`KeyTransparencyScripts/lib/render_config.py`](KeyTransparencyScripts/lib/render_config.py).
  The `FIGURE_PROFILES` dict controls every experiment's sweep values.
- Change per-datapoint duration:
  `KT_RUN_DURATION=180 ./KeyTransparencyScripts/run_fig4a.sh` (default 90 s).
- Change which hosts the sweep uses: set `KT_SERVER_HOST`, `KT_CLIENT_HOST`,
  `KT_SSH_USER`, `KT_SSH_KEY` in `KeyTransparencyScripts/nodes.env`. See
  `nodes.env.template` for the full list.

## Recovery from partial runs

- Each installer is idempotent — re-run safely.
- If an experiment crashes mid-sweep: delete
  `~/Smaran/logs/<the failing sweep>/` and re-run the script. Cache is
  keyed on a *completed* sweep only; a partial sweep is discarded.
- To force a fresh sweep even if a cached completed run exists:
  `KT_FORCE_RERUN=1 ./run_figXX.sh`.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `Permission denied (publickey)` when running an experiment | node0 can't SSH into itself or node1 | Re-run the inter-node SSH setup in section 2. |
| `go: command not found` during the sweep | Non-interactive SSH doesn't see `/usr/local/go/bin` | `sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go` on the server node. |
| `ValueError: Could not find avg_latency` from the plotter | ktbench ran in put-mode when Fig 4 needed get-mode. Should not happen on `artifact-eval` branch. | Ensure you are on `artifact-eval` branch (`git rev-parse --abbrev-ref HEAD`). |
| `no free nodes of type r6615/c6420` on Instantiate | CloudLab is contended | Either wait, reserve via <https://www.cloudlab.us/reserve.php>, or substitute (e.g. r6525 client). |
| Port 3191 already in use | Prior ktserver did not shut down | `pkill ktserver` on the server node. |

## Getting help

- Repo: <https://github.com/distopialabs/Smaran>
- File an issue tagged `artifact-eval` if a reviewer step fails and this
  README does not resolve it.
