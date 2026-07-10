# Smaran — Artifact Evaluation

Reproduces Figures **4a, 4b, 4c, and 5** from §7.1 of the Smaran paper.

**Total time:** ~30 min human attention + ~3 hours unattended compute.

**Public artifact:** <https://github.com/distopialabs/Smaran/tree/artifact-eval>

**Reference outputs:** the four PDFs our team produced live under [`reference_pdfs/`](reference_pdfs/) — glance at those first so you know the shape to expect.

---

## Fastest path: one command from your laptop

After you finish Step 1 below (provisioning a CloudLab experiment), run this on **your laptop**:

```bash
curl -sLO https://raw.githubusercontent.com/distopialabs/Smaran/artifact-eval/run_ae.sh
bash run_ae.sh <cloudlab-username> <node0-hostname> full   # or 'quick' for ~90 min
```

That single command SSHes into node0, clones the repo, sets up inter-node SSH, runs all four experiments, and copies the PDFs back to `~/Desktop/smaran-ae-output/` on your laptop.

If you'd rather step through it manually, follow Steps 2–4 below.

---

## 1. Provision CloudLab nodes (~10 min)

1. Log in at <https://www.cloudlab.us>. (Free for academics; sign up at <https://www.cloudlab.us/signup.php>.)
2. **Experiments → Instantiate a Profile** → search `smaran-kt-ae` (project `DistopiaLabs`) → **Instantiate**.
3. Keep defaults:
   - **Boot from pre-built image** ✓ (skips install)
   - `serverHW = r6615`, `clientHW = c6420`
   - Cluster: **Clemson**
4. Wait ~10 min for status = **Ready**. Copy the SSH command for **node0**.

## 2. Set up node0 (~2 min)

SSH into node0 and paste:

```bash
git clone --branch artifact-eval --recurse-submodules \
    https://github.com/distopialabs/Smaran.git ~/Smaran
cd ~/Smaran

[ -f ~/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519
cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/id_ed25519.pub | ssh -o StrictHostKeyChecking=accept-new node1 "cat >> ~/.ssh/authorized_keys"
ssh-keyscan -H node0 node1 >> ~/.ssh/known_hosts 2>/dev/null
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env
```

### 2a. Optional 3-minute smoke test

Before spending 3 h on the full sweep, run a smoke test to confirm your environment works end-to-end:

```bash
./KeyTransparencyScripts/smoke_test.sh
```

Expected: prints `Running experiment Figure 4a`, three `Running <protocol> with 2 versions` lines, `Plotting`, then `Saved: ~/Smaran/output/fig4a_latency.pdf`. Finishes in ~3 minutes. If this fails, don't start the full sweep.

## 3. Run the experiments (~3 hours)

```bash
./KeyTransparencyScripts/run_fig4a.sh   # ~80 min — versions {2,4,8,16,32,64,128,256,512,1024,2047}
./KeyTransparencyScripts/run_fig4b.sh   # ~5 s (cached from 4a)
./KeyTransparencyScripts/run_fig4c.sh   # ~5 s (cached from 4a)
./KeyTransparencyScripts/run_fig5.sh    # ~90–120 min — users {10k,50k,100k,200k,500k,1M}
```

Each per-point line prints a running ETA, e.g. `Running Optiks with 128 versions [4/15, ~26 min remaining]`.

PDFs land in `~/Smaran/output/`:
`fig4a_latency.pdf`, `fig4b_throughput.pdf`, `fig4c_payload.pdf`, `fig5_put_throughput.pdf`.

## 4. Verify against paper

Automated shape check (runs in ~1 s):

```bash
python3 ~/Smaran/KeyTransparencyScripts/verify.py
```

Reports pass/fail on each qualitative claim from §7.1 (protocol ordering, monotonicity, key ratios). Exit code 0 = all shape checks pass.

Then eyeball the PDFs:

```bash
# on your laptop
scp <user>@<node0-host>:'~/Smaran/output/*.pdf' ~/Desktop/smaran-ae-output/
open ~/Desktop/smaran-ae-output/*.pdf
```

Compare against `reference_pdfs/` in the repo — trend + protocol ordering are the evaluation criteria (absolute numbers depend on hardware).

| Figure | Trend |
|---|---|
| **4a Latency** | Coniks steepest rise, reaches ~5 s at 2047. Optiks linear rise. Smaran near-flat then climbs. |
| **4b Throughput** | Optiks highest at low versions, crosses below Smaran near 128–256. Smaran near-flat then declines. Coniks lowest throughout. |
| **4c Payload** | Optiks and Coniks track together, growing steeply. Smaran grows slowly. |
| **5 Put throughput** | Broken y-axis. Upper: Optiks and Smaran in the tens of thousands ops/s with mild decline. Lower: Coniks flat at ~640 ops/s. |

---

## Optional: quick sweep (~90 min instead of ~3 h)

Fewer sweep points, same shape. Substitute the quick versions for Step 3:

```bash
./QuickTesting-KeyTransparency/run_fig4a_quick.sh   # ~35 min — versions {2,16,128,256,2047}
./QuickTesting-KeyTransparency/run_fig4b_quick.sh   # ~5 s (cached)
./QuickTesting-KeyTransparency/run_fig4c_quick.sh   # ~5 s (cached)
./QuickTesting-KeyTransparency/run_fig5_quick.sh    # ~45 min — users {10k,200k,1M}
```

Or from your laptop: `bash run_ae.sh <user> <host> quick`.

## Optional: install from source

If at Step 1 you unchecked the pre-built image, run these before Step 3:

```bash
./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"
```

---

## Data notes

- **Single run per point.** The paper averages 3 runs; the AE runs each point once to fit in ~3 h. Individual points may show noise (Fig 4a Smaran at 700/1500 versions; Fig 5 Optiks between 50k–1M). Overall trends are unaffected.
- **Coniks fork.** The submodule is `coniks-history-extension` (fork of official CONIKS). Its per-request cost is user-count-independent, which is why our Fig 5 Coniks line is flat while the paper's declines. Fig 4 shape matches the paper.

## Troubleshooting

The run scripts auto-clean stale processes and `/tmp/coniks.sock` at start, so most previously-common failures are handled automatically. Remaining issues:

| Issue | Fix |
|---|---|
| `Permission denied (publickey)` between nodes | Re-run the SSH block in Step 2. |
| `no free nodes of type r6615/c6420` at Instantiate | Switch to `r6525` / `r6520` on the profile form. |
| Experiment hangs > 5 min | Ctrl-C, delete latest `~/Smaran/logs/2026-*`, re-run. |
| Something else | Open an issue at <https://github.com/distopialabs/Smaran/issues>. |
