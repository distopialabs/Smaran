# Smaran — Artifact Evaluation (Reviewer Guide)

**What this evaluates**: The Key-Transparency experiments from §7.1 of the Smaran paper — Figures **4a, 4b, 4c, 5**.

**Total time**: ~35 minutes of your attention + ~90 minutes of compute.

**Public artifact**: <https://github.com/distopialabs/Smaran/tree/artifact-eval>

---

## Step 1 — Get two CloudLab nodes (~10 min)

1. Log in at <https://www.cloudlab.us>. If you don't have a CloudLab account, sign up (free for academics, ~24 hr approval): <https://www.cloudlab.us/signup.php>.
2. Click **Experiments → Instantiate a Profile**.
3. In the search box, type `smaran-kt-ae` and select it (project `DistopiaLabs`).
4. Click **Instantiate**. Leave defaults:
   - `Boot from the pre-built Smaran image (recommended)` — **checked** (saves you the install phase).
   - `serverHW = r6615`, `clientHW = c6420`.
   - Cluster: Clemson.
5. Click **Next → Finish**. Wait ~5–10 minutes until status = **Ready**.
6. Click the **List View** tab. Copy the two SSH commands (for `node0` and `node1`).

## Step 2 — Set up the environment (~2 min)

SSH into **`node0`** (the first hostname):

```bash
ssh <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us
```

Once on `node0`, paste this block:

```bash
# Fetch the artifact-eval branch of the code
git clone --branch artifact-eval --recurse-submodules \
    https://github.com/distopialabs/Smaran.git ~/Smaran
cd ~/Smaran

# Set up inter-node SSH so node0 can drive experiments on node1
[ -f ~/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -N '' -f ~/.ssh/id_ed25519
cat ~/.ssh/id_ed25519.pub >> ~/.ssh/authorized_keys
cat ~/.ssh/id_ed25519.pub | ssh -o StrictHostKeyChecking=accept-new node1 "cat >> ~/.ssh/authorized_keys"
ssh-keyscan -H node0 node1 >> ~/.ssh/known_hosts 2>/dev/null
sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go

# Configure the runner (defaults are fine on CloudLab)
cp KeyTransparencyScripts/nodes.env.template KeyTransparencyScripts/nodes.env
```

That's it — you skip the install step because the image already has Go, Python packages, and apt dependencies baked in.

**If you unchecked the "Boot from pre-built image" option at Instantiate**, you have a plain Ubuntu 22.04 machine. Then also run:
```bash
./KeyTransparencyScripts/install_coniks.sh    # prints "Installing Coniks"
./KeyTransparencyScripts/install_optiks.sh    # prints "Installing Optiks"
./KeyTransparencyScripts/install_smaran.sh    # prints "Installing Smaran"
```

## Step 3 — Run the experiments

Two paths — pick one. Both produce the same 4 PDFs; "full" uses every point from the paper, "quick" uses a reduced sweep that still shows the same trends.

### Path A: Quick sweep (~90 min compute total) — recommended for AE

Fig 4 sweeps 5 version counts {2, 16, 128, 256, 2047}. Fig 5 sweeps 3 user counts {10k, 0.2M, 1M}.

```bash
cd ~/Smaran
./QuickTesting-KeyTransparency/run_fig4a_quick.sh   # ~35 min
./QuickTesting-KeyTransparency/run_fig4b_quick.sh   # ~5 s (cached from 4a)
./QuickTesting-KeyTransparency/run_fig4c_quick.sh   # ~5 s (cached from 4a)
./QuickTesting-KeyTransparency/run_fig5_quick.sh    # ~45 min
```

### Path B: Full sweep (~3 h compute total) — paper reproduction

Fig 4 sweeps 11 version counts {2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2047}. Fig 5 sweeps 6 user counts {10k, 50k, 100k, 200k, 500k, 1M}.

```bash
cd ~/Smaran
./KeyTransparencyScripts/run_fig4a.sh               # ~80 min
./KeyTransparencyScripts/run_fig4b.sh               # ~5 s (cached from 4a)
./KeyTransparencyScripts/run_fig4c.sh               # ~5 s (cached from 4a)
./KeyTransparencyScripts/run_fig5.sh                # ~90–120 min
```

Both paths write PDFs to `~/Smaran/output/` and CSV summaries to `~/Smaran/logs/<latest-sweep>/output/`.

**What each script prints:**
- First line: `Running experiment Figure <yy>`
- Per data point: `Running <Coniks|Optiks|Smaran> with <x> versions` (or `users` for Fig 5)
- On finish: `Plotting`, then `Saved: /users/.../Smaran/output/figXX.pdf`

**Where the PDFs go:**
```
~/Smaran/output/
  ├── fig4a_latency.pdf
  ├── fig4b_throughput.pdf
  ├── fig4c_payload.pdf
  └── fig5_put_throughput.pdf
```

## Step 4 — Compare against the paper

Copy the PDFs to your laptop (from **your laptop's** terminal):
```bash
mkdir -p ~/Desktop/smaran-ae-output
scp <your-cloudlab-username>@clnodeXXX.clemson.cloudlab.us:~/Smaran/output/*.pdf ~/Desktop/smaran-ae-output/
open ~/Desktop/smaran-ae-output/*.pdf
```

Compare each PDF to the paper's Figures 4a/4b/4c and 5. Absolute numbers depend on hardware; the **qualitative shape and protocol ordering** are what to check:

| Figure | Trend to look for |
|---|---|
| **4a — Latency vs versions** | All three protocols rise with versions. Coniks steepest (reaches ~5s at 2047). Optiks rises linearly through mid range. Smaran near-flat ~25–40 ms up to 512 versions, then climbs to ~200 ms at 2047. |
| **4b — Throughput vs versions** | All three drop with versions. Optiks highest at low versions (~10k qps at 2), crosses below Smaran around 128–256 versions, then declines. Smaran near-flat until 512, then declines. Coniks lowest at every point. |
| **4c — Payload vs versions** | Optiks and Coniks payloads grow steeply and track closely (100s → 1000s KiB). Smaran grows much more slowly (single KiB → ~100 KiB). |
| **5 — Put throughput vs users** | Broken y-axis: upper panel shows Optiks and Smaran in the tens-of-thousands of ops/s range with mild decline as users grow; lower panel shows Coniks flat at ~640 ops/s across the sweep. |

### Data notes

Absolute numbers differ from paper because hardware and measurement conditions differ:

- **Single run per point.** The paper averages **three runs**; the AE quick sweep runs each point once to fit in ~90 min. Consequence: individual points may show noise (see Fig 4a at 700/1500 versions, Fig 5 Optiks between 50k and 1M). The overall trend is unaffected.
- **Coniks fork.** The submodule is  (fork of official CONIKS with a Merkle-tree extension for versioned queries). Its per-request cost characteristics differ from the paper's official CONIKS in Fig 5, which is why our Fig 5 Coniks line is flat while the paper's declines. Fig 4 (monitoring queries) matches paper shape.
- **Coniks bench patch.**  fixes a race condition in the fork's bench (opens the monitor TCP connection *after* the load phase completes, so it isn't idle-killed). Applied automatically by .

If shapes match, the artifact reproduces the paper's key claims.

**Also produced**: CSV summaries at `~/Smaran/logs/<latest-sweep>/output/kt_query_summary.csv` (Fig 4) and `kt_put_summary.csv` (Fig 5), one row per (protocol, sweep-value) with throughput / latency / payload numbers.


## Claim-to-figure mapping

| Paper claim | Script | Output PDF |
|---|---|---|
| Smaran cuts monitoring-query latency 99.1% vs Coniks and 82.5% vs Optiks at 2047 versions | `run_fig4a_quick.sh` | `fig4a_latency.pdf` |
| Smaran delivers 113× throughput vs Coniks and 5.7× vs Optiks at 2047 versions | `run_fig4b_quick.sh` | `fig4b_throughput.pdf` |
| Smaran's response payload is 97.0% lower than Coniks and 95.2% lower than Optiks | `run_fig4c_quick.sh` | `fig4c_payload.pdf` |
| Smaran yields ~130× higher key-update throughput than Coniks at 10K users, ~115× at 1M users | `run_fig5_quick.sh` | `fig5_put_throughput.pdf` |

---

## Common issues

| Issue | Fix |
|---|---|
| `Permission denied (publickey)` when a script tries to SSH between nodes | Re-run the SSH setup block in Step 2. |
| Script says `no free nodes of type r6615/c6420` at Instantiate time | Try again in a few hours, or reserve via <https://www.cloudlab.us/reserve.php>. Alternatively substitute `r6525` for either type on the parameter form. |
| Port 3191 already in use | An earlier `ktserver` didn't shut down. Run `pkill ktserver` on node0. |
| An experiment hangs partway or you Ctrl-C it | Delete `~/Smaran/logs/<the failing sweep>/` and re-run the script. Cache only recognizes completed sweeps. |
| Smaran throughput comes out at 0 for the 1M-user point | The KZG precomputation was still running when the client tried to connect. Bump `server_startup_wait_seconds` in `KeyTransparencyScripts/configs/fig5_quick.toml` from 60 to 120 and re-run. |

---

## When you're done

Terminate the CloudLab experiment (**Experiments → My Experiments → Terminate**) to free the nodes.

If any step fails, open an issue at <https://github.com/distopialabs/Smaran/issues> or contact the authors.
