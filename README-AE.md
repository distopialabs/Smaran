# Smaran — Artifact Evaluation

Reproduces the paper's evaluation figures. Two application domains:

- **Key Transparency (§7.1)** — Figs 4a, 4b, 4c, 5
- **Decentralized Ledger (§7.2)** — Figs 6, 7

---

## Navigation

Click a leaf to jump to the exact instructions for that path.

```
Start
  │
  ├── 1. Provision CloudLab              →  §1
  │
  └── 2. Pick an experiment domain
        │
        ├── Key Transparency (§7.1)      →  §2-KT
        │     │
        │     ├── Full sweep (~3 h)      →  §KT-Full
        │     │     ├── Automated        →  §KT-Full-Auto
        │     │     └── Manual           →  §KT-Full-Manual
        │     │
        │     ├── Quick sweep (~90 min)  →  §KT-Quick
        │     │     ├── Automated        →  §KT-Quick-Auto
        │     │     └── Manual           →  §KT-Quick-Manual
        │     │
        │     └── Smoke test (~3 min)    →  §KT-Smoke
        │
        └── Decentralized Ledger (§7.2)  →  §2-DL
```

Jump list:
[Provision CloudLab](#1-provision-cloudlab)  |
[KT overview](#2-kt-key-transparency-71)  |
[KT Full](#kt-full-sweep-3-hours)  |
[KT Quick](#kt-quick-sweep-90-min)  |
[KT Smoke](#kt-smoke-test-3-min)  |
[DL](#2-dl-decentralized-ledger-72)

---

## 1. Provision CloudLab

<a id="1-provision-cloudlab"></a>

Time budget: **~10 min human, ~10 min compute.**

1. Log in at <https://www.cloudlab.us>. (Free for academics; sign up at <https://www.cloudlab.us/signup.php>.)
2. **Experiments → Instantiate a Profile** → search `smaran-kt-ae` in project `DistopiaLabs` → **Instantiate**.
3. Keep defaults: **Boot from pre-built image** ✓, `serverHW = r6615`, `clientHW = c6420`, Cluster = **Clemson**.
4. Wait ~10 min for status = **Ready**. Copy the SSH command for **node0**.
5. SSH into node0 and run the setup script:

   ```bash
   git clone --branch kt --recurse-submodules https://github.com/distopialabs/Smaran.git ~/Smaran
   cd ~/Smaran
   ./setup_cloudlab.sh
   ```

   `setup_cloudlab.sh` verifies the environment, sets up inter-node SSH, and then asks whether you want to run KT or DL. Pick the one you care about, and it will chain into the right runner.

---

## 2-KT. Key Transparency (§7.1)

<a id="2-kt-key-transparency-71"></a>

Pick a sweep depth. All three land the resulting PDFs in `~/Smaran/output/`.

| Path | Total time | What it does |
|---|---|---|
| [Full](#kt-full-sweep-3-hours) | ~3 hours | Every point from the paper. Fig 4: 11 versions. Fig 5: 6 user counts. |
| [Quick](#kt-quick-sweep-90-min) | ~90 min | Reduced point set that still shows the same qualitative shape. |
| [Smoke](#kt-smoke-test-3-min) | ~3 min | One-point test that validates the whole pipeline before you commit to a real sweep. Recommended first. |

The fastest way is: `./setup_cloudlab.sh` → pick KT → pick your sweep depth. The subsections below give the manual-step equivalent if you want to run individual scripts by hand.

---

### KT Full sweep (~3 hours)

<a id="kt-full-sweep-3-hours"></a>

Sweeps versions `{2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2047}` (Figs 4a/4b/4c) and users `{10k, 50k, 100k, 200k, 500k, 1M}` (Fig 5).

#### KT Full — Automated

<a id="kt-full-auto"></a>

```bash
cd ~/Smaran
./run_kt.sh
# choose option [1] FULL
```

Or from your laptop, one command end-to-end:

```bash
curl -sLO https://raw.githubusercontent.com/distopialabs/Smaran/kt/run_ae.sh
bash run_ae.sh <cloudlab-user> <node0-host> full
```

The laptop-side version SSHes in, runs the setup, runs all four figures, and copies the PDFs back to `~/Desktop/smaran-ae-output/`.

#### KT Full — Manual (individual scripts)

<a id="kt-full-manual"></a>

```bash
cd ~/Smaran
./KeyTransparencyScripts/run_fig4a.sh   # ~80 min
./KeyTransparencyScripts/run_fig4b.sh   # ~5 s (uses Fig 4a's cache)
./KeyTransparencyScripts/run_fig4c.sh   # ~5 s (uses Fig 4a's cache)
./KeyTransparencyScripts/run_fig5.sh    # ~90-120 min
```

---

### KT Quick sweep (~90 min)

<a id="kt-quick-sweep-90-min"></a>

Sweeps versions `{2, 16, 128, 256, 2047}` (Figs 4a/4b/4c) and users `{10k, 0.2M, 1M}` (Fig 5). Same protocol ordering and qualitative shape as the full sweep, in a fraction of the time.

#### KT Quick — Automated

<a id="kt-quick-auto"></a>

```bash
cd ~/Smaran
./run_kt.sh
# choose option [2] QUICK
```

Or from your laptop:

```bash
bash run_ae.sh <cloudlab-user> <node0-host> quick
```

#### KT Quick — Manual (individual scripts)

<a id="kt-quick-manual"></a>

```bash
cd ~/Smaran
./QuickTesting-KeyTransparency/run_fig4a_quick.sh   # ~35 min
./QuickTesting-KeyTransparency/run_fig4b_quick.sh   # ~5 s (cached)
./QuickTesting-KeyTransparency/run_fig4c_quick.sh   # ~5 s (cached)
./QuickTesting-KeyTransparency/run_fig5_quick.sh    # ~45 min
```

---

### KT Smoke test (~3 min)

<a id="kt-smoke-test-3-min"></a>

Runs one sweep point for each of the three protocols. Fastest way to catch build or environment issues before starting a real sweep.

```bash
cd ~/Smaran
./KeyTransparencyScripts/smoke_test.sh
```

Expected output: `Running experiment Figure 4a`, three `Running <protocol> with 2 versions` lines, `Plotting`, `Saved: .../fig4a_latency.pdf`. If any of that is missing, don't start a real sweep — fix the underlying issue first.

---

## 2-DL. Decentralized Ledger (§7.2)

<a id="2-dl-decentralized-ledger-72"></a>

*[Placeholder — the Decentralized-Ledger portion is authored separately. Instructions will land in this section when that portion is merged.]*

---

## Install from source (skip the CloudLab image)

If at Step 1 you unchecked the pre-built image, run these before starting any experiment:

```bash
./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"
```

## Verify + compare

After any KT sweep finishes:

```bash
# Automated shape-check against the paper's qualitative claims
python3 KeyTransparencyScripts/verify.py

# Copy PDFs to your laptop (from your laptop's terminal)
scp <user>@<node0-host>:'~/Smaran/output/*.pdf' ~/Desktop/smaran-ae-output/
open ~/Desktop/smaran-ae-output/*.pdf
```

Reference PDFs from our own runs live in [`reference_pdfs/`](reference_pdfs/) for direct comparison.

## Data notes

- **Single run per point.** Paper averages 3 runs. Individual points may show noise; overall trend is unaffected.
- **Coniks fork.** Submodule is `coniks-history-extension` (fork of official CONIKS). Its per-request cost is user-count-independent, which is why our Fig 5 Coniks line is flat.

## Troubleshooting

Every experiment script auto-cleans stale processes and `/tmp/coniks.sock` at start. Remaining issues:

| Issue | Fix |
|---|---|
| `Permission denied (publickey)` between nodes | Re-run `./setup_cloudlab.sh`. |
| `no free nodes of type r6615/c6420` | Switch to `r6525` / `r6520` on the profile form. |
| Experiment hangs > 5 min | Ctrl-C, delete latest `~/Smaran/logs/2026-*`, re-run. |
| Something else | Open an issue at <https://github.com/distopialabs/Smaran/issues>. |
