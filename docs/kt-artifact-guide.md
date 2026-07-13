# Smaran — Artifact Evaluation

Reproduces the paper's evaluation figures:

- **Key Transparency (§7.1)** — Figures 4a, 4b, 4c, 5
- **Decentralized Ledger (§7.2)** — Figures 6, 7 *(same repo — full guide in the [root README](../README.md))*

**Total time:** ~30 min of your attention + ~3 hours of unattended compute for the full sweep, or ~90 min for the quick sweep.

You will run **exactly two commands total** (cd and `./run.sh`). Everything else is menu-driven — or copy-paste the one-line equivalents shown below.

---

## Before you start · Register an SSH key on your CloudLab account

CloudLab installs your public SSH key on the nodes at boot time. If your account has no key registered before you instantiate, you will get `Permission denied (publickey)` when you try to SSH in.

1. On your laptop, generate a key if you do not have one:
   ```bash
   ls ~/.ssh/id_ed25519.pub || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519
   cat ~/.ssh/id_ed25519.pub
   ```
2. Copy the whole `ssh-ed25519 ...` line.
3. Open <https://www.cloudlab.us/manage_profile.php?nav=ssh>, click **Add Key**, paste, save.
4. Now you can proceed to Step 1.

If you already instantiated the profile before adding a key, terminate that experiment and instantiate again — CloudLab only pushes keys at node boot.

---

## Step 1 · Instantiate the CloudLab profile 💻 (on your laptop)

1. Log in at <https://www.cloudlab.us>. (Free for academics; sign up at <https://www.cloudlab.us/signup.php>.)
2. Open the profile directly: <https://www.cloudlab.us/p/distopialabs-PG0/smaran-artifact>. Click **Instantiate**.
3. Keep the defaults on the parameter form (the paper's node pair: r6615 server + c6420 client at Clemson).
4. Click **Next → Finish**. Wait until the **Startup** column shows **Finished** for both nodes (~10 min; node setup builds the binaries automatically).
5. Click **List View**. You will see two nodes: **node0** (client, c6420 — the only node you touch) and **node1** (server, r6615). Copy the SSH command for **node0** — it looks like:

   ```
   ssh <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us
   ```

---

## Step 2 · SSH into node0 💻 (on your laptop)

Paste the SSH command you just copied into your terminal. You are now on **node0**.

---

## Step 3 · One script, two menus 🖥️ (on node0)

On node0, run these two commands (the profile already cloned the repo):

```bash
cd /local/repository
./run.sh
```

(On a machine without the profile, clone it yourself first:
`git clone --branch unified-artifact --recurse-submodules https://github.com/distopialabs/Smaran.git`.)

`./run.sh` prepares the environment (inter-node SSH, PATHs, cleans stale
state), asks two questions — what (`kt`) and at which scale — then starts the
run **detached** and prints the one-line command that skips the menus next
time. The KT sweeps:

| Command | Compute time | What it does |
|---|---|---|
| `./run.sh start full kt` | ~3 hours | Every point from the paper: Fig 4 with 11 version counts {2 … 2047}, Fig 5 with 6 user counts {10k … 1M}. Highest fidelity. |
| `./run.sh start quick kt` | ~90 min | Reduced points (Fig 4: 5 version counts; Fig 5: 3 user counts) but same trend and protocol ordering as the paper. |
| `./run.sh start smoke kt` | ~4 min | One sweep point across the three protocols. Just checks that the whole pipeline (build, SSH, server, bench, plotting) works end-to-end. |

Individual figures work too: `./run.sh start quick fig4a`.

**Recommended first-timer flow:** `./run.sh start smoke kt` → wait ~4 min →
`./run.sh status` shows `done` and `./run.sh results` lists
`output/fig4a_latency.pdf` → then `./run.sh start full kt` (or `quick`).

Because runs are detached, closing your terminal loses nothing:

```bash
./run.sh status    # which figure is running, elapsed vs. estimate
./run.sh follow    # live log (Ctrl+C only stops watching)
./run.sh stop      # abort; rerunning redoes only unfinished work
```

While a sweep runs you'll see a live ETA per point, e.g.:

```
Running experiment Figure 4a
Running Coniks with 2 versions [1/33]
Running Coniks with 4 versions [2/33, ~78 min remaining]
...
```

When it's done, PDFs land in `output/` at the repo root (`/local/repository/output/` on the profile):
`fig4a_latency.pdf`, `fig4b_throughput.pdf`, `fig4c_payload.pdf`, `fig5_put_throughput.pdf`.

---

## Step 4 · Verify + copy PDFs to your laptop

**On node0** 🖥️ — automated shape check against the paper's qualitative claims:

```bash
python3 KeyTransparencyScripts/verify.py
```

Exit code 0 means every claim in §7.1 held on your data.

**On your laptop** 💻 — copy the PDFs down and open them:

```bash
mkdir -p ~/Desktop/smaran-ae-output
scp <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us:'/local/repository/output/*.pdf' ~/Desktop/smaran-ae-output/
open ~/Desktop/smaran-ae-output/*.pdf
```

Compare against the paper's Figures 4a/4b/4c/5. Reference PDFs from our own runs are in [`reference_pdfs/`](reference_pdfs/); shapes should match.

### Expected shapes

| Figure | Trend |
|---|---|
| **4a Latency** | Coniks steepest rise, reaches ~5 s at 2047 versions. Optiks linear rise. Smaran near-flat, then climbs after 512. |
| **4b Throughput** | Optiks highest at low versions, crosses below Smaran around 128–256. Smaran near-flat then declines. Coniks lowest throughout. |
| **4c Payload** | Optiks and Coniks track together, growing steeply. Smaran grows much more slowly. |
| **5 Put throughput** | Broken y-axis. Upper: Optiks and Smaran in tens of thousands ops/s with mild decline. Lower: Coniks flat at ~640 ops/s. |

---

## Alternative: one command from your laptop 💻

If you'd rather skip logging into node0 yourself, run this on your laptop after Step 1:

```bash
curl -sLO https://raw.githubusercontent.com/distopialabs/Smaran/unified-artifact/run_ae.sh
bash run_ae.sh <your-cloudlab-username> <node0-hostname> start full kt   # or quick/smoke
bash run_ae.sh <your-cloudlab-username> <node0-hostname> status          # any time
bash run_ae.sh <your-cloudlab-username> <node0-hostname> fetch           # PDFs -> ~/Desktop/smaran-ae-output
```

Same `run.sh` on node0, driven over SSH — start, check progress, and copy the figures back without ever logging in yourself.

---

## Install from source 🖥️ *(only on machines without the profile's automatic setup)*

Before running the sweeps, run:

```bash
./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"
```

---

## Data notes

- **Single run per point.** The paper averages 3 runs; the AE runs each point once to fit in ~3 hours. Individual points may look noisier than the paper (Fig 4a Smaran at 700 and 1500 versions, Fig 5 Optiks between 50k–1M users). Overall shape and protocol ordering are preserved.
- **Coniks fork.** The submodule is `coniks-history-extension`, a fork of official CONIKS with a Merkle Patricia Trie extension. Its per-request cost is user-count-independent, which is why our Fig 5 Coniks curve is flat while the paper's declines slightly. Fig 4 shape matches the paper.

---

## Decentralized Ledger (§7.2)

The Decentralized-Ledger portion of the artifact lives in this same repo. Its full reviewer guide is the [root README](../README.md); the quick entry point is `./run.sh start quick dl`.

---

## Troubleshooting

Every experiment script auto-cleans stale processes and `/tmp/coniks.sock` at start, so most previously-common failures are handled automatically. Remaining issues:

| Issue | Fix |
|---|---|
| `Permission denied (publickey)` between nodes | Rerun `./run.sh setup` on node0. |
| `no free nodes of type r6615/c6420` at Instantiate | On the profile parameter form, switch to `r6525` / `r6520`. Same qualitative results. |
| Experiment hangs > 5 min | Ctrl-C, delete the newest directory under `logs/2026-*` in the repo, rerun. |
| Anything else | Open an issue at <https://github.com/distopialabs/Smaran/issues>. |
