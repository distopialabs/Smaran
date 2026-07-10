# Smaran — Artifact Evaluation for KT

Reproduces Figures **4a, 4b, 4c, and 5** from §7.1 of the Smaran paper.

**Total time:** ~30 min human attention + ~3 hours unattended compute.

**Public artifact:** <https://github.com/distopialabs/Smaran/tree/artifact-eval>

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

SSH into node0, then paste:

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

## 3. Run the experiments (~3 hours)

```bash
./KeyTransparencyScripts/run_fig4a.sh   # ~80 min — versions {2,4,8,16,32,64,128,256,512,1024,2047}
./KeyTransparencyScripts/run_fig4b.sh   # ~5 s (cached from 4a)
./KeyTransparencyScripts/run_fig4c.sh   # ~5 s (cached from 4a)
./KeyTransparencyScripts/run_fig5.sh    # ~90–120 min — users {10k,50k,100k,200k,500k,1M}
```

PDFs land in `~/Smaran/output/`:
`fig4a_latency.pdf`, `fig4b_throughput.pdf`, `fig4c_payload.pdf`, `fig5_put_throughput.pdf`.

## 4. Compare to the paper

From your laptop:
```bash
scp <cloudlab-user>@<node0-host>:'~/Smaran/output/*.pdf' ~/Desktop/
open ~/Desktop/*.pdf
```

Absolute numbers depend on hardware; the **shape and protocol ordering** are what to check:

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

## Optional: install from source

If at Step 1 you unchecked the pre-built image (plain Ubuntu 22.04 boot), run these before Step 3:

```bash
./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"
```

---

## Data notes

- **Single run per point.** The paper averages 3 runs; the AE runs each point once to fit in ~3 h. Individual points may look noisy (Fig 4a Smaran at 700/1500 versions; Fig 5 Optiks between 50k–1M). Overall trends are unaffected.
- **Coniks fork.** The submodule is `coniks-history-extension` (fork of official CONIKS). Its per-request cost is user-count-independent, which is why our Fig 5 Coniks line is flat while the paper's declines. Fig 4 shape matches the paper.

## Troubleshooting
`
| Issue | Fix |
|---|---|
| `Permission denied (publickey)` between nodes | Re-run the SSH block in Step 2. |
| `no free nodes of type r6615/c6420` | Switch to `r6525` / `r6520` on the profile form. |
| Port 3191 or `/tmp/coniks.sock` in use | `pkill ktserver coniksserver && sudo rm -f /tmp/coniks.sock` on node0. |
| Experiment hangs > 5 min | Ctrl-C, delete latest `~/Smaran/logs/2026-*`, re-run. |
