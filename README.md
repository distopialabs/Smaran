# Smaran — Artifact Evaluation

Reproduces the paper's evaluation figures:

- **Key Transparency (§7.1)** — Figures 4a, 4b, 4c, 5
- **Decentralized Ledger (§7.2)** — Figures 6, 7 *(separate branch — see [DL section](#decentralized-ledger-72))*

**Total time:** ~30 min of your attention + ~3 hours of unattended compute for the full sweep, or ~90 min for the quick sweep.

You will run **exactly three commands total** (git clone, cd, and `./setup_cloudlab.sh`). Everything else is menu-driven.

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
2. Open the profile directly: <https://www.cloudlab.us/p/DistopiaLabs/smaran-kt-ae>. Click **Instantiate**.
3. Keep the defaults on the parameter form:
   - **Boot from pre-built image** ✓  *(saves ~30 min of installation)*
   - `serverHW = r6615`, `clientHW = c6420`
   - Cluster: **Clemson**
4. Click **Next → Finish**. Wait ~10 minutes until status = **Ready**.
5. Click **List View**. You will see two nodes: **node0** (server, r6615) and **node1** (client, c6420). Copy the SSH command for **node0** — it looks like:

   ```
   ssh <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us
   ```

---

## Step 2 · SSH into node0 💻 (on your laptop)

Paste the SSH command you just copied into your terminal. You are now on **node0**.

---

## Step 3 · One script, two menus 🖥️ (on node0)

On node0, run these three commands:

```bash
git clone --branch KT-artifact --recurse-submodules https://github.com/distopialabs/Smaran.git ~/Smaran
cd ~/Smaran
./setup_cloudlab.sh
```

That's it. `setup_cloudlab.sh` prepares the environment (inter-node SSH, PATHs, cleans stale state), then walks you through two menus.

### Menu 1 — pick a domain

At the end of setup, you will see:

```
=======================================================
  Setup complete. Which experiment do you want to run?
=======================================================
   [1] Key Transparency  (Section 7.1: Figs 4a, 4b, 4c, 5)
   [2] Decentralized Ledger  (Section 7.2: Figs 6, 7)
   [q] Quit (I will run it later manually)
> Choice [1/2/q]:
```

- **Type `1`** to run the Key-Transparency experiments. It chains into Menu 2 below.
- **Type `2`** for Decentralized Ledger — see [that section](#decentralized-ledger-72). The DL portion is on a separate branch; the runner here will point you at it.
- **Type `q`** to stop here. You can rerun `./run_kt.sh` yourself later.

### Menu 2 — pick a sweep depth (KT only)

Once you pick `1` above, you'll see:

```
=======================================================
  Smaran Key Transparency (Section 7.1)
=======================================================
   [1] FULL   sweep (~3 hours; reproduces every point from paper)
                Fig 4: 11 versions {2,4,8,16,32,64,128,256,512,1024,2047}
                Fig 5: 6 user counts {10k,50k,100k,200k,500k,1M}
   [2] QUICK  sweep (~90 min; reduced points, same qualitative shape)
                Fig 4: 5 versions {2,16,128,256,2047}
                Fig 5: 3 user counts {10k,0.2M,1M}
   [3] SMOKE  test (~3 min; validates pipeline only, one point)
   [q] Quit
> Choice [1/2/3/q]:
```

**Recommended first-timer flow:** `3` (smoke) → confirm you see `Saved: .../fig4a_latency.pdf` at the end → then rerun `./run_kt.sh` and pick `1` (full) or `2` (quick).

| Choice | Compute time | What it does |
|---|---|---|
| **[1] FULL** | ~3 hours | Every point from the paper. Highest fidelity. |
| **[2] QUICK** | ~90 min | Reduced points but same trend and protocol ordering as the paper. |
| **[3] SMOKE** | ~3 min | One sweep point across the three protocols. Just checks that the whole pipeline (build, SSH, server, bench, plotting) works end-to-end. |

While a sweep runs you'll see a live ETA per point, e.g.:

```
Running experiment Figure 4a
Running Coniks with 2 versions [1/33]
Running Coniks with 4 versions [2/33, ~78 min remaining]
...
```

When it's done, PDFs land in `~/Smaran/output/`:
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
scp <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us:'~/Smaran/output/*.pdf' ~/Desktop/smaran-ae-output/
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
curl -sLO https://raw.githubusercontent.com/distopialabs/Smaran/KT-artifact/run_ae.sh
chmod +x run_ae.sh
bash run_ae.sh <your-cloudlab-username> <node0-hostname> full   # or 'quick'
```

Does the whole flow (SSH → setup → sweep → scp back) in one shot. Same result as Steps 2–4.

---

## Install from source 🖥️ *(only if you unchecked the pre-built image at Step 1)*

Before Menu 2, run:

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

The Decentralized-Ledger portion of the artifact lives on a separate branch: <https://github.com/distopialabs/Smaran/tree/timing_debug>. Follow the README there to reproduce Figures 6 and 7.

---

## Troubleshooting

Every experiment script auto-cleans stale processes and `/tmp/coniks.sock` at start, so most previously-common failures are handled automatically. Remaining issues:

| Issue | Fix |
|---|---|
| `Permission denied (publickey)` between nodes | Rerun `./setup_cloudlab.sh` on node0. |
| `no free nodes of type r6615/c6420` at Instantiate | On the profile parameter form, switch to `r6525` / `r6520`. Same qualitative results. |
| Experiment hangs > 5 min | Ctrl-C, delete the newest directory under `~/Smaran/logs/2026-*`, rerun. |
| Anything else | Open an issue at <https://github.com/distopialabs/Smaran/issues>. |
