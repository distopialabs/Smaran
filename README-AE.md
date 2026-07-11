# Smaran — Artifact Evaluation

Reproduces the paper's evaluation figures:

- **Key Transparency (§7.1)** — Figures 4a, 4b, 4c, 5
- **Decentralized Ledger (§7.2)** — Figures 6, 7 *(separate branch — see [DL section](#decentralized-ledger-72))*

Everything below refers to your own CloudLab experiment. Each reviewer instantiates the profile in their own account, gets their own two nodes, and runs the artifact on those nodes. Nothing is shared with the authors.

**Total time:** ~30 min of your attention + ~3 hours of unattended compute (full sweep). A 90-minute quick path is also documented.

---

## Where you run each command

Two shells are involved. Every code block below is labeled with which one:

- 💻 **[laptop]** — your own machine's terminal
- 🖥️ **[node0]** — the SSH session you open into the CloudLab server node

---

## Step 1 · Instantiate the CloudLab profile 💻

Time: ~10 min human + ~10 min waiting for CloudLab.

1. Log in at <https://www.cloudlab.us>. (Free for academics; sign up at <https://www.cloudlab.us/signup.php>.)
2. **Experiments → Instantiate a Profile**.
3. Search for the profile named `smaran-kt-ae` in project `DistopiaLabs`. Click **Next**.
4. Keep defaults on the parameter form:
   - **Boot from pre-built image** ✓ *(saves ~30 min of installation)*
   - `serverHW = r6615`, `clientHW = c6420`
   - Cluster: **Clemson**
5. Click **Next → Finish**. Wait ~10 min until the experiment status shows **Ready**.
6. Click **List View**. You'll see two nodes — **node0** (server, r6615) and **node1** (client, c6420). Copy the SSH command for **node0** — it looks like `ssh <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us`.

You now have two nodes provisioned in your own account. Everything after this runs on those.

---

## Step 2 · SSH into node0 and set up 🖥️

Time: ~2 min.

Open a terminal on your laptop and paste the SSH command you copied in Step 1. You are now on **node0**.

On node0, run:

```bash
git clone --branch kt --recurse-submodules https://github.com/distopialabs/Smaran.git ~/Smaran
cd ~/Smaran
./setup_cloudlab.sh
```

`setup_cloudlab.sh` verifies the environment, generates an SSH key between node0 and node1 so node0 can drive node1, and prompts you at the end:

```
Which experiment do you want to run?
   [1] Key Transparency  (Section 7.1: Figs 4a, 4b, 4c, 5)
   [2] Decentralized Ledger  (Section 7.2: Figs 6, 7)
   [q] Quit
```

Pick `1` for Key Transparency and it chains into the next step automatically. (Or pick `q` and run `./run_kt.sh` yourself when you're ready.)

---

## Step 3 · Run the Key-Transparency experiments 🖥️

`run_kt.sh` (invoked either by `setup_cloudlab.sh` or directly) asks:

```
[1] FULL   sweep (~3 hours; reproduces every point from paper)
[2] QUICK  sweep (~90 min; reduced points, same qualitative shape)
[3] SMOKE  test (~3 min; validates the pipeline only)
[q] Quit
```

Recommended first-timer flow: **[3] SMOKE** → confirm it prints `Saved: .../fig4a_latency.pdf` at the end → then rerun `./run_kt.sh` and pick **[1] FULL** or **[2] QUICK**.

### What each option does

| Option | Compute time | Sweep points |
|---|---|---|
| **FULL** | ~3 hours | Fig 4: versions `{2,4,8,16,32,64,128,256,512,1024,2047}`. Fig 5: users `{10k,50k,100k,200k,500k,1M}`. |
| **QUICK** | ~90 min | Fig 4: versions `{2,16,128,256,2047}`. Fig 5: users `{10k,0.2M,1M}`. Same protocol ordering and qualitative shape as full. |
| **SMOKE** | ~3 min | Single point per protocol at 2 versions. Pipeline sanity only. |

PDFs land in `~/Smaran/output/`:
`fig4a_latency.pdf`, `fig4b_throughput.pdf`, `fig4c_payload.pdf`, `fig5_put_throughput.pdf`.

---

## Step 4 · Verify + copy PDFs to your laptop

**On node0** 🖥️ — automated shape check against the paper's claims:

```bash
python3 KeyTransparencyScripts/verify.py
```

Exit code 0 means every qualitative claim in §7.1 held on your data.

**On your laptop** 💻 — copy the PDFs down and open them:

```bash
mkdir -p ~/Desktop/smaran-ae-output
scp <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us:'~/Smaran/output/*.pdf' ~/Desktop/smaran-ae-output/
open ~/Desktop/smaran-ae-output/*.pdf
```

Compare each PDF against the paper's Figures 4a/4b/4c/5. Reference PDFs from our own runs live in [`reference_pdfs/`](reference_pdfs/); shapes should match.

### Expected shapes

| Figure | Trend |
|---|---|
| **4a Latency** | Coniks steepest rise, reaches ~5 s at 2047 versions. Optiks linear rise. Smaran near-flat, then climbs after 512. |
| **4b Throughput** | Optiks highest at low versions, crosses below Smaran around 128–256. Smaran near-flat then declines. Coniks lowest throughout. |
| **4c Payload** | Optiks and Coniks track together, growing steeply. Smaran grows much more slowly. |
| **5 Put throughput** | Broken y-axis. Upper: Optiks and Smaran in tens of thousands ops/s with mild decline. Lower: Coniks flat at ~640 ops/s. |

---

## Everything in one command (alternative to Steps 2–4) 💻

If you'd rather skip logging into node0 yourself, run this on your laptop after Step 1:

```bash
curl -sLO https://raw.githubusercontent.com/distopialabs/Smaran/kt/run_ae.sh
chmod +x run_ae.sh
bash run_ae.sh <your-cloudlab-username> <node0-hostname> full   # or 'quick'
```

This SSHes into your node0, does the setup, runs all four figures, and copies the PDFs back to `~/Desktop/smaran-ae-output/` on your laptop. About 3 hours end-to-end for the full sweep.

---

## Install from source 🖥️ *(only if you unchecked the pre-built image at Step 1)*

```bash
./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"
```

Run these on node0 before Step 3.

---

## Data notes

- **Single run per point.** The paper averages 3 runs; the AE runs each point once to fit in ~3 hours. Individual points may look noisier than the paper (Fig 4a Smaran at 700 and 1500 versions, Fig 5 Optiks between 50k–1M users). Overall shape and protocol ordering are preserved.
- **Coniks fork.** The submodule is `coniks-history-extension`, a fork of official CONIKS with a Merkle Patricia Trie extension. Its per-request cost is user-count-independent, which is why our Fig 5 Coniks curve is flat while the paper's declines slightly. Fig 4 shape matches the paper.

---

## Decentralized Ledger (§7.2)

The Decentralized-Ledger portion of the artifact is on a separate branch: <https://github.com/distopialabs/Smaran/tree/timing_debug>. Follow the README on that branch to reproduce Figures 6 and 7.

The two portions are on separate branches because their Smaran cores have diverged in ways that would require full architectural reconciliation to unify. This will be done post-deadline; the AE-submission split does not affect the correctness of either portion.

---

## Troubleshooting

Every experiment script auto-cleans stale processes and `/tmp/coniks.sock` at start, so most previously-common failures are handled automatically. Remaining issues:

| Issue | Fix |
|---|---|
| `Permission denied (publickey)` between nodes | Rerun `./setup_cloudlab.sh` on node0. |
| `no free nodes of type r6615/c6420` at Instantiate | On the profile parameter form, switch to `r6525` / `r6520`. Same qualitative results. |
| Experiment hangs > 5 min | Ctrl-C, delete the newest directory under `~/Smaran/logs/2026-*`, rerun. |
| Anything else | Open an issue at <https://github.com/distopialabs/Smaran/issues>. |
